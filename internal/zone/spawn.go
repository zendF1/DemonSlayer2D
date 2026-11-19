package zone

import "demonslayer2d/internal/pb"

// campState is a camp's live state in this zone.
type campState struct {
	Camp
	alive int
}

// spawnAll fills every camp to its Count. It runs once, when the zone opens.
func (z *Zone) spawnAll() {
	for i := range z.area.Camps {
		c := &campState{Camp: z.area.Camps[i]}
		z.camps = append(z.camps, c)
		for n := c.Count - c.alive; n > 0; n-- {
			z.spawn(c)
		}
	}
}

// spawn adds one monster to c: on a random camp tile, at a random level in the
// camp's range, with full HP.
func (z *Zone) spawn(c *campState) {
	if len(c.Tiles) == 0 || c.MaxLevel < c.MinLevel {
		z.log.Error("camp cannot spawn", "camp", c.ID, "tiles", len(c.Tiles), "min_level", c.MinLevel, "max_level", c.MaxLevel)
		return
	}
	level := c.MinLevel + z.rng.IntN(c.MaxLevel-c.MinLevel+1)
	stats, ok := z.area.Levels[level]
	if !ok {
		z.log.Error("no monster stats for level", "camp", c.ID, "level", level)
		return
	}
	pos := c.Tiles[z.rng.IntN(len(c.Tiles))]
	m := &monster{
		entity: entity{
			id:    z.cfg.NewID(),
			kind:  pb.EntityKind_ENTITY_KIND_MONSTER,
			name:  c.Kind.Name,
			level: level,
			pos:   pos,
			hp:    stats.HP,
			maxHP: stats.HP,
		},
		camp:       c,
		home:       pos,
		attackers:  make(map[uint64]struct{}),
		state:      idle,
		nextWander: z.tickNo + z.wanderDelay(),
	}
	m.cell = z.cellOf(pos)
	z.monsters[m.id] = m
	z.addToCell(&m.entity)
	c.alive++
}
