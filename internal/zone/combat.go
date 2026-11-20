package zone

import (
	"maps"
	"slices"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

const (
	// attackRangeSlack is added to the slash range because the client sees
	// monsters two ticks late (spec 5.1).
	attackRangeSlack = 0.5
	// corpseSec is how long a dead monster lies before it is removed (spec 4.2).
	corpseSec = 1.0
)

// tickEvents are the combat events of one tick. syncView sends each viewer the
// ones about entities it can see.
//
// Casts, hits, deaths and level-ups are snapshots taken when they happen. An HP
// change records only the entity id, once per tick, and addEvents fills in
// the HP and max HP the entity has at the end of the tick: clients apply hits
// first and hp_changes last, so a heal recorded before a later hit in the
// same tick cannot leave them showing too much HP.
type tickEvents struct {
	casts     []*pb.Cast
	hits      []*pb.Hit
	deaths    []*pb.Death
	hpChanges []uint64 // entity ids, without duplicates
	levelUps  []*pb.LevelUp
	looks     []*pb.LookChange
}

// damagePlayer applies a hit of dmg from attacker to p and kills p at 0 HP.
// ranged marks a monster's shot.
func (z *Zone) damagePlayer(p *player, attacker uint64, dmg int, ranged bool) {
	if p.dead {
		return
	}
	p.hp = max(p.hp-dmg, 0)
	z.events.hits = append(z.events.hits, &pb.Hit{AttackerId: attacker, TargetId: p.id, Damage: int32(dmg), HpAfter: int32(p.hp), Ranged: ranged})
	if p.hp == 0 {
		z.killPlayer(p)
	}
}

// killPlayer makes p a corpse that ignores movement until it respawns, by
// request or automatically at deadUntil. Its direction is zeroed here, and
// stays zero while dead because move ignores the dead.
func (z *Zone) killPlayer(p *player) {
	p.dead = true
	if p.dir != (Vec2{}) {
		p.dir = Vec2{}
		p.changed = true
	}
	p.deadUntil = z.tickNo + z.ticks(autoRespawnSec)
	p.restTicks = 0
	// Monsters lose a dead target (spec 3.6) and pick another at their next
	// think. They also forget p as an attacker, so once respawned it is not
	// chased for a hit from before its death.
	for _, m := range z.monsters {
		if m.target == p.id {
			m.target = 0
		}
		delete(m.attackers, p.id)
	}
	z.events.deaths = append(z.events.deaths, &pb.Death{Id: p.id})
}

// hpChanged records an HP change that did not come from a hit. Viewers get
// e's HP as it stands at the end of the tick.
func (z *Zone) hpChanged(e *entity) {
	if !slices.Contains(z.events.hpChanges, e.id) {
		z.events.hpChanges = append(z.events.hpChanges, e.id)
	}
}

// entityByID finds a player or monster in the zone.
func (z *Zone) entityByID(id uint64) *entity {
	if p := z.players[id]; p != nil {
		return &p.entity
	}
	if m := z.monsters[id]; m != nil {
		return &m.entity
	}
	return nil
}

// addEvents puts into wt the events p may see: casts by visible casters, hits
// whose attacker or target is visible to p, and deaths, HP changes,
// level-ups and look changes of visible entities.
// A player always sees its own events. A Hit can name an entity p has had no
// EntityEnter for (only one side in view); the client ignores unknown ids.
func (z *Zone) addEvents(p *player, wt *pb.WorldTick) {
	sees := func(id uint64) bool {
		_, ok := p.visible[id]
		return ok || id == p.id
	}
	for _, c := range z.events.casts {
		if sees(c.CasterId) {
			wt.Casts = append(wt.Casts, c)
		}
	}
	for _, h := range z.events.hits {
		if sees(h.AttackerId) || sees(h.TargetId) {
			wt.Hits = append(wt.Hits, h)
		}
	}
	for _, d := range z.events.deaths {
		if sees(d.Id) {
			wt.Deaths = append(wt.Deaths, d)
		}
	}
	for _, id := range z.events.hpChanges {
		if e := z.entityByID(id); e != nil && sees(id) {
			wt.HpChanges = append(wt.HpChanges, &pb.HpChange{Id: id, Hp: int32(e.hp), MaxHp: int32(e.maxHP)})
		}
	}
	for _, l := range z.events.levelUps {
		if sees(l.Id) {
			wt.LevelUps = append(wt.LevelUps, l)
		}
	}
	for _, l := range z.events.looks {
		if sees(l.Id) {
			wt.Looks = append(wt.Looks, l)
		}
	}
}

// queueAttack makes c its player's pending attack for this tick. A newer
// attack replaces an older one, which is answered ON_COOLDOWN.
func (z *Zone) queueAttack(c attackCmd) {
	p, ok := z.players[c.id]
	if !ok {
		// The player has left or been evicted, so there is no outbox to answer
		// on; the gateway sees the membership end and stops sending.
		return
	}
	if old := p.pendingAttack; old != nil {
		z.replyError(p, old.seq, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN)
	}
	p.pendingAttack = &c
}

// resolveAttacks resolves each player's pending slash and then its pending
// cast, player by player in id order, before anyone moves (spec 6.2, phase 2
// spec 4.1). Replies go out at once, ahead of the WorldTick.
func (z *Zone) resolveAttacks() {
	for _, id := range slices.Sorted(maps.Keys(z.players)) {
		p := z.players[id]
		if c := p.pendingAttack; c != nil {
			p.pendingAttack = nil
			z.resolveAttack(p, c)
		}
		if c := p.pendingCast; c != nil {
			p.pendingCast = nil
			z.resolveCast(p, c)
		}
	}
}

// resolveAttack checks p's slash c and starts it if it passes.
func (z *Zone) resolveAttack(p *player, c *attackCmd) {
	m, code := z.checkAttack(p, c.target)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	p.slashReady = z.tickNo + z.ticks(combat.SlashCooldown.Seconds())
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_AttackResp{AttackResp: &pb.AttackResp{}}})
	z.slash(p, m)
}

// checkAttack validates an attack by p on target in the order of spec 5.1.
// It returns the target monster, or the error code to answer.
func (z *Zone) checkAttack(p *player, target uint64) (*monster, pb.ErrorCode) {
	if p.dead {
		return nil, pb.ErrorCode_ERROR_CODE_DEAD
	}
	if z.area.InSafe(p.pos) {
		return nil, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE
	}
	m := z.monsters[target]
	if m == nil || m.dead {
		return nil, pb.ErrorCode_ERROR_CODE_INVALID_TARGET
	}
	if distance(p.pos, m.pos) > combat.SlashRange+attackRangeSlack || lineBlocked(z.area.Grid, p.pos, m.pos) {
		return nil, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
	}
	if z.tickNo < p.slashReady {
		return nil, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
	}
	return m, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// slash starts p's default slash on m (attack phases spec 2): its damage is
// fixed now and lands SlashWindup later, with no flight. Viewers get a Cast
// with an empty skill id.
func (z *Zone) slash(p *player, m *monster) {
	windup := int(combat.SlashWindup.Milliseconds())
	due := z.tickNo + z.impactTicks(windup, 0)
	z.schedule(impact{due: due, attacker: p.id, target: m.id, damage: z.damageTo(m, combat.SlashBase(p.stats), p.level)})
	d := dirTo(p.pos, m.pos)
	z.events.casts = append(z.events.casts, &pb.Cast{CasterId: p.id, TargetId: m.id, DirX: d.X, DirY: d.Y,
		WindupMs: int32(windup), Impacts: []*pb.CastImpact{{TargetId: m.id, ImpactMs: z.impactMS(due)}}})
}

// damageTo is the damage a hit of base damage before m's defense does to m,
// from an attacker of the given level. Attacks fix it at the press;
// applyToMonster lands it.
func (z *Zone) damageTo(m *monster, base, level int) int {
	return combat.Damage(base, z.area.Levels[m.level].Defense, level)
}

// killMonster makes m a corpse, gives killer its Xu and drops (game-design
// 5.8, phase 3a spec 4.3) and schedules its region's respawn. killer is the
// player whose impact brought m to 0 HP, nil if it has left the zone. Its EXP
// has already been paid hit by hit.
func (z *Zone) killMonster(m *monster, killer *player) {
	m.dead, m.diedAt = true, z.tickNo
	z.stopMonster(m)
	m.target = 0
	clear(m.attackers)
	z.events.deaths = append(z.events.deaths, &pb.Death{Id: m.id})
	m.region.pending = append(m.region.pending, m.diedAt+z.ticks(m.region.Kind.RespawnSec))
	z.loot(m, killer)
}

// replyError answers seq with an ErrorResp carrying code.
func (z *Zone) replyError(p *player, seq uint32, code pb.ErrorCode) {
	p.out.Send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Error{Error: &pb.ErrorResp{Code: code, Detail: code.String()}}})
}
