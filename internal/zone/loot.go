package zone

import (
	"math"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/pb"
)

// dropSpread is how far from the corpse items dropped together are placed
// (phase 3a spec 4.5), in tiles.
const dropSpread = 0.6

// groundItem is a dropped item or stack lying in the AOI grid. Only the
// character owner sees it and may pick it up; it is gone at tick expires.
// Ground items are never saved (phase 3a spec 1.3).
type groundItem struct {
	entity
	owner   int64 // the killer's character id, so it survives a re-join
	item    items.Item
	expires uint32
}

// loot gives m's killer what m drops (phase 3a spec 4.3): Xu straight into
// the wallet, the rest on the ground by the corpse. A nil killer (the
// attacker has left the zone) or an area without item data gets nothing.
// The killer may be dead.
func (z *Zone) loot(m *monster, killer *player) {
	d := z.area.Items
	if killer == nil || d == nil {
		return
	}
	class, _ := classes.FromPB(killer.class)
	xu, drops := items.RollDrops(z.rng, d, m.region.Kind.Rank, m.level, class)
	if xu > 0 {
		killer.xu += xu
		killer.statsDirty = true
	}
	expires := z.tickNo + z.ticks(d.GroundSec)
	for i, it := range drops {
		g := &groundItem{
			entity: entity{id: z.cfg.NewID(), kind: pb.EntityKind_ENTITY_KIND_ITEM, name: itemName(d, it.Code),
				pos: z.dropAt(m.pos, i, len(drops))},
			owner: killer.charID, item: it, expires: expires,
		}
		g.cell = z.cellOf(g.pos)
		z.ground[g.id] = g
		z.addToCell(&g.entity)
	}
}

// dropAt is where the i-th of n items dropped at pos goes (phase 3a spec
// 4.5): dropSpread tiles away at angle 2πi/n, or pos itself when n is 1 or
// that tile is not walkable.
func (z *Zone) dropAt(pos Vec2, i, n int) Vec2 {
	if n <= 1 {
		return pos
	}
	a := 2 * math.Pi * float64(i) / float64(n)
	p := Vec2{pos.X + float32(dropSpread*math.Cos(a)), pos.Y + float32(dropSpread*math.Sin(a))}
	if z.area.Grid.Blocked(floor(p.X), floor(p.Y)) {
		return pos
	}
	return p
}

// runGroundTimers removes the ground items whose time is up; their owner
// gets EntityLeave in syncView.
func (z *Zone) runGroundTimers() {
	for id, g := range z.ground {
		if z.tickNo >= g.expires {
			z.removeFromCell(&g.entity)
			delete(z.ground, id)
		}
	}
}

// seesGround reports whether p may see the ground item id: only its owner
// does, by character, whichever entity it now plays as.
func (z *Zone) seesGround(p *player, id uint64) bool {
	g := z.ground[id]
	return g != nil && g.owner == p.charID
}

// itemName is the display name of code in d.
func itemName(d *gamedata.Items, code string) string {
	if e, ok := d.Equipment[code]; ok {
		return e.Name
	}
	return d.Stackables[code].Name
}
