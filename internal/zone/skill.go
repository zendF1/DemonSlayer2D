package zone

import (
	"cmp"
	"math"
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
// lands it on its victims (spec 4.6), after moving a dash's caster (spec
// 4.5). An error spends nothing.
func (z *Zone) resolveCast(p *player, c *castCmd) {
	s, n, code := z.checkCast(p, c.skillID)
	var primary *monster
	var dir Vec2
	if code == pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		primary, dir, code = z.aim(p, c, s, n)
	}
	land := p.pos
	if code == pb.ErrorCode_ERROR_CODE_UNSPECIFIED && s.Dash {
		land, code = z.dashLanding(p, primary)
	}
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	p.mp -= s.MP[n-1]
	p.skillReady[s.ID] = z.tickNo + z.ticks(float64(s.CooldownMS[n-1])/1000)
	p.statsDirty = true
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_CastResp{CastResp: &pb.CastResp{}}})
	hit := z.victims(p, s, n, primary, dir)
	ev := &pb.Cast{CasterId: p.id, SkillId: s.ID, DirX: dir.X, DirY: dir.Y}
	switch {
	case s.Shape == gamedata.ShapeCircleSelf:
		// The named target only orders the hits; name it only if it was hit.
		if primary != nil && len(hit) > 0 && hit[0] == primary {
			ev.TargetId = primary.id
		}
	case primary != nil:
		ev.TargetId = primary.id
	}
	z.events.casts = append(z.events.casts, ev)
	if land != p.pos {
		// The dash: everyone in view, the caster too, jumps to the landing
		// point. p.dir is kept, so movePlayers goes on from there this tick.
		p.pos, p.changed, p.snap = land, true, true
		z.updateCell(&p.entity)
	}
	// The cast is one action: a level gained on its first hit does not
	// change the damage of the rest.
	base, level := combat.SkillBase(p.stats, s.Damage == gamedata.DamageMagic, s.CoefPct[n-1]), p.level
	for _, m := range hit {
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

// aim finds the primary target and direction of p's cast c of s at skill
// level n (phase 2 spec 4.3), or the error to answer. The direction is the
// one the Cast event carries (spec 4.6): towards the target for single and
// circle_target, the decided aim for cone and line, zero for circle_self.
//
// single and circle_target need a living monster within the level's range
// plus attackRangeSlack and in sight, as a slash does. cone and line aim at a
// living named monster at any range, else along the normalised dir; with
// neither the cast is a BAD_REQUEST. Dash skills aim as single-target skills;
// resolveCast then moves the caster (dashLanding).
func (z *Zone) aim(p *player, c *castCmd, s gamedata.Skill, n int) (*monster, Vec2, pb.ErrorCode) {
	m := z.monsters[c.target]
	if m != nil && m.dead {
		m = nil
	}
	switch s.Shape {
	case gamedata.ShapeSingle, gamedata.ShapeCircleTarget:
		if m == nil {
			return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET
		}
		if distance(p.pos, m.pos) > float32(s.Range[n-1])+attackRangeSlack || lineBlocked(z.area.Grid, p.pos, m.pos) {
			return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
		}
		return m, dirTo(p.pos, m.pos), pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	case gamedata.ShapeCone, gamedata.ShapeLine:
		var d Vec2
		if m != nil {
			d = dirTo(p.pos, m.pos)
		}
		if d == (Vec2{}) {
			d = unitDir(c.dir) // also when the target stands on the caster
		}
		if m == nil && d == (Vec2{}) {
			return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		}
		return m, d, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	case gamedata.ShapeCircleSelf:
		// No target needed; a living one named is only hit first.
		return m, Vec2{}, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	return nil, Vec2{}, pb.ErrorCode_ERROR_CODE_BAD_REQUEST
}

// dashGap is how far short of its target a dash lands, in tiles.
const dashGap = 1

// dashLanding is where p's dash at target lands (phase 2 spec 4.5): dashGap
// short of target on the segment from p, or p's own position when target is
// no further than that. The answer is OUT_OF_RANGE when p's box would not fit
// there (it overlaps a blocked tile, which a walkable landing tile alone does
// not rule out) or the landing point is in a safe zone.
//
// The segment from p to the landing point is part of the one from p to
// target, which aim has already found in sight, so it is not blocked either.
func (z *Zone) dashLanding(p *player, target *monster) (Vec2, pb.ErrorCode) {
	d := distance(p.pos, target.pos)
	if d <= dashGap {
		return p.pos, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	u := dirTo(p.pos, target.pos)
	land := Vec2{target.pos.X - u.X*dashGap, target.pos.Y - u.Y*dashGap}
	if !boxFits(z.area.Grid, land) || z.area.InSafe(land) {
		return p.pos, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
	}
	return land, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// boxFits reports whether an entity's box centred on pos overlaps no blocked
// tile, with the same edge tolerance as Step.
func boxFits(g Grid, pos Vec2) bool {
	for tx := floor(pos.X - halfSize + eps); tx <= floor(pos.X+halfSize-eps); tx++ {
		for ty := floor(pos.Y - halfSize + eps); ty <= floor(pos.Y+halfSize-eps); ty++ {
			if g.Blocked(tx, ty) {
				return false
			}
		}
	}
	return true
}

// victims are the monsters p's cast of s at skill level n hits, aimed at
// primary along dir as aim decided, in hit order (phase 2 spec 4.4): the
// primary target first if it is in the area, then the rest by distance to
// the area's centre, ties by lower id, at most max_targets of them. Only
// living monsters in sight of the area's origin count, and one standing on
// the origin is always inside.
//
// circle_target's origin and centre are the primary target; the other
// shapes' are the caster. A cone reaches range[n-1] within cone_deg/2 of
// dir; a line runs range[n-1] along dir and is width wide. Neither adds
// attackRangeSlack.
func (z *Zone) victims(p *player, s gamedata.Skill, n int, primary *monster, dir Vec2) []*monster {
	if s.Shape == gamedata.ShapeSingle {
		return []*monster{primary}
	}
	origin := p.pos
	if s.Shape == gamedata.ShapeCircleTarget {
		origin = primary.pos
	}
	var reach float32 // how far from the origin the area extends
	var inArea func(v Vec2, d float32) bool
	switch s.Shape {
	case gamedata.ShapeCircleSelf, gamedata.ShapeCircleTarget:
		reach = float32(s.Radius)
		inArea = func(_ Vec2, d float32) bool { return d <= reach }
	case gamedata.ShapeCone:
		reach = float32(s.Range[n-1])
		cosHalf := math.Cos(s.ConeDeg / 2 * math.Pi / 180)
		inArea = func(v Vec2, d float32) bool {
			return d <= reach && float64(dot(v, dir))/float64(d) >= cosHalf
		}
	case gamedata.ShapeLine:
		length, half := float32(s.Range[n-1]), float32(s.Width/2)
		reach = length + half
		inArea = func(v Vec2, _ float32) bool {
			along, across := dot(v, dir), dir.X*v.Y-dir.Y*v.X
			return along >= 0 && along <= length && abs32(across) <= half
		}
	default:
		return nil
	}
	inside := func(m *monster) bool {
		if m.dead || lineBlocked(z.area.Grid, origin, m.pos) {
			return false
		}
		v := Vec2{m.pos.X - origin.X, m.pos.Y - origin.Y}
		d := distance(origin, m.pos)
		if d == 0 {
			return true
		}
		if dir == (Vec2{}) && (s.Shape == gamedata.ShapeCone || s.Shape == gamedata.ShapeLine) {
			return false // aimed at a target on the caster: no direction to reach along
		}
		return inArea(v, d)
	}
	var out []*monster
	if primary != nil && inside(primary) {
		out = append(out, primary)
	}
	var rest []*monster
	for _, m := range z.monstersInBox(Vec2{origin.X - reach, origin.Y - reach}, Vec2{origin.X + reach, origin.Y + reach}) {
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

// minDirLen is the shortest cast dir taken as a direction; anything shorter
// is noise from a stick at rest and counts as zero.
const minDirLen = 1e-3

// unitDir is d scaled to length 1, or zero when d is shorter than
// minDirLen.
func unitDir(d Vec2) Vec2 {
	l := float32(math.Hypot(float64(d.X), float64(d.Y)))
	if l < minDirLen {
		return Vec2{}
	}
	return Vec2{d.X / l, d.Y / l}
}

func dot(a, b Vec2) float32 { return a.X*b.X + a.Y*b.Y }

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

// dirTo is the unit vector from a to b, or zero when they coincide.
func dirTo(a, b Vec2) Vec2 {
	d := distance(a, b)
	if d == 0 {
		return Vec2{}
	}
	return Vec2{(b.X - a.X) / d, (b.Y - a.Y) / d}
}
