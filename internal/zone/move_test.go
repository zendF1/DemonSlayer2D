package zone

import (
	"math"
	"strings"
	"testing"
)

type testGrid struct {
	w, h    int
	blocked map[[2]int]bool
}

func gridFrom(rows ...string) *testGrid {
	g := &testGrid{w: len(rows[0]), h: len(rows), blocked: map[[2]int]bool{}}
	for y, r := range rows {
		for x := 0; x < len(r); x++ {
			if r[x] == '#' {
				g.blocked[[2]int{x, y}] = true
			}
		}
	}
	return g
}

func openGrid(w, h int) *testGrid {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = strings.Repeat(".", w)
	}
	return gridFrom(rows...)
}

func (g *testGrid) Blocked(x, y int) bool {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return true
	}
	return g.blocked[[2]int{x, y}]
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

var room = []string{
	"#####",
	"#...#",
	"#...#",
	"#####",
}

func TestStepOpenArea(t *testing.T) {
	p := Step(openGrid(6, 3), Vec2{1.5, 1.5}, Vec2{1, 0}, 0.4)
	if !near(p.X, 1.9) || !near(p.Y, 1.5) {
		t.Fatalf("got %+v", p)
	}
}

func TestStepStopsAtWall(t *testing.T) {
	p := Step(gridFrom(room...), Vec2{1.5, 1.5}, Vec2{-1, 0}, 1)
	if !near(p.X, 1.3) || !near(p.Y, 1.5) {
		t.Fatalf("got %+v, want x=1.3 (box edge on the wall)", p)
	}
}

func TestStepSlidesAlongWall(t *testing.T) {
	p := Step(gridFrom(room...), Vec2{1.5, 1.5}, ClampDir(-1, 1), 1)
	if !near(p.X, 1.3) {
		t.Fatalf("x = %v, want 1.3", p.X)
	}
	if !near(p.Y, 1.5+0.7071) {
		t.Fatalf("y = %v, want to keep sliding down to ~2.207", p.Y)
	}
}

func TestStepNoTunnelling(t *testing.T) {
	p := Step(gridFrom(".#...."), Vec2{0.5, 0.5}, Vec2{1, 0}, 10)
	if !near(p.X, 0.7) {
		t.Fatalf("x = %v, a 1-tile wall must stop a long step at 0.7", p.X)
	}
}

func TestStepOutsideMapIsBlocked(t *testing.T) {
	p := Step(gridFrom("..."), Vec2{0.5, 0.5}, Vec2{-1, 0}, 1)
	if !near(p.X, 0.3) {
		t.Fatalf("x = %v, want 0.3", p.X)
	}
}

func TestStepNeverExceedsDistance(t *testing.T) {
	from := Vec2{10, 10}
	p := Step(openGrid(20, 20), from, ClampDir(0.6, 0.8), 0.4)
	dx, dy := float64(p.X-from.X), float64(p.Y-from.Y)
	if d := math.Hypot(dx, dy); d > 0.4+1e-4 {
		t.Fatalf("moved %v, want <= 0.4", d)
	}
}

func TestClampDir(t *testing.T) {
	if d := ClampDir(3, 4); !near(d.X, 0.6) || !near(d.Y, 0.8) {
		t.Fatalf("ClampDir(3,4) = %+v", d)
	}
	if d := ClampDir(0.5, 0); d != (Vec2{0.5, 0}) {
		t.Fatalf("short vectors are kept, got %+v", d)
	}
	if d := ClampDir(float32(math.NaN()), 1); d != (Vec2{}) {
		t.Fatalf("NaN must become zero, got %+v", d)
	}
	if d := ClampDir(float32(math.Inf(1)), 0); d != (Vec2{}) {
		t.Fatalf("Inf must become zero, got %+v", d)
	}
}
