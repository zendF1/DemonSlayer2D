package zone

import (
	"maps"
	"math"
	"slices"
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
)

type monster struct {
	entity
	camp  *campState
	home  Vec2 // where it spawned
	state monsterState

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
			z.wander(m)
		}
	}
}

// wander walks an idle monster to a random spot near home every 4-8 s.
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
	step := wanderSpeed * float32(m.camp.Kind.Speed) * z.cfg.MoveSpeed / float32(z.cfg.TickRate)
	if z.walkTo(m, m.wanderTo, step) {
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
// monster grid. It stops m and returns true once m has arrived or is stuck.
func (z *Zone) walkTo(m *monster, to Vec2, step float32) bool {
	d := distance(m.pos, to)
	if d < arriveEps {
		z.stopMonster(m)
		return true
	}
	dir := Vec2{(to.X - m.pos.X) / d, (to.Y - m.pos.Y) / d}
	np := Step(z.mgrid, m.pos, dir, min(step, d))
	if np == m.pos {
		z.stopMonster(m)
		return true
	}
	arrived := distance(np, to) < arriveEps
	if arrived {
		np = to
		dir = Vec2{}
	}
	m.pos, m.dir, m.changed = np, dir, true
	z.updateCell(&m.entity)
	return arrived
}

func (z *Zone) stopMonster(m *monster) {
	if m.dir != (Vec2{}) {
		m.dir = Vec2{}
		m.changed = true
	}
}

// ticks converts seconds to a whole number of ticks, rounding up, at least 1.
func (z *Zone) ticks(sec float64) uint32 {
	// The epsilon keeps float error (0.7*10 = 7.000000000000001) from adding a tick.
	n := math.Ceil(sec*float64(z.cfg.TickRate) - 1e-9)
	return uint32(max(n, 1))
}

func distance(a, b Vec2) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}
