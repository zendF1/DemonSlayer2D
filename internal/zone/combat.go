package zone

import (
	"maps"
	"math"
	"slices"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

const (
	// autoRespawnSec is how long a dead player lies before the server
	// respawns it (spec 3.6).
	autoRespawnSec = 60
	// attackRangeSlack is added to the slash range because the client sees
	// monsters two ticks late (spec 5.1).
	attackRangeSlack = 0.5
	// corpseSec is how long a dead monster lies before it is removed (spec 4.2).
	corpseSec = 1.0
)

// tickEvents are the combat events of one tick. syncView sends each viewer the
// ones about entities it can see.
//
// Hits, deaths and level-ups are snapshots taken when they happen. An HP
// change records only the entity id, once per tick, and addEvents fills in
// the HP and max HP the entity has at the end of the tick: clients apply hits
// first and hp_changes last, so a heal recorded before a later hit in the
// same tick cannot leave them showing too much HP.
type tickEvents struct {
	hits      []*pb.Hit
	deaths    []*pb.Death
	hpChanges []uint64 // entity ids, without duplicates
	levelUps  []*pb.LevelUp
}

// damagePlayer applies a hit of dmg from attacker to p and kills p at 0 HP.
func (z *Zone) damagePlayer(p *player, attacker uint64, dmg int) {
	if p.dead {
		return
	}
	p.hp = max(p.hp-dmg, 0)
	z.events.hits = append(z.events.hits, &pb.Hit{AttackerId: attacker, TargetId: p.id, Damage: int32(dmg), HpAfter: int32(p.hp)})
	if p.hp == 0 {
		z.killPlayer(p)
	}
}

// killPlayer makes p a corpse that ignores movement until it respawns, by
// request or automatically at deadUntil.
func (z *Zone) killPlayer(p *player) {
	p.dead = true
	if p.dir != (Vec2{}) {
		p.dir = Vec2{}
		p.changed = true
	}
	p.deadUntil = z.tickNo + z.ticks(autoRespawnSec)
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

// addEvents puts into wt the events p may see: hits whose attacker or target
// is visible to p, and deaths, HP changes and level-ups of visible entities.
// A player always sees its own events. A Hit can name an entity p has had no
// EntityEnter for (only one side in view); the client ignores unknown ids.
func (z *Zone) addEvents(p *player, wt *pb.WorldTick) {
	sees := func(id uint64) bool {
		_, ok := p.visible[id]
		return ok || id == p.id
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
}

// setStats gives p new derived stats and the max HP that goes with them.
// Current HP is the caller's business.
func setStats(p *player, s combat.Stats) {
	p.stats = s
	p.maxHP = s.MaxHP
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

// resolveAttacks resolves each player's pending attack, in id order, before
// anyone moves (spec 6.2). Replies go out at once, ahead of the WorldTick.
func (z *Zone) resolveAttacks() {
	for _, id := range slices.Sorted(maps.Keys(z.players)) {
		p := z.players[id]
		c := p.pendingAttack
		if c == nil {
			continue
		}
		p.pendingAttack = nil
		m, code := z.checkAttack(p, c.target)
		if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
			z.replyError(p, c.seq, code)
			continue
		}
		p.lastAttack = z.tickNo
		p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_AttackResp{AttackResp: &pb.AttackResp{}}})
		z.slash(p, m)
	}
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
	if m == nil || m.dead || m.state == returning {
		return nil, pb.ErrorCode_ERROR_CODE_INVALID_TARGET
	}
	if distance(p.pos, m.pos) > combat.SlashRange+attackRangeSlack || lineBlocked(z.area.Grid, p.pos, m.pos) {
		return nil, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
	}
	if p.lastAttack != 0 && z.tickNo-p.lastAttack < z.ticks(combat.SlashCooldown.Seconds()) {
		return nil, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
	}
	return m, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// lineBlocked reports whether the segment from a to b crosses a blocked tile.
// It walks every tile the segment passes through (Amanatides-Woo traversal).
// Where the segment passes exactly through a tile corner, it counts as
// blocked if either tile beside the corner is. The endpoints' own tiles do
// not count.
func lineBlocked(g Grid, a, b Vec2) bool {
	sx, sy := floor(a.X), floor(a.Y)
	ex, ey := floor(b.X), floor(b.Y)
	x, y := sx, sy
	blocked := func(tx, ty int) bool {
		return !(tx == sx && ty == sy) && !(tx == ex && ty == ey) && g.Blocked(tx, ty)
	}
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	stepX, tMaxX, tDeltaX := axis(float64(a.X), dx, x)
	stepY, tMaxY, tDeltaY := axis(float64(a.Y), dy, y)
	// Each step moves one tile along x or y (two at a corner), so the walk
	// ends after at most this many tiles even if float error skips the end tile.
	for n := abs(ex-x) + abs(ey-y); n > 0 && (x != ex || y != ey); {
		switch d := tMaxX - tMaxY; {
		case math.Abs(d) < 1e-9: // through a corner
			if blocked(x+stepX, y) || blocked(x, y+stepY) {
				return true
			}
			x, y = x+stepX, y+stepY
			tMaxX, tMaxY = tMaxX+tDeltaX, tMaxY+tDeltaY
			n -= 2
		case d < 0:
			x += stepX
			tMaxX += tDeltaX
			n--
		default:
			y += stepY
			tMaxY += tDeltaY
			n--
		}
		if blocked(x, y) {
			return true
		}
	}
	return false
}

// axis sets up one axis of the tile traversal from position p moving by d,
// starting in tile t: the step direction, the segment fraction at which the
// first tile edge is crossed, and the fraction between edges.
func axis(p, d float64, t int) (step int, tMax, tDelta float64) {
	switch {
	case d > 0:
		return 1, (float64(t+1) - p) / d, 1 / d
	case d < 0:
		return -1, (p - float64(t)) / -d, 1 / -d
	default:
		return 0, math.Inf(1), math.Inf(1)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// slash lands p's default slash on m: an idle monster turns on its attacker,
// and one brought to 0 HP dies.
func (z *Zone) slash(p *player, m *monster) {
	dmg := combat.Damage(combat.SlashBase(p.stats), z.area.Levels[m.level].Defense, p.level)
	m.hp = max(m.hp-dmg, 0)
	z.events.hits = append(z.events.hits, &pb.Hit{AttackerId: p.id, TargetId: m.id, Damage: int32(dmg), HpAfter: int32(m.hp)})
	m.attackers[p.id] = struct{}{}
	if m.state == idle {
		m.target, m.state = p.id, chase
	}
	if m.hp == 0 {
		z.killMonster(m, p)
	}
}

// killMonster makes m a corpse, schedules its camp's respawn and gives the
// killer its EXP.
func (z *Zone) killMonster(m *monster, killer *player) {
	m.dead, m.diedAt = true, z.tickNo
	z.stopMonster(m)
	m.target = 0
	clear(m.attackers)
	z.events.deaths = append(z.events.deaths, &pb.Death{Id: m.id})
	m.camp.pending = append(m.camp.pending, m.diedAt+z.ticks(m.camp.Kind.RespawnSec))
	z.gainExp(killer, combat.MonsterExp(m.level))
}

// gainExp adds gain to p's EXP. Each level gained brings PointsPerLevel free
// points and full HP and MP, and is saved at once (spec 6.3).
func (z *Zone) gainExp(p *player, gain int64) {
	level, exp := combat.GainExp(p.level, p.exp, gain)
	if level == p.level && exp == p.exp {
		return // at the level cap
	}
	p.statsDirty = true
	gained := level - p.level
	p.level, p.exp = level, exp
	if gained == 0 {
		return
	}
	p.freePoints += combat.PointsPerLevel * gained
	setStats(p, combat.Compute(classes.IsMagic(p.class), p.level, p.points))
	p.hp, p.mp = p.maxHP, p.stats.MaxMP
	z.events.levelUps = append(z.events.levelUps, &pb.LevelUp{Id: p.id, Level: int32(p.level)})
	z.hpChanged(&p.entity)
	z.saver.Save(z.state(p), nil)
}

// replyError answers seq with an ErrorResp carrying code.
func (z *Zone) replyError(p *player, seq uint32, code pb.ErrorCode) {
	p.out.Send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Error{Error: &pb.ErrorResp{Code: code, Detail: code.String()}}})
}

// selfStats is p's own view of its progression and stats (spec 5.2).
func selfStats(p *player) *pb.SelfStats {
	s := p.stats
	return &pb.SelfStats{
		Level:        int32(p.level),
		Exp:          p.exp,
		ExpNext:      combat.ExpToNext(p.level),
		FreePoints:   int32(p.freePoints),
		Strength:     int32(p.points.Str),
		Agility:      int32(p.points.Agi),
		Vitality:     int32(p.points.Vit),
		Intellect:    int32(p.points.Int),
		Hp:           int32(p.hp),
		MaxHp:        int32(p.maxHP),
		Mp:           int32(p.mp),
		MaxMp:        int32(s.MaxMP),
		PhysAttack:   int32(s.PhysAttack),
		MagicAttack:  int32(s.MagicAttack),
		PhysDefense:  int32(s.PhysDefense),
		MagicDefense: int32(s.MagicDefense),
		Accuracy:     int32(s.Accuracy),
		Evasion:      int32(s.Evasion),
		CritRateBp:   int32(s.CritRateBP),
	}
}
