package zone

import (
	"maps"
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
	// lineSampleStep is how often lineBlocked samples a segment, in tiles.
	lineSampleStep = 0.25
	// corpseSec is how long a dead monster lies before it is removed (spec 4.2).
	corpseSec = 1.0
)

// tickEvents are the combat events of one tick. syncView sends each viewer the
// ones about entities it can see.
type tickEvents struct {
	hits      []*pb.Hit
	deaths    []*pb.Death
	hpChanges []*pb.HpChange
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

// hpChanged records an HP change that did not come from a hit.
func (z *Zone) hpChanged(e *entity) {
	z.events.hpChanges = append(z.events.hpChanges, &pb.HpChange{Id: e.id, Hp: int32(e.hp), MaxHp: int32(e.maxHP)})
}

// queueAttack makes c its player's pending attack for this tick. A newer
// attack replaces an older one, which is answered ON_COOLDOWN.
func (z *Zone) queueAttack(c attackCmd) {
	p, ok := z.players[c.id]
	if !ok {
		return
	}
	if old := p.pendingAttack; old != nil {
		z.fail(p, old.seq, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN)
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
			z.fail(p, c.seq, code)
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

// lineBlocked reports whether the segment from a to b crosses a blocked tile,
// sampling it every lineSampleStep tiles. The endpoints' own tiles do not count.
func lineBlocked(g Grid, a, b Vec2) bool {
	ax, ay, bx, by := floor(a.X), floor(a.Y), floor(b.X), floor(b.Y)
	n := int(distance(a, b)/lineSampleStep) + 1
	for i := 1; i < n; i++ {
		f := float32(i) / float32(n)
		tx, ty := floor(a.X+(b.X-a.X)*f), floor(a.Y+(b.Y-a.Y)*f)
		if tx == ax && ty == ay || tx == bx && ty == by {
			continue
		}
		if g.Blocked(tx, ty) {
			return true
		}
	}
	return false
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

// fail answers seq with an ErrorResp carrying code.
func (z *Zone) fail(p *player, seq uint32, code pb.ErrorCode) {
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
