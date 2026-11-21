// Package mapdata loads map files: collision grid, spawn and respawn points,
// safe zones, rest points, monster regions and portals.
package mapdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// Point is a position in tile units; (12.5, 3.5) is the centre of tile (12, 3).
type Point struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

type Map struct {
	ID     string
	Name   string
	Type   string
	Width  int
	Height int
	Spawns map[string]Point
	// MinZones is how many zones of this map are always open.
	MinZones int
	// Respawns are the spawn points marked respawn, sorted by id; RespawnIDs
	// holds their ids in the same order.
	Respawns   []Point
	RespawnIDs []string

	SafeZones  []Rect
	RestPoints []RestPoint
	Regions    []Region
	// Portals are in file order.
	Portals []Portal

	// Town is the map a return scroll leads to (itself for a town).
	Town string
	// NPCs are in file order.
	NPCs []NPC

	blocked []bool
}

// NPC roles.
const (
	RoleShop       = "shop"
	RoleBlacksmith = "blacksmith"
	RoleCarriage   = "carriage"
)

// MinNPCDistance is the least distance, in tiles, between two NPCs of a map.
const MinNPCDistance = 1.5

// NPC stands at Pos in a safe zone. Shop is the shop id of a RoleShop NPC;
// Routes are the destinations of a RoleCarriage NPC.
type NPC struct {
	ID, Name, Role, Shop string
	Pos                  Point
	Routes               []Route
}

// Route is a carriage trip to the default spawn point of Map for Xu.
type Route struct {
	Map string
	Xu  int64
}

// NPC returns the NPC with this id.
func (m *Map) NPC(id string) (NPC, bool) {
	for _, n := range m.NPCs {
		if n.ID == id {
			return n, true
		}
	}
	return NPC{}, false
}

// Portal moves a player who walks into Rect to the spawn point ToSpawn of map ToMap.
type Portal struct {
	ID      string
	Rect    Rect
	ToMap   string
	ToSpawn string
}

// Portal returns the portal with this id.
func (m *Map) Portal(id string) (Portal, bool) {
	for _, p := range m.Portals {
		if p.ID == id {
			return p, true
		}
	}
	return Portal{}, false
}

// RectDistance is the distance from p to the nearest point of
// [r.X, r.X+r.W] x [r.Y, r.Y+r.H]; 0 when p is inside.
func RectDistance(p Point, r Rect) float32 {
	x, y := float64(p.X), float64(p.Y)
	dx := max(float64(r.X)-x, 0, x-float64(r.X+r.W))
	dy := max(float64(r.Y)-y, 0, y-float64(r.Y+r.H))
	return float32(math.Hypot(dx, dy))
}

// MinArrivalDistance is how far, in tiles, a portal's arrival point must be
// from every portal of its map, so arriving never walks into a portal.
const MinArrivalDistance = 2

// Rect is a rectangle of tiles: tile (i, j) belongs to it when X <= i < X+W and Y <= j < Y+H.
type Rect struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

// ContainsTile reports whether tile (tx, ty) lies in the rectangle.
func (r Rect) ContainsTile(tx, ty int) bool {
	return tx >= r.X && tx < r.X+r.W && ty >= r.Y && ty < r.Y+r.H
}

// RestPoint heals players within Radius tiles of (X, Y).
type RestPoint struct {
	ID     string  `json:"id"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Radius float32 `json:"radius"`
}

// Region is an area of one monster kind: Count monsters with levels
// MinLevel..MaxLevel spread over the region tiles of Rects, respawning at Nest.
type Region struct {
	ID       string `json:"id"`
	Monster  string `json:"monster"`
	MinLevel int    `json:"min_level"`
	MaxLevel int    `json:"max_level"`
	Count    int    `json:"count"`
	Rects    []Rect `json:"rects"`
	Nest     Point  `json:"nest"`
}

var validTypes = map[string]bool{"town": true, "training": true, "battle": true, "instance": true}

type spawnFile struct {
	ID      string  `json:"id"`
	X       float32 `json:"x"`
	Y       float32 `json:"y"`
	Respawn bool    `json:"respawn"`
}

type portalFile struct {
	ID      string `json:"id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	W       int    `json:"w"`
	H       int    `json:"h"`
	ToMap   string `json:"to_map"`
	ToSpawn string `json:"to_spawn"`
}

type routeFile struct {
	Map string `json:"map"`
	Xu  int64  `json:"xu"`
}

type npcFile struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Role   string      `json:"role"`
	Shop   string      `json:"shop"`
	X      float32     `json:"x"`
	Y      float32     `json:"y"`
	Routes []routeFile `json:"routes"`
}

type mapFile struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Type        string       `json:"type"`
	Width       int          `json:"width"`
	Height      int          `json:"height"`
	MinZones    int          `json:"min_zones"`
	Collision   []string     `json:"collision"`
	SpawnPoints []spawnFile  `json:"spawn_points"`
	SafeZones   []Rect       `json:"safe_zones"`
	RestPoints  []RestPoint  `json:"rest_points"`
	Regions     []Region     `json:"regions"`
	Portals     []portalFile `json:"portals"`
	Town        string       `json:"town"`
	NPCs        []npcFile    `json:"npcs"`
}

// Parse decodes and validates one map file.
func Parse(data []byte) (*Map, error) {
	var f mapFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("mapdata: decode (unknown field?): %w", err)
	}
	if f.ID == "" {
		return nil, errors.New("mapdata: missing id")
	}
	if !validTypes[f.Type] {
		return nil, fmt.Errorf("mapdata %s: invalid type %q", f.ID, f.Type)
	}
	if f.Width <= 0 || f.Height <= 0 {
		return nil, fmt.Errorf("mapdata %s: width and height must be positive", f.ID)
	}
	if len(f.Collision) != f.Height {
		return nil, fmt.Errorf("mapdata %s: collision has %d rows, want %d", f.ID, len(f.Collision), f.Height)
	}
	m := &Map{
		ID: f.ID, Name: f.Name, Type: f.Type, Width: f.Width, Height: f.Height,
		Spawns:  make(map[string]Point),
		blocked: make([]bool, f.Width*f.Height),
	}
	for y, row := range f.Collision {
		if len(row) != f.Width {
			return nil, fmt.Errorf("mapdata %s: collision row %d has length %d, want %d", f.ID, y, len(row), f.Width)
		}
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case '.':
			case '#':
				m.blocked[y*f.Width+x] = true
			default:
				return nil, fmt.Errorf("mapdata %s: invalid char %q at (%d,%d)", f.ID, row[x], x, y)
			}
		}
	}
	for _, sp := range f.SpawnPoints {
		if sp.ID == "" {
			return nil, fmt.Errorf("mapdata %s: spawn point without id", f.ID)
		}
		if _, dup := m.Spawns[sp.ID]; dup {
			return nil, fmt.Errorf("mapdata %s: duplicate spawn point %q", f.ID, sp.ID)
		}
		p := Point{sp.X, sp.Y}
		if !m.Walkable(p) {
			return nil, fmt.Errorf("mapdata %s: spawn point %q at (%.2f, %.2f) is not walkable", f.ID, sp.ID, sp.X, sp.Y)
		}
		m.Spawns[sp.ID] = p
		if sp.Respawn {
			m.RespawnIDs = append(m.RespawnIDs, sp.ID)
		}
	}
	if _, ok := m.Spawns["default"]; !ok {
		return nil, fmt.Errorf("mapdata %s: missing default spawn point", f.ID)
	}
	if f.MinZones < 1 || f.MinZones > 10 {
		return nil, fmt.Errorf("mapdata %s: min_zones %d is not in 1-10", f.ID, f.MinZones)
	}
	m.MinZones = f.MinZones
	sort.Strings(m.RespawnIDs)
	for _, id := range m.RespawnIDs {
		m.Respawns = append(m.Respawns, m.Spawns[id])
	}
	if err := m.parseTriggers(&f); err != nil {
		return nil, err
	}
	if err := m.parsePortals(f.Portals); err != nil {
		return nil, err
	}
	if err := m.Reachable(); err != nil {
		return nil, err
	}
	m.Town = f.Town
	if err := m.parseNPCs(f.NPCs); err != nil {
		return nil, err
	}
	return m, nil
}

// parseNPCs validates the NPCs on this map; Link checks their routes' targets.
func (m *Map) parseNPCs(npcs []npcFile) error {
	reach := m.reachSet()
	for _, nf := range npcs {
		if nf.ID == "" {
			return fmt.Errorf("mapdata %s: npc without id", m.ID)
		}
		if _, dup := m.NPC(nf.ID); dup {
			return fmt.Errorf("mapdata %s: duplicate npc %q", m.ID, nf.ID)
		}
		if nf.Name == "" {
			return fmt.Errorf("mapdata %s: npc %q: missing name", m.ID, nf.ID)
		}
		switch nf.Role {
		case RoleShop, RoleBlacksmith, RoleCarriage:
		default:
			return fmt.Errorf("mapdata %s: npc %q: invalid role %q", m.ID, nf.ID, nf.Role)
		}
		if nf.Role == RoleShop && nf.Shop == "" {
			return fmt.Errorf("mapdata %s: npc %q: role shop needs a shop", m.ID, nf.ID)
		}
		if nf.Role != RoleShop && nf.Shop != "" {
			return fmt.Errorf("mapdata %s: npc %q: shop set on a non-shop npc", m.ID, nf.ID)
		}
		if nf.Role == RoleCarriage && len(nf.Routes) == 0 {
			return fmt.Errorf("mapdata %s: npc %q: a carriage needs routes", m.ID, nf.ID)
		}
		if nf.Role != RoleCarriage && len(nf.Routes) > 0 {
			return fmt.Errorf("mapdata %s: npc %q: routes set on a non-carriage npc", m.ID, nf.ID)
		}
		n := NPC{ID: nf.ID, Name: nf.Name, Role: nf.Role, Shop: nf.Shop, Pos: Point{nf.X, nf.Y}}
		seen := map[string]bool{}
		for _, r := range nf.Routes {
			switch {
			case r.Xu < 1:
				return fmt.Errorf("mapdata %s: npc %q: route to %q: xu must be at least 1", m.ID, nf.ID, r.Map)
			case r.Map == m.ID:
				return fmt.Errorf("mapdata %s: npc %q: route to its own map", m.ID, nf.ID)
			case seen[r.Map]:
				return fmt.Errorf("mapdata %s: npc %q: duplicate route to %q", m.ID, nf.ID, r.Map)
			}
			seen[r.Map] = true
			n.Routes = append(n.Routes, Route(r))
		}
		if !m.Walkable(n.Pos) {
			return fmt.Errorf("mapdata %s: npc %q at (%.2f, %.2f) is not walkable", m.ID, n.ID, n.Pos.X, n.Pos.Y)
		}
		if !m.InSafeZone(n.Pos) {
			return fmt.Errorf("mapdata %s: npc %q is outside the safe zones", m.ID, n.ID)
		}
		if !reach(n.Pos) {
			return fmt.Errorf("mapdata %s: npc %q is unreachable from default", m.ID, n.ID)
		}
		for _, o := range m.NPCs {
			if math.Hypot(float64(o.Pos.X-n.Pos.X), float64(o.Pos.Y-n.Pos.Y)) < MinNPCDistance {
				return fmt.Errorf("mapdata %s: npcs %q and %q are closer than %.1f tiles", m.ID, o.ID, n.ID, MinNPCDistance)
			}
		}
		m.NPCs = append(m.NPCs, n)
	}
	return nil
}

// parseTriggers fills and validates safe zones, rest points and regions.
func (m *Map) parseTriggers(f *mapFile) error {
	seen := map[string]bool{}
	for _, z := range f.SafeZones {
		if seen[z.ID] {
			return fmt.Errorf("mapdata %s: duplicate safe zone %q", m.ID, z.ID)
		}
		seen[z.ID] = true
		if z.W <= 0 || z.H <= 0 {
			return fmt.Errorf("mapdata %s: safe zone %q: w and h must be positive", m.ID, z.ID)
		}
		if z.X < 0 || z.Y < 0 || z.X+z.W > m.Width || z.Y+z.H > m.Height {
			return fmt.Errorf("mapdata %s: safe zone %q is outside the map", m.ID, z.ID)
		}
		m.SafeZones = append(m.SafeZones, z)
	}
	if len(m.SafeZones) == 0 {
		return fmt.Errorf("mapdata %s: no safe zone", m.ID)
	}
	seen = map[string]bool{}
	for _, r := range f.RestPoints {
		if seen[r.ID] {
			return fmt.Errorf("mapdata %s: duplicate rest point %q", m.ID, r.ID)
		}
		seen[r.ID] = true
		if !(r.Radius > 0) {
			return fmt.Errorf("mapdata %s: rest point %q: radius must be positive", m.ID, r.ID)
		}
		if !m.restInSafeZones(r) {
			return fmt.Errorf("mapdata %s: rest point %q reaches outside the safe zones", m.ID, r.ID)
		}
		m.RestPoints = append(m.RestPoints, r)
	}
	if !m.InSafeZone(m.Spawns["default"]) {
		return fmt.Errorf("mapdata %s: default spawn point is outside the safe zones", m.ID)
	}
	if len(m.Respawns) == 0 {
		return fmt.Errorf("mapdata %s: no respawn point", m.ID)
	}
	for i, p := range m.Respawns {
		if !m.InSafeZone(p) {
			return fmt.Errorf("mapdata %s: respawn point %q is outside the safe zones", m.ID, m.RespawnIDs[i])
		}
	}
	if m.Type == "town" {
		// A town is safe everywhere (game-design 5.1): no monsters, no PK.
		if len(f.Regions) > 0 {
			return fmt.Errorf("mapdata %s: a town has monster regions", m.ID)
		}
		for ty := 0; ty < m.Height; ty++ {
			for tx := 0; tx < m.Width; tx++ {
				if !m.Blocked(tx, ty) && !m.safeTile(tx, ty) {
					return fmt.Errorf("mapdata %s: town tile (%d,%d) is walkable and outside the safe zones", m.ID, tx, ty)
				}
			}
		}
	}
	return m.parseRegions(f.Regions)
}

// parsePortals validates the portals on this map; Link checks their targets.
func (m *Map) parsePortals(portals []portalFile) error {
	owner := map[[2]int]string{}
	for _, pf := range portals {
		if pf.ID == "" {
			return fmt.Errorf("mapdata %s: portal without id", m.ID)
		}
		if _, dup := m.Portal(pf.ID); dup {
			return fmt.Errorf("mapdata %s: duplicate portal %q", m.ID, pf.ID)
		}
		if pf.W <= 0 || pf.H <= 0 {
			return fmt.Errorf("mapdata %s: portal %q: w and h must be positive", m.ID, pf.ID)
		}
		if pf.X < 0 || pf.Y < 0 || pf.X+pf.W > m.Width || pf.Y+pf.H > m.Height {
			return fmt.Errorf("mapdata %s: portal %q is outside the map", m.ID, pf.ID)
		}
		for ty := pf.Y; ty < pf.Y+pf.H; ty++ {
			for tx := pf.X; tx < pf.X+pf.W; tx++ {
				if m.Blocked(tx, ty) {
					return fmt.Errorf("mapdata %s: portal %q: tile (%d,%d) is not walkable", m.ID, pf.ID, tx, ty)
				}
				if other, taken := owner[[2]int{tx, ty}]; taken {
					return fmt.Errorf("mapdata %s: portals %q and %q share tile (%d,%d)", m.ID, other, pf.ID, tx, ty)
				}
				owner[[2]int{tx, ty}] = pf.ID
			}
		}
		if pf.ToMap == "" || pf.ToSpawn == "" {
			return fmt.Errorf("mapdata %s: portal %q: to_map and to_spawn must be set", m.ID, pf.ID)
		}
		if pf.ToMap == m.ID {
			return fmt.Errorf("mapdata %s: portal %q leads to its own map", m.ID, pf.ID)
		}
		m.Portals = append(m.Portals, Portal{
			ID: pf.ID, Rect: Rect{ID: pf.ID, X: pf.X, Y: pf.Y, W: pf.W, H: pf.H}, ToMap: pf.ToMap, ToSpawn: pf.ToSpawn,
		})
	}
	return nil
}

// reachSet returns a test for whether the tile containing a point is walkable
// and reachable, in four directions, from the tile of the default spawn point.
func (m *Map) reachSet() func(Point) bool {
	tileOf := func(p Point) [2]int {
		return [2]int{int(math.Floor(float64(p.X))), int(math.Floor(float64(p.Y)))}
	}
	start := tileOf(m.Spawns["default"])
	seen := make([]bool, m.Width*m.Height)
	seen[start[1]*m.Width+start[0]] = true
	queue := [][2]int{start}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := t[0]+d[0], t[1]+d[1]
			if !m.Blocked(nx, ny) && !seen[ny*m.Width+nx] {
				seen[ny*m.Width+nx] = true
				queue = append(queue, [2]int{nx, ny})
			}
		}
	}
	return func(p Point) bool {
		t := tileOf(p)
		return !m.Blocked(t[0], t[1]) && seen[t[1]*m.Width+t[0]]
	}
}

// Reachable checks that, walking in four directions over walkable tiles from
// the tile of the default spawn point, a player reaches every spawn point, at
// least one tile of every portal, every region's nest and every rest point's
// centre.
func (m *Map) Reachable() error {
	reached := m.reachSet()
	ids := make([]string, 0, len(m.Spawns))
	for id := range m.Spawns {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if !reached(m.Spawns[id]) {
			return fmt.Errorf("mapdata %s: spawn point %q is unreachable from default", m.ID, id)
		}
	}
	for _, p := range m.Portals {
		ok := false
		for ty := p.Rect.Y; ty < p.Rect.Y+p.Rect.H && !ok; ty++ {
			for tx := p.Rect.X; tx < p.Rect.X+p.Rect.W && !ok; tx++ {
				ok = reached(Point{float32(tx) + 0.5, float32(ty) + 0.5})
			}
		}
		if !ok {
			return fmt.Errorf("mapdata %s: portal %q is unreachable from default", m.ID, p.ID)
		}
	}
	for _, r := range m.Regions {
		if !reached(r.Nest) {
			return fmt.Errorf("mapdata %s: region %q: nest (%.2f, %.2f) is unreachable from default", m.ID, r.ID, r.Nest.X, r.Nest.Y)
		}
	}
	for _, r := range m.RestPoints {
		if !reached(Point{r.X, r.Y}) {
			return fmt.Errorf("mapdata %s: rest point %q is unreachable from default", m.ID, r.ID)
		}
	}
	return nil
}

// parseRegions validates monster regions (the safe zones are already known).
func (m *Map) parseRegions(regions []Region) error {
	seen := map[string]bool{}
	owner := map[[2]int]string{}
	for _, r := range regions {
		if r.ID == "" {
			return fmt.Errorf("mapdata %s: region without id: missing id", m.ID)
		}
		if seen[r.ID] {
			return fmt.Errorf("mapdata %s: duplicate region %q", m.ID, r.ID)
		}
		seen[r.ID] = true
		if len(r.Rects) == 0 {
			return fmt.Errorf("mapdata %s: region %q needs at least one rect", m.ID, r.ID)
		}
		for i, rc := range r.Rects {
			if rc.W <= 0 || rc.H <= 0 {
				return fmt.Errorf("mapdata %s: region %q: rect %d: w and h must be positive", m.ID, r.ID, i)
			}
			if rc.X < 0 || rc.Y < 0 || rc.X+rc.W > m.Width || rc.Y+rc.H > m.Height {
				return fmt.Errorf("mapdata %s: region %q: rect %d is outside the map", m.ID, r.ID, i)
			}
		}
		if r.MinLevel < 1 {
			return fmt.Errorf("mapdata %s: region %q: min_level must be at least 1", m.ID, r.ID)
		}
		if r.MinLevel > r.MaxLevel {
			return fmt.Errorf("mapdata %s: region %q: min_level %d > max_level %d", m.ID, r.ID, r.MinLevel, r.MaxLevel)
		}
		if r.Count < 1 {
			return fmt.Errorf("mapdata %s: region %q: count must be positive", m.ID, r.ID)
		}
		tiles := m.RegionTiles(r)
		if len(tiles) < r.Count {
			return fmt.Errorf("mapdata %s: region %q has %d region tiles, want at least %d", m.ID, r.ID, len(tiles), r.Count)
		}
		nestOK := false
		for _, p := range tiles {
			k := [2]int{int(p.X), int(p.Y)}
			if other, taken := owner[k]; taken {
				return fmt.Errorf("mapdata %s: regions %q and %q share tile (%d,%d)", m.ID, other, r.ID, k[0], k[1])
			}
			owner[k] = r.ID
			if k == [2]int{int(math.Floor(float64(r.Nest.X))), int(math.Floor(float64(r.Nest.Y)))} {
				nestOK = true
			}
		}
		if !nestOK {
			return fmt.Errorf("mapdata %s: region %q: nest (%.2f, %.2f) is not on a region tile", m.ID, r.ID, r.Nest.X, r.Nest.Y)
		}
		m.Regions = append(m.Regions, r)
	}
	return nil
}

// RegionTiles returns the centres of r's tiles, sorted by (y, x): walkable
// tiles inside at least one rect, outside the safe zones grown by one tile on
// every side. Overlapping rects count a tile once.
func (m *Map) RegionTiles(r Region) []Point {
	var out []Point
	for ty := 0; ty < m.Height; ty++ {
		for tx := 0; tx < m.Width; tx++ {
			if m.Blocked(tx, ty) || m.nearSafeZone(tx, ty) {
				continue
			}
			for _, rc := range r.Rects {
				if rc.ContainsTile(tx, ty) {
					out = append(out, Point{float32(tx) + 0.5, float32(ty) + 0.5})
					break
				}
			}
		}
	}
	return out
}

// nearSafeZone reports whether tile (tx, ty) is in a safe zone or beside one.
func (m *Map) nearSafeZone(tx, ty int) bool {
	for _, z := range m.SafeZones {
		if tx >= z.X-1 && tx < z.X+z.W+1 && ty >= z.Y-1 && ty < z.Y+z.H+1 {
			return true
		}
	}
	return false
}

// restInSafeZones reports whether every tile within r's radius is in a safe
// zone, so no one can rest where monsters reach. A tile that the circle only
// touches at its edge counts, which errs on the strict side.
func (m *Map) restInSafeZones(r RestPoint) bool {
	x, y, rad := float64(r.X), float64(r.Y), float64(r.Radius)
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsInf(rad, 0) {
		return false
	}
	for tx := int(math.Floor(x - rad)); tx <= int(math.Floor(x+rad)); tx++ {
		for ty := int(math.Floor(y - rad)); ty <= int(math.Floor(y+rad)); ty++ {
			// The point of the tile nearest the centre.
			nx := min(max(x, float64(tx)), float64(tx+1))
			ny := min(max(y, float64(ty)), float64(ty+1))
			if math.Hypot(nx-x, ny-y) <= rad && !m.safeTile(tx, ty) {
				return false
			}
		}
	}
	return true
}

func (m *Map) safeTile(tx, ty int) bool {
	for _, z := range m.SafeZones {
		if z.ContainsTile(tx, ty) {
			return true
		}
	}
	return false
}

// InSafeZone reports whether the tile containing p is in a safe zone.
func (m *Map) InSafeZone(p Point) bool {
	x, y := float64(p.X), float64(p.Y)
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return false
	}
	return m.safeTile(int(math.Floor(x)), int(math.Floor(y)))
}

// Blocked reports whether tile (tx, ty) blocks movement. Tiles outside the map are blocked.
func (m *Map) Blocked(tx, ty int) bool {
	if tx < 0 || ty < 0 || tx >= m.Width || ty >= m.Height {
		return true
	}
	return m.blocked[ty*m.Width+tx]
}

// Walkable reports whether the tile containing p is walkable.
func (m *Map) Walkable(p Point) bool {
	x, y := float64(p.X), float64(p.Y)
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return false
	}
	return !m.Blocked(int(math.Floor(x)), int(math.Floor(y)))
}

// LoadDir parses every *.json file in dir and indexes the maps by id.
func LoadDir(dir string) (map[string]*Map, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("mapdata: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("mapdata: no map files in %s", dir)
	}
	out := make(map[string]*Map, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("mapdata: %w", err)
		}
		m, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if _, dup := out[m.ID]; dup {
			return nil, fmt.Errorf("mapdata: duplicate map id %q in %s", m.ID, p)
		}
		out[m.ID] = m
	}
	if err := Link(out); err != nil {
		return nil, err
	}
	return out, nil
}

// linkTown checks the (mandatory) town and the carriage routes of m against the other maps.
func linkTown(maps map[string]*Map, m *Map) error {
	if m.Town == "" {
		return fmt.Errorf("mapdata %s: missing town", m.ID)
	}
	if m.Type == "town" && m.Town != m.ID {
		return fmt.Errorf("mapdata %s: a town must be its own town, not %q", m.ID, m.Town)
	}
	town, ok := maps[m.Town]
	if !ok {
		return fmt.Errorf("mapdata %s: town %q is not a map", m.ID, m.Town)
	}
	if town.Type != "town" {
		return fmt.Errorf("mapdata %s: town %q is not a town", m.ID, m.Town)
	}
	for _, n := range m.NPCs {
		for _, r := range n.Routes {
			if _, ok := maps[r.Map]; !ok {
				return fmt.Errorf("mapdata %s: npc %q: route to unknown map %q", m.ID, n.ID, r.Map)
			}
		}
	}
	return nil
}

// Link checks the town and routes (phase 4b spec 2.1) and the portals across
// maps (phase 4a spec 2.1): each portal leads
// to a loaded map and one of its spawn points, at least MinArrivalDistance
// tiles from every portal of that map. Whether every map can be reached and
// every portal has a reverse is checked by tests on the real maps, so tests
// can use small maps that stand alone.
func Link(maps map[string]*Map) error {
	ids := make([]string, 0, len(maps))
	for id := range maps {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		m := maps[id]
		if err := linkTown(maps, m); err != nil {
			return err
		}
		for _, p := range m.Portals {
			target, ok := maps[p.ToMap]
			if !ok {
				return fmt.Errorf("mapdata %s: portal %q: unknown map %q", m.ID, p.ID, p.ToMap)
			}
			arrival, ok := target.Spawns[p.ToSpawn]
			if !ok {
				return fmt.Errorf("mapdata %s: portal %q: map %q has no spawn point %q", m.ID, p.ID, p.ToMap, p.ToSpawn)
			}
			for _, q := range target.Portals {
				if d := RectDistance(arrival, q.Rect); d < MinArrivalDistance {
					return fmt.Errorf("mapdata %s: portal %q: arrival %q is %.2f tiles from portal %q of map %q, want at least %d",
						m.ID, p.ID, p.ToSpawn, d, q.ID, p.ToMap, MinArrivalDistance)
				}
			}
		}
	}
	return nil
}
