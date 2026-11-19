package zone

import "math"

// Vec2 is a position or direction in tile units.
type Vec2 struct{ X, Y float32 }

// Grid reports blocked tiles; *mapdata.Map implements it.
type Grid interface {
	Blocked(tx, ty int) bool
}

const (
	halfSize   = 0.3  // entities are 0.6x0.6 boxes centred on their position
	maxSubstep = 0.25 // < halfSize so a step can never skip over a tile edge
	eps        = 1e-4
)

// ClampDir limits a direction to length 1 and turns NaN/Inf into zero.
func ClampDir(x, y float32) Vec2 {
	if bad(x) || bad(y) {
		return Vec2{}
	}
	if l2 := float64(x)*float64(x) + float64(y)*float64(y); l2 > 1 {
		l := float32(math.Sqrt(l2))
		return Vec2{x / l, y / l}
	}
	return Vec2{x, y}
}

func bad(v float32) bool {
	f := float64(v)
	return math.IsNaN(f) || math.IsInf(f, 0)
}

func floor(v float32) int { return int(math.Floor(float64(v))) }

func distance(a, b Vec2) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

// Step moves pos by dir*dist, resolving X then Y in sub-steps so the box slides along walls.
func Step(g Grid, pos, dir Vec2, dist float32) Vec2 {
	if dist <= 0 || dir == (Vec2{}) {
		return pos
	}
	n := int(math.Ceil(float64(dist / maxSubstep)))
	sx := dir.X * dist / float32(n)
	sy := dir.Y * dist / float32(n)
	for i := 0; i < n; i++ {
		pos.X = moveX(g, pos, sx)
		pos.Y = moveY(g, pos, sy)
	}
	return pos
}

func moveX(g Grid, pos Vec2, dx float32) float32 {
	if dx == 0 {
		return pos.X
	}
	nx := pos.X + dx
	top, bottom := floor(pos.Y-halfSize+eps), floor(pos.Y+halfSize-eps)
	if dx > 0 {
		col := floor(nx + halfSize - eps)
		for row := top; row <= bottom; row++ {
			if g.Blocked(col, row) {
				return float32(col) - halfSize
			}
		}
	} else {
		col := floor(nx - halfSize + eps)
		for row := top; row <= bottom; row++ {
			if g.Blocked(col, row) {
				return float32(col+1) + halfSize
			}
		}
	}
	return nx
}

func moveY(g Grid, pos Vec2, dy float32) float32 {
	if dy == 0 {
		return pos.Y
	}
	ny := pos.Y + dy
	left, right := floor(pos.X-halfSize+eps), floor(pos.X+halfSize-eps)
	if dy > 0 {
		row := floor(ny + halfSize - eps)
		for col := left; col <= right; col++ {
			if g.Blocked(col, row) {
				return float32(row) - halfSize
			}
		}
	} else {
		row := floor(ny - halfSize + eps)
		for col := left; col <= right; col++ {
			if g.Blocked(col, row) {
				return float32(row+1) + halfSize
			}
		}
	}
	return ny
}

// lineBlocked reports whether the segment from a to b crosses a blocked tile.
// It walks every tile the segment passes through (Amanatides-Woo traversal).
// Where the segment passes exactly through a tile corner, it counts as
// blocked if either tile beside the corner is. The endpoints' own tiles do
// not count.
func lineBlocked(g Grid, a, b Vec2) bool {
	sx, sy := floor(a.X), floor(a.Y)
	ex, ey := floor(b.X), floor(b.Y)
	x, y := sx, sy
	blocked := func(tx, ty int) bool {
		return !(tx == sx && ty == sy) && !(tx == ex && ty == ey) && g.Blocked(tx, ty)
	}
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	stepX, tMaxX, tDeltaX := axis(float64(a.X), dx, x)
	stepY, tMaxY, tDeltaY := axis(float64(a.Y), dy, y)
	// Each step moves one tile along x or y (two at a corner), so the walk
	// ends after at most this many tiles even if float error skips the end tile.
	for n := abs(ex-x) + abs(ey-y); n > 0 && (x != ex || y != ey); {
		switch d := tMaxX - tMaxY; {
		case math.Abs(d) < 1e-9: // through a corner
			if blocked(x+stepX, y) || blocked(x, y+stepY) {
				return true
			}
			x, y = x+stepX, y+stepY
			tMaxX, tMaxY = tMaxX+tDeltaX, tMaxY+tDeltaY
			n -= 2
		case d < 0:
			x += stepX
			tMaxX += tDeltaX
			n--
		default:
			y += stepY
			tMaxY += tDeltaY
			n--
		}
		if blocked(x, y) {
			return true
		}
	}
	return false
}

// axis sets up one axis of the tile traversal from position p moving by d,
// starting in tile t: the step direction, the segment fraction at which the
// first tile edge is crossed, and the fraction between edges.
func axis(p, d float64, t int) (step int, tMax, tDelta float64) {
	switch {
	case d > 0:
		return 1, (float64(t+1) - p) / d, 1 / d
	case d < 0:
		return -1, (p - float64(t)) / -d, 1 / -d
	default:
		return 0, math.Inf(1), math.Inf(1)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
