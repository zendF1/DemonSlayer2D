package mapdata

import (
	"encoding/json"
	"strings"
	"testing"
)

func mapJSON(t *testing.T, mutate func(m map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"id": "t1", "name": "Test", "type": "training", "width": 4, "height": 3,
		"collision":    []string{"####", "#..#", "####"},
		"spawn_points": []map[string]any{{"id": "default", "x": 1.5, "y": 1.5}},
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
			m["spawn_points"] = []map[string]any{{"id": "default", "x": 0.5, "y": 0.5}}
		},
		"default spawn": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{{"id": "gate", "x": 1.5, "y": 1.5}}
		},
		"duplicate spawn": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{
				{"id": "default", "x": 1.5, "y": 1.5}, {"id": "default", "x": 2.5, "y": 1.5},
			}
		},
		"unknown field": func(m map[string]any) { m["portals"] = []string{} },
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
// tiles 1..3 x 1..3, a rest point and a camp away from the safe zone.
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
		m["camps"] = []map[string]any{{
			"id": "c1", "monster": "wanderer", "x": 7.0, "y": 5.0, "radius": 2.0,
			"count": 3, "min_level": 1, "max_level": 2,
		}}
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
	if len(m.Camps) != 1 {
		t.Fatalf("camps = %+v", m.Camps)
	}
	c := m.Camps[0]
	if c.ID != "c1" || c.Monster != "wanderer" || c.X != 7 || c.Y != 5 || c.Radius != 2 ||
		c.Count != 3 || c.MinLevel != 1 || c.MaxLevel != 2 {
		t.Fatalf("camp = %+v", c)
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

	tiles := m.CampTiles(c)
	if len(tiles) < c.Count {
		t.Fatalf("only %d camp tiles", len(tiles))
	}
	have := map[Point]bool{}
	for i, p := range tiles {
		have[p] = true
		if !m.Walkable(p) {
			t.Errorf("camp tile %v is not walkable", p)
		}
		dx, dy := float64(p.X-c.X), float64(p.Y-c.Y)
		if dx*dx+dy*dy > float64(c.Radius*c.Radius) {
			t.Errorf("camp tile %v is beyond the radius", p)
		}
		if i > 0 {
			q := tiles[i-1]
			if q.Y > p.Y || (q.Y == p.Y && q.X >= p.X) {
				t.Errorf("camp tiles not sorted by (y, x): %v then %v", q, p)
			}
		}
	}
	if have[Point{6.5, 4.5}] {
		t.Error("camp tiles include the blocked tile (6,4)")
	}
	if have[Point{3.5, 5.5}] || have[Point{9.5, 5.5}] {
		t.Error("camp tiles include tiles beyond the radius or in the wall")
	}
	if !have[Point{7.5, 4.5}] || !have[Point{7.5, 5.5}] {
		t.Error("camp tiles miss tiles near the centre")
	}
}

func TestParseTriggersOptional(t *testing.T) {
	m, err := Parse(mapJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.SafeZones)+len(m.RestPoints)+len(m.Camps) != 0 || m.InSafeZone(Point{1.5, 1.5}) {
		t.Fatal("a map without triggers must have none")
	}
}

func TestParseTriggerErrors(t *testing.T) {
	camp := func(m map[string]any, set func(c map[string]any)) {
		c := m["camps"].([]map[string]any)[0]
		set(c)
	}
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
			m["spawn_points"] = []map[string]any{{"id": "default", "x": 6.5, "y": 1.5}}
		},
		"camp \"c1\": min_level 3 > max_level 2": func(m map[string]any) {
			camp(m, func(c map[string]any) { c["min_level"] = 3 })
		},
		"camp \"c1\": count must be positive": func(m map[string]any) {
			camp(m, func(c map[string]any) { c["count"] = 0 })
		},
		"camp \"c1\": radius must be positive": func(m map[string]any) {
			camp(m, func(c map[string]any) { c["radius"] = 0 })
		},
		"camp \"c1\" has": func(m map[string]any) {
			camp(m, func(c map[string]any) { c["count"] = 100 })
		},
		"camp \"c1\": tile": func(m map[string]any) {
			camp(m, func(c map[string]any) { c["x"], c["y"], c["count"] = 3.0, 3.0, 1 })
		},
		"duplicate camp \"c1\"": func(m map[string]any) {
			c := m["camps"].([]map[string]any)[0]
			m["camps"] = []map[string]any{c, c}
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
	if len(m.SafeZones) != 1 || len(m.RestPoints) != 1 || len(m.Camps) != 4 {
		t.Fatalf("kumotori_1 has %d safe zones, %d rest points, %d camps", len(m.SafeZones), len(m.RestPoints), len(m.Camps))
	}
	if !m.InSafeZone(m.Spawns["default"]) {
		t.Error("default spawn is not in the safe zone")
	}
	for _, c := range m.Camps {
		if n := len(m.CampTiles(c)); n < 12 {
			t.Errorf("camp %s has %d tiles, want >= 12", c.ID, n)
		}
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
