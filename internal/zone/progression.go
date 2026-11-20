package zone

import (
	"maps"
	"slices"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

// recompute derives p's stats from its class, level, points and learned
// passives (phase 2 spec 3.2) and sets the max HP that goes with them.
// Current HP and MP are the caller's business. Every stat change in the zone
// goes through here.
func (z *Zone) recompute(p *player) {
	var pas combat.Passive
	for id, lv := range p.skills {
		if s, ok := z.area.Skills[id]; ok && !s.Active() {
			pas = pas.Add(s.PassiveAt(lv))
		}
	}
	p.stats = combat.Compute(classes.IsMagic(p.class), p.level, p.points, pas)
	p.maxHP = p.stats.MaxHP
}

// skillLevels lists p's learned skills sorted by id.
func skillLevels(p *player) []*pb.SkillLevel {
	out := make([]*pb.SkillLevel, 0, len(p.skills))
	for _, id := range slices.Sorted(maps.Keys(p.skills)) {
		out = append(out, &pb.SkillLevel{SkillId: id, Level: int32(p.skills[id])})
	}
	return out
}

// payExpShare gives p the EXP that counted more damage on m earns (spec 3.4):
// the share of p's summed damage on m of the monster's total EXP, less what m
// has already paid p, so no fraction is lost over several hits. A hit from a
// player more than ExpLevelWindow levels away pays nothing and is not counted
// (phase 2 spec 2.4).
func (z *Zone) payExpShare(p *player, m *monster, counted int) {
	if !combat.ExpEligible(p.level, m.level) {
		return
	}
	m.dmgBy[p.id] += counted
	share := combat.ExpShare(z.area.Levels[m.level].Exp, m.maxHP, m.dmgBy[p.id])
	gain := share - m.expPaid[p.id]
	m.expPaid[p.id] = share
	z.gainExp(p, gain)
}

// gainExp adds gain to p's EXP and pushes p its new stats. Each level gained
// brings PointsPerLevel free points and one skill point (phase 2 spec 3.1);
// a level-up also restores full HP and MP and is saved at once (spec 6.3). A
// player at MaxLevel, or a gain of 0, changes nothing.
func (z *Zone) gainExp(p *player, gain int64) {
	if gain <= 0 {
		return
	}
	level, exp := z.area.Exp.Gain(p.level, p.exp, gain)
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
	p.skillPoints += gained
	z.recompute(p)
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
	z.recompute(p)
	p.hp += p.maxHP - oldHP
	p.mp += p.stats.MaxMP - oldMP
	if p.maxHP != oldHP {
		z.hpChanged(&p.entity)
	}
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_AllocatePointsResp{AllocatePointsResp: &pb.AllocatePointsResp{Stats: z.selfStats(p)}}})
}

// selfStats is p's own view of its progression and stats (spec 5.2).
func (z *Zone) selfStats(p *player) *pb.SelfStats {
	s := p.stats
	return &pb.SelfStats{
		Level:        int32(p.level),
		Exp:          p.exp,
		ExpNext:      z.area.Exp.ToNext(p.level),
		FreePoints:   int32(p.freePoints),
		SkillPoints:  int32(p.skillPoints),
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
