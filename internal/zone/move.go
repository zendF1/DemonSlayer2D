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
