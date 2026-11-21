package zone

import (
	"maps"
	"math"
	"slices"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/pb"
)

// spawnSamples is how many random tiles best-candidate placement tries per monster.
const spawnSamples = 8

// regionState is a region's live state in this zone.
type regionState struct {
	Region
	tiles   map[[2]int]struct{} // the region tiles, by tile coordinates
	present int                 // monsters in the zone, corpses included
	pending []uint32            // ticks at which a new monster is due, one per death
}

func newRegionState(r Region) *regionState {
	s := &regionState{Region: r, tiles: make(map[[2]int]struct{}, len(r.Tiles))}
	for _, t := range r.Tiles {
		s.tiles[[2]int{floor(t.X), floor(t.Y)}] = struct{}{}
	}
	return s
}

// hasTile reports whether tile (tx, ty) is one of the region's tiles.
func (r *regionState) hasTile(tx, ty int) bool {
	_, ok := r.tiles[[2]int{tx, ty}]
	return ok
}

// nearestTile is the region tile nearest p, the first in Tiles order on a
// tie. The region has at least one tile.
func (r *regionState) nearestTile(p Vec2) Vec2 {
	best, bestD := r.Tiles[0], distance(r.Tiles[0], p)
	for _, c := range r.Tiles[1:] {
		if d := distance(c, p); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

// nearMargin reports whether p lies inside one of the region's rects grown
// by margin tiles on every side.
func (r *regionState) nearMargin(p Vec2, margin float32) bool {
	for _, rc := range r.Rects {
		if p.X >= float32(rc.X)-margin && p.X < float32(rc.X+rc.W)+margin &&
			p.Y >= float32(rc.Y)-margin && p.Y < float32(rc.Y+rc.H)+margin {
			return true
		}
	}
	return false
}

// regionGrid is a Grid on which every tile outside r's region tiles is blocked.
type regionGrid struct{ r *regionState }

func (g regionGrid) Blocked(tx, ty int) bool { return !g.r.hasTile(tx, ty) }

// spawnAll fills every region to its Count, spreading the monsters over the
// region by best-candidate placement (spec 4.1). It runs once, when the zone
// opens.
func (z *Zone) spawnAll() {
	for i := range z.area.Regions {
		r := newRegionState(z.area.Regions[i])
		z.regions = append(z.regions, r)
		if len(r.Tiles) == 0 {
			z.log.Error("region has no tiles", "region", r.ID)
			continue
		}
		var placed []Vec2
		for n := r.Count - r.present; n > 0; n-- {
			pos := z.bestCandidate(r.Tiles, placed)
			if z.spawn(r, pos) {
				placed = append(placed, pos)
			}
		}
	}
}

// bestCandidate draws spawnSamples random tiles and returns the one farthest
// from its nearest placed position. Ties go to the earlier sample, so with
// nothing placed yet it is the first sample.
func (z *Zone) bestCandidate(tiles, placed []Vec2) Vec2 {
	var best Vec2
	bestD := float32(-1)
	for range spawnSamples {
		c := tiles[z.rng.IntN(len(tiles))]
		d := float32(math.Inf(1))
		for _, p := range placed {
			d = min(d, distance(c, p))
		}
		if d > bestD {
			best, bestD = c, d
		}
	}
	return best
}

// statsOf is the level row of a monster of kind at level, with HP and EXP
// scaled by the kind's hp_pct and attack by its attack_pct; defense is the
// row's (phase 4d spec 3.2). Every read of a monster's level stats goes
// through it, so none forgets the kind's percentages. ok is false when the
// level has no row.
func (z *Zone) statsOf(kind *gamedata.Kind, level int) (gamedata.Level, bool) {
	l, ok := z.area.Levels[level]
	if !ok {
		return l, false
	}
	l.HP = combat.ScalePct(l.HP, kind.HPPct)
	l.Exp = int64(combat.ScalePct(int(l.Exp), kind.HPPct))
	l.Attack = combat.ScalePct(l.Attack, kind.AttackPct)
	return l, true
}

// monsterStats is statsOf m's kind and level.
func (z *Zone) monsterStats(m *monster) gamedata.Level {
	l, _ := z.statsOf(&m.region.Kind, m.level)
	return l
}

// monsterHitDefense is the defense of p that m's hits meet: the magic one
// when m's kind deals magic damage, else the physical one (phase 4d spec 3.2).
func (z *Zone) monsterHitDefense(m *monster, p *player) int {
	if m.region.Kind.Magic() {
		return p.stats.MagicDefense
	}
	return p.stats.PhysDefense
}

// spawn adds one monster of r at pos, at a random level in r's range, with
// full HP. It reports whether it did.
func (z *Zone) spawn(r *regionState, pos Vec2) bool {
	if r.MaxLevel < r.MinLevel {
		z.log.Error("region cannot spawn", "region", r.ID, "min_level", r.MinLevel, "max_level", r.MaxLevel)
		return false
	}
	level := r.MinLevel + z.rng.IntN(r.MaxLevel-r.MinLevel+1)
	stats, ok := z.statsOf(&r.Kind, level)
	if !ok {
		z.log.Error("no monster stats for level", "region", r.ID, "level", level)
		return false
	}
	m := &monster{
		entity: entity{
			id:    z.cfg.NewID(),
			kind:  pb.EntityKind_ENTITY_KIND_MONSTER,
			name:  r.Kind.Name,
			level: level,
			pos:   pos,
			hp:    stats.HP,
			maxHP: stats.HP,
		},
		region:     r,
		attackers:  make(map[uint64]uint32),
		dmgBy:      make(map[uint64]int),
		expPaid:    make(map[uint64]int64),
		state:      wander,
		nextWander: z.tickNo + z.restDelay(),
	}
	m.cell = z.cellOf(pos)
	z.monsters[m.id] = m
	z.addToCell(&m.entity)
	r.present++
	return true
}

// runSpawnTimers removes corpses corpseSec after death, so viewers get
// EntityLeave, and spawns the monsters whose respawn is due at their region's
// nest (spec 4.1).
func (z *Zone) runSpawnTimers() {
	gone := z.ticks(corpseSec)
	for _, id := range slices.Sorted(maps.Keys(z.monsters)) {
		if m := z.monsters[id]; m.dead && z.tickNo >= m.diedAt+gone {
			z.removeFromCell(&m.entity)
			delete(z.monsters, id)
			m.region.present--
		}
	}
	for _, r := range z.regions {
		due := 0
		r.pending = slices.DeleteFunc(r.pending, func(at uint32) bool {
			if z.tickNo >= at {
				due++
				return true
			}
			return false
		})
		for ; due > 0; due-- {
			z.spawn(r, r.Nest)
		}
	}
}
