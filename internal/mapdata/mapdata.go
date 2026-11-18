// Package mapdata loads map files: size, tile collision grid and spawn points.
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
type Point struct{ X, Y float32 }

type Map struct {
	ID     string
	Name   string
	Type   string
	Width  int
	Height int
	Spawns map[string]Point

	blocked []bool
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
	return m, nil
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
