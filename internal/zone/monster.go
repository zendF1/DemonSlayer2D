package zone

import (
	"maps"
	"slices"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

type monsterState int

// Monster states (spec 4.2).
const (
	idle      monsterState = iota // wanders around home
	chase                         // walks to its target
	attack                        // hits its target every AttackSec
	returning                     // walks home, ignoring everyone
)

const (
	wanderRadius = 2   // tiles from home
	wanderMinSec = 4.0 // pause between wanders
	wanderMaxSec = 8.0
	wanderSpeed  = 0.5 // factor of the kind's speed
	arriveEps    = 1e-3
	// minProgress is the fraction of a step a walk must gain towards its target
	// each tick. Sliding along a wall gains less and less, so below this the
	// monster counts as stuck instead of crawling in place.
	minProgress = 0.1
	// chaseStop is how close a chasing monster walks to its target.
	chaseStop = 1.3
	// returnStuckSec is how long a returning monster may stay stuck before it
	// is put back home.
	returnStuckSec = 2.0
)

type walkResult int

const (
	walkMoving  walkResult = iota // still on its way
	walkArrived                   // at the target, stopped
	walkStuck                     // can make no real progress, stopped
)

type monster struct {
	entity
	camp  *campState
	home  Vec2 // where it spawned
	state monsterState

	target     uint64              // player it chases or attacks; 0 when none
	attackers  map[uint64]struct{} // players who hit it since it last went home
	nextAttack uint32              // first tick it may hit again
	stuck      uint32              // consecutive ticks a returning monster could not move

	wanderTo   Vec2
	nextWander uint32 // tick at which an idle monster picks a new wander target
}

// thinkMonsters runs every live monster's behaviour for this tick, in id order
// so a seeded zone always plays out the same way.
func (z *Zone) thinkMonsters() {
	for _, id := range slices.Sorted(maps.Keys(z.monsters)) {
		m := z.monsters[id]
		if m.dead {
			continue
		}
		switch m.state {
		case idle:
			if p := z.nearestInAggro(m); p != nil {
				m.target, m.state = p.id, chase
				z.fight(m)
			} else {
				z.wander(m)
			}
		case chase, attack:
			z.fight(m)
		case returning:
			z.goHome(m)
		}
	}
}

// validTarget returns the player with id if a monster may chase or hit it: alive,
// in this zone and outside the safe zones.
func (z *Zone) validTarget(id uint64) *player {
	p := z.players[id]
	if p == nil || p.dead || z.area.InSafe(p.pos) {
		return nil
	}
	return p
}

// nearer reports whether p is nearer to m than best, breaking ties by the
// lower id so a seeded zone always picks the same player.
func nearer(m *monster, p, best *player) bool {
	if best == nil {
		return true
	}
	dp, db := distance(m.pos, p.pos), distance(m.pos, best.pos)
	return dp < db || dp == db && p.id < best.id
}

// nearestInAggro is the nearest valid player within the kind's aggro range.
// Only the AOI cells around m are searched.
func (z *Zone) nearestInAggro(m *monster) *player {
	reach := float32(m.camp.Kind.AggroRange)
	var best *player
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for id, e := range z.cells[cellKey{m.cell.X + dx, m.cell.Y + dy}] {
				if e.kind != pb.EntityKind_ENTITY_KIND_PLAYER {
					continue
				}
				if p := z.validTarget(id); p != nil && distance(m.pos, p.pos) <= reach && nearer(m, p, best) {
					best = p
				}
			}
		}
	}
	return best
}

// retarget picks a new target once the old one is lost (spec 4.2): the nearest
// valid player who has hit m, else the nearest valid player in aggro range.
func (z *Zone) retarget(m *monster) *player {
	var best *player
	for id := range m.attackers {
		if p := z.validTarget(id); p != nil && nearer(m, p, best) {
			best = p
		}
	}
	if best != nil {
		return best
	}
	return z.nearestInAggro(m)
}

// fight runs a monster in chase or attack: it keeps its target while the
// target stays valid, goes home when lured past the leash range, walks up to
// the target and hits it every AttackSec once in range.
func (z *Zone) fight(m *monster) {
	t := z.validTarget(m.target)
	if t == nil {
		if t = z.retarget(m); t == nil {
			z.startReturn(m)
			return
		}
		m.target = t.id
	}
	k := &m.camp.Kind
	if distance(m.pos, m.home) > float32(k.LeashRange) {
		z.startReturn(m)
		return
	}
	reach := float32(k.AttackRange)
	if m.state == attack && distance(m.pos, t.pos) > reach {
		m.state = chase
	}
	if m.state == chase {
		z.chaseStep(m, t)
		if distance(m.pos, t.pos) > reach {
			return
		}
		m.state = attack
	}
	z.stopMonster(m)
	if z.tickNo >= m.nextAttack {
		dmg := combat.Damage(z.area.Levels[m.level].Attack, t.stats.PhysDefense, m.level)
		z.damagePlayer(t, m.id, dmg)
		m.nextAttack = z.tickNo + z.ticks(k.AttackSec)
	}
}

// chaseStep walks m straight towards t until it is chaseStop tiles away. A
// stuck monster just stands; it keeps its state (spec 4.2).
func (z *Zone) chaseStep(m *monster, t *player) {
	// Stop a little inside the attack range: a walk ends within arriveEps of
	// its target, and float error must not leave m just beyond reach. The
	// reach/2 floor only matters for absurdly short ranges.
	reach := float32(m.camp.Kind.AttackRange)
	stop := min(chaseStop, max(reach-2*arriveEps, reach/2))
	d := distance(m.pos, t.pos)
	if d <= stop {
		z.stopMonster(m)
		return
	}
	f := stop / d
	to := Vec2{t.pos.X + (m.pos.X-t.pos.X)*f, t.pos.Y + (m.pos.Y-t.pos.Y)*f}
	z.walkTo(m, to, z.monsterStep(m))
}

// startReturn sends m home, forgetting its target and attackers.
func (z *Zone) startReturn(m *monster) {
	m.state, m.target, m.stuck = returning, 0, 0
	clear(m.attackers)
	z.goHome(m)
}

// goHome walks a returning monster home at full speed. One stuck for
// returnStuckSec in a row is put straight back home.
func (z *Zone) goHome(m *monster) {
	switch z.walkTo(m, m.home, z.monsterStep(m)) {
	case walkArrived:
		z.arriveHome(m)
	case walkStuck:
		if m.stuck++; m.stuck >= z.ticks(returnStuckSec) {
			m.pos, m.changed, m.snap = m.home, true, true
			z.updateCell(&m.entity)
			z.arriveHome(m)
		}
	default:
		m.stuck = 0
	}
}

// arriveHome makes a returned monster idle at home with full HP.
func (z *Zone) arriveHome(m *monster) {
	if m.pos != m.home {
		m.pos, m.changed = m.home, true
		z.updateCell(&m.entity)
	}
	z.stopMonster(m)
	if m.hp != m.maxHP {
		m.hp = m.maxHP
		z.hpChanged(&m.entity)
	}
	m.state, m.stuck = idle, 0
	m.nextWander = z.tickNo + z.wanderDelay()
}

// monsterStep is how far m walks in one tick at full speed.
func (z *Zone) monsterStep(m *monster) float32 {
	return float32(m.camp.Kind.Speed) * z.cfg.MoveSpeed / float32(z.cfg.TickRate)
}

// wander walks an idle monster to a random spot near home every 4-8 s. The
// straight path stays within wanderRadius of home, but sliding along a wall can
// take it a little beyond.
func (z *Zone) wander(m *monster) {
	if m.dir == (Vec2{}) {
		if z.tickNo < m.nextWander {
			return
		}
		to, ok := z.wanderTarget(m)
		if !ok {
			m.nextWander = z.tickNo + z.wanderDelay()
			return
		}
		m.wanderTo = to
	}
	if z.walkTo(m, m.wanderTo, wanderSpeed*z.monsterStep(m)) != walkMoving { // stuck just ends this walk
		m.nextWander = z.tickNo + z.wanderDelay()
	}
}

// wanderTarget picks a random tile centre within wanderRadius of home that
// monsters may stand on, other than the one m stands on.
func (z *Zone) wanderTarget(m *monster) (Vec2, bool) {
	hx, hy := floor(m.home.X), floor(m.home.Y)
	var opts []Vec2
	for ty := hy - wanderRadius; ty <= hy+wanderRadius; ty++ {
		for tx := hx - wanderRadius; tx <= hx+wanderRadius; tx++ {
			c := Vec2{float32(tx) + 0.5, float32(ty) + 0.5}
			if c == m.pos || distance(c, m.home) > wanderRadius || z.mgrid.Blocked(tx, ty) {
				continue
			}
			opts = append(opts, c)
		}
	}
	if len(opts) == 0 {
		return Vec2{}, false
	}
	return opts[z.rng.IntN(len(opts))], true
}

func (z *Zone) wanderDelay() uint32 {
	return z.ticks(wanderMinSec + (wanderMaxSec-wanderMinSec)*z.rng.Float64())
}

// walkTo moves m one tick of at most step tiles straight towards to, on the
// monster grid, sliding along walls. Once m arrives, or would gain less than
// minProgress of a step towards to, it stops m and says which.
func (z *Zone) walkTo(m *monster, to Vec2, step float32) walkResult {
	d := distance(m.pos, to)
	if d < arriveEps {
		z.stopMonster(m)
		return walkArrived
	}
	move := min(step, d)
	dir := Vec2{(to.X - m.pos.X) / d, (to.Y - m.pos.Y) / d}
	np := Step(z.mgrid, m.pos, dir, move)
	left := distance(np, to)
	if left >= arriveEps && d-left < minProgress*move {
		z.stopMonster(m)
		return walkStuck
	}
	res := walkMoving
	if left < arriveEps {
		np, dir, res = to, Vec2{}, walkArrived
	}
	m.pos, m.dir, m.changed = np, dir, true
	z.updateCell(&m.entity)
	return res
}

func (z *Zone) stopMonster(m *monster) {
	if m.dir != (Vec2{}) {
		m.dir = Vec2{}
		m.changed = true
	}
}
