package zone

import (
	"cmp"
	"slices"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/pb"
)

type castCmd struct {
	id      uint64
	seq     uint32
	skillID string
	target  uint64
	dir     Vec2
}

// Cast queues a cast of skillID by player id, at target or along dir (phase
// 2 spec 4.1). A non-finite dir becomes zero and a longer one is cut to
// length 1. The zone answers seq with CastResp or ErrorResp on the player's
// outbox. It never blocks and reports false if the inbox is full.
func (z *Zone) Cast(id uint64, seq uint32, skillID string, target uint64, dir Vec2) bool {
	select {
	case z.inbox <- castCmd{id: id, seq: seq, skillID: skillID, target: target, dir: ClampDir(dir.X, dir.Y)}:
		return true
	default:
		return false
	}
}

// queueCast makes c its player's pending cast for this tick, separate from
// the pending slash. A newer cast replaces an older one, which is answered
// ON_COOLDOWN.
func (z *Zone) queueCast(c castCmd) {
	p, ok := z.players[c.id]
	if !ok {
		return // left or evicted; see queueAttack
	}
	if old := p.pendingCast; old != nil {
		z.replyError(p, old.seq, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN)
	}
	p.pendingCast = &c
}

// resolveCast checks p's cast c in the order of phase 2 spec 4.2 and, if it
// passes, spends its MP, starts its cooldown, answers it, announces it and
// lands it on its victims (spec 4.6). An error spends nothing.
func (z *Zone) resolveCast(p *player, c *castCmd) {
	s, n, code := z.checkCast(p, c.skillID)
	var primary *monster
	var dir Vec2
	if code == pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		primary, dir, code = z.aim(p, c, s)
	}
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	p.mp -= s.MP[n-1]
	p.skillReady[s.ID] = z.tickNo + z.ticks(float64(s.CooldownMS[n-1])/1000)
	p.statsDirty = true
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_CastResp{CastResp: &pb.CastResp{}}})
	ev := &pb.Cast{CasterId: p.id, SkillId: s.ID, DirX: dir.X, DirY: dir.Y}
	if primary != nil {
		ev.TargetId = primary.id
	}
	z.events.casts = append(z.events.casts, ev)
	// The cast is one action: a level gained on its first hit does not
	// change the damage of the rest.
	base, level := combat.SkillBase(p.stats, s.Damage == gamedata.DamageMagic, s.CoefPct[n-1]), p.level
	for _, m := range z.victims(p, s, n, primary) {
		z.strike(p, m, base, level, s.ID)
	}
}

// checkCast runs the checks of phase 2 spec 4.2 that do not depend on the
// shape. It returns the skill and p's level in it, or the error to answer.
func (z *Zone) checkCast(p *player, skillID string) (gamedata.Skill, int, pb.ErrorCode) {
	if p.dead {
		return gamedata.Skill{}, 0, pb.ErrorCode_ERROR_CODE_DEAD
	}
	if z.area.InSafe(p.pos) {
		return gamedata.Skill{}, 0, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE
	}
	s, known := z.area.Skills[skillID]
	n := p.skills[skillID]
	if !known || !s.Active() || classes.ToPB(s.Class) != p.class || n < 1 || n > gamedata.SkillLevels {
		return gamedata.Skill{}, 0, pb.ErrorCode_ERROR_CODE_SKILL_NOT_LEARNED
	}
	if z.tickNo < p.skillReady[skillID] {
		return gamedata.Skill{}, 0, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
	}
	if p.mp < s.MP[n-1] {
		return gamedata.Skill{}, 0, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_MP
	}
	return s, n, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// aim finds the primary target and direction of p's cast c of s (phase 2
// spec 4.3), or the error to answer. Dash skills cast as plain single-target
// skills for now.
func (z *Zone) aim(p *player, c *castCmd, s gamedata.Skill) (*monster, Vec2, pb.ErrorCode) {
	switch s.Shape {
	case gamedata.ShapeSingle:
		m := z.monsters[c.target]
		if m == nil || m.dead {
			return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET
		}
		if distance(p.pos, m.pos) > float32(s.CastRange)+attackRangeSlack || lineBlocked(z.area.Grid, p.pos, m.pos) {
			return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
		}
		return m, dirTo(p.pos, m.pos), pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	case gamedata.ShapeCircleSelf:
		// No target needed; a living one named is only hit first.
		if m := z.monsters[c.target]; m != nil && !m.dead {
			return m, Vec2{}, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
		}
		return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_BAD_REQUEST
}

// victims are the monsters p's cast of s at skill level n hits, in hit order
// (phase 2 spec 4.4): the primary target first if it is in the area, then
// the rest by distance to the area's centre, ties by lower id, at most
// max_targets of them. Only living monsters in sight of the area's origin
// count.
func (z *Zone) victims(p *player, s gamedata.Skill, n int, primary *monster) []*monster {
	if s.Shape == gamedata.ShapeSingle {
		return []*monster{primary}
	}
	origin := p.pos // circle_self: origin and centre are the caster
	r := float32(s.Radius)
	inside := func(m *monster) bool {
		return !m.dead && distance(origin, m.pos) <= r && !lineBlocked(z.area.Grid, origin, m.pos)
	}
	var out []*monster
	if primary != nil && inside(primary) {
		out = append(out, primary)
	}
	var rest []*monster
	for _, m := range z.monstersInBox(Vec2{origin.X - r, origin.Y - r}, Vec2{origin.X + r, origin.Y + r}) {
		if m != primary && inside(m) {
			rest = append(rest, m)
		}
	}
	slices.SortFunc(rest, func(a, b *monster) int {
		return cmp.Or(cmp.Compare(distance(origin, a.pos), distance(origin, b.pos)), cmp.Compare(a.id, b.id))
	})
	out = append(out, rest...)
	return out[:min(len(out), s.MaxTargets[n-1])]
}

// monstersInBox returns the monsters, dead or alive, whose position lies in
// the box from lo to hi. It looks only at the AOI cells the box overlaps.
func (z *Zone) monstersInBox(lo, hi Vec2) []*monster {
	c0, c1 := z.cellOf(lo), z.cellOf(hi)
	var out []*monster
	for cx := c0.X; cx <= c1.X; cx++ {
		for cy := c0.Y; cy <= c1.Y; cy++ {
			for id, e := range z.cells[cellKey{cx, cy}] {
				if e.kind != pb.EntityKind_ENTITY_KIND_MONSTER || e.pos.X < lo.X || e.pos.X > hi.X || e.pos.Y < lo.Y || e.pos.Y > hi.Y {
					continue
				}
				out = append(out, z.monsters[id])
			}
		}
	}
	return out
}

// dirTo is the unit vector from a to b, or zero when they coincide.
func dirTo(a, b Vec2) Vec2 {
	d := distance(a, b)
	if d == 0 {
		return Vec2{}
	}
	return Vec2{(b.X - a.X) / d, (b.Y - a.Y) / d}
}
