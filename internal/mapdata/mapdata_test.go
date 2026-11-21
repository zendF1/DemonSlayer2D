package mapdata

import (
	"encoding/json"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func mapJSON(t *testing.T, mutate func(m map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"id": "t1", "name": "Test", "type": "training", "width": 4, "height": 3, "min_zones": 1,
		"collision":    []string{"####", "#..#", "####"},
		"safe_zones":   []map[string]any{{"id": "camp", "x": 1, "y": 1, "w": 2, "h": 1}},
		"spawn_points": []map[string]any{{"id": "default", "x": 1.5, "y": 1.5, "respawn": true}},
	}
	if mutate != nil {
		mutate(m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseValid(t *testing.T) {
	m, err := Parse(mapJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "t1" || m.Width != 4 || m.Height != 3 {
		t.Fatalf("bad header: %+v", m)
	}
	if !m.Blocked(0, 0) || m.Blocked(1, 1) || m.Blocked(2, 1) || !m.Blocked(3, 1) {
		t.Fatal("collision grid mismatch")
	}
	if got := m.Spawns["default"]; got != (Point{1.5, 1.5}) {
		t.Fatalf("default spawn = %+v", got)
	}
}

func TestBlockedOutOfBounds(t *testing.T) {
	m, err := Parse(mapJSON(t, func(m map[string]any) { m["collision"] = []string{"....", "....", "...."} }))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]int{{-1, 0}, {0, -1}, {4, 0}, {0, 3}} {
		if !m.Blocked(c[0], c[1]) {
			t.Errorf("Blocked(%d,%d) = false, want true outside the map", c[0], c[1])
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"missing id":       func(m map[string]any) { m["id"] = "" },
		"invalid type":     func(m map[string]any) { m["type"] = "castle" },
		"rows":             func(m map[string]any) { m["collision"] = []string{"####", "#..#"} },
		"row 1 has length": func(m map[string]any) { m["collision"] = []string{"####", "#...#", "####"} },
		"invalid char":     func(m map[string]any) { m["collision"] = []string{"####", "#.x#", "####"} },
		"not walkable": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{{"id": "default", "x": 0.5, "y": 0.5, "respawn": true}}
		},
		"default spawn": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{{"id": "gate", "x": 1.5, "y": 1.5}}
		},
		"duplicate spawn": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{
				{"id": "default", "x": 1.5, "y": 1.5}, {"id": "default", "x": 2.5, "y": 1.5},
			}
		},
		"unknown field": func(m map[string]any) { m["teleports"] = []string{} },
	}
	for want, mutate := range cases {
		_, err := Parse(mapJSON(t, mutate))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got err %v", want, err)
		}
	}
}

func TestLoadDirRepoMaps(t *testing.T) {
	maps, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := maps["kumotori_1"]
	if !ok {
		t.Fatal("kumotori_1 not loaded")
	}
	if m.Width != 64 || m.Height != 48 || !m.Walkable(m.Spawns["default"]) {
		t.Fatalf("unexpected kumotori_1: %dx%d", m.Width, m.Height)
	}
}

// ______________________TEST TRIGGERS_____________________

// triggerJSON is a 10x8 map with one blocked tile at (6,4), a safe zone on
// tiles 1..3 x 1..3 and a rest point in it.
func triggerJSON(t *testing.T, mutate func(m map[string]any)) []byte {
	t.Helper()
	return mapJSON(t, func(m map[string]any) {
		m["width"], m["height"] = 10, 8
		m["collision"] = []string{
			"##########",
			"#........#",
			"#........#",
			"#........#",
			"#.....#..#",
			"#........#",
			"#........#",
			"##########",
		}
		m["safe_zones"] = []map[string]any{{"id": "camp", "x": 1, "y": 1, "w": 3, "h": 3}}
		m["rest_points"] = []map[string]any{{"id": "fire", "x": 2.5, "y": 2.5, "radius": 1.4}}
		if mutate != nil {
			mutate(m)
		}
	})
}

func TestParseTriggers(t *testing.T) {
	m, err := Parse(triggerJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.SafeZones) != 1 || m.SafeZones[0] != (Rect{ID: "camp", X: 1, Y: 1, W: 3, H: 3}) {
		t.Fatalf("safe zones = %+v", m.SafeZones)
	}
	if len(m.RestPoints) != 1 || m.RestPoints[0] != (RestPoint{ID: "fire", X: 2.5, Y: 2.5, Radius: 1.4}) {
		t.Fatalf("rest points = %+v", m.RestPoints)
	}

	// Safe zone: tiles 1..3 inside, tile 4 (the edge tile just outside) and tile 0 outside.
	for p, want := range map[Point]bool{
		{1.0, 1.0}: true, {3.99, 3.99}: true, {2.5, 2.5}: true,
		{4.0, 1.5}: false, {1.5, 4.0}: false, {0.99, 1.5}: false, {7.5, 5.5}: false,
	} {
		if got := m.InSafeZone(p); got != want {
			t.Errorf("InSafeZone(%v) = %v, want %v", p, got, want)
		}
	}
}

func TestParseTriggersOptional(t *testing.T) {
	m, err := Parse(mapJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.RestPoints)+len(m.Regions)+len(m.Portals) != 0 {
		t.Fatal("a map without rest points, regions and portals must have none")
	}
}

func TestParseTriggerErrors(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"safe zone \"camp\" is outside the map": func(m map[string]any) {
			m["safe_zones"] = []map[string]any{{"id": "camp", "x": 8, "y": 1, "w": 3, "h": 3}}
		},
		"safe zone \"camp\": w and h must be positive": func(m map[string]any) {
			m["safe_zones"] = []map[string]any{{"id": "camp", "x": 1, "y": 1, "w": 0, "h": 3}}
		},
		"duplicate safe zone \"camp\"": func(m map[string]any) {
			z := map[string]any{"id": "camp", "x": 1, "y": 1, "w": 3, "h": 3}
			m["safe_zones"] = []map[string]any{z, z}
		},
		"rest point \"fire\": radius must be positive": func(m map[string]any) {
			m["rest_points"] = []map[string]any{{"id": "fire", "x": 2.5, "y": 2.5, "radius": 0}}
		},
		"duplicate rest point \"fire\"": func(m map[string]any) {
			r := map[string]any{"id": "fire", "x": 2.5, "y": 2.5, "radius": 1.4}
			m["rest_points"] = []map[string]any{r, r}
		},
		"default spawn point is outside the safe zones": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{{"id": "default", "x": 6.5, "y": 1.5, "respawn": true}}
		},
	}
	for want, mutate := range cases {
		_, err := Parse(triggerJSON(t, mutate))
		want = strings.TrimSuffix(want, " (nil)")
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "mapdata t1") {
			t.Errorf("%s: got err %v", want, err)
		}
	}
}

// Every position within a rest point's radius must be in a safe zone, or a
// player could rest where monsters can reach it.
func TestRestPointMustFitInSafeZones(t *testing.T) {
	porch := map[string]any{"id": "porch", "x": 4, "y": 1, "w": 1, "h": 3} // tiles 4 x 1..3
	for _, c := range []struct {
		name          string
		x, y, radius  float64
		porch, wantOK bool
	}{
		{"inside", 2.5, 2.5, 1.4, false, true},
		{"centre outside", 6.5, 2.5, 1.4, false, false},
		{"edge reaches tile 4", 2.5, 2.5, 1.5, false, false},
		{"edge reaches row 4", 2.5, 2.7, 1.4, false, false},
		{"reaches tile 4 without the porch", 3.0, 2.5, 1.4, false, false},
		{"covered by two safe zones", 3.0, 2.5, 1.4, true, true},
	} {
		_, err := Parse(triggerJSON(t, func(m map[string]any) {
			m["rest_points"] = []map[string]any{{"id": "fire", "x": c.x, "y": c.y, "radius": c.radius}}
			if c.porch {
				m["safe_zones"] = append(m["safe_zones"].([]map[string]any), porch)
			}
		}))
		if c.wantOK && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.wantOK && (err == nil || !strings.Contains(err.Error(), `rest point "fire" reaches outside the safe zones`)) {
			t.Errorf("%s: got err %v, want the rest point rejected", c.name, err)
		}
	}
}

func TestKumotoriLoads(t *testing.T) {
	maps, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	m := maps["kumotori_1"]
	if m == nil {
		t.Fatal("kumotori_1 not loaded")
	}
	// The camp and, from phase 4a, the safe zone at the east portal.
	if len(m.SafeZones) != 2 || m.SafeZones[0].ID != "camp" || m.SafeZones[1].ID != "gate_east" || len(m.RestPoints) != 1 {
		t.Fatalf("kumotori_1 has safe zones %+v, %d rest points", m.SafeZones, len(m.RestPoints))
	}
	if !m.InSafeZone(m.Spawns["default"]) {
		t.Error("default spawn is not in the safe zone")
	}
}

// kumotori_1 has the four regions of spec 2.2, each nest on one of its tiles.
func TestKumotoriRegions(t *testing.T) {
	maps, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	m := maps["kumotori_1"]
	type want struct {
		id, monster        string
		minLevel, maxLevel int
		count              int
		nest               Point
	}
	wants := []want{
		{"r1", "wanderer", 1, 2, 8, Point{22.5, 10.5}},
		{"r2", "wanderer", 2, 3, 7, Point{47.5, 9.5}},
		{"r3", "snow_ghoul", 3, 4, 7, Point{15.5, 34.5}},
		{"r4", "snow_ghoul", 4, 5, 8, Point{52.5, 40.5}},
	}
	if len(m.Regions) != len(wants) {
		t.Fatalf("kumotori_1 has %d regions, want %d", len(m.Regions), len(wants))
	}
	total := 0
	for i, w := range wants {
		r := m.Regions[i]
		got := want{r.ID, r.Monster, r.MinLevel, r.MaxLevel, r.Count, r.Nest}
		if got != w {
			t.Errorf("region %d = %+v, want %+v", i, got, w)
		}
		if !slices.Contains(m.RegionTiles(r), r.Nest) {
			t.Errorf("region %s: nest %v is not on a region tile", r.ID, r.Nest)
		}
		total += r.Count
	}
	if total != 30 {
		t.Errorf("kumotori_1 has %d monsters, want 30", total)
	}
}

// ______________________TEST REGIONS_____________________

// regionJSON adds one region "r" to triggerJSON (10x8, safe zone on tiles
// 1..3 x 1..3, one blocked tile at (6,4)). Its rects overlap and also touch
// the safe zone, its 1-tile margin and the blocked tile.
func regionJSON(t *testing.T, mutate func(r map[string]any, m map[string]any)) []byte {
	t.Helper()
	return triggerJSON(t, func(m map[string]any) {
		r := map[string]any{
			"id": "r", "monster": "wanderer", "min_level": 1, "max_level": 2, "count": 3,
			"rects": []map[string]any{{"x": 5, "y": 1, "w": 3, "h": 3}, {"x": 7, "y": 2, "w": 2, "h": 4}, {"x": 3, "y": 3, "w": 4, "h": 3}},
			"nest":  map[string]any{"x": 5.5, "y": 1.5},
		}
		m["regions"] = []map[string]any{r}
		if mutate != nil {
			mutate(r, m)
		}
	})
}

func TestRegionTiles(t *testing.T) {
	m, err := Parse(regionJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Regions) != 1 {
		t.Fatalf("regions: %+v", m.Regions)
	}
	r := m.Regions[0]
	if r.ID != "r" || r.Monster != "wanderer" || r.MinLevel != 1 || r.MaxLevel != 2 || r.Count != 3 ||
		len(r.Rects) != 3 || r.Nest != (Point{5.5, 1.5}) {
		t.Fatalf("region: %+v", r)
	}
	// Dropped: the blocked tile (6,4), which rect 2 covers, the safe zone and
	// its 1-tile margin (x or y in 0..4 beside it). Tiles where rects overlap
	// are listed once.
	cols := map[int][]int{1: {5, 6, 7}, 2: {5, 6, 7, 8}, 3: {5, 6, 7, 8}, 4: {5, 7, 8}, 5: {3, 4, 5, 6, 7, 8}}
	var want []Point
	for y := 1; y <= 5; y++ {
		for _, x := range cols[y] {
			want = append(want, Point{float32(x) + 0.5, float32(y) + 0.5})
		}
	}
	got := m.RegionTiles(r)
	if len(got) != len(want) {
		t.Fatalf("tiles %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tiles %v, want %v", got, want)
		}
	}
}

func TestParseRegionErrors(t *testing.T) {
	rect := func(r map[string]any, i int) map[string]any { return r["rects"].([]map[string]any)[i] }
	cases := map[string]func(r map[string]any, m map[string]any){
		`duplicate region "r"`: func(r, m map[string]any) {
			m["regions"] = []map[string]any{r, r}
		},
		`region "r": rect 1: w and h must be positive`: func(r, m map[string]any) { rect(r, 1)["w"] = 0 },
		`region "r": rect 0: w and h must be positive`: func(r, m map[string]any) { rect(r, 0)["h"] = -1 },
		`region "r": rect 2 is outside the map`:        func(r, m map[string]any) { rect(r, 2)["x"] = 9 },
		`region "r": rect 0 is outside the map`:        func(r, m map[string]any) { rect(r, 0)["y"] = -1 },
		`missing id`:                                   func(r, m map[string]any) { r["id"] = "" },
		`region "r" needs at least one rect`:           func(r, m map[string]any) { r["rects"] = []map[string]any{} },
		`region "r" needs at least one rect (nil)`:     func(r, m map[string]any) { delete(r, "rects") },
		`region "r": min_level must be at least 1`:     func(r, m map[string]any) { r["min_level"] = 0 },
		`region "r": min_level 3 > max_level 2`:        func(r, m map[string]any) { r["min_level"] = 3 },
		`region "r": count must be positive`:           func(r, m map[string]any) { r["count"] = 0 },
		`region "r" has 20 region tiles, want at least 21`: func(r, m map[string]any) {
			r["count"] = 21
		},
		`region "r": nest (2.50, 2.50) is not on a region tile`: func(r, m map[string]any) {
			r["nest"] = map[string]any{"x": 2.5, "y": 2.5} // safe zone
		},
		`region "r": nest (4.50, 4.50) is not on a region tile`: func(r, m map[string]any) {
			r["nest"] = map[string]any{"x": 4.5, "y": 4.5} // margin
		},
		`region "r": nest (6.50, 4.50) is not on a region tile`: func(r, m map[string]any) {
			r["nest"] = map[string]any{"x": 6.5, "y": 4.5} // blocked, inside rect 2
		},
		`region "r": nest (9.50, 1.50) is not on a region tile`: func(r, m map[string]any) {
			r["nest"] = map[string]any{"x": 9.5, "y": 1.5} // outside the rects
		},
		`regions "r" and "s" share tile (7,2)`: func(r, m map[string]any) {
			m["regions"] = []map[string]any{r, {
				"id": "s", "monster": "wanderer", "min_level": 1, "max_level": 1, "count": 1,
				"rects": []map[string]any{{"x": 7, "y": 2, "w": 1, "h": 1}},
				"nest":  map[string]any{"x": 7.5, "y": 2.5},
			}}
		},
	}
	for want, mutate := range cases {
		_, err := Parse(regionJSON(t, mutate))
		want = strings.TrimSuffix(want, " (nil)")
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "mapdata t1") {
			t.Errorf("want error containing %q (naming the map), got %v", want, err)
		}
	}
}

func TestParseRegionCountAtLimit(t *testing.T) {
	if _, err := Parse(regionJSON(t, func(r, m map[string]any) { r["count"] = 20 })); err != nil {
		t.Fatalf("count equal to the tile count must be accepted: %v", err)
	}
}

// ______________________TEST PORTALS AND RESPAWNS_____________________

// portalJSON is a valid 16x8 open map "t1" with min_zones 2: safe zones camp
// (tiles 1..4) and gate (12..14), a rest point and three respawn points
// (zeta and default in the camp, alpha in the gate), a region r between the
// safe zones and the portals to_west (tiles 1 x 3..4) and to_east (14 x 3..4).
func portalJSON(t *testing.T, mutate func(m map[string]any)) []byte {
	t.Helper()
	return mapJSON(t, func(m map[string]any) {
		m["width"], m["height"], m["min_zones"] = 16, 8, 2
		rows := []string{strings.Repeat("#", 16)}
		for range 6 {
			rows = append(rows, "#"+strings.Repeat(".", 14)+"#")
		}
		m["collision"] = append(rows, strings.Repeat("#", 16))
		m["safe_zones"] = []map[string]any{
			{"id": "camp", "x": 1, "y": 1, "w": 4, "h": 6}, {"id": "gate", "x": 12, "y": 1, "w": 3, "h": 6},
		}
		m["rest_points"] = []map[string]any{{"id": "fire", "x": 2.5, "y": 2.5, "radius": 1.0}}
		m["spawn_points"] = []map[string]any{
			{"id": "default", "x": 2.5, "y": 4.5, "respawn": true},
			{"id": "zeta", "x": 3.5, "y": 5.5, "respawn": true},
			{"id": "alpha", "x": 13.5, "y": 5.5, "respawn": true},
			{"id": "plain", "x": 8.5, "y": 6.5},
		}
		m["regions"] = []map[string]any{{"id": "r", "monster": "wanderer", "min_level": 1, "max_level": 1, "count": 2,
			"rects": []map[string]any{{"x": 6, "y": 1, "w": 5, "h": 6}}, "nest": map[string]any{"x": 8.5, "y": 3.5}}}
		m["portals"] = []map[string]any{
			{"id": "to_west", "x": 1, "y": 3, "w": 1, "h": 2, "to_map": "a", "to_spawn": "from_t1"},
			{"id": "to_east", "x": 14, "y": 3, "w": 1, "h": 2, "to_map": "b", "to_spawn": "from_t1"},
		}
		if mutate != nil {
			mutate(m)
		}
	})
}

// setTiles returns m's collision with the listed tiles set to c.
func setTiles(m map[string]any, c byte, tiles ...[2]int) {
	rows := slices.Clone(m["collision"].([]string))
	for _, tl := range tiles {
		b := []byte(rows[tl[1]])
		b[tl[0]] = c
		rows[tl[1]] = string(b)
	}
	m["collision"] = rows
}

func portalAt(m map[string]any, i int) map[string]any { return m["portals"].([]map[string]any)[i] }

func TestParsePortalsAndRespawns(t *testing.T) {
	m, err := Parse(portalJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if m.MinZones != 2 {
		t.Errorf("MinZones = %d", m.MinZones)
	}
	if !slices.Equal(m.RespawnIDs, []string{"alpha", "default", "zeta"}) ||
		!slices.Equal(m.Respawns, []Point{{13.5, 5.5}, {2.5, 4.5}, {3.5, 5.5}}) {
		t.Errorf("respawns %v at %v, want alpha, default, zeta sorted by id", m.RespawnIDs, m.Respawns)
	}
	want := []Portal{
		{ID: "to_west", Rect: Rect{ID: "to_west", X: 1, Y: 3, W: 1, H: 2}, ToMap: "a", ToSpawn: "from_t1"},
		{ID: "to_east", Rect: Rect{ID: "to_east", X: 14, Y: 3, W: 1, H: 2}, ToMap: "b", ToSpawn: "from_t1"},
	}
	if !slices.Equal(m.Portals, want) {
		t.Errorf("portals %+v, want %+v (file order)", m.Portals, want)
	}
	if p, ok := m.Portal("to_east"); !ok || p != want[1] {
		t.Errorf("Portal(to_east) = %+v, %v", p, ok)
	}
	if _, ok := m.Portal("to_nowhere"); ok {
		t.Error("Portal(to_nowhere) found")
	}
	if _, ok := m.Spawns["plain"]; !ok {
		t.Error("a spawn point without respawn is still a spawn point")
	}
}

func TestParsePortalErrors(t *testing.T) {
	town := func(m map[string]any) {
		m["type"] = "town"
		m["safe_zones"] = []map[string]any{{"id": "town", "x": 1, "y": 1, "w": 14, "h": 6}}
		m["regions"] = []map[string]any{}
	}
	cases := map[string]func(m map[string]any){
		"min_zones 0 is not in 1-10":           func(m map[string]any) { m["min_zones"] = 0 },
		"min_zones 11 is not in 1-10":          func(m map[string]any) { m["min_zones"] = 11 },
		"min_zones 0 is not in 1-10 (missing)": func(m map[string]any) { delete(m, "min_zones") },
		"no respawn point": func(m map[string]any) {
			for _, sp := range m["spawn_points"].([]map[string]any) {
				delete(sp, "respawn")
			}
		},
		`respawn point "mid" is outside the safe zones`: func(m map[string]any) {
			m["spawn_points"] = append(m["spawn_points"].([]map[string]any), map[string]any{"id": "mid", "x": 6.5, "y": 6.5, "respawn": true})
		},
		"no safe zone": func(m map[string]any) {
			m["safe_zones"], m["rest_points"] = []map[string]any{}, []map[string]any{}
		},
		"a town has monster regions": func(m map[string]any) {
			town(m)
			m["regions"] = []map[string]any{{"id": "r", "monster": "wanderer", "min_level": 1, "max_level": 1, "count": 1,
				"rects": []map[string]any{{"x": 6, "y": 1, "w": 5, "h": 6}}, "nest": map[string]any{"x": 8.5, "y": 3.5}}}
		},
		"town tile (5,1) is walkable and outside the safe zones": func(m map[string]any) {
			town(m)
			m["safe_zones"] = []map[string]any{
				{"id": "camp", "x": 1, "y": 1, "w": 4, "h": 6}, {"id": "gate", "x": 6, "y": 1, "w": 9, "h": 6},
			}
		},
		`portal without id`: func(m map[string]any) { portalAt(m, 1)["id"] = "" },
		`duplicate portal "to_west"`: func(m map[string]any) {
			portalAt(m, 1)["id"] = "to_west"
		},
		`portal "to_east": w and h must be positive`: func(m map[string]any) { portalAt(m, 1)["w"] = 0 },
		`portal "to_east" is outside the map`:        func(m map[string]any) { portalAt(m, 1)["w"] = 3 },
		`portal "to_east" is outside the map (negative)`: func(m map[string]any) {
			portalAt(m, 1)["y"] = -1
		},
		`portal "to_east": tile (15,3) is not walkable`: func(m map[string]any) { portalAt(m, 1)["x"] = 15 },
		`portals "to_west" and "to_east" share tile (2,4)`: func(m map[string]any) {
			portalAt(m, 0)["w"] = 2
			p := portalAt(m, 1)
			p["x"], p["y"] = 2, 4
		},
		`portal "to_east" leads to its own map`:             func(m map[string]any) { portalAt(m, 1)["to_map"] = "t1" },
		`portal "to_east": to_map and to_spawn must be set`: func(m map[string]any) { portalAt(m, 1)["to_spawn"] = "" },
		`portal "to_east": to_map and to_spawn must be set (to_map)`: func(m map[string]any) {
			portalAt(m, 1)["to_map"] = ""
		},
		`spawn point "alpha" is unreachable from default`: func(m map[string]any) {
			setTiles(m, '#', [2]int{13, 4}, [2]int{12, 5}, [2]int{14, 5}, [2]int{13, 6})
		},
		`region "r": nest (8.50, 3.50) is unreachable from default`: func(m map[string]any) {
			setTiles(m, '#', [2]int{8, 2}, [2]int{7, 3}, [2]int{9, 3}, [2]int{8, 4})
		},
		`portal "to_east" is unreachable from default`: func(m map[string]any) {
			setTiles(m, '#', [2]int{13, 1}, [2]int{13, 2}, [2]int{13, 3}, [2]int{13, 4}, [2]int{13, 5}, [2]int{13, 6})
			m["spawn_points"].([]map[string]any)[2]["x"] = 12.5
		},
		`rest point "fire" is unreachable from default`: func(m map[string]any) {
			setTiles(m, '#', [2]int{2, 1}, [2]int{1, 2}, [2]int{3, 2}, [2]int{2, 3})
		},
	}
	for want, mutate := range cases {
		_, err := Parse(portalJSON(t, mutate))
		if i := strings.Index(want, " ("); i > 0 && strings.HasSuffix(want, ")") {
			want = want[:i]
		}
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "mapdata t1") {
			t.Errorf("want error containing %q (naming the map), got %v", want, err)
		}
	}
}

// A town whose walkable tiles are all in safe zones and that has no regions parses.
func TestParseTown(t *testing.T) {
	_, err := Parse(portalJSON(t, func(m map[string]any) {
		m["type"] = "town"
		m["safe_zones"] = []map[string]any{{"id": "town", "x": 1, "y": 1, "w": 14, "h": 6}}
		m["regions"] = []map[string]any{}
	}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestRectDistance(t *testing.T) {
	r := Rect{X: 2, Y: 3, W: 4, H: 2} // [2, 6] x [3, 5]
	for _, c := range []struct {
		p    Point
		want float32
	}{
		{Point{3, 4}, 0}, {Point{6, 5}, 0}, {Point{2, 3.5}, 0}, // inside and on the edge
		{Point{7.5, 4}, 1.5}, {Point{4, 1}, 2}, // beside an edge
		{Point{9, 9}, 5}, {Point{0.5, 1}, 2.5}, // beyond a corner
	} {
		if got := RectDistance(c.p, r); math.Abs(float64(got-c.want)) > 1e-6 {
			t.Errorf("RectDistance(%v) = %v, want %v", c.p, got, c.want)
		}
	}
}

// asTown makes m a town of its own: every walkable tile safe, no regions.
func asTown(m map[string]any) {
	m["type"], m["town"] = "town", m["id"]
	m["safe_zones"] = []map[string]any{{"id": "town", "x": 1, "y": 1, "w": 14, "h": 6}}
	m["regions"] = []map[string]any{}
}

// linkPair is two maps joined both ways: a's to_b leads to b's from_a (2.5
// tiles from b's to_a) and b's to_a to a's from_b. a is the town of both.
func linkPair(t *testing.T, mutateA, mutateB func(m map[string]any)) map[string]*Map {
	t.Helper()
	parse := func(id string, portal map[string]any, arrival map[string]any, mutate func(m map[string]any)) *Map {
		m, err := Parse(portalJSON(t, func(m map[string]any) {
			m["id"] = id
			m["town"] = "a"
			if id == "a" {
				asTown(m)
			}
			m["portals"] = []map[string]any{portal}
			m["spawn_points"] = append(m["spawn_points"].([]map[string]any), arrival)
			if mutate != nil {
				mutate(m)
			}
		}))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := parse("a", map[string]any{"id": "to_b", "x": 14, "y": 3, "w": 1, "h": 2, "to_map": "b", "to_spawn": "from_a"},
		map[string]any{"id": "from_b", "x": 11.5, "y": 4.0}, mutateA)
	b := parse("b", map[string]any{"id": "to_a", "x": 1, "y": 3, "w": 1, "h": 2, "to_map": "a", "to_spawn": "from_b"},
		map[string]any{"id": "from_a", "x": 4.5, "y": 4.0}, mutateB)
	return map[string]*Map{"a": a, "b": b}
}

func TestLink(t *testing.T) {
	if err := Link(linkPair(t, nil, nil)); err != nil {
		t.Fatalf("a valid pair: %v", err)
	}
	cases := map[string][2]func(m map[string]any){
		`mapdata a: portal "to_b": unknown map "c"`: {func(m map[string]any) { portalAt(m, 0)["to_map"] = "c" }, nil},
		`mapdata a: portal "to_b": map "b" has no spawn point "from_x"`: {
			func(m map[string]any) { portalAt(m, 0)["to_spawn"] = "from_x" }, nil,
		},
		`mapdata a: portal "to_b": arrival "from_a" is 1.50 tiles from portal "to_a" of map "b", want at least 2`: {
			nil, func(m map[string]any) {
				sps := m["spawn_points"].([]map[string]any)
				sps[len(sps)-1]["x"] = 3.5
			},
		},
	}
	for want, mut := range cases {
		err := Link(linkPair(t, mut[0], mut[1]))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error containing %q, got %v", want, err)
		}
	}
}

func TestLoadDirLinks(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("t1.json", portalJSON(t, func(m map[string]any) {
		asTown(m)
		m["portals"] = []map[string]any{portalAt(m, 0)} // to_west leads to a map that is not there
	}))
	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), `portal "to_west": unknown map "a"`) {
		t.Fatalf("LoadDir must check portals across maps, got %v", err)
	}
	write("t1.json", portalJSON(t, func(m map[string]any) { asTown(m); m["portals"] = []map[string]any{} }))
	if _, err := LoadDir(dir); err != nil {
		t.Fatal(err)
	}
}

// ______________________TEST REAL MAPS_____________________

// The committed maps are what tools/mapgen.py writes (phase 4a spec 2.2).
func TestRepoMapsMatchGenerator(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	out, err := exec.Command(py, "../../tools/mapgen.py", "--check").CombinedOutput()
	if err != nil {
		t.Fatalf("mapgen.py --check: %v\n%s", err, out)
	}
}

func TestRepoMapsLoad(t *testing.T) {
	loaded, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fujikasane_1", "kumotori_1", "sagiri_1", "sagiri_2", "ubuyashiki"}
	if got := slices.Sorted(maps.Keys(loaded)); !slices.Equal(got, want) {
		t.Fatalf("maps %v, want %v", got, want)
	}
}

// Phase 4a spec 2.1: every map is reachable from kumotori_1 through portals
// and every portal A -> B has a portal B -> A.
func TestRepoMapsPortalGraph(t *testing.T) {
	loaded, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{"kumotori_1": true}
	queue := []string{"kumotori_1"}
	for len(queue) > 0 {
		m := loaded[queue[0]]
		queue = queue[1:]
		for _, p := range m.Portals {
			back := slices.ContainsFunc(loaded[p.ToMap].Portals, func(q Portal) bool { return q.ToMap == m.ID })
			if !back {
				t.Errorf("%s: portal %s has no reverse portal in %s", m.ID, p.ID, p.ToMap)
			}
			if !seen[p.ToMap] {
				seen[p.ToMap] = true
				queue = append(queue, p.ToMap)
			}
		}
	}
	if len(seen) != len(loaded) {
		t.Errorf("reached %v from kumotori_1, want all %d maps", seen, len(loaded))
	}
	for id, m := range loaded {
		want := 2
		if m.Type == "town" {
			want = 1
		}
		if m.MinZones != want || len(m.Respawns) == 0 {
			t.Errorf("%s: min_zones %d (want %d), respawns %v", id, m.MinZones, want, m.RespawnIDs)
		}
	}
	if k := loaded["kumotori_1"]; !slices.Equal(k.RespawnIDs, []string{"default"}) || k.Respawns[0] != (Point{5.5, 5.5}) {
		t.Errorf("kumotori_1 respawns %v at %v, want default at (5.5, 5.5)", k.RespawnIDs, k.Respawns)
	}
}

// ______________________TEST NPCS_____________________

// npcJSON is portalJSON with a town and two NPCs inside the camp: smith (a
// blacksmith) at (3.5, 1.5) and cart (a carriage with one route) at (3.5, 5.5).
func npcJSON(t *testing.T, mutate func(m map[string]any)) []byte {
	t.Helper()
	return portalJSON(t, func(m map[string]any) {
		m["town"] = "t1"
		m["npcs"] = []map[string]any{
			{"id": "smith", "name": "Thợ rèn", "role": "blacksmith", "x": 3.5, "y": 1.5},
			{"id": "cart", "name": "Xa phu", "role": "carriage", "x": 3.5, "y": 5.5,
				"routes": []map[string]any{{"map": "a", "xu": 50}}},
		}
		if mutate != nil {
			mutate(m)
		}
	})
}

func npcAt(m map[string]any, i int) map[string]any { return m["npcs"].([]map[string]any)[i] }

func TestParseNPCs(t *testing.T) {
	m, err := Parse(npcJSON(t, func(m map[string]any) {
		m["npcs"] = append(m["npcs"].([]map[string]any),
			map[string]any{"id": "shop", "name": "Tạp hóa", "role": "shop", "shop": "general", "x": 1.5, "y": 3.5})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if m.Town != "t1" || len(m.NPCs) != 3 {
		t.Fatalf("town %q, npcs %+v", m.Town, m.NPCs)
	}
	if m.NPCs[0].ID != "smith" || m.NPCs[1].ID != "cart" || m.NPCs[2].ID != "shop" {
		t.Fatalf("npcs not in file order: %+v", m.NPCs)
	}
	cart, ok := m.NPC("cart")
	if !ok || cart.Role != RoleCarriage || cart.Pos != (Point{3.5, 5.5}) ||
		len(cart.Routes) != 1 || cart.Routes[0] != (Route{Map: "a", Xu: 50}) {
		t.Fatalf("cart = %+v, %v", cart, ok)
	}
	if shop, _ := m.NPC("shop"); shop.Role != RoleShop || shop.Shop != "general" {
		t.Fatalf("shop = %+v", shop)
	}
	if _, ok := m.NPC("nobody"); ok {
		t.Fatal("NPC found an unknown id")
	}
	// No npcs at all is fine.
	if m, err := Parse(portalJSON(t, nil)); err != nil || len(m.NPCs) != 0 {
		t.Fatalf("map without npcs: %v %v", m, err)
	}
}

func TestParseNPCErrors(t *testing.T) {
	cases := map[string]func(m map[string]any){
		`outside the safe zones`: func(m map[string]any) { npcAt(m, 0)["x"], npcAt(m, 0)["y"] = 8.5, 3.5 },
		`is not walkable`:        func(m map[string]any) { setTiles(m, '#', [2]int{3, 1}) },
		`unreachable from default`: func(m map[string]any) {
			setTiles(m, '#', [2]int{2, 6}, [2]int{1, 5})
			npcAt(m, 0)["x"], npcAt(m, 0)["y"] = 1.5, 6.5
		},
		`duplicate npc "smith"`:               func(m map[string]any) { npcAt(m, 1)["id"] = "smith" },
		`npc without id`:                      func(m map[string]any) { npcAt(m, 0)["id"] = "" },
		`npc "smith": missing name`:           func(m map[string]any) { npcAt(m, 0)["name"] = "" },
		`invalid role "wizard"`:               func(m map[string]any) { npcAt(m, 0)["role"] = "wizard" },
		`"smith": shop set on a non-shop npc`: func(m map[string]any) { npcAt(m, 0)["shop"] = "general" },
		`"smith": role shop needs a shop`:     func(m map[string]any) { npcAt(m, 0)["role"] = "shop" },
		`"smith": routes set on a non-carriage npc`: func(m map[string]any) {
			npcAt(m, 0)["routes"] = []map[string]any{{"map": "a", "xu": 50}}
		},
		`"cart": a carriage needs routes`: func(m map[string]any) { delete(npcAt(m, 1), "routes") },
		`"cart": route to "a": xu must be at least 1`: func(m map[string]any) {
			npcAt(m, 1)["routes"] = []map[string]any{{"map": "a", "xu": 0}}
		},
		`"cart": duplicate route to "a"`: func(m map[string]any) {
			npcAt(m, 1)["routes"] = []map[string]any{{"map": "a", "xu": 50}, {"map": "a", "xu": 60}}
		},
		`"cart": route to its own map`: func(m map[string]any) {
			npcAt(m, 1)["routes"] = []map[string]any{{"map": "t1", "xu": 50}}
		},
		`closer than 1.5 tiles`: func(m map[string]any) { npcAt(m, 1)["x"], npcAt(m, 1)["y"] = 3.5, 2.5 },
	}
	for want, mut := range cases {
		_, err := Parse(npcJSON(t, mut))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error containing %q, got %v", want, err)
		}
	}
	// Exactly 1.5 tiles apart is allowed.
	if _, err := Parse(npcJSON(t, func(m map[string]any) { npcAt(m, 1)["x"], npcAt(m, 1)["y"] = 3.5, 3.0 })); err != nil {
		t.Errorf("1.5 tiles apart: %v", err)
	}
}

// townPair is linkPair with a carriage in b.
func townPair(t *testing.T, mutateA, mutateB func(m map[string]any)) map[string]*Map {
	t.Helper()
	return linkPair(t, mutateA, func(m map[string]any) {
		m["npcs"] = []map[string]any{{"id": "cart", "name": "Xa phu", "role": "carriage", "x": 3.5, "y": 5.5,
			"routes": []map[string]any{{"map": "a", "xu": 50}}}}
		if mutateB != nil {
			mutateB(m)
		}
	})
}

func TestLinkTown(t *testing.T) {
	if err := Link(townPair(t, nil, nil)); err != nil {
		t.Fatalf("a valid pair: %v", err)
	}
	cases := map[string][2]func(m map[string]any){
		`mapdata b: missing town`:                {nil, func(m map[string]any) { delete(m, "town") }},
		`mapdata b: town "zzz" is not a map`:     {nil, func(m map[string]any) { m["town"] = "zzz" }},
		`mapdata b: town "b" is not a town`:      {nil, func(m map[string]any) { m["town"] = "b" }},
		`mapdata a: a town must be its own town`: {func(m map[string]any) { m["town"] = "b" }, nil},
		`mapdata b: npc "cart": route to unknown map "zzz"`: {nil, func(m map[string]any) {
			npcAt(m, 0)["routes"] = []map[string]any{{"map": "zzz", "xu": 50}}
		}},
	}
	for want, mut := range cases {
		err := Link(townPair(t, mut[0], mut[1]))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error containing %q, got %v", want, err)
		}
	}
}

// Phase 4b spec 2.2: the NPCs of the five maps.
func TestRepoMapsNPCs(t *testing.T) {
	loaded, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	for id, m := range loaded {
		if m.Town != "ubuyashiki" {
			t.Errorf("%s: town %q, want ubuyashiki", id, m.Town)
		}
	}
	routes := func(m *Map) map[string]int64 {
		cart, ok := m.NPC("xa_phu")
		if !ok || cart.Role != RoleCarriage || cart.Name != "Xa phu" || len(cart.Routes) != 4 {
			t.Fatalf("%s: xa_phu = %+v, %v", m.ID, cart, ok)
		}
		out := map[string]int64{}
		for _, r := range cart.Routes {
			out[r.Map] = r.Xu
		}
		return out
	}
	price := map[string]int64{"ubuyashiki": 50, "kumotori_1": 50, "sagiri_1": 120, "sagiri_2": 220, "fujikasane_1": 320}
	town := loaded["ubuyashiki"]
	if len(town.NPCs) != 4 {
		t.Fatalf("ubuyashiki npcs %+v, want 4", town.NPCs)
	}
	if n, ok := town.NPC("tap_hoa"); !ok || n.Role != RoleShop || n.Shop != "general" || n.Name != "Tạp hóa" {
		t.Errorf("tap_hoa = %+v, %v", n, ok)
	}
	if n, ok := town.NPC("tho_ren"); !ok || n.Role != RoleBlacksmith || n.Name != "Thợ rèn" {
		t.Errorf("tho_ren = %+v, %v", n, ok)
	}
	for to, xu := range routes(town) {
		if to == "ubuyashiki" || xu != price[to] {
			t.Errorf("ubuyashiki route %s %d, want %d", to, xu, price[to])
		}
	}
	for _, id := range []string{"kumotori_1", "sagiri_1", "sagiri_2", "fujikasane_1"} {
		m := loaded[id]
		if len(m.NPCs) != 3 {
			t.Errorf("%s npcs %+v, want 3", id, m.NPCs)
		}
		if n, ok := m.NPC("duoc_su"); !ok || n.Role != RoleShop || n.Shop != "apothecary" || n.Name != "Dược sư" {
			t.Errorf("%s: duoc_su = %+v, %v", id, n, ok)
		}
		got := routes(m)
		if _, self := got[id]; self {
			t.Errorf("%s: route to itself", id)
		}
		for to, xu := range got {
			if xu != price[to] {
				t.Errorf("%s route %s %d, want %d", id, to, xu, price[to])
			}
		}
	}
	k := loaded["kumotori_1"]
	if n, _ := k.NPC("duoc_su"); n.Pos != (Point{8.5, 3.5}) {
		t.Errorf("kumotori duoc_su at %v, want (8.5, 3.5)", n.Pos)
	}
	if n, _ := k.NPC("xa_phu"); n.Pos != (Point{8.5, 7.5}) {
		t.Errorf("kumotori xa_phu at %v, want (8.5, 7.5)", n.Pos)
	}
}

// ______________________TEST STASH DATA_____________________

func TestParseStorageNPC(t *testing.T) {
	m, err := Parse(npcJSON(t, func(m map[string]any) {
		m["npcs"] = append(m["npcs"].([]map[string]any),
			map[string]any{"id": "kho", "name": "Thủ kho", "role": "storage", "x": 1.5, "y": 3.5})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := m.NPC("kho"); !ok || n.Role != RoleStorage || n.Shop != "" || len(n.Routes) != 0 {
		t.Fatalf("kho = %+v, %v", n, ok)
	}
	cases := map[string]func(n map[string]any){
		`"kho": shop set on a non-shop npc`:       func(n map[string]any) { n["shop"] = "general" },
		`"kho": routes set on a non-carriage npc`: func(n map[string]any) { n["routes"] = []map[string]any{{"map": "a", "xu": 50}} },
	}
	for want, mut := range cases {
		_, err := Parse(npcJSON(t, func(m map[string]any) {
			n := map[string]any{"id": "kho", "name": "Thủ kho", "role": "storage", "x": 1.5, "y": 3.5}
			mut(n)
			m["npcs"] = append(m["npcs"].([]map[string]any), n)
		}))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error containing %q, got %v", want, err)
		}
	}
}

// Phase 4c spec 2.2: every map has a storage keeper within 3 tiles of default.
func TestRepoMapsStorage(t *testing.T) {
	loaded, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 5 {
		t.Fatalf("%d maps", len(loaded))
	}
	for id, m := range loaded {
		n, ok := m.NPC("thu_kho")
		if !ok || n.Role != RoleStorage || n.Name != "Thủ kho" || n.Shop != "" || len(n.Routes) != 0 {
			t.Errorf("%s: thu_kho = %+v, %v", id, n, ok)
			continue
		}
		def := m.Spawns["default"]
		if d := math.Hypot(float64(n.Pos.X-def.X), float64(n.Pos.Y-def.Y)); d > 3 {
			t.Errorf("%s: thu_kho %.2f tiles from default", id, d)
		}
	}
	if n, _ := loaded["kumotori_1"].NPC("thu_kho"); n.Pos != (Point{3.5, 7.5}) {
		t.Errorf("kumotori thu_kho at %v, want (3.5, 7.5)", n.Pos)
	}
}
