package zone

import (
	"math"

	"demonslayer2d/internal/pb"
)

type cellKey struct{ X, Y int }

// entity is the state players and monsters share: what viewers see of them
// and where they sit in the AOI grid.
type entity struct {
	id    uint64
	kind  pb.EntityKind
	name  string
	class pb.CharClass // unspecified for monsters
	level int

	pos, dir Vec2
	cell     cellKey

	changed bool // pos or dir changed this tick; sent as EntityMove
	snap    bool // viewers jump to pos instead of interpolating

	hp, maxHP int
	dead      bool

	// weaponEnhance is the enhance level of a player's worn weapon when
	// wearable, else 0; viewers draw the glow from it (phase 3b spec 4.3).
	weaponEnhance int
}

// enterEnv is the EntityEnter for e; a ground item carries its item.
func (z *Zone) enterEnv(e *entity) *pb.Envelope {
	en := &pb.EntityEnter{
		Id:        e.id,
		Kind:      e.kind,
		Name:      e.name,
		CharClass: e.class,
		Level:     int32(e.level),
		X:         e.pos.X,
		Y:         e.pos.Y,
		DirX:      e.dir.X,
		DirY:      e.dir.Y,
		Hp:        int32(e.hp),
		MaxHp:     int32(e.maxHP),
		Dead:      e.dead,

		WeaponEnhance: int32(e.weaponEnhance),
	}
	if g := z.ground[e.id]; g != nil && e.kind == pb.EntityKind_ENTITY_KIND_ITEM {
		en.Item = itemInfo(&g.item)
	}
	return &pb.Envelope{Payload: &pb.Envelope_EntityEnter{EntityEnter: en}}
}

func (z *Zone) cellOf(pos Vec2) cellKey {
	size := float64(z.cfg.AOICellSize)
	return cellKey{int(math.Floor(float64(pos.X) / size)), int(math.Floor(float64(pos.Y) / size))}
}

func (z *Zone) addToCell(e *entity) {
	c := z.cells[e.cell]
	if c == nil {
		c = make(map[uint64]*entity)
		z.cells[e.cell] = c
	}
	c[e.id] = e
}

func (z *Zone) removeFromCell(e *entity) {
	if c := z.cells[e.cell]; c != nil {
		delete(c, e.id)
		if len(c) == 0 {
			delete(z.cells, e.cell)
		}
	}
}

func (z *Zone) updateCell(e *entity) {
	if nc := z.cellOf(e.pos); nc != e.cell {
		z.removeFromCell(e)
		e.cell = nc
		z.addToCell(e)
	}
}

// nearby returns the entities in the 3x3 AOI cells around c.
func (z *Zone) nearby(c cellKey) map[uint64]*entity {
	near := make(map[uint64]*entity)
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for id, e := range z.cells[cellKey{c.X + dx, c.Y + dy}] {
				near[id] = e
			}
		}
	}
	return near
}
