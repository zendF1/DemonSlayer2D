package zone

import (
	"maps"
	"slices"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

type monsterState int

// Monster states (spec 4). chase and attack together are the spec's chase.
const (
	wander monsterState = iota // rests, then walks to a random region tile nearby
	shoot                      // stands still, shooting the nearest player in sight
	chase                      // walks to its target
	attack                     // hits its target every AttackSec
	back                       // walks back onto its region, ignoring everyone
)

const (
	wanderReach = 6   // tiles from its position a wander walk may go
	restMinSec  = 2.0 // pause between wander walks
	restMaxSec  = 5.0
	wanderSpeed = 0.5 // factor of the kind's speed
	arriveEps   = 1e-3
	// minProgress is the fraction of a step a walk must gain towards its target
	// each tick. Sliding along a wall gains less and less, so below this the
	// monster counts as stuck instead of crawling in place.
	minProgress = 0.1
	// chaseStop is how close a chasing monster walks to its target.
	chaseStop = 1.3
	// returnStuckSec is how long a monster going back may stay stuck before
	// it is put at its region's nest.
	returnStuckSec = 2.0
	// recentHitSec is how recently a player must have hit a monster to be
	// chased once the monster lets its target go (spec 4.4).
	recentHitSec = 5.0
)

type walkResult int

const (
	walkMoving  walkResult = iota // still on its way
	walkArrived                   // at the target, stopped
	walkStuck                     // can make no real progress, stopped
)

type monster struct {
	entity
	region *regionState
	state  monsterState

	target     uint64            // player it chases or attacks; 0 when none
	chaseFrom  Vec2              // where the current chase started
	safeSince  uint32            // tick its target was first seen in a safe zone this time; 0 while outside
	outSince   uint32            // tick its target was first seen beyond the region margin this time; 0 while inside
	attackers  map[uint64]uint32 // the last tick each player hit it
	nextAttack uint32            // first tick it may hit again
	stuck      uint32            // consecutive ticks a monster going back could not move
	diedAt     uint32            // tick it died at, while dead

	// Over its whole life: the damage each player has done to it that counts
	// for EXP (overkill does not), and the EXP paid to each.
	dmgBy   map[uint64]int
	expPaid map[uint64]int64

	wanderTo   Vec2
	nextWander uint32 // tick at which a resting monster picks a new wander target
	nextShot   uint32 // tick of a shooting monster's next shot
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
		case wander, shoot:
			if p := z.grudgeTarget(m); p != nil {
				z.startChase(m, p.id)
				z.fight(m)
			} else if m.state == shoot {
				z.shoot(m)
			} else if z.nearestShootable(m) != nil {
				z.startShoot(m)
			} else {
				z.wander(m)
			}
		case chase, attack:
			z.fight(m)
		case back:
			if p := z.grudgeTarget(m); p != nil {
				z.startChase(m, p.id)
				z.fight(m)
			} else {
				z.goBack(m)
			}
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

// nearestShootable is the nearest valid player m may shoot (spec 3.2): within
// the kind's detect range with no blocked tile between them. Only the AOI
// cells around m are searched.
func (z *Zone) nearestShootable(m *monster) *player {
	reach := float32(m.region.Kind.DetectRange)
	var best *player
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for id, e := range z.cells[cellKey{m.cell.X + dx, m.cell.Y + dy}] {
				if e.kind != pb.EntityKind_ENTITY_KIND_PLAYER {
					continue
				}
				p := z.validTarget(id)
				if p != nil && distance(m.pos, p.pos) <= reach && !lineBlocked(z.area.Grid, m.pos, p.pos) && nearer(m, p, best) {
					best = p
				}
			}
		}
	}
	return best
}

// startShoot stops m to shoot; the first shot comes after the windup (spec 4.3).
func (z *Zone) startShoot(m *monster) {
	z.stopMonster(m)
	m.state = shoot
	m.nextShot = z.tickNo + z.ticks(m.region.Kind.ShootWindupSec)
}

// shoot runs a shooting monster. It stands still, and on each shot picks the
// nearest player it may shoot. With nobody left it goes back to wandering.
// Shooting never gives it a target or starts a chase.
func (z *Zone) shoot(m *monster) {
	if z.tickNo < m.nextShot {
		return
	}
	p := z.nearestShootable(m)
	if p == nil {
		z.startWander(m)
		return
	}
	dmg := combat.MonsterRangedDamage(z.area.Levels[m.level].Attack, p.stats.PhysDefense, m.level)
	z.damagePlayer(p, m.id, dmg, true)
	m.nextShot += z.ticks(m.region.Kind.ShootSec)
}

// startWander makes m wander, starting with a rest.
func (z *Zone) startWander(m *monster) {
	z.stopMonster(m)
	m.state = wander
	m.nextWander = z.tickNo + z.restDelay()
}

// startChase sets m on player id, the chase starting where m stands.
func (z *Zone) startChase(m *monster, id uint64) {
	m.target, m.state, m.chaseFrom, m.stuck, m.safeSince, m.outSince = id, chase, m.pos, 0, 0, 0
}

// grudgeTarget is the nearest player m bears a grudge against (spec 4.2,
// checked in wander, shoot and back): one
// who hit it within GrudgeSec, is a valid target, and is within the detect
// range with a clear line. Such a player is chased on sight, not shot.
func (z *Zone) grudgeTarget(m *monster) *player {
	z.forgetOldAttackers(m)
	reach := float32(m.region.Kind.DetectRange)
	var best *player
	for id := range m.attackers {
		p := z.validTarget(id)
		if p != nil && distance(m.pos, p.pos) <= reach && !lineBlocked(z.area.Grid, m.pos, p.pos) && nearer(m, p, best) {
			best = p
		}
	}
	return best
}

// forgetOldAttackers drops the players who last hit m more than GrudgeSec ago.
func (z *Zone) forgetOldAttackers(m *monster) {
	old := z.ticks(m.region.Kind.GrudgeSec)
	for id, at := range m.attackers {
		if z.tickNo-at > old {
			delete(m.attackers, id)
		}
	}
}

// chaseTarget returns m's target if m keeps chasing it (spec 4.4), and
// whether m may hit it now. The target is let go when it is dead or gone,
// when m is beyond the chase range of where the chase started, when it has
// stayed in a safe zone for SafeWaitSec, or, outside a safe zone, when it has
// stayed beyond the region margin for MarginGraceSec. While the target is in
// a safe zone m keeps it but may not hit it, and the margin does not count.
func (z *Zone) chaseTarget(m *monster) (*player, bool) {
	p := z.players[m.target]
	k := &m.region.Kind
	if p == nil || p.dead || distance(m.pos, m.chaseFrom) > float32(k.ChaseRange) {
		return nil, false
	}
	if z.area.InSafe(p.pos) {
		if m.safeSince == 0 {
			m.safeSince = z.tickNo
		}
		if z.tickNo-m.safeSince >= z.ticks(k.SafeWaitSec) {
			return nil, false
		}
		return p, false
	}
	m.safeSince = 0
	if m.region.nearMargin(p.pos, float32(k.RegionMargin)) {
		m.outSince = 0
		return p, true
	}
	if m.outSince == 0 {
		m.outSince = z.tickNo
	}
	if z.tickNo-m.outSince >= z.ticks(k.MarginGraceSec) {
		return nil, false
	}
	return p, true
}

// fallback picks who m chases instead of its target: the nearest valid
// player other than that target who hit it within recentHitSec and stands
// within the region margin.
func (z *Zone) fallback(m *monster, old uint64) *player {
	z.forgetOldAttackers(m)
	recent := z.ticks(recentHitSec)
	var best *player
	for id, at := range m.attackers {
		if id == old || z.tickNo-at > recent {
			continue
		}
		if p := z.validTarget(id); p != nil && m.region.nearMargin(p.pos, float32(m.region.Kind.RegionMargin)) && nearer(m, p, best) {
			best = p
		}
	}
	return best
}

// fight runs a monster in chase or attack. First it lets its target go as
// chaseTarget says; it then turns on a recent attacker, or goes back to its
// region. While the target waits in a safe zone, a recent attacker outside
// is turned on at once; with none, the monster walks up to the target as
// close as the monster grid allows, so it stands at the edge. Otherwise it
// walks up to the target and hits it every AttackSec once in range with a
// clear line, the same line of sight a player's slash needs. It never shoots.
func (z *Zone) fight(m *monster) {
	t, hittable := z.chaseTarget(m)
	if !hittable {
		if f := z.fallback(m, m.target); f != nil {
			t = f
			z.startChase(m, t.id) // fallback only picks hittable players
		} else if t == nil {
			z.startBack(m)
			return
		} else {
			m.state = chase
			z.chaseStep(m, t)
			return
		}
	}
	k := &m.region.Kind
	if m.state == attack && !z.canHit(m, t) {
		m.state = chase
	}
	if m.state == chase {
		z.chaseStep(m, t)
		if !z.canHit(m, t) {
			return
		}
		m.state = attack
	}
	z.stopMonster(m)
	if z.tickNo >= m.nextAttack {
		dmg := combat.Damage(z.area.Levels[m.level].Attack, t.stats.PhysDefense, m.level)
		z.damagePlayer(t, m.id, dmg, false)
		m.nextAttack = z.tickNo + z.ticks(k.AttackSec)
	}
}

// canHit reports whether t is within m's attack range with no blocked tile
// between them.
func (z *Zone) canHit(m *monster, t *player) bool {
	return distance(m.pos, t.pos) <= float32(m.region.Kind.AttackRange) && !lineBlocked(z.area.Grid, m.pos, t.pos)
}

// chaseStep walks m straight towards t until it is chaseStop tiles away. While
// a wall blocks the line to t it walks at t itself, however close, so sliding
// along the wall takes it round the corner. A stuck monster just stands; it
// keeps its state (spec 4.2).
func (z *Zone) chaseStep(m *monster, t *player) {
	step := z.chaseSpeed(m)
	if lineBlocked(z.area.Grid, m.pos, t.pos) {
		z.walkTo(m, t.pos, step)
		return
	}
	// Stop a little inside the attack range: a walk ends within arriveEps of
	// its target, and float error must not leave m just beyond reach. The
	// reach/2 floor only matters for absurdly short ranges.
	reach := float32(m.region.Kind.AttackRange)
	stop := min(chaseStop, max(reach-2*arriveEps, reach/2))
	d := distance(m.pos, t.pos)
	if d <= stop {
		z.stopMonster(m)
		return
	}
	f := stop / d
	to := Vec2{t.pos.X + (m.pos.X-t.pos.X)*f, t.pos.Y + (m.pos.Y-t.pos.Y)*f}
	z.walkTo(m, to, step)
}

// startBack sends m back to its region, forgetting its target (spec 4.5).
func (z *Zone) startBack(m *monster) {
	m.state, m.target, m.stuck = back, 0, 0
	z.goBack(m)
}

// goBack runs a monster going back. On a region tile it wanders again; off
// the region it walks at full speed to the nearest region tile. One stuck for
// returnStuckSec in a row is put at the region's nest. It does not heal, and
// its EXP tracking stays.
func (z *Zone) goBack(m *monster) {
	r := m.region
	if r.hasTile(floor(m.pos.X), floor(m.pos.Y)) {
		z.startWander(m)
		return
	}
	switch z.walkTo(m, r.nearestTile(m.pos), z.monsterStep(m)) {
	case walkArrived:
		z.startWander(m)
	case walkStuck:
		if m.stuck++; m.stuck >= z.ticks(returnStuckSec) {
			m.pos, m.changed, m.snap = r.Nest, true, true
			z.updateCell(&m.entity)
			z.startWander(m)
		}
	default:
		m.stuck = 0
	}
}

// monsterStep is how far m walks in one tick at full speed.
func (z *Zone) monsterStep(m *monster) float32 {
	return float32(m.region.Kind.Speed) * z.cfg.MoveSpeed / float32(z.cfg.TickRate)
}

// chaseSpeed is how far m moves in one tick while chasing, faster than a
// player so a retreating one cannot kite it (spec 4.4).
func (z *Zone) chaseSpeed(m *monster) float32 {
	return float32(m.region.Kind.ChaseSpeed) * z.cfg.MoveSpeed / float32(z.cfg.TickRate)
}

// wander runs a wandering monster (spec 4.2): it rests restMinSec to
// restMaxSec, then walks at wanderSpeed to a random region tile within
// wanderReach, and rests again once it arrives or is stuck.
func (z *Zone) wander(m *monster) {
	if m.dir == (Vec2{}) {
		if z.tickNo < m.nextWander {
			return
		}
		to, ok := z.wanderTarget(m)
		if !ok {
			m.nextWander = z.tickNo + z.restDelay()
			return
		}
		m.wanderTo = to
	}
	if z.walkTo(m, m.wanderTo, wanderSpeed*z.monsterStep(m)) != walkMoving {
		m.nextWander = z.tickNo + z.restDelay()
	}
}

// wanderTarget picks a random tile of m's region within wanderReach of m,
// other than the tile m stands on, whose straight line from m crosses only
// region tiles. The walk there then stays in the region, even round the
// inside corner of an L-shaped region, and never heads into a wall.
func (z *Zone) wanderTarget(m *monster) (Vec2, bool) {
	tx, ty := floor(m.pos.X), floor(m.pos.Y)
	inside := regionGrid{m.region}
	var opts []Vec2
	for _, c := range m.region.Tiles {
		if (floor(c.X) == tx && floor(c.Y) == ty) || distance(c, m.pos) > wanderReach || lineBlocked(inside, m.pos, c) {
			continue
		}
		opts = append(opts, c)
	}
	if len(opts) == 0 {
		return Vec2{}, false
	}
	return opts[z.rng.IntN(len(opts))], true
}

// restDelay is a random rest between wander walks.
func (z *Zone) restDelay() uint32 {
	return z.ticks(restMinSec + (restMaxSec-restMinSec)*z.rng.Float64())
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
