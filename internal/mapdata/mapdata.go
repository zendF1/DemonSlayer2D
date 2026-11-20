// Package mapdata loads map files: collision grid, spawn points, safe zones, rest points and monster camps and regions.
package mapdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

	SafeZones  []Rect
	RestPoints []RestPoint
	Camps      []Camp
	Regions    []Region

	blocked []bool
}

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

// Camp is a monster spawn area: Count monsters of kind Monster, with levels
// MinLevel..MaxLevel, on walkable tiles within Radius tiles of (X, Y).
type Camp struct {
	ID       string  `json:"id"`
	Monster  string  `json:"monster"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Radius   float32 `json:"radius"`
	Count    int     `json:"count"`
	MinLevel int     `json:"min_level"`
	MaxLevel int     `json:"max_level"`
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
	ID string  `json:"id"`
	X  float32 `json:"x"`
	Y  float32 `json:"y"`
}

type mapFile struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Collision   []string    `json:"collision"`
	SpawnPoints []spawnFile `json:"spawn_points"`
	SafeZones   []Rect      `json:"safe_zones"`
	RestPoints  []RestPoint `json:"rest_points"`
	Camps       []Camp      `json:"camps"`
	Regions     []Region    `json:"regions"`
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
	}
	if _, ok := m.Spawns["default"]; !ok {
		return nil, fmt.Errorf("mapdata %s: missing default spawn point", f.ID)
	}
	if err := m.parseTriggers(&f); err != nil {
		return nil, err
	}
	return m, nil
}

// parseTriggers fills and validates safe zones, rest points and camps.
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
	if len(m.SafeZones) > 0 && !m.InSafeZone(m.Spawns["default"]) {
		return fmt.Errorf("mapdata %s: default spawn point is outside the safe zones", m.ID)
	}
	seen = map[string]bool{}
	for _, c := range f.Camps {
		if seen[c.ID] {
			return fmt.Errorf("mapdata %s: duplicate camp %q", m.ID, c.ID)
		}
		seen[c.ID] = true
		camp := c
		if c.MinLevel > c.MaxLevel {
			return fmt.Errorf("mapdata %s: camp %q: min_level %d > max_level %d", m.ID, c.ID, c.MinLevel, c.MaxLevel)
		}
		if c.Count <= 0 {
			return fmt.Errorf("mapdata %s: camp %q: count must be positive", m.ID, c.ID)
		}
		if !(c.Radius > 0) {
			return fmt.Errorf("mapdata %s: camp %q: radius must be positive", m.ID, c.ID)
		}
		tiles := m.CampTiles(camp)
		if len(tiles) < c.Count {
			return fmt.Errorf("mapdata %s: camp %q has %d camp tiles, want at least %d", m.ID, c.ID, len(tiles), c.Count)
		}
		for _, p := range tiles {
			if m.InSafeZone(p) {
				return fmt.Errorf("mapdata %s: camp %q: tile (%d,%d) is in a safe zone", m.ID, c.ID, int(p.X), int(p.Y))
			}
		}
		m.Camps = append(m.Camps, camp)
	}
	return m.parseRegions(f.Regions)
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

// CampTiles returns the centres of the walkable tiles whose centre is within
// c.Radius of the camp centre, sorted by (y, x).
func (m *Map) CampTiles(c Camp) []Point {
	r := float64(c.Radius)
	x0, x1 := int(math.Floor(float64(c.X)-r)), int(math.Ceil(float64(c.X)+r))
	y0, y1 := int(math.Floor(float64(c.Y)-r)), int(math.Ceil(float64(c.Y)+r))
	var out []Point
	for ty := y0; ty <= y1; ty++ {
		for tx := x0; tx <= x1; tx++ {
			if m.Blocked(tx, ty) {
				continue
			}
			dx, dy := float64(tx)+0.5-float64(c.X), float64(ty)+0.5-float64(c.Y)
			if dx*dx+dy*dy <= r*r {
				out = append(out, Point{float32(tx) + 0.5, float32(ty) + 0.5})
			}
		}
	}
	return out
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
	return out, nil
}
