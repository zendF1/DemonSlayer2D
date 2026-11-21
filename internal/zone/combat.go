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
// same tick cannot leave them showing too much HP. A look change is recorded
// the same way and carries the entity's whole look at the end of the tick
// (phase 5a spec 4.9).
type tickEvents struct {
	casts     []*pb.Cast
	hits      []*pb.Hit
	deaths    []*pb.Death
	hpChanges []uint64 // entity ids, without duplicates
	levelUps  []*pb.LevelUp
	looks     []uint64 // entity ids, without duplicates
}

// killer is who landed the blow that brought a player to 0 HP: a monster
// (its id and kind name) or a player (its id and name, kept from the impact
// so they survive the attacker leaving the zone).
type killer struct {
	id       uint64
	name     string
	byPlayer bool
}

// damagePlayer applies a hit of dmg from the monster k to p and kills p at
// 0 HP. ranged marks the monster's shot.
func (z *Zone) damagePlayer(p *player, k killer, dmg int, ranged bool) {
	z.hurtPlayer(p, &pb.Hit{AttackerId: k.id, TargetId: p.id, Damage: int32(dmg), Ranged: ranged}, k)
}

// hurtPlayer lands h, whose target is p, filling in its HP after; p dies at
// 0 HP, the blow being k's. A dead p takes nothing and no Hit is sent.
func (z *Zone) hurtPlayer(p *player, h *pb.Hit, k killer) {
	if p.dead {
		return
	}
	p.hp = max(p.hp-int(h.Damage), 0)
	h.HpAfter = int32(p.hp)
	z.events.hits = append(z.events.hits, h)
	if p.hp == 0 {
		z.killPlayer(p, k)
	}
}

// killPlayer makes p a corpse that ignores movement until it respawns, by
// request or automatically at deadUntil. Its direction is zeroed here, and
// stays zero while dead because move ignores the dead. It notes the killer
// for p's DeathNotice and, when a player killed a PK-on p on a map with the
// PK penalty (phase 5a spec 4.7), takes combat.PkExpLoss of p's EXP, which
// may go negative, and saves at once; a loss of 0 (level 90) saves nothing.
// The killer gets nothing.
func (z *Zone) killPlayer(p *player, k killer) {
	var lost int64
	if k.byPlayer && z.area.PkPenalty && p.pkOn {
		lost = combat.PkExpLoss(z.area.Exp.ToNext(p.level))
	}
	if lost > 0 {
		p.exp -= lost
		p.statsDirty = true
		z.saver.Save(z.state(p), nil)
	}
	p.deathNotice = &pb.DeathNotice{KillerId: k.id, KillerName: k.name, ByPlayer: k.byPlayer, ExpLost: lost}
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

// lookChanged records a change of e's look (weapon glow, PK mode). Viewers
// get e's whole look as it stands at the end of the tick.
func (z *Zone) lookChanged(e *entity) {
	if !slices.Contains(z.events.looks, e.id) {
		z.events.looks = append(z.events.looks, e.id)
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
	for _, id := range z.events.looks {
		if e := z.entityByID(id); e != nil && sees(id) {
			wt.Looks = append(wt.Looks, &pb.LookChange{Id: id, WeaponEnhance: int32(e.weaponEnhance), PkOn: e.pkOn})
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
	f, code := z.checkAttack(p, c.target)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	p.slashReady = z.tickNo + z.ticks(combat.SlashCooldown.Seconds())
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_AttackResp{AttackResp: &pb.AttackResp{}}})
	z.slash(p, f)
}

// foe is the target of a player's attack: a monster or another player,
// exactly one of them set (phase 5a spec 4.2). The zero foe is no target.
type foe struct {
	m *monster
	p *player
}

func (f foe) ok() bool { return f.m != nil || f.p != nil }

func (f foe) ent() *entity {
	if f.m != nil {
		return &f.m.entity
	}
	return &f.p.entity
}

func (f foe) pos() Vec2  { return f.ent().pos }
func (f foe) id() uint64 { return f.ent().id }

// checkAttack validates an attack by p on target in the order of phase 5a
// spec 4.3. It returns the target, or the error code to answer.
func (z *Zone) checkAttack(p *player, target uint64) (foe, pb.ErrorCode) {
	if p.dead {
		return foe{}, pb.ErrorCode_ERROR_CODE_DEAD
	}
	if z.area.InSafe(p.pos) {
		return foe{}, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE
	}
	f, code := z.foeOf(p, target)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return foe{}, code
	}
	if distance(p.pos, f.pos()) > combat.SlashRange+attackRangeSlack || lineBlocked(z.area.Grid, p.pos, f.pos()) {
		return foe{}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
	}
	if z.tickNo < p.slashReady {
		return foe{}, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
	}
	return f, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// foeOf finds the target id a named by its press: a living monster, or a
// player a may hit (checkPlayerTarget). World ids are unique, so the
// monsters are looked at first. Anything else is the error to answer.
func (z *Zone) foeOf(a *player, id uint64) (foe, pb.ErrorCode) {
	if m := z.monsters[id]; m != nil {
		if m.dead {
			return foe{}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET
		}
		return foe{m: m}, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	b, code := z.checkPlayerTarget(a, id)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return foe{}, code
	}
	return foe{p: b}, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// checkPlayerTarget checks a player target id of a's slash or cast in the
// order of phase 5a spec 4.2, after a's own DEAD and IN_SAFE_ZONE checks:
// INVALID_TARGET when id is a itself, not a player in the zone or dead, then
// the PK rules of pvpCheck.
func (z *Zone) checkPlayerTarget(a *player, id uint64) (*player, pb.ErrorCode) {
	b := z.players[id]
	if b == nil || b == a || b.dead {
		return nil, pb.ErrorCode_ERROR_CODE_INVALID_TARGET
	}
	if code := z.pvpCheck(a, b); code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return nil, code
	}
	return b, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// pvpCheck is steps 2 to 4 of phase 5a spec 4.2 for a hitting b, which area
// skills also apply to each player in their area: PK_FORBIDDEN when the area
// allows no PvP, IN_SAFE_ZONE when b stands in a safe zone, PK_FORBIDDEN
// when the PK table does not let a hit b, which it never does for a teammate.
func (z *Zone) pvpCheck(a, b *player) pb.ErrorCode {
	switch {
	case !z.area.PvP:
		return pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN
	case z.area.InSafe(b.pos):
		return pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE
	case !combat.PvPAllowed(a.pkOn, b.pkOn, sameParty(a, b)):
		return pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// sameParty is true when a and b are in one party (phase 5b spec 4.7).
func sameParty(a, b *player) bool {
	return a.partyID() != 0 && a.partyID() == b.partyID()
}

// slash starts p's default slash on f (attack phases spec 2): its damage is
// fixed now and lands SlashWindup later, with no flight. Viewers get a Cast
// with an empty skill id.
func (z *Zone) slash(p *player, f foe) {
	windup := int(combat.SlashWindup.Milliseconds())
	due := z.tickNo + z.impactTicks(windup, 0)
	z.schedule(z.hitOn(p, f, combat.SlashBase(p.stats), p.level, false, "", due))
	d := dirTo(p.pos, f.pos())
	z.events.casts = append(z.events.casts, &pb.Cast{CasterId: p.id, TargetId: f.id(), DirX: d.X, DirY: d.Y,
		WindupMs: int32(windup), Impacts: []*pb.CastImpact{{TargetId: f.id(), ImpactMs: z.impactMS(due)}}})
}

// hitOn is p's impact on f due at tick due, of base damage before f's
// defense from an attacker of the given level: magic picks a player's magic
// defense (a slash is physical); skillID is empty for a slash. The damage is
// fixed now, at the press. A hit on a player also starts the PK-off lock of
// both sides now, to pvpLockSec after due (phase 5a spec 1.3), so neither
// can turn PK off while it is in flight; applyPvP refreshes it on landing.
func (z *Zone) hitOn(p *player, f foe, base, level int, magic bool, skillID string, due uint32) impact {
	im := impact{due: due, attacker: p.id, target: f.id(), skillID: skillID}
	if f.p != nil {
		im.targetIsPlayer, im.attackerName = true, p.name
		im.damage = z.damageToPlayer(f.p, base, level, magic)
		lock := due + z.ticks(pvpLockSec)
		p.pvpUntil = max(p.pvpUntil, lock)
		f.p.pvpUntil = max(f.p.pvpUntil, lock)
	} else {
		im.damage = z.damageTo(f.m, base, level)
	}
	return im
}

// damageTo is the damage a hit of base damage before m's defense does to m,
// from an attacker of the given level. Attacks fix it at the press;
// applyToMonster lands it.
func (z *Zone) damageTo(m *monster, base, level int) int {
	return combat.Damage(base, z.monsterStats(m).Defense, level)
}

// damageToPlayer is the damage a player's hit of base damage before b's
// defense does to b, from an attacker of the given level, against b's magic
// defense when magic, else its physical one (phase 5a spec 4.5). Attacks fix
// it at the press; applyPvP lands it.
func (z *Zone) damageToPlayer(b *player, base, level int, magic bool) int {
	def := b.stats.PhysDefense
	if magic {
		def = b.stats.MagicDefense
	}
	return combat.PvPDamage(base, def, level)
}

// killMonster makes m a corpse, gives killer its Xu and drops (game-design
// 5.8, phase 3a spec 4.3) and its quest credit (phase 6b spec 4.3), and
// schedules its region's respawn. killer is the player whose impact brought
// m to 0 HP, nil if it has left the zone. Its EXP has already been paid hit
// by hit.
func (z *Zone) killMonster(m *monster, killer *player) {
	m.dead, m.diedAt = true, z.tickNo
	z.stopMonster(m)
	m.target = 0
	clear(m.attackers)
	z.events.deaths = append(z.events.deaths, &pb.Death{Id: m.id})
	m.region.pending = append(m.region.pending, m.diedAt+z.ticks(m.region.Kind.RespawnSec))
	z.loot(m, killer)
	z.creditQuests(m, killer)
}

// replyError answers seq with an ErrorResp carrying code.
func (z *Zone) replyError(p *player, seq uint32, code pb.ErrorCode) {
	p.out.Send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Error{Error: &pb.ErrorResp{Code: code, Detail: code.String()}}})
}
