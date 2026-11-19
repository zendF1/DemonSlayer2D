package zone

import (
	"slices"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

// setStats gives p new derived stats and the max HP that goes with them.
// Current HP is the caller's business.
func setStats(p *player, s combat.Stats) {
	p.stats = s
	p.maxHP = s.MaxHP
}

// payExpShare gives p the EXP that counted more damage on m earns (spec 3.4):
// the share of p's summed damage on m, less what m has already paid p, so
// no fraction is lost over several hits.
func (z *Zone) payExpShare(p *player, m *monster, counted int) {
	m.dmgBy[p.id] += counted
	share := combat.ExpShare(combat.MonsterExp(m.level), m.maxHP, m.dmgBy[p.id])
	gain := share - m.expPaid[p.id]
	m.expPaid[p.id] = share
	z.gainExp(p, gain)
}

// gainExp adds gain to p's EXP and pushes p its new stats. Each level gained
// brings PointsPerLevel free points and full HP and MP, and is saved at once
// (spec 6.3). A player at MaxLevel, or a gain of 0, changes nothing.
func (z *Zone) gainExp(p *player, gain int64) {
	if gain <= 0 {
		return
	}
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

// allocate spends free potential points (spec 3.5). Current HP and MP rise by
// as much as their max, and the new points are saved at once (spec 6.3).
func (z *Zone) allocate(c allocateCmd) {
	p, ok := z.players[c.id]
	if !ok {
		return // left or evicted; see queueAttack
	}
	a := c.points
	fields := []int{a.Str, a.Agi, a.Vit, a.Int}
	switch {
	case p.dead:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_DEAD)
		return
	case slices.Min(fields) < 0:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	// Each field is checked alone first, so the sum below cannot overflow.
	case slices.Max(fields) > p.freePoints || a.Total() > p.freePoints:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS)
		return
	case a.Total() == 0:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	}
	p.points.Str += a.Str
	p.points.Agi += a.Agi
	p.points.Vit += a.Vit
	p.points.Int += a.Int
	p.freePoints -= a.Total()
	oldHP, oldMP := p.maxHP, p.stats.MaxMP
	setStats(p, combat.Compute(classes.IsMagic(p.class), p.level, p.points))
	p.hp += p.maxHP - oldHP
	p.mp += p.stats.MaxMP - oldMP
	if p.maxHP != oldHP {
		z.hpChanged(&p.entity)
	}
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_AllocatePointsResp{AllocatePointsResp: &pb.AllocatePointsResp{Stats: selfStats(p)}}})
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
