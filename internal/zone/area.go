package zone

import (
	"demonslayer2d/internal/gamedata"
)

// Rect is a rectangle of tiles: X..X+W-1 by Y..Y+H-1.
type Rect struct{ X, Y, W, H int }

func (r Rect) containsTile(tx, ty int) bool {
	return tx >= r.X && tx < r.X+r.W && ty >= r.Y && ty < r.Y+r.H
}

// Circle is a disc in tile units.
type Circle struct {
	C Vec2
	R float32
}

// Camp is where a group of monsters of one kind spawns. Tiles are the centres
// of the walkable tiles in the camp; each spawn picks one at random.
type Camp struct {
	ID                        string
	Tiles                     []Vec2
	Count, MinLevel, MaxLevel int
	Kind                      gamedata.Kind
}

// Area is everything the zone knows about its map: collision, map triggers and
// the monsters that live there. Levels is shared between zones and read-only.
type Area struct {
	Grid    Grid
	Safe    []Rect
	Rests   []Circle
	Respawn Vec2
	Camps   []Camp
	Levels  map[int]gamedata.Level
}

// InSafe reports whether the tile containing p is in a safe zone.
func (a *Area) InSafe(p Vec2) bool {
	if bad(p.X) || bad(p.Y) {
		return false
	}
	return a.safeTile(floor(p.X), floor(p.Y))
}

func (a *Area) safeTile(tx, ty int) bool {
	for _, r := range a.Safe {
		if r.containsTile(tx, ty) {
			return true
		}
	}
	return false
}

// monsterGrid is the grid monsters move on: safe-zone tiles block them too (spec 4.2).
func (a *Area) monsterGrid() Grid {
	if len(a.Safe) == 0 {
		return a.Grid
	}
	return monsterGrid{a}
}

type monsterGrid struct{ a *Area }

func (g monsterGrid) Blocked(tx, ty int) bool {
	return g.a.Grid.Blocked(tx, ty) || g.a.safeTile(tx, ty)
}
