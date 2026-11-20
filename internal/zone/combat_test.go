package zone

import (
	"maps"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
)

// newFightZone builds a zone on a 60x60 open grid with the safe rect (0,0,10,10)
// and a single testKind monster of the given level standing at home. Its
// region has home as its only tile and nest, but its rect is the whole map,
// so the region margin never ends a chase.
func newFightZone(t *testing.T, level int, home Vec2) (*Zone, *monster) {
	t.Helper()
	return newFightZoneOf(t, testKind, level, home)
}

func newFightZoneOf(t *testing.T, kind gamedata.Kind, level int, home Vec2) (*Zone, *monster) {
	t.Helper()
	cfg := testConfig(10)
	area := Area{
		Grid:    openGrid(60, 60),
		Safe:    []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Respawn: Vec2{5.5, 5.5},
		Regions: []Region{{ID: "r1", Tiles: []Vec2{home}, Rects: []Rect{{X: 0, Y: 0, W: 60, H: 60}}, Nest: home,
			Count: 1, MinLevel: level, MaxLevel: level, Kind: kind}},
		Levels: testLevels,
		Exp:    testExp,
	}
	z := New(cfg, area, &fakeSaver{}, quietLog())
	if len(z.monsters) != 1 {
		t.Fatalf("monsters = %d, want 1", len(z.monsters))
	}
	for _, m := range z.monsters {
		return z, m
	}
	panic("unreachable")
}

func joinHP(t *testing.T, z *Zone, id uint64, x, y float32, hp int) *fakeOut {
	t.Helper()
	in := info(id, x, y)
	in.HP = hp
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func ticksIn(envs []*pb.Envelope) []*pb.WorldTick {
	var wts []*pb.WorldTick
	for _, e := range envs {
		if wt := e.GetWorldTick(); wt != nil {
			wts = append(wts, wt)
		}
	}
	return wts
}

func hitsIn(envs []*pb.Envelope) []*pb.Hit {
	var hs []*pb.Hit
	for _, wt := range ticksIn(envs) {
		hs = append(hs, wt.Hits...)
	}
	return hs
}

func deathsIn(envs []*pb.Envelope) []uint64 {
	var ids []uint64
	for _, wt := range ticksIn(envs) {
		for _, d := range wt.Deaths {
			ids = append(ids, d.Id)
		}
	}
	return ids
}

func hpChangesIn(envs []*pb.Envelope) []*pb.HpChange {
	var cs []*pb.HpChange
	for _, wt := range ticksIn(envs) {
		cs = append(cs, wt.HpChanges...)
	}
	return cs
}

// ______________________TEST MONSTER AI_____________________

// engage puts m in a chase on player id from where it stands, the way a
// slash does. Tests of melee use it now that a passer-by is shot instead of
// chased.
func engage(m *monster, id uint64) { m.target, m.state, m.chaseFrom = id, chase, m.pos }

// teleport puts player id at pos, as if it had walked there.
func teleport(z *Zone, id uint64, pos Vec2) {
	p := z.players[id]
	p.pos = pos
	z.updateCell(&p.entity)
}

// rangedHits splits envs' hits into ranged and melee ones.
func rangedHits(envs []*pb.Envelope) (ranged, melee []*pb.Hit) {
	for _, h := range hitsIn(envs) {
		if h.Ranged {
			ranged = append(ranged, h)
		} else {
			melee = append(melee, h)
		}
	}
	return ranged, melee
}

// A player who steps within the detect range with a clear line is shot: the
// first shot after the windup, then one every shoot_sec, each for the ranged
// damage. The monster stands still and never chases (spec 4.3).
func TestShootPasserBy(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	a := joinNow(t, z, 1, home.X+7, home.Y) // in view, out of range
	v := joinNow(t, z, 2, home.X, home.Y+7)
	now := time.Now()
	for i := 0; i < 10; i++ {
		z.advance(now)
	}
	if hs := hitsIn(a.take()); len(hs) != 0 || m.state != wander {
		t.Fatalf("out of range: hits %v state %v, want none and wandering", hs, m.state)
	}
	v.take()
	teleport(z, 1, Vec2{home.X + 4.9, home.Y})
	stepAt := z.tickNo + 1 // the tick that sees it
	p := z.players[1]
	want := int32(combat.MonsterRangedDamage(testLevels[1].Attack, p.stats.PhysDefense, 1))
	if want != 10 {
		t.Fatalf("setup: ranged damage %d, want 10 (half of the melee 20)", want)
	}
	var shots []uint32
	for z.tickNo < stepAt+100 {
		z.advance(now)
		envs := a.take()
		ranged, melee := rangedHits(envs)
		if len(melee) != 0 {
			t.Fatalf("tick %d: melee hits %v from a shooting monster", z.tickNo, melee)
		}
		for _, h := range ranged {
			hp := int32(500) - want*int32(len(shots)+1)
			if h.AttackerId != m.id || h.TargetId != 1 || h.Damage != want || h.HpAfter != hp {
				t.Fatalf("tick %d: shot %v, want %d hits 1 for %d leaving %d", z.tickNo, h, m.id, want, hp)
			}
			shots = append(shots, z.tickNo)
		}
		if vr, _ := rangedHits(v.take()); len(vr) != len(ranged) {
			t.Fatalf("tick %d: viewer saw %d shots, target %d", z.tickNo, len(vr), len(ranged))
		}
		if m.pos != home || m.dir != (Vec2{}) {
			t.Fatalf("tick %d: a shooting monster moved to %v dir %v", z.tickNo, m.pos, m.dir)
		}
		if m.target != 0 || m.state == chase || m.state == attack {
			t.Fatalf("tick %d: state %v target %d, want shooting without a target", z.tickNo, m.state, m.target)
		}
	}
	windup, every := z.ticks(testKind.ShootWindupSec), z.ticks(testKind.ShootSec)
	wantShots := []uint32{stepAt + windup, stepAt + windup + every, stepAt + windup + 2*every, stepAt + windup + 3*every, stepAt + windup + 4*every}
	if !slices.Equal(shots, wantShots) {
		t.Fatalf("shots on ticks %v, want %v (windup %d, then every %d)", shots, wantShots, windup, every)
	}
	if m.state != shoot {
		t.Fatalf("state %v, want shoot", m.state)
	}
}

// Nobody is shot beyond the detect range, in a safe zone or behind a wall.
func TestShootIgnoresFarSafeAndHidden(t *testing.T) {
	wall := func(z *Zone) {
		rows := make([]string, 60)
		for y := range rows {
			r := []byte(strings.Repeat(".", 60))
			if y >= 25 && y <= 36 {
				r[32] = '#'
			}
			rows[y] = string(r)
		}
		z.area.Grid = gridFrom(rows...)
		z.mgrid = z.area.monsterGrid()
	}
	for _, c := range []struct {
		name   string
		home   Vec2
		player Vec2
		setup  func(z *Zone)
	}{
		{"5.1 tiles away", Vec2{30.5, 30.5}, Vec2{35.6, 30.5}, nil},
		{"in the safe zone", Vec2{11.5, 5.5}, Vec2{9.5, 5.5}, nil}, // tile 9 is the safe rect's last column
		{"behind a wall", Vec2{30.5, 30.5}, Vec2{33.5, 30.5}, wall},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m := newFightZone(t, 1, c.home)
			if c.setup != nil {
				c.setup(z)
			}
			a := joinNow(t, z, 1, c.player.X, c.player.Y)
			now := time.Now()
			for i := 0; i < 50; i++ {
				z.advance(now)
				if m.state != wander || m.target != 0 {
					t.Fatalf("tick %d: state %v target %d, want a wandering monster", i, m.state, m.target)
				}
			}
			if hs := hitsIn(a.take()); len(hs) != 0 || z.players[1].hp != 500 {
				t.Fatalf("hits %v hp %d, want none", hs, z.players[1].hp)
			}
		})
	}
}

// A player who walks out of range is not shot again: at the next shot time
// the monster finds nobody and wanders, starting with a rest.
func TestShootStopsWhenPlayerLeaves(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	a := joinNow(t, z, 1, home.X+3, home.Y)
	now := time.Now()
	for len(hitsIn(a.take())) == 0 {
		if z.tickNo > 20 {
			t.Fatal("never shot")
		}
		z.advance(now)
	}
	next := m.nextShot
	teleport(z, 1, Vec2{home.X + 8, home.Y})
	for z.tickNo < next {
		z.advance(now)
		if hs := hitsIn(a.take()); len(hs) != 0 {
			t.Fatalf("tick %d: hit %v after the player left", z.tickNo, hs)
		}
	}
	if m.state != wander || m.target != 0 || m.dir != (Vec2{}) {
		t.Fatalf("at the shot time: state %v target %d dir %v, want wandering", m.state, m.target, m.dir)
	}
	if rest := m.nextWander - z.tickNo; rest < z.ticks(restMinSec) || rest > z.ticks(restMaxSec) {
		t.Fatalf("rest of %d ticks, want %d-%d", rest, z.ticks(restMinSec), z.ticks(restMaxSec))
	}
}

// nextShot advances z until a monster shoots and returns that tick's ranged hits.
func nextShot(t *testing.T, z *Zone) []*pb.Hit {
	t.Helper()
	now := time.Now()
	for range 100 {
		z.advance(now)
		var shots []*pb.Hit
		for _, h := range z.events.hits {
			if h.Ranged {
				shots = append(shots, h)
			}
		}
		if len(shots) > 0 {
			return shots
		}
	}
	t.Fatal("no shot in 100 ticks")
	return nil
}

// Each shot goes to the nearest player at that moment, a tie to the lower id.
func TestShootPicksNearest(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, _ := newFightZone(t, 1, home)
	joinNow(t, z, 1, home.X, home.Y+4)   // farther
	joinNow(t, z, 2, home.X-2.5, home.Y) // nearer
	if hs := nextShot(t, z); len(hs) != 1 || hs[0].TargetId != 2 {
		t.Fatalf("first shot %v, want the nearer player 2 shot", hs)
	}
	teleport(z, 2, Vec2{home.X - 4.5, home.Y}) // now 1 is the nearer
	if hs := nextShot(t, z); len(hs) != 1 || hs[0].TargetId != 1 {
		t.Fatalf("second shot %v, want player 1 shot", hs)
	}
	teleport(z, 2, Vec2{home.X + 4, home.Y})
	if hs := nextShot(t, z); len(hs) != 1 || hs[0].TargetId != 1 {
		t.Fatalf("tied shot %v, want the lower id 1 shot", hs)
	}
}

// A shot can kill: the player dies as from a melee hit, the monster does not
// shoot the corpse, and the player respawns as usual.
func TestShotKillsPlayer(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	a := joinHP(t, z, 1, home.X+3, home.Y, 1)
	v := joinNow(t, z, 2, home.X, home.Y+7)
	p := z.players[1]
	now := time.Now()
	for !p.dead && z.tickNo < 20 {
		z.advance(now)
	}
	ea, ev := a.take(), v.take()
	if hs := hitsIn(ea); len(hs) != 1 || !hs[0].Ranged || hs[0].HpAfter != 0 || hs[0].AttackerId != m.id {
		t.Fatalf("hits %v, want one ranged hit leaving 0 HP", hs)
	}
	if !p.dead || p.hp != 0 || p.deadUntil != z.tickNo+z.ticks(autoRespawnSec) {
		t.Fatalf("dead %v hp %d deadUntil %d, want dead with the usual respawn timer", p.dead, p.hp, p.deadUntil)
	}
	if !slices.Equal(deathsIn(ea), []uint64{1}) || !slices.Equal(deathsIn(ev), []uint64{1}) {
		t.Fatalf("deaths %v / %v, want [1] for both", deathsIn(ea), deathsIn(ev))
	}
	for next := m.nextShot; z.tickNo < next; {
		z.advance(now)
	}
	if hs := hitsIn(a.take()); len(hs) != 0 || m.state != wander {
		t.Fatalf("after the kill: hits %v state %v, want no shot at the corpse and wandering", hs, m.state)
	}
	if !z.Respawn(1, 3) {
		t.Fatal("inbox full")
	}
	z.advance(now)
	if r, _ := respawnRespFor(a.take(), 3); r == nil || p.dead || p.pos != z.area.Respawn || p.hp != p.maxHP {
		t.Fatalf("respawn %v dead %v at %v hp %d, want alive and full at the respawn point", r, p.dead, p.pos, p.hp)
	}
}

// A slash on a shooting monster turns it on the attacker: it chases and hits
// in melee and shoots no more.
func TestHitStartsChaseFromShoot(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	a := joinNow(t, z, 1, home.X+2.5, home.Y) // out of its 1.5 attack range
	now := time.Now()
	for len(hitsIn(a.take())) == 0 {
		if z.tickNo > 20 {
			t.Fatal("never shot")
		}
		z.advance(now)
	}
	if m.state != shoot {
		t.Fatalf("state %v, want shoot", m.state)
	}
	teleport(z, 1, Vec2{home.X + 1.9, home.Y}) // in slash range with the slack
	attackNow(t, z, 1, 5, m.id)
	envs := a.take()
	if attackRespAt(envs, 5) < 0 {
		t.Fatalf("slash rejected: %v", errorFor(envs, 5))
	}
	if m.state != chase && m.state != attack || m.target != 1 {
		t.Fatalf("state %v target %d, want chasing the attacker", m.state, m.target)
	}
	melee := 0
	for i := 0; i < 40; i++ {
		z.advance(now)
		for _, h := range hitsIn(a.take()) {
			if h.AttackerId != m.id {
				continue
			}
			if h.Ranged {
				t.Fatalf("tick %d: ranged hit %v from a chasing monster", z.tickNo, h)
			}
			melee++
		}
	}
	if melee == 0 || m.state != attack {
		t.Fatalf("melee hits %d state %v, want it to close in and hit", melee, m.state)
	}
}

// wanderRegion builds a zone whose region is rect minus the blocked block,
// with count monsters, and a viewer in the safe zone who sees it all.
func wanderRegion(t *testing.T, seed uint64, rect Rect, block Rect, count int) (*Zone, *regionState, *fakeOut) {
	t.Helper()
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		for x := range r {
			if block.containsTile(x, y) {
				r[x] = '#'
			}
		}
		rows[y] = string(r)
	}
	var tiles []Vec2
	for _, c := range rectTiles(rect) {
		if !block.containsTile(floor(c.X), floor(c.Y)) {
			tiles = append(tiles, c)
		}
	}
	z, _ := newRegionZone(t, seed, Region{ID: "r", Tiles: tiles, Rects: []Rect{rect}, Nest: tiles[0],
		Count: count, MinLevel: 1, MaxLevel: 2, Kind: testKind})
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	v := joinNow(t, z, 1, 5.5, 5.5)
	return z, z.regions[0], v
}

// With nobody around, monsters walk at half speed and only ever stand on
// region tiles, walls inside the region included (spec 4.2).
func TestWanderStaysInRegion(t *testing.T) {
	z, r, v := wanderRegion(t, 3, Rect{X: 11, Y: 1, W: 9, H: 18}, Rect{X: 14, Y: 8, W: 3, H: 3}, 8)
	step := wanderSpeed * testKind.Speed * 4 / 10
	start := map[uint64]Vec2{}
	prev := map[uint64]Vec2{}
	for id, m := range z.monsters {
		start[id], prev[id] = m.pos, m.pos
	}
	moved, broadcast := map[uint64]bool{}, map[uint64]bool{}
	now := time.Now()
	for i := 0; i < 2000; i++ {
		z.advance(now)
		for _, mv := range lastTick(v.take()).GetMoves() {
			broadcast[mv.Id] = true
		}
		for id, m := range z.monsters {
			if !r.hasTile(floor(m.pos.X), floor(m.pos.Y)) {
				t.Fatalf("tick %d: monster %d at %v is off the region", i, id, m.pos)
			}
			if d := dist(m.pos, prev[id]); d > step+1e-4 {
				t.Fatalf("tick %d: monster %d moved %.4f tiles, faster than the wander step %.4f", i, id, d, step)
			}
			if m.state != wander {
				t.Fatalf("tick %d: monster %d state %v with nobody around", i, id, m.state)
			}
			prev[id] = m.pos
			if m.pos != start[id] {
				moved[id] = true
			}
		}
	}
	for id := range start {
		if !moved[id] || !broadcast[id] {
			t.Fatalf("monster %d moved=%v broadcast=%v, want both", id, moved[id], broadcast[id])
		}
	}
}

// lRegion is r1 of kumotori_1 on an open grid: rects {12,1,20,19} and
// {1,12,11,8} round the safe zone {1,1,10,10}, without the safe zone and its
// 1-tile margin.
func lRegion(t *testing.T, seed uint64, count int) (*Zone, *regionState) {
	t.Helper()
	rects := []Rect{{X: 12, Y: 1, W: 20, H: 19}, {X: 1, Y: 12, W: 11, H: 8}}
	margin := Rect{X: 0, Y: 0, W: 12, H: 12}
	var tiles []Vec2
	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			if margin.containsTile(x, y) || !rects[0].containsTile(x, y) && !rects[1].containsTile(x, y) {
				continue
			}
			tiles = append(tiles, Vec2{float32(x) + 0.5, float32(y) + 0.5})
		}
	}
	z, _ := newRegionZone(t, seed, Region{ID: "r1", Tiles: tiles, Rects: rects, Nest: Vec2{22.5, 10.5},
		Count: count, MinLevel: 1, MaxLevel: 2, Kind: testKind})
	z.area.Safe = []Rect{{X: 1, Y: 1, W: 10, H: 10}}
	z.mgrid = z.area.monsterGrid()
	return z, z.regions[0]
}

// Round the inside corner of an L-shaped region, wanderers keep to its tiles
// instead of cutting across the safe zone's margin.
func TestWanderStaysInLRegion(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		z, r := lRegion(t, seed, 8)
		now := time.Now()
		for i := 0; i < 2500; i++ {
			z.advance(now)
			for id, m := range z.monsters {
				if !r.hasTile(floor(m.pos.X), floor(m.pos.Y)) {
					t.Fatalf("seed %d tick %d: monster %d at %v is off the region (walking to %v)", seed, i, id, m.pos, m.wanderTo)
				}
			}
		}
	}
}

// segmentOnTiles reports whether every point of the segment a-b lies on one
// of r's tiles, sampled every 0.01 tiles.
func segmentOnTiles(r *regionState, a, b Vec2) bool {
	n := int(dist(a, b)/0.01) + 1
	for i := 0; i <= n; i++ {
		f := float32(i) / float32(n)
		p := Vec2{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f}
		if !r.hasTile(floor(p.X), floor(p.Y)) {
			return false
		}
	}
	return true
}

// Wander targets are other region tiles at most 6 tiles away, reached by a
// straight line over region tiles only; some lie beyond 5 tiles.
func TestWanderTargets(t *testing.T) {
	t.Run("inside corner", func(t *testing.T) {
		z, r := lRegion(t, 1, 1)
		m := &monster{entity: entity{pos: Vec2{12.5, 9.5}}, region: r}
		for i := 0; i < 1000; i++ {
			to, ok := z.wanderTarget(m)
			if !ok || to == m.pos || !r.hasTile(floor(to.X), floor(to.Y)) || dist(to, m.pos) > wanderReach {
				t.Fatalf("target %v ok=%v, want another region tile within %d", to, ok, wanderReach)
			}
			if !segmentOnTiles(r, m.pos, to) {
				t.Fatalf("target %v: the way there from %v leaves the region", to, m.pos)
			}
		}
	})
	t.Run("reach", func(t *testing.T) {
		open := Rect{X: 10, Y: 10, W: 40, H: 40}
		z, _ := newRegionZone(t, 1, Region{ID: "open", Tiles: rectTiles(open), Rects: []Rect{open}, Nest: Vec2{30.5, 30.5},
			Count: 1, MinLevel: 1, MaxLevel: 1, Kind: testKind})
		m := &monster{entity: entity{pos: Vec2{30.5, 30.5}}, region: z.regions[0]}
		beyond5 := 0
		for i := 0; i < 1000; i++ {
			to, ok := z.wanderTarget(m)
			d := dist(to, m.pos)
			if !ok || d > wanderReach || to == m.pos {
				t.Fatalf("target %v ok=%v at %.2f tiles, want within %d", to, ok, d, wanderReach)
			}
			if d > 5 {
				beyond5++
			}
		}
		if beyond5 == 0 {
			t.Fatal("no target beyond 5 tiles: the reach is shorter than 6")
		}
	})
}

// A monster that sees someone to shoot while it walks stops at once, and
// viewers are told it has stopped.
func TestStartShootStopsWalkingMonster(t *testing.T) {
	open := Rect{X: 20, Y: 20, W: 20, H: 20}
	z, _ := newRegionZone(t, 1, Region{ID: "open", Tiles: rectTiles(open), Rects: []Rect{open}, Nest: Vec2{30.5, 30.5},
		Count: 1, MinLevel: 1, MaxLevel: 1, Kind: testKind})
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	m.pos = Vec2{25.5, 25.5}
	z.updateCell(&m.entity)
	a := joinNow(t, z, 1, 25.5, 32.5) // in view, out of range
	z.advance(time.Now())
	a.take()
	m.wanderTo, m.dir, m.nextWander = Vec2{30.5, 25.5}, Vec2{1, 0}, 0 // mid-walk east
	teleport(z, 1, Vec2{25.5, 29.5})
	z.advance(time.Now())
	if m.state != shoot || m.dir != (Vec2{}) {
		t.Fatalf("state %v dir %v, want shooting and stopped", m.state, m.dir)
	}
	if mv := moveFor(lastTick(a.take()), m.id); mv == nil || mv.DirX != 0 || mv.DirY != 0 {
		t.Fatalf("viewer move %v, want one with a zero direction", mv)
	}
}

// One monster in a big region gets well beyond 6 tiles from its spawn point
// over time, resting 2-5 s between walks.
func TestWanderReachesAcrossRegion(t *testing.T) {
	z, _, _ := wanderRegion(t, 5, Rect{X: 20, Y: 20, W: 30, H: 30}, Rect{X: 33, Y: 33, W: 2, H: 2}, 1)
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	start := m.pos
	far := 0.0
	walking, stoppedAt, walks := false, z.tickNo, 0
	now := time.Now()
	for i := 0; i < 6000 && (far <= 6 || walks < 10); i++ {
		z.advance(now)
		far = max(far, dist(m.pos, start))
		switch {
		case !walking && m.dir != (Vec2{}):
			if rest := z.tickNo - stoppedAt; walks > 0 && (rest < z.ticks(restMinSec) || rest > z.ticks(restMaxSec)) {
				t.Fatalf("tick %d: rested %d ticks, want %d-%d", z.tickNo, rest, z.ticks(restMinSec), z.ticks(restMaxSec))
			}
			walking = true
		case walking && m.dir == (Vec2{}):
			walking, stoppedAt = false, z.tickNo
			walks++
		}
	}
	if far <= 6 || walks < 10 {
		t.Fatalf("after %d walks the farthest from spawn is %.2f tiles, want beyond 6", walks, far)
	}
}

func TestMonsterAttacksOnInterval(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	v := joinNow(t, z, 2, 30.5, 36.5) // in view, out of detect range
	engage(m, 1)
	now := time.Now()
	var hitTicks []int
	for i := 1; i <= 31; i++ {
		z.advance(now)
		ha, hv := hitsIn(a.take()), hitsIn(v.take())
		if len(ha) != len(hv) {
			t.Fatalf("tick %d: target got %d hits, viewer %d", i, len(ha), len(hv))
		}
		for j, h := range ha {
			// Level-1 monster (attack 30) on a level-1 player (defense 10): 30*(1-10/30) = 20.
			want := 500 - 20*(len(hitTicks)+1)
			if h.AttackerId != m.id || h.TargetId != 1 || h.Damage != 20 || h.HpAfter != int32(want) {
				t.Fatalf("tick %d: hit %v, want %d hits 1 for 20 leaving %d", i, h, m.id, want)
			}
			if hv[j].String() != h.String() {
				t.Fatalf("tick %d: viewer got %v, target %v", i, hv[j], h)
			}
			hitTicks = append(hitTicks, i)
		}
	}
	if !slices.Equal(hitTicks, []int{1, 16, 31}) {
		t.Fatalf("hits on ticks %v, want 1, 16 and 31 (every 15 ticks)", hitTicks)
	}
	if hp := z.players[1].hp; hp != 440 {
		t.Fatalf("hp = %d, want 440", hp)
	}
	if m.pos != (Vec2{30.5, 30.5}) {
		t.Fatalf("a monster within 1.3 tiles must not move, at %v", m.pos)
	}
}

// The chase stops inside the attack range even when the range is not above
// the 1.3-tile chase stop, so float error cannot leave it chasing forever.
func TestMonsterHitsWithShortAttackRange(t *testing.T) {
	// Approaches from several angles: each rounds the stop point differently.
	starts := []Vec2{{33.5, 30.5}, {33.1, 32.3}, {32.7, 33.2}, {28.1, 32.9}, {31.9, 27.4}}
	for _, reach := range []float64{1.3, 1.2, 1.0} {
		for _, p := range starts {
			k := testKind
			k.AttackRange = reach
			z, m := newFightZoneOf(t, k, 1, Vec2{30.5, 30.5})
			joinNow(t, z, 1, p.X, p.Y)
			engage(m, 1)
			now := time.Now()
			hit := false
			for i := 0; i < 40 && !hit; i++ {
				z.advance(now)
				for _, h := range z.events.hits {
					if h.Ranged {
						t.Fatalf("attack range %v from %v: a chasing monster shot %v", reach, p, h)
					}
					hit = true
				}
			}
			if !hit {
				t.Fatalf("attack range %v from %v: never hit, %v at %.7f tiles", reach, p, m.state, dist(m.pos, z.players[1].pos))
			}
		}
	}
}

// A monster in reach but across a wall corner must not hit; it walks round
// the corner and hits once it has a clear line, as a player's slash needs.
// This is the 4x4 block at (45-48, 30-33) on kumotori_1 in region r4.
func TestMonsterNeedsLineOfSight(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{52.5, 40.5})
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		if y >= 30 && y <= 33 {
			copy(r[45:49], "####")
		}
		rows[y] = string(r)
	}
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	joinNow(t, z, 1, 49.3, 33.5)
	m.pos = Vec2{47.6, 34.4} // far enough back that it needs more than a tick to see round the corner
	z.updateCell(&m.entity)
	engage(m, 1)
	p := z.players[1]
	if d := distance(m.pos, p.pos); d > float32(testKind.AttackRange) || !lineBlocked(z.area.Grid, m.pos, p.pos) {
		t.Fatalf("setup: %.2f tiles apart, blocked %v; want in reach behind the corner", d, lineBlocked(z.area.Grid, m.pos, p.pos))
	}
	start := m.pos
	now := time.Now()
	hitAt := 0
	for i := 1; i <= 20 && hitAt == 0; i++ {
		z.advance(now)
		if len(z.events.hits) == 0 {
			continue
		}
		hitAt = i
		if lineBlocked(z.area.Grid, m.pos, p.pos) {
			t.Fatalf("tick %d: hit from %v through the wall", i, m.pos)
		}
	}
	if hitAt == 0 {
		t.Fatalf("never hit: %v at %v, %.2f tiles away", m.state, m.pos, distance(m.pos, p.pos))
	}
	if hitAt == 1 || m.pos == start {
		t.Fatalf("hit on tick %d from %v, want it to walk round the corner from %v first", hitAt, m.pos, start)
	}
	if m.state != attack || p.hp != 480 {
		t.Fatalf("state %v hp %d, want attacking with one hit landed", m.state, p.hp)
	}
}

func TestPlayerDiesAtZeroHP(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinHP(t, z, 1, 31.5, 30.5, 20)
	v := joinNow(t, z, 2, 30.5, 36.5)
	engage(m, 1)
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{0, 1}})
	z.advance(time.Now())
	p := z.players[1]
	if !p.dead || p.hp != 0 || p.dir != (Vec2{}) {
		t.Fatalf("hp %d dead %v dir %v, want dead at 0 HP, standing still", p.hp, p.dead, p.dir)
	}
	if want := z.tickNo + z.ticks(60); p.deadUntil != want {
		t.Fatalf("deadUntil %d, want %d", p.deadUntil, want)
	}
	ea, ev := a.take(), v.take()
	if hs := hitsIn(ea); len(hs) != 1 || hs[0].HpAfter != 0 || hs[0].AttackerId != m.id {
		t.Fatalf("hits %v, want one hit leaving 0 HP", hs)
	}
	if !slices.Equal(deathsIn(ea), []uint64{1}) || !slices.Equal(deathsIn(ev), []uint64{1}) {
		t.Fatalf("deaths: player %v viewer %v, want [1] for both", deathsIn(ea), deathsIn(ev))
	}

	pos := p.pos
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{1, 0}})
	now := time.Now()
	for i := 0; i < 20; i++ {
		z.advance(now)
		envs := a.take()
		if wt := lastTick(envs); wt.AckInputSeq != 2 {
			t.Fatalf("ack %d, want 2: a dead player's input is still acked", wt.AckInputSeq)
		}
		if hs := hitsIn(envs); len(hs) != 0 {
			t.Fatalf("a dead player was hit again: %v", hs)
		}
	}
	if p.pos != pos || p.dir != (Vec2{}) {
		t.Fatalf("dead player at %v dir %v, want still at %v", p.pos, p.dir, pos)
	}
	if m.target == 1 {
		t.Fatal("the monster must drop a dead target")
	}
}

func TestEventsOnlyForVisible(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinHP(t, z, 1, 31.5, 30.5, 20)
	f := joinNow(t, z, 2, 5.5, 5.5) // safe zone, three cells away
	engage(m, 1)
	z.advance(time.Now())
	if len(hitsIn(a.take())) != 1 {
		t.Fatal("the target must see its own hit")
	}
	ef := f.take()
	if hs, ds := hitsIn(ef), deathsIn(ef); len(hs) != 0 || len(ds) != 0 {
		t.Fatalf("a far viewer got hits %v deaths %v", hs, ds)
	}
}

// A viewer who sees only the attacker still gets the Hit; the client ignores
// ids it has no EntityEnter for.
func TestHitWithOnlyAttackerVisible(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{39.5, 30.5}) // AOI cell (3,3)
	joinNow(t, z, 1, 40.5, 30.5)                 // target in cell (4,3)
	v := joinNow(t, z, 2, 20.5, 30.5)            // cell (2,3): sees (3,3), not (4,3)
	engage(m, 1)
	z.advance(time.Now())
	envs := v.take()
	if slices.Contains(enters(envs), 1) || !slices.Contains(enters(envs), m.id) {
		t.Fatalf("viewer enters %v, want the monster but not the target", enters(envs))
	}
	if hs := hitsIn(envs); len(hs) != 1 || hs[0].AttackerId != m.id || hs[0].TargetId != 1 {
		t.Fatalf("viewer hits %v, want the monster's hit on 1", hs)
	}
}

// ______________________TEST CHASE_____________________

// boxRegion builds a zone whose one region is the 11x11 rect (25,25)-(35,35),
// with one monster of kind k moved to (30.5, 30.5).
func boxRegion(t *testing.T, k gamedata.Kind) (*Zone, *monster) {
	t.Helper()
	box := Rect{X: 25, Y: 25, W: 11, H: 11}
	z, _ := newRegionZone(t, 1, Region{ID: "box", Tiles: rectTiles(box), Rects: []Rect{box}, Nest: Vec2{30.5, 26.5},
		Count: 1, MinLevel: 1, MaxLevel: 1, Kind: k})
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	m.pos = Vec2{30.5, 30.5}
	z.updateCell(&m.entity)
	return z, m
}

func chasing(m *monster) bool { return m.state == chase || m.state == attack }

// A player who leads a chase more than region_margin tiles beyond the region's
// rects is let go once it has stayed there for margin_grace_sec. The monster keeps its HP, walks back onto
// the region and wanders (spec 4.4, 4.5).
func TestChaseGivesUpOutsideRegionMargin(t *testing.T) {
	k := testKind
	k.ChaseRange = 100 // only the margin can end this chase
	z, m := boxRegion(t, k)
	a := joinNow(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	if !chasing(m) || m.target != 1 {
		t.Fatalf("state %v target %d, want a chase", m.state, m.target)
	}
	hp := m.hp
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{1, 0}}) // east, faster than it walks
	edge := float32(25 + 11 + k.RegionMargin)       // x where the expanded rect ends
	grace := z.ticks(k.MarginGraceSec)
	p := z.players[1]
	now := time.Now()
	gaveUp := false
	var outAt uint32
	for i := 0; i < 100 && !gaveUp; i++ {
		z.advance(now)
		for _, c := range hpChangesIn(a.take()) {
			if c.Id == m.id {
				t.Fatalf("tick %d: HpChange %v for the monster, want its HP untouched", i, c)
			}
		}
		if p.pos.X >= edge && outAt == 0 {
			outAt = z.tickNo
		}
		switch {
		case outAt == 0 || z.tickNo < outAt+grace:
			if !chasing(m) || m.target != 1 {
				t.Fatalf("tick %d: player at x %.2f, out since tick %d, but state %v target %d", z.tickNo, p.pos.X, outAt, m.state, m.target)
			}
		default:
			if chasing(m) || m.target != 0 {
				t.Fatalf("tick %d: player beyond the margin for %d ticks, but state %v target %d", z.tickNo, grace, m.state, m.target)
			}
			gaveUp = true
		}
	}
	if !gaveUp {
		t.Fatal("the player never got beyond the margin")
	}
	// The player waits just past the margin, within sight of the monster all
	// the way back: its grudge must not pull it out again (spec 4.2).
	teleport(z, 1, Vec2{edge + 0.5, 30.5})
	z.move(moveCmd{id: 1, seq: 3, dir: Vec2{}})
	r := m.region
	home := false
	for i := 0; i < 100; i++ {
		z.advance(now)
		if chasing(m) {
			t.Fatalf("tick %d: chasing %d again at %v, the player beyond the margin at %v", i, m.target, m.pos, p.pos)
		}
		if m.state != back && r.hasTile(floor(m.pos.X), floor(m.pos.Y)) {
			home = true
		}
	}
	if !home || m.hp != hp {
		t.Fatalf("state %v at %v hp %d, want it back on the region with %d HP", m.state, m.pos, m.hp, hp)
	}
}

// A chase given up for the chase range is not restarted by the grudge: the
// player who led it stays in sight, and the monster goes back (spec 4.2).
func TestChaseRangeGiveUpIsNotRestarted(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home) // whole-map rect: only the chase range ends this chase
	joinNow(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{1, 0}})
	now := time.Now()
	for i := 0; i < 100 && chasing(m); i++ {
		z.advance(now)
	}
	if chasing(m) {
		t.Fatal("setup: the chase never ended")
	}
	z.move(moveCmd{id: 1, seq: 3, dir: Vec2{}}) // stop, within sight
	p := z.players[1]
	if d := distance(m.pos, p.pos); d > float32(testKind.DetectRange) {
		t.Fatalf("setup: player %.2f tiles away, want within sight", d)
	}
	far := 0.0
	for i := 0; i < 100; i++ {
		z.advance(now)
		far = max(far, dist(m.pos, home))
		if chasing(m) {
			t.Fatalf("tick %d: chasing %d again, %.1f tiles from home", i, m.target, dist(m.pos, home))
		}
	}
	if m.state == back || floor(m.pos.X) != floor(home.X) {
		t.Fatalf("state %v at %v, want it home", m.state, m.pos)
	}
}

// A target behind a wall cannot be reached: after returnStuckSec stuck at the
// wall, the monster turns on a player who hits it meanwhile, instead of
// pushing at the wall while that player slashes it freely (spec 4.4).
func TestUnreachableTargetIsDropped(t *testing.T) {
	rows := make([]string, 60)
	for y := range rows {
		rows[y] = strings.Repeat(".", 60)
	}
	rows[20] = strings.Repeat("#", 60)     // a full wall, as row 20 of kumotori without its gaps
	box := Rect{X: 10, Y: 5, W: 30, H: 15} // y 5..19; the margin reaches y 22
	z, _ := newRegionZone(t, 1, Region{ID: "box", Tiles: rectTiles(box), Rects: []Rect{box}, Nest: Vec2{25.5, 10.5},
		Count: 1, MinLevel: 2, MaxLevel: 2, Kind: testKind}) // level 2: 104 HP, so it outlives the wait
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	m.pos = Vec2{25.5, 17.5}
	z.updateCell(&m.entity)
	joinNow(t, z, 1, 25.5, 18.5)
	a := newBot(z, 1)
	a.act(Vec2{}, m.id) // A slashes, then gets behind the wall, inside the margin
	tickBots(z, a)
	teleport(z, 1, Vec2{25.5, 21.5})
	joinNow(t, z, 2, 27.5, 18.5)
	b := newBot(z, 2)
	for i := 0; i < 50 && b.melee == 0; i++ { // 5 s
		var s uint64
		if b.canSlash(m) {
			s = m.id
		}
		var dir Vec2
		if distance(b.p().pos, m.pos) > 1.8 {
			dir = unit(b.p().pos, m.pos)
		}
		b.act(dir, s)
		tickBots(z, a, b)
	}
	t.Logf("B landed %d slashes and took %d hits", b.slashes, b.melee)
	if b.melee == 0 {
		t.Fatalf("B slashed %d times unhit while the monster pushed at the wall: state %v target %d at %v", b.slashes, m.state, m.target, m.pos)
	}
	if m.target == 1 {
		t.Fatal("hit B but still targets A behind the wall")
	}
}

// With the margin out of the way, a chase ends once the monster is more than
// chase_range from where the chase started. It walks back at full speed
// without healing.
func TestChaseGivesUpBeyondChaseRange(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	a := joinNow(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	hp := m.hp
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{1, 0}})
	chaseStep, step := testKind.ChaseSpeed*4/10, testKind.Speed*4/10
	now := time.Now()
	far := 0.0
	for i := 0; i < 100 && chasing(m); i++ {
		z.advance(now)
		far = max(far, dist(m.pos, home))
	}
	if chasing(m) || m.target != 0 {
		t.Fatalf("state %v target %d, want the chase over", m.state, m.target)
	}
	if far <= testKind.ChaseRange || far > testKind.ChaseRange+chaseStep+1e-3 {
		t.Fatalf("farthest from the chase start %.3f, want just beyond the chase range %v", far, testKind.ChaseRange)
	}
	z.move(moveCmd{id: 1, seq: 3, dir: Vec2{}})
	teleport(z, 1, Vec2{55.5, 55.5})
	prev := m.pos
	for i := 0; i < 60 && m.state == back; i++ {
		z.advance(now)
		if d := dist(m.pos, prev); m.state == back && d < step-1e-3 {
			t.Fatalf("tick %d: walking back %.3f tiles a tick, want full speed %.3f", i, d, step)
		}
		prev = m.pos
	}
	if m.state != wander || floor(m.pos.X) != floor(home.X) || floor(m.pos.Y) != floor(home.Y) || m.hp != hp {
		t.Fatalf("state %v at %v hp %d, want wandering on its tile with %d HP", m.state, m.pos, m.hp, hp)
	}
	for _, c := range hpChangesIn(a.take()) {
		if c.Id == m.id {
			t.Fatalf("HpChange %v for the monster, want no heal", c)
		}
	}
}

// The chase range counts from where the chase starts, not from the spawn
// point: a monster that has wandered far from its spawn still chases.
func TestChaseStartsWhereItWasHit(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.pos = Vec2{30.5 + float32(testKind.ChaseRange) + 5, 30.5}
	z.updateCell(&m.entity)
	a := joinNow(t, z, 1, m.pos.X+1, m.pos.Y)
	attackNow(t, z, 1, 1, m.id)
	now := time.Now()
	melee := 0
	for i := 0; i < 30; i++ {
		z.advance(now)
		if !chasing(m) || m.target != 1 {
			t.Fatalf("tick %d: state %v target %d, want it chasing its attacker", i, m.state, m.target)
		}
		_, ms := rangedHits(a.take())
		melee += len(ms)
	}
	if melee == 0 {
		t.Fatal("no melee hit on the attacker")
	}
}

// A chasing monster is faster than a player: one who runs straight away at
// full speed after a hit is caught and hit within 2 s (spec 4.4).
func TestChaseOutrunsRetreatingPlayer(t *testing.T) {
	k := testKind
	k.ChaseRange, k.RegionMargin = 100, 100
	z, m := boxRegion(t, k)
	a := joinNow(t, z, 1, 32.4, 30.5) // 1.9 tiles: in slash range, out of its attack range
	attackNow(t, z, 1, 1, m.id)
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{1, 0}}) // straight away from it
	a.take()
	now := time.Now()
	for i := 1; i <= 20; i++ {
		z.advance(now)
		if _, melee := rangedHits(a.take()); len(melee) > 0 {
			return
		}
	}
	t.Fatalf("no melee hit within 2 s: monster at %v, player at %v", m.pos, z.players[1].pos)
}

// safeEdgeZone puts one monster at r1's safe edge, on the r1-like region of
// lRegion: the safe rect (1,1,10,10) ends at x = 11, and the region starts at
// x = 12, beyond the 1-tile margin.
func safeEdgeZone(t *testing.T) (*Zone, *monster) {
	t.Helper()
	z, _ := lRegion(t, 1, 1)
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	m.pos = Vec2{12.9, 5.5}
	z.updateCell(&m.entity)
	return z, m
}

// The review's safe-zone poke: step out, slash, step back in, rest inside
// until the slash is ready, repeat. The monster waits at the edge and hits
// the player as soon as it steps out (spec 4.4).
func TestSafeZonePokeGetsHit(t *testing.T) {
	z, m := safeEdgeZone(t)
	a := joinNow(t, z, 1, 11.3, 5.5) // one step outside the safe edge, 1.6 tiles from the monster
	p := z.players[1]
	seq := uint32(0)
	next := func() uint32 { seq++; return seq }
	now := time.Now()
	hits := 0
	for cycle := 0; cycle < 5 && hits == 0; cycle++ {
		if z.area.InSafe(p.pos) {
			t.Fatalf("cycle %d: the poke starts outside, player at %v", cycle, p.pos)
		}
		// Slash and step back in on the same tick.
		z.move(moveCmd{id: 1, seq: next(), dir: Vec2{-1, 0}})
		attackNow(t, z, 1, next(), m.id)
		z.move(moveCmd{id: 1, seq: next(), dir: Vec2{}})
		if !z.area.InSafe(p.pos) {
			t.Fatalf("cycle %d: the player should be back in the safe zone, at %v", cycle, p.pos)
		}
		for i := 0; i < 6; i++ { // wait inside for the slash cooldown
			z.advance(now)
		}
		// Step out for one tick.
		z.move(moveCmd{id: 1, seq: next(), dir: Vec2{1, 0}})
		z.advance(now)
		z.move(moveCmd{id: 1, seq: next(), dir: Vec2{}})
		for _, h := range hitsIn(a.take()) {
			if h.AttackerId == m.id && h.TargetId == 1 {
				hits++
			}
		}
	}
	if hits == 0 {
		t.Fatalf("five pokes and no hit back: state %v target %d at %v", m.state, m.target, m.pos)
	}
}

// A target that stays in a safe zone is kept for safe_wait_sec, during which
// the monster waits at the edge; then the chase ends, without healing.
func TestSafeZoneWaitExpires(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{13.5, 5.5})
	a := joinNow(t, z, 1, 12.5, 5.5)
	attackNow(t, z, 1, 1, m.id)
	hp := m.hp
	teleport(z, 1, Vec2{7.5, 5.5}) // inside the safe rect (0,0,10,10), out of its reach
	wait := int(z.ticks(testKind.SafeWaitSec))
	now := time.Now()
	for i := 0; i < wait; i++ {
		z.advance(now)
		if !chasing(m) || m.target != 1 {
			t.Fatalf("tick %d of the wait: state %v target %d, want it waiting for the target", i, m.state, m.target)
		}
		if z.area.InSafe(m.pos) {
			t.Fatalf("tick %d: the monster is in the safe zone at %v", i, m.pos)
		}
	}
	if m.pos.X > 10.31 {
		t.Fatalf("waited at %v, want at the safe edge (x 10.3)", m.pos)
	}
	z.advance(now)
	if chasing(m) || m.target != 0 {
		t.Fatalf("after %d ticks: state %v target %d, want the chase over", wait, m.state, m.target)
	}
	for i := 0; i < 60 && m.state != wander; i++ {
		z.advance(now)
	}
	if m.state != wander || m.hp != hp {
		t.Fatalf("state %v hp %d, want wandering with its %d HP", m.state, m.hp, hp)
	}
	for _, c := range hpChangesIn(a.take()) {
		if c.Id == m.id {
			t.Fatalf("HpChange %v for the monster, want no heal", c)
		}
	}
}

// Stepping out of the safe zone, even for one tick, starts the wait again.
func TestSafeZoneWaitResets(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{13.5, 5.5})
	joinNow(t, z, 1, 12.5, 5.5)
	attackNow(t, z, 1, 1, m.id)
	wait := int(z.ticks(testKind.SafeWaitSec))
	now := time.Now()
	teleport(z, 1, Vec2{7.5, 5.5})
	for i := 0; i < wait-10; i++ {
		z.advance(now)
	}
	teleport(z, 1, Vec2{14.5, 8.5}) // out for one tick
	z.advance(now)
	teleport(z, 1, Vec2{7.5, 5.5})
	for i := 0; i < wait; i++ {
		z.advance(now)
		if !chasing(m) || m.target != 1 {
			t.Fatalf("tick %d after going back in: state %v target %d, want the wait restarted", i, m.state, m.target)
		}
	}
	z.advance(now)
	if chasing(m) || m.target != 0 {
		t.Fatalf("state %v target %d, want the chase over %d ticks after going back in", m.state, m.target, wait)
	}
}

// ______________________TEST ANTI-KITE_____________________

// bot drives one player tick by tick and counts the hits it gives and takes.
type bot struct {
	z             *Zone
	id            uint64
	seq           uint32
	slashes       int
	melee, ranged int // hits taken
}

// newBot makes player id unkillable, so a run measures hits, not deaths.
func newBot(z *Zone, id uint64) *bot {
	z.players[id].hp, z.players[id].maxHP = 1<<30, 1<<30
	return &bot{z: z, id: id, seq: 100}
}

func (b *bot) p() *player { return b.z.players[b.id] }

// slashReady reports whether a slash sent now is off cooldown next tick.
func (b *bot) slashReady() bool {
	p := b.p()
	return p.lastAttack == 0 || b.z.tickNo+1-p.lastAttack >= b.z.ticks(combat.SlashCooldown.Seconds())
}

// canSlash reports whether a slash at m sent now would be accepted.
func (b *bot) canSlash(m *monster) bool {
	p := b.p()
	return !b.z.area.InSafe(p.pos) && b.slashReady() && distance(p.pos, m.pos) <= combat.SlashRange+attackRangeSlack &&
		!lineBlocked(b.z.area.Grid, p.pos, m.pos)
}

// act sets the bot's direction for the next tick and, if slash is set, slashes it.
func (b *bot) act(dir Vec2, slash uint64) {
	b.seq++
	b.z.move(moveCmd{id: b.id, seq: b.seq, dir: dir})
	if slash != 0 {
		b.seq++
		b.z.Attack(b.id, b.seq, slash)
	}
}

// tickBots runs one tick and counts its hits for the bots.
func tickBots(z *Zone, bots ...*bot) {
	z.advance(time.Now())
	for _, h := range z.events.hits {
		for _, b := range bots {
			if h.TargetId == b.id && h.Ranged {
				b.ranged++
			} else if h.TargetId == b.id {
				b.melee++
			}
			if h.AttackerId == b.id {
				b.slashes++
			}
		}
	}
}

func unit(a, b Vec2) Vec2 {
	d := distance(a, b)
	if d == 0 {
		return Vec2{}
	}
	return Vec2{(b.X - a.X) / d, (b.Y - a.Y) / d}
}

// A player who slashes near the region edge and runs straight out is hit:
// the monster reaches as far as a slash (2.0 tiles), and a target beyond the
// margin is chased on for margin_grace_sec (spec 4.4).
func TestMarginKiteGetsHit(t *testing.T) {
	for _, c := range []struct {
		name            string
		monster, player float32 // x on row 30.5; the box region ends at x 36, its margin at 39
	}{
		{"from 1.4 tiles outside the region", 35.5, 37.4},
		{"from beyond the margin", 37.6, 39.5},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m := boxRegion(t, testKind)
			m.pos = Vec2{c.monster, 30.5}
			z.updateCell(&m.entity)
			joinNow(t, z, 1, c.player, 30.5)
			b := newBot(z, 1)
			b.act(Vec2{1, 0}, m.id) // slash and run east
			tickBots(z, b)
			if b.slashes != 1 {
				t.Fatalf("setup: the slash did not land")
			}
			for i := 0; i < 30 && b.melee == 0; i++ {
				b.act(Vec2{1, 0}, 0)
				tickBots(z, b)
			}
			if b.melee == 0 {
				t.Fatalf("the player ran off unhit: state %v target %d, monster at %v, player at %v", m.state, m.target, m.pos, b.p().pos)
			}
		})
	}
}

// The review's corner poke at r1's safe-zone corner (11,11): the player waits
// inside, walks out, slashes the first tick it can and walks back to its
// start, again and again. Over about 2000 starts, directions and monster
// positions, no run may kill the monster without the player taking a hit.
func TestCornerPokeGetsHit(t *testing.T) {
	dirs := []Vec2{{0.4472, 0.8944}, {0.8944, 0.4472}, {0.7071, 0.7071}, {0, 1}, {1, 0}}
	tries, kills, free := 0, 0, 0
	for _, ms := range []Vec2{{11.3, 9.5}, {11.3, 6.5}, {9.5, 11.3}, {6.5, 11.3}} {
		for sx := float32(6.0); sx < 11; sx += 0.5 {
			for sy := float32(6.0); sy < 11; sy += 0.5 {
				for _, d := range dirs {
					z, _ := lRegion(t, 1, 1)
					var m *monster
					for _, mm := range z.monsters {
						m = mm
					}
					m.pos = ms
					z.updateCell(&m.entity)
					joinNow(t, z, 1, sx, sy)
					b := newBot(z, 1)
					if !z.area.InSafe(b.p().pos) {
						continue
					}
					start := Vec2{sx, sy}
					engage(m, 1)
					m.attackers[1] = 1
					tries++
					out := true
					for i := 0; i < 3000 && !m.dead && b.melee == 0; i++ {
						p := b.p()
						var dir Vec2
						var s uint64
						switch {
						case !z.area.InSafe(p.pos) && b.canSlash(m):
							s, dir, out = m.id, Vec2{-d.X, -d.Y}, false
						case !out && distance(p.pos, start) > 0.21:
							dir = unit(p.pos, start)
						case !out:
							if b.slashReady() {
								out, dir = true, d
							}
						default:
							dir = d
						}
						b.act(dir, s)
						tickBots(z, b)
					}
					if m.dead {
						kills++
						if b.melee+b.ranged == 0 {
							free++
							if free <= 3 {
								t.Errorf("killed with no hit taken: monster at %v, start %v, dir %v, %d slashes", ms, start, d, b.slashes)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d corner pokes, %d kills, %d without a hit taken", tries, kills, free)
	if tries < 1000 {
		t.Fatalf("only %d tries", tries)
	}
}

// pokeRun is the review's poke from inside the safe edge: wait inside until
// ready says so, step out, slash on the first tick it can, step back in.
func pokeRun(t *testing.T, ready func(z *Zone, m *monster, b *bot) bool) (*monster, *bot) {
	t.Helper()
	z, m := safeEdgeZone(t)
	joinNow(t, z, 1, 10.9, 5.5)
	b := newBot(z, 1)
	in, out := Vec2{-1, 0}, Vec2{1, 0}
	waiting := true
	for i := 0; i < 3000 && !m.dead; i++ {
		inSafe := z.area.InSafe(b.p().pos)
		switch {
		case !inSafe && b.canSlash(m):
			b.act(in, m.id)
			waiting = true
		case !inSafe:
			b.act(in, 0)
			waiting = true
		case waiting && b.slashReady() && ready(z, m, b):
			b.act(out, 0)
			waiting = false
		default:
			b.act(Vec2{}, 0)
		}
		tickBots(z, b)
	}
	return m, b
}

// A player who hides until the monster gives up waiting, then pokes again,
// is chased and hit at once: the monster bears a grudge (spec 4.2).
func TestWaitOutPokeGetsHit(t *testing.T) {
	m, b := pokeRun(t, func(z *Zone, m *monster, b *bot) bool { return m.target == 0 })
	if b.slashes == 0 {
		t.Fatal("the player never slashed")
	}
	if b.melee == 0 {
		t.Fatalf("%d slashes (dead %v, %d HP) without the player taking a hit", b.slashes, m.dead, m.hp)
	}
}

// While a monster waits at the safe edge for A, B's slash turns it on B,
// who is hit (spec 4.4).
func TestBaitDoesNotProtectSecondPlayer(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{13.5, 5.5})
	joinNow(t, z, 1, 11.6, 5.5)
	a := newBot(z, 1)
	a.act(Vec2{-1, 0}, m.id) // A slashes and runs into the safe rect (0,0,10,10)
	tickBots(z, a)
	for i := 0; i < 10; i++ {
		a.act(Vec2{-1, 0}, 0)
		tickBots(z, a)
	}
	if !z.area.InSafe(a.p().pos) || m.target != 1 || !chasing(m) {
		t.Fatalf("setup: A at %v, target %d state %v, want the monster waiting for A in the safe zone", a.p().pos, m.target, m.state)
	}
	a.act(Vec2{}, 0)
	joinNow(t, z, 2, m.pos.X+1.2, m.pos.Y+1.2)
	b := newBot(z, 2)
	b.act(Vec2{}, m.id)
	tickBots(z, a, b)
	if b.slashes != 1 || m.target != 2 || !chasing(m) {
		t.Fatalf("B slashes %d, target %d state %v, want the monster on B", b.slashes, m.target, m.state)
	}
	for i := 0; i < 20 && b.melee == 0; i++ {
		b.act(Vec2{}, 0)
		tickBots(z, a, b)
	}
	if b.melee == 0 {
		t.Fatal("B was never hit")
	}
}

// A grudge lasts grudge_sec from the player's last hit: within it, the player
// stepping into sight is chased at once; after it, the player is shot again
// like any passer-by (spec 4.2).
func TestGrudgeExpires(t *testing.T) {
	for _, c := range []struct {
		name  string
		after float64 // seconds from the hit to stepping out
		chase bool
	}{{"within the grudge", 7, true}, {"after the grudge", 10.1, false}} {
		t.Run(c.name, func(t *testing.T) {
			z, m := newFightZone(t, 1, Vec2{13.5, 5.5})
			joinNow(t, z, 1, 12.5, 5.5)
			attackNow(t, z, 1, 1, m.id)
			hitAt := z.tickNo
			teleport(z, 1, Vec2{5.5, 5.5}) // deep in the safe zone
			now := time.Now()
			for z.tickNo < hitAt+z.ticks(c.after)-1 {
				z.advance(now)
			}
			if m.state != wander {
				t.Fatalf("state %v before stepping out, want it wandering after the safe-zone wait", m.state)
			}
			teleport(z, 1, Vec2{m.pos.X + 1, m.pos.Y + 3.5}) // in sight, out of its reach
			z.advance(now)
			if c.chase && (!chasing(m) || m.target != 1) {
				t.Fatalf("state %v target %d, want an instant chase", m.state, m.target)
			}
			if !c.chase {
				if chasing(m) || m.state != shoot {
					t.Fatalf("state %v target %d, want it shooting, not chasing", m.state, m.target)
				}
				if hs := nextShot(t, z); len(hs) != 1 || hs[0].TargetId != 1 {
					t.Fatalf("shots %v, want player 1 shot", hs)
				}
			}
		})
	}
}

// A monster going back after the safe-zone wait still bears its grudge: a
// player who steps out in sight to poke it is chased and hit at once, before
// the slash and the step back in (spec 4.5).
func TestBackMonsterTurnsOnRecentAttacker(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{13.5, 5.5}) // its one region tile, well off the safe edge
	joinNow(t, z, 1, 12.0, 5.5)
	b := newBot(z, 1)
	b.act(Vec2{-1, 0}, m.id) // slash and head into the safe rect (0,0,10,10), to x 9.6
	tickBots(z, b)
	for i := 0; i < 5; i++ {
		b.act(Vec2{-1, 0}, 0)
		tickBots(z, b)
	}
	b.act(Vec2{}, 0)
	for i := 0; i < 100 && m.state != back; i++ {
		tickBots(z, b)
	}
	if m.state != back || !z.area.InSafe(b.p().pos) || m.region.hasTile(floor(m.pos.X), floor(m.pos.Y)) {
		t.Fatalf("setup: state %v at %v, player at %v; want the monster going back from off its region", m.state, m.pos, b.p().pos)
	}
	hits := b.melee
	b.act(Vec2{1, 0}, 0) // step out, in sight
	tickBots(z, b)
	if z.area.InSafe(b.p().pos) {
		t.Fatalf("setup: the player did not step out, at %v", b.p().pos)
	}
	b.act(Vec2{-1, 0}, m.id) // slash and step back in
	tickBots(z, b)
	if b.melee == hits {
		t.Fatalf("poked a monster going back without being hit: state %v target %d at %v", m.state, m.target, m.pos)
	}
}

// Once chasing, a monster keeps its target when someone else hits it, but
// remembers when each one last hit it.
func TestChaseKeepsTargetWhenOthersHit(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	joinNow(t, z, 1, 31.5, 30.5)
	joinNow(t, z, 2, 29.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	first := z.tickNo
	z.advance(time.Now())
	attackNow(t, z, 2, 1, m.id)
	if m.target != 1 || !chasing(m) {
		t.Fatalf("target %d state %v, want still on the first attacker", m.target, m.state)
	}
	if m.attackers[1] != first || m.attackers[2] != z.tickNo {
		t.Fatalf("attackers %v, want 1 at %d and 2 at %d", m.attackers, first, z.tickNo)
	}
	for i := 0; i < 20; i++ {
		z.advance(time.Now())
		if m.target != 1 {
			t.Fatalf("tick %d: target switched to %d", i, m.target)
		}
	}
}

// When the target is let go, the monster turns on the nearest player who hit
// it in the last 5 s, chasing from where it stands; with nobody that recent it
// gives up (spec 4.4).
func TestChaseFallsBackToRecentAttacker(t *testing.T) {
	k := testKind
	k.ChaseRange = 100
	for _, c := range []struct {
		name  string
		wait  int  // ticks between B's hit and the tick before A leaves
		bOut  bool // B has also left the margin
		fresh bool
	}{{"recent", 0, false, true}, {"exactly 5 s ago", 49, false, true}, {"5.1 s ago", 50, false, false},
		{"recent but beyond the margin", 0, true, false}} {
		t.Run(c.name, func(t *testing.T) {
			z, m := boxRegion(t, k)
			joinNow(t, z, 1, 31.5, 30.5)
			joinNow(t, z, 2, 29.5, 30.5)
			attackNow(t, z, 1, 1, m.id) // A first: the target
			attackNow(t, z, 2, 1, m.id)
			for i := 0; i < c.wait; i++ {
				z.advance(time.Now())
			}
			if m.target != 1 {
				t.Fatalf("target %d, want A", m.target)
			}
			z.damagePlayer(z.players[1], 0, 10000, false) // A dies: the monster lets it go
			if c.bOut {
				teleport(z, 2, Vec2{30.5, 50.5})
			}
			from := m.pos
			z.advance(time.Now())
			if c.fresh && (m.target != 2 || !chasing(m) || m.chaseFrom != from) {
				t.Fatalf("target %d state %v chaseFrom %v at %v, want B chased from here", m.target, m.state, m.chaseFrom, m.pos)
			}
			if !c.fresh && (m.target != 0 || chasing(m)) {
				t.Fatalf("target %d state %v, want the chase given up", m.target, m.state)
			}
		})
	}
}

// A player who died is forgotten as an attacker: after respawning near the
// monster within 5 s of its hit, it is not chased when the target is let go.
func TestDeadAttackerIsNotFallenBackOn(t *testing.T) {
	k := testKind
	k.ChaseRange = 100
	z, m := boxRegion(t, k)
	z.area.Respawn = Vec2{28.5, 30.5} // inside the region, outside any safe zone
	joinNow(t, z, 1, 31.5, 30.5)
	joinNow(t, z, 2, 29.5, 30.5)
	attackNow(t, z, 1, 1, m.id) // A first: the target
	attackNow(t, z, 2, 1, m.id)
	z.damagePlayer(z.players[2], 0, 10000, false)
	if !z.Respawn(2, 2) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if b := z.players[2]; b.dead || z.validTarget(2) == nil || m.target != 1 {
		t.Fatalf("B dead %v valid %v, target %d: want B back and valid, A still chased", b.dead, z.validTarget(2) != nil, m.target)
	}
	teleport(z, 1, Vec2{50.5, 30.5}) // A leaves the margin
	for i := 0; i < 40 && chasing(m) && m.target == 1; i++ {
		z.advance(time.Now())
	}
	if m.target != 0 || chasing(m) {
		t.Fatalf("target %d state %v, want the chase given up, not B chased", m.target, m.state)
	}
}

// A monster that lets its target go while standing on a region tile wanders
// again on the same tick, without moving (spec 4.5).
func TestGiveUpOnRegionTileWandersAtOnce(t *testing.T) {
	k := testKind
	k.ChaseRange = 100
	z, m := boxRegion(t, k)
	m.pos = Vec2{30.7, 30.3} // inside tile (30,30), off its centre
	z.updateCell(&m.entity)
	joinNow(t, z, 1, 31.7, 30.3)
	engage(m, 1)
	z.damagePlayer(z.players[1], 0, 10000, false) // the target dies: the monster lets it go
	z.advance(time.Now())
	if m.state != wander || m.target != 0 || m.pos != (Vec2{30.7, 30.3}) || m.dir != (Vec2{}) {
		t.Fatalf("state %v target %d at %v dir %v, want wandering where it stood", m.state, m.target, m.pos, m.dir)
	}
}

// A monster going back does not shoot, even at a player in its detect range;
// it shoots only once it wanders again on its region.
func TestBackMonsterDoesNotShoot(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	m.pos, m.state = Vec2{36.5, 30.5}, back
	z.updateCell(&m.entity)
	joinNow(t, z, 1, 33.5, 32.5) // within 5 tiles all the way back, with a clear line
	now := time.Now()
	wandered := false
	for i := 0; i < 60; i++ {
		z.advance(now)
		if m.state == wander {
			wandered = true
		}
		for _, h := range z.events.hits {
			if !h.Ranged {
				continue
			}
			if !wandered {
				t.Fatalf("tick %d: shot %v while going back (state %v at %v)", i, h, m.state, m.pos)
			}
			return
		}
	}
	t.Fatalf("wandered=%v, but never shot the player in range afterwards", wandered)
}

// A monster on its way back can be hit; the hit starts a new chase.
func TestBackMonsterIsAttackable(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.pos, m.state = Vec2{33.5, 30.5}, back
	z.updateCell(&m.entity)
	a := joinNow(t, z, 1, 34.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	envs := a.take()
	if attackRespAt(envs, 1) < 0 {
		t.Fatalf("slash on a monster going back rejected: %v", errorFor(envs, 1))
	}
	if !chasing(m) || m.target != 1 || m.chaseFrom != (Vec2{33.5, 30.5}) {
		t.Fatalf("state %v target %d chaseFrom %v, want a chase from where it was hit", m.state, m.target, m.chaseFrom)
	}
}

// A monster that cannot get back is put at its region's nest after 2 s stuck,
// with a snap and no heal (spec 4.5).
func TestBackStuckSnapsToNest(t *testing.T) {
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		if y >= 15 && y <= 45 {
			r[33] = '#' // between it and the region, too long to slide around
		}
		rows[y] = string(r)
	}
	var tiles []Vec2
	for y := 20; y <= 40; y++ {
		tiles = append(tiles, Vec2{30.5, float32(y) + 0.5})
	}
	nest := Vec2{30.5, 22.5}
	z, _ := newRegionZone(t, 1, Region{ID: "col", Tiles: tiles, Rects: []Rect{{X: 30, Y: 20, W: 1, H: 21}}, Nest: nest,
		Count: 1, MinLevel: 1, MaxLevel: 1, Kind: testKind})
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	m.pos, m.hp, m.state = Vec2{35.5, 30.5}, 10, back
	m.dmgBy[1], m.expPaid[1] = 46, 8
	z.updateCell(&m.entity)
	v := joinNow(t, z, 1, 41.5, 36.5) // in view, out of range
	now := time.Now()
	doneAt := 0
	var snap *pb.EntityMove
	for i := 1; i <= 60 && doneAt == 0; i++ {
		z.advance(now)
		envs := v.take()
		if mv := moveFor(lastTick(envs), m.id); mv != nil && mv.Snap {
			snap = mv
		}
		for _, c := range hpChangesIn(envs) {
			if c.Id == m.id {
				t.Fatalf("tick %d: HpChange %v, want no heal", i, c)
			}
		}
		if m.state == wander {
			doneAt = i
		}
	}
	// About 4 ticks to reach the wall, then 20 ticks (2 s) stuck.
	if doneAt < 20 || doneAt > 30 {
		t.Fatalf("wandering at tick %d, want snapped to the nest after 2 s stuck", doneAt)
	}
	if m.pos != nest || m.hp != 10 || m.snap {
		t.Fatalf("at %v hp %d snap %v, want at the nest, 10 HP and snap cleared after the tick", m.pos, m.hp, m.snap)
	}
	if snap == nil || snap.X != nest.X || snap.Y != nest.Y {
		t.Fatalf("viewer move %v, want a snap to the nest", snap)
	}
	if m.dmgBy[1] != 46 || m.expPaid[1] != 8 {
		t.Fatalf("dmgBy %v expPaid %v, want the EXP tracking kept", m.dmgBy, m.expPaid)
	}
}

// ______________________TEST COMBAT_____________________

// attackNow queues an attack by id on target with seq and runs one tick.
func attackNow(t *testing.T, z *Zone, id uint64, seq uint32, target uint64) {
	t.Helper()
	if !z.Attack(id, seq, target) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func errorFor(envs []*pb.Envelope, seq uint32) *pb.ErrorResp {
	for _, e := range envs {
		if er := e.GetError(); er != nil && e.Seq == seq {
			return er
		}
	}
	return nil
}

func attackRespAt(envs []*pb.Envelope, seq uint32) int {
	for i, e := range envs {
		if e.GetAttackResp() != nil && e.Seq == seq {
			return i
		}
	}
	return -1
}

func selfStatsIn(envs []*pb.Envelope) []*pb.SelfStats {
	var ss []*pb.SelfStats
	for _, e := range envs {
		if s := e.GetSelfStats(); s != nil {
			ss = append(ss, s)
		}
	}
	return ss
}

func TestAttackValidation(t *testing.T) {
	home := Vec2{30.5, 30.5}
	for _, c := range []struct {
		name   string
		setup  func(z *Zone, m *monster) uint64 // returns the target id
		player Vec2
		code   pb.ErrorCode
	}{
		{"dead attacker", func(z *Zone, m *monster) uint64 { z.players[1].dead = true; return m.id },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"attacker in a safe zone", func(z *Zone, m *monster) uint64 {
			m.pos = Vec2{11.0, 5.5}
			z.updateCell(&m.entity)
			return m.id
		}, Vec2{9.5, 5.5}, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"unknown id", func(z *Zone, m *monster) uint64 { return 999999 },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"a player id", func(z *Zone, m *monster) uint64 { joinNow(t, z, 2, 31.5, 31.5); return 2 },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"dead monster", func(z *Zone, m *monster) uint64 { m.dead = true; return m.id },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"2.1 tiles away", func(z *Zone, m *monster) uint64 { return m.id },
			Vec2{32.6, 30.5}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"a wall in between", func(z *Zone, m *monster) uint64 {
			rows := make([]string, 60)
			for y := range rows {
				r := []byte(strings.Repeat(".", 60))
				if y == 30 {
					r[29] = '#'
				}
				rows[y] = string(r)
			}
			z.area.Grid = gridFrom(rows...)
			z.mgrid = z.area.monsterGrid()
			return m.id
		}, Vec2{28.6, 30.5}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"a second attack after 7 ticks", func(z *Zone, m *monster) uint64 {
			attackNow(t, z, 1, 41, m.id)
			for i := 0; i < 6; i++ {
				z.advance(time.Now())
			}
			return m.id
		}, Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN},
		// Several failures at once: the first check in spec 5.1 order wins.
		{"dead in a safe zone with a bad target", func(z *Zone, m *monster) uint64 { z.players[1].dead = true; return 999999 },
			Vec2{5.5, 5.5}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"in a safe zone with a bad target", func(z *Zone, m *monster) uint64 { return 999999 },
			Vec2{5.5, 5.5}, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"far and on cooldown", func(z *Zone, m *monster) uint64 {
			attackNow(t, z, 1, 41, m.id)
			p := z.players[1]
			p.pos = Vec2{m.pos.X + 3, m.pos.Y}
			z.updateCell(&p.entity)
			return m.id
		}, Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m := newFightZone(t, 1, home)
			a := joinNow(t, z, 1, c.player.X, c.player.Y)
			target := c.setup(z, m)
			hp, last := m.hp, z.players[1].lastAttack
			a.take()
			attackNow(t, z, 1, 42, target)
			envs := a.take()
			er := errorFor(envs, 42)
			if er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v with seq 42", er, c.code)
			}
			if attackRespAt(envs, 42) >= 0 || m.hp != hp {
				t.Fatalf("a rejected attack must not land: hp %d, want %d", m.hp, hp)
			}
			if got := z.players[1].lastAttack; got != last {
				t.Fatalf("lastAttack %d, want %d: a rejected attack must not start the cooldown", got, last)
			}
		})
	}
}

func TestAttackHitsAndAggro(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	// Too short-sighted to notice the attacker by itself: only the hit can make it chase.
	m.region.Kind.DetectRange = 0.5
	a := joinNow(t, z, 1, 32.4, 30.5) // 1.9 tiles: in slash range with the slack, and so in its 2.0 attack range
	v := joinNow(t, z, 2, 30.5, 36.5) // a viewer
	a.take()
	v.take()
	attackNow(t, z, 1, 9, m.id)
	envs := a.take()
	ri, wi := attackRespAt(envs, 9), -1
	for i, e := range envs {
		if e.GetWorldTick() != nil {
			wi = i
		}
	}
	if ri < 0 || wi < 0 || ri > wi {
		t.Fatalf("AttackResp at %d, WorldTick at %d: the reply must come first", ri, wi)
	}
	// Level-1 slash (phys attack 20) on a level-1 monster (defense 8): 20*(1-8/28) = 14.
	if m.hp != 56-14 {
		t.Fatalf("monster hp %d, want 42", m.hp)
	}
	// The slash turns the monster on its attacker, who stands within its
	// reach, so it hits back on the same tick: 30*(1-10/30) = 20.
	want := []*pb.Hit{
		{AttackerId: 1, TargetId: m.id, Damage: 14, HpAfter: 42},
		{AttackerId: m.id, TargetId: 1, Damage: 20, HpAfter: 480},
	}
	for name, got := range map[string][]*pb.Hit{"attacker": hitsIn(envs), "viewer": hitsIn(v.take())} {
		if len(got) != len(want) || got[0].String() != want[0].String() || got[1].String() != want[1].String() {
			t.Fatalf("%s hits %v, want %v", name, got, want)
		}
	}
	if !chasing(m) || m.target != 1 {
		t.Fatalf("state %v target %d, want a wandering monster to chase its attacker", m.state, m.target)
	}
	if at, ok := m.attackers[1]; !ok || at != z.tickNo || m.chaseFrom != (Vec2{30.5, 30.5}) {
		t.Fatalf("attackers %v chaseFrom %v, want the hit's tick recorded and the chase starting where it stood", m.attackers, m.chaseFrom)
	}

	// The cooldown is 8 ticks: an attack 8 ticks after the last is accepted.
	for i := 0; i < 7; i++ {
		z.advance(time.Now())
	}
	a.take()
	attackNow(t, z, 1, 10, m.id)
	if envs := a.take(); attackRespAt(envs, 10) < 0 {
		t.Fatalf("attack 8 ticks later rejected: %v", errorFor(envs, 10))
	}
}

func TestRangeSlack(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 32.49, 30.5)
	attackNow(t, z, 1, 3, m.id)
	if envs := a.take(); attackRespAt(envs, 3) < 0 {
		t.Fatalf("an attack at 1.99 tiles must be accepted, got %v", errorFor(envs, 3))
	}
}

func TestNewerAttackReplacesPending(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	z.Attack(1, 5, m.id)
	attackNow(t, z, 1, 6, m.id)
	envs := a.take()
	if er := errorFor(envs, 5); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("the replaced attack got %v, want ON_COOLDOWN", er)
	}
	if attackRespAt(envs, 6) < 0 || m.hp != 56-14 {
		t.Fatalf("the newer attack must land once: hp %d", m.hp)
	}
}

func TestLineBlocked(t *testing.T) {
	g := gridFrom(
		"......",
		"..#...",
		"......",
	)
	for _, c := range []struct {
		a, b Vec2
		want bool
	}{
		{Vec2{0.5, 1.5}, Vec2{4.5, 1.5}, true},
		{Vec2{0.5, 0.5}, Vec2{4.5, 0.5}, false},
		{Vec2{1.5, 1.5}, Vec2{2.9, 1.5}, false}, // an endpoint's own tile does not count
		{Vec2{1.5, 1.5}, Vec2{1.5, 1.5}, false},
		{Vec2{0.5, 2.5}, Vec2{5.5, 0.5}, true}, // a shallow slope through the wall tile
		{Vec2{0.5, 0.5}, Vec2{1.9, 2.9}, false},
	} {
		if got := lineBlocked(g, c.a, c.b); got != c.want {
			t.Errorf("lineBlocked(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}

	// A diagonal through the corner between two walls is blocked, even with
	// one of them, but not with both side tiles open.
	for _, c := range []struct {
		rows []string
		want bool
	}{
		{[]string{".#.", "#..", "..."}, true},
		{[]string{".#.", "...", "..."}, true},
		{[]string{"...", "#..", "..."}, true},
		{[]string{"...", "...", "..."}, false},
	} {
		for _, ab := range [][2]Vec2{{{0.5, 0.5}, {1.5, 1.5}}, {{1.5, 1.5}, {0.5, 0.5}}, {{0.5, 0.5}, {2.5, 2.5}}} {
			if got := lineBlocked(gridFrom(c.rows...), ab[0], ab[1]); got != c.want {
				t.Errorf("grid %v: lineBlocked(%v, %v) = %v, want %v", c.rows, ab[0], ab[1], got, c.want)
			}
		}
	}
}

// Each slash pays its EXP share at once: the share of 14, 28, 42 and 56 of
// the monster's 56 HP (spec 3.4), with a SelfStats push after each.
func TestKillPaysExpPerHitAndRespawns(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	v := joinNow(t, z, 2, 30.5, 36.5)
	reg := z.regions[0]
	now := time.Now()
	var diedAt uint32
	for i := 0; i < 4; i++ {
		if i > 0 {
			for j := 0; j < 7; j++ {
				z.advance(now)
			}
		}
		a.take()
		attackNow(t, z, 1, uint32(10+i), m.id)
		envs := a.take()
		if attackRespAt(envs, uint32(10+i)) < 0 {
			t.Fatalf("slash %d rejected: %v", i+1, errorFor(envs, uint32(10+i)))
		}
		if m.hp != 56-14*(i+1) && !(i == 3 && m.hp == 0) {
			t.Fatalf("after slash %d hp %d", i+1, m.hp)
		}
		want := combat.ExpShare(testLevels[1].Exp, testLevels[1].HP, 14*(i+1))
		if p := z.players[1]; p.exp != want || p.level != 1 {
			t.Fatalf("after slash %d level %d exp %d, want level 1 with %d EXP", i+1, p.level, p.exp, want)
		}
		ss := selfStatsIn(envs)
		if len(ss) != 1 || ss[0].Exp != want || envs[len(envs)-1].GetSelfStats() == nil || envs[len(envs)-1].Seq != 0 {
			t.Fatalf("after slash %d SelfStats %v, want one push with %d EXP after the WorldTick", i+1, ss, want)
		}
		if saves := z.saver.(*fakeSaver).all(); len(saves) != 0 {
			t.Fatalf("after slash %d saves %+v, want none: EXP without a level-up waits for the periodic save", i+1, saves)
		}
		if i < 3 {
			continue
		}
		diedAt = z.tickNo
		if !m.dead || !slices.Equal(deathsIn(envs), []uint64{m.id}) || !slices.Equal(deathsIn(v.take()), []uint64{m.id}) {
			t.Fatalf("dead=%v deaths %v, want the monster dead and a Death for everyone", m.dead, deathsIn(envs))
		}
	}
	if z.regions[0] != reg || reg.present != 1 {
		t.Fatalf("a corpse still counts as present until removed, present %d", reg.present)
	}
	z.advance(now)
	if ss := selfStatsIn(a.take()); len(ss) != 0 {
		t.Fatalf("SelfStats pushed again without a change: %v", ss)
	}

	old := m.id
	var leftAt, newAt uint32
	var newID uint64
	for z.tickNo < diedAt+100 && newID == 0 {
		z.advance(now)
		envs := v.take()
		if slices.Contains(leaves(envs), old) {
			leftAt = z.tickNo
		}
		for _, id := range enters(envs) {
			if id != old && id != 1 {
				newID, newAt = id, z.tickNo
			}
		}
	}
	if leftAt != diedAt+10 {
		t.Fatalf("EntityLeave at tick %d, want %d (1 s after death)", leftAt, diedAt+10)
	}
	if _, still := z.monsters[old]; still {
		t.Fatal("the corpse must be removed from the zone")
	}
	if newAt != diedAt+80 || newID == 0 {
		t.Fatalf("new monster %d at tick %d, want a new id at %d (8 s after death)", newID, newAt, diedAt+80)
	}
	if nm := z.monsters[newID]; nm == nil || nm.hp != 56 || nm.dead || reg.present != 1 || len(reg.pending) != 0 {
		t.Fatalf("respawned monster %+v present %d pending %v, want a full one", nm, reg.present, reg.pending)
	}
}

// ______________________TEST PROGRESSION_____________________

// A hit's EXP share can level a player up mid-fight, with no kill.
func TestLevelUp(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 31.5, 30.5)
	share := combat.ExpShare(testLevels[1].Exp, testLevels[1].HP, 14)
	in.Exp, in.HP, in.MP = testExp.ToNext(1)-share+1, 100, 20 // the slash's share passes level 1 by 1
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 30.5, 36.5)
	sv := z.saver.(*fakeSaver)
	m.nextAttack = 1 << 30 // it survives the slash; keep it from hitting back
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	if m.dead || m.hp != 42 {
		t.Fatalf("monster dead=%v hp %d, want it alive at 42", m.dead, m.hp)
	}
	if p.level != 2 || p.exp != 1 || p.freePoints != 10 {
		t.Fatalf("level %d exp %d free %d, want 2, 1, 10", p.level, p.exp, p.freePoints)
	}
	if p.maxHP != 560 || p.hp != 560 || p.mp != 120 || p.stats.MaxMP != 120 || p.stats.PhysAttack != 25 {
		t.Fatalf("hp %d/%d mp %d stats %+v, want full at level-2 stats", p.hp, p.maxHP, p.mp, p.stats)
	}
	envs := a.take()
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		var lv []*pb.LevelUp
		for _, wt := range ticksIn(es) {
			lv = append(lv, wt.LevelUps...)
		}
		if len(lv) != 1 || lv[0].Id != 1 || lv[0].Level != 2 {
			t.Fatalf("%s level-ups %v, want 1 to level 2", name, lv)
		}
		var hc *pb.HpChange
		for _, c := range hpChangesIn(es) {
			if c.Id == 1 {
				hc = c
			}
		}
		if hc == nil || hc.Hp != 560 || hc.MaxHp != 560 {
			t.Fatalf("%s HpChange %v, want 560/560", name, hc)
		}
	}
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Level != 2 || ss[0].FreePoints != 10 || ss[0].MaxHp != 560 {
		t.Fatalf("SelfStats %v, want level 2 with 10 free points", ss)
	}
	saves := sv.all()
	if len(saves) != 1 || saves[0].Level != 2 || saves[0].Exp != 1 || saves[0].FreePoints != 10 || saves[0].HP != 560 || saves[0].MP != 120 {
		t.Fatalf("saves %+v, want one immediate save at level 2", saves)
	}
}

// HpChange carries end-of-tick HP: a level-up heal at step 2 followed by a
// monster hit at step 4 must leave viewers with the real HP.
func TestLevelUpThenHitSameTick(t *testing.T) {
	home := Vec2{30.5, 30.5}
	area := Area{
		Grid:    openGrid(60, 60),
		Respawn: Vec2{5.5, 5.5},
		Regions: []Region{{ID: "r1", Tiles: []Vec2{home}, Rects: []Rect{{X: 0, Y: 0, W: 60, H: 60}}, Nest: home,
			Count: 2, MinLevel: 1, MaxLevel: 1, Kind: testKind}},
		Levels: testLevels,
		Exp:    testExp,
	}
	z := New(testConfig(10), area, &fakeSaver{}, quietLog())
	ids := slices.Sorted(maps.Keys(z.monsters))
	victim := z.monsters[ids[0]]
	in := info(1, 31.5, 30.5)
	in.Exp, in.HP = testExp.ToNext(1)-combat.ExpShare(testLevels[1].Exp, testLevels[1].HP, 14), 100 // the slash's share reaches level 2
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 30.5, 36.5)
	attackNow(t, z, 1, 1, victim.id)
	p := z.players[1]
	if p.level != 2 || p.hp == p.maxHP {
		t.Fatalf("level %d hp %d/%d, want a level-up and then a hit in the same tick", p.level, p.hp, p.maxHP)
	}
	for name, envs := range map[string][]*pb.Envelope{"player": a.take(), "viewer": v.take()} {
		// A client applies hits, then hp_changes.
		hp := int32(-1)
		for _, wt := range ticksIn(envs) {
			for _, h := range wt.Hits {
				if h.TargetId == 1 {
					hp = h.HpAfter
				}
			}
			for _, c := range wt.HpChanges {
				if c.Id == 1 {
					hp = c.Hp
					if c.MaxHp != int32(p.maxHP) {
						t.Fatalf("%s: HpChange max %d, want %d", name, c.MaxHp, p.maxHP)
					}
				}
			}
		}
		if hp != int32(p.hp) {
			t.Fatalf("%s sees HP %d, real HP %d", name, hp, p.hp)
		}
	}
}

// Two players on one monster each earn the share of their own damage, the
// one who does not land the kill included, and together never more than it
// is worth.
func TestExpSharedByDamage(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	joinNow(t, z, 1, 31.5, 30.5) // slashes for 14
	in := info(2, 29.5, 30.5)
	in.Points = combat.Points{Str: 5} // phys attack 30: slashes for 21
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p1, p2 := z.players[1], z.players[2]
	total, hp := testLevels[1].Exp, testLevels[1].HP
	now := time.Now()
	// Tick 1: 1 hits for 14 (42 left), then 2 for 21 (21 left).
	z.Attack(1, 1, m.id)
	attackNow(t, z, 2, 1, m.id)
	if want1, want2 := combat.ExpShare(total, hp, 14), combat.ExpShare(total, hp, 21); m.hp != 21 || p1.exp != want1 || p2.exp != want2 {
		t.Fatalf("hp %d exp %d/%d, want 21 and %d/%d", m.hp, p1.exp, p2.exp, want1, want2)
	}
	// 1 hits twice more: 14 (7 left) and the kill, which counts only 7.
	for seq := uint32(2); seq <= 3; seq++ {
		for range 7 {
			z.advance(now)
		}
		attackNow(t, z, 1, seq, m.id)
	}
	if !m.dead {
		t.Fatalf("monster hp %d, want it dead", m.hp)
	}
	// 1 counted 35 of 56 HP, 2 counted 21.
	if want1, want2 := combat.ExpShare(total, hp, 35), combat.ExpShare(total, hp, 21); p1.exp != want1 || p2.exp != want2 || want2 == 0 {
		t.Fatalf("exp %d/%d, want %d for the killer and %d > 0 for the other", p1.exp, p2.exp, want1, want2)
	}
	if paid := p1.exp + p2.exp; paid > total {
		t.Fatalf("paid %d in all, more than the monster's %d", paid, total)
	}
}

// Damage past the monster's remaining HP earns nothing, though the Hit still
// shows all of it.
func TestOverkillEarnsNoExp(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	m.hp = 9
	total, hp := testLevels[1].Exp, testLevels[1].HP
	if combat.ExpShare(total, hp, 9) >= combat.ExpShare(total, hp, 14) {
		t.Fatal("setup: 9 and 14 damage must earn different shares")
	}
	a.take()
	attackNow(t, z, 1, 1, m.id)
	hs := hitsIn(a.take())
	if len(hs) != 1 || hs[0].Damage != 14 || hs[0].HpAfter != 0 || !m.dead {
		t.Fatalf("hits %v dead %v, want a 14-damage kill", hs, m.dead)
	}
	// Only the 9 HP left count, not the 14 dealt.
	if p, want := z.players[1], combat.ExpShare(total, hp, 9); p.exp != want || m.dmgBy[1] != 9 {
		t.Fatalf("exp %d counted %d, want %d EXP for 9 damage", p.exp, m.dmgBy[1], want)
	}
}

// A monster that gives up a chase keeps its HP and its damage tracking, so
// the player who comes back and kills it is paid the rest of its EXP: no
// more, no less (spec 4.5).
func TestGiveUpKeepsExpTracking(t *testing.T) {
	home := Vec2{13.5, 5.5}
	z, m := newFightZone(t, 1, home)
	joinNow(t, z, 1, 12.5, 5.5)
	p := z.players[1]
	attackNow(t, z, 1, 1, m.id)
	first := combat.ExpShare(testLevels[1].Exp, testLevels[1].HP, 14)
	if p.exp != first || m.dmgBy[1] != 14 || m.expPaid[1] != first || !chasing(m) {
		t.Fatalf("exp %d dmgBy %v expPaid %v state %v, want %d, 14, %d and a chase", p.exp, m.dmgBy, m.expPaid, m.state, first, first)
	}
	teleport(z, 1, Vec2{8.5, 5.5}) // into the safe zone: the chase ends after the wait
	now := time.Now()
	for i := 0; i < 100 && m.state != wander; i++ {
		z.advance(now)
	}
	if m.state != wander || m.hp != 42 || m.dmgBy[1] != 14 || m.expPaid[1] != first {
		t.Fatalf("state %v hp %d dmgBy %v expPaid %v, want wandering with 42 HP and the tracking kept", m.state, m.hp, m.dmgBy, m.expPaid)
	}
	teleport(z, 1, Vec2{m.pos.X - 1, m.pos.Y})
	for seq := uint32(2); !m.dead && seq < 10; seq++ {
		for i := 0; i < 8; i++ {
			z.advance(now)
		}
		attackNow(t, z, 1, seq, m.id)
	}
	if !m.dead || p.exp != testLevels[1].Exp {
		t.Fatalf("dead %v exp %d, want the kill to bring the total to the monster's %d EXP", m.dead, p.exp, testLevels[1].Exp)
	}
}

// A player at the level cap gains nothing and gets no SelfStats push.
func TestNoExpAtMaxLevel(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 31.5, 30.5)
	in.Level = combat.MaxLevel
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	a.take()
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	if p.level != combat.MaxLevel || p.exp != 0 {
		t.Fatalf("level %d exp %d, want the cap with 0 EXP", p.level, p.exp)
	}
	if ss := selfStatsIn(a.take()); len(ss) != 0 {
		t.Fatalf("SelfStats %v, want none", ss)
	}
	if n := len(z.saver.(*fakeSaver).all()); n != 0 {
		t.Fatalf("%d saves, want none", n)
	}
}

// Only a player within ExpLevelWindow levels of the monster earns EXP from a
// hit. A hit outside the window still lands but pays nothing and is not
// counted towards the monster's EXP shares (phase 2 spec 2.4).
func TestExpOnlyWithinLevelWindow(t *testing.T) {
	for _, c := range []struct {
		level int
		want  int64
	}{{12, 0}, {11, testLevels[1].Exp}} {
		z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
		in := info(1, 31.5, 30.5)
		in.Level = c.level
		a := &fakeOut{}
		if err := z.join(in, a); err != nil {
			t.Fatal(err)
		}
		a.take()
		attackNow(t, z, 1, 1, m.id)
		p := z.players[1]
		if len(hitsIn(a.take())) != 1 || m.hp == m.maxHP {
			t.Fatalf("level %d: the slash must land (monster hp %d/%d)", c.level, m.hp, m.maxHP)
		}
		if p.level != c.level || p.exp != c.want {
			t.Fatalf("level %d: now level %d exp %d, want exp %d", c.level, p.level, p.exp, c.want)
		}
		if _, counted := m.dmgBy[1]; counted != (c.want > 0) {
			t.Fatalf("level %d: dmgBy %v, want an entry only when EXP is paid", c.level, m.dmgBy)
		}
	}
}

func TestHpChangeOncePerEntityPerTick(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 30.5, 36.5)
	z.advance(time.Now())
	a.take()
	z.hpChanged(&m.entity)
	m.hp = 9
	z.hpChanged(&m.entity)
	wt := &pb.WorldTick{}
	z.addEvents(z.players[1], wt)
	if len(wt.HpChanges) != 1 || wt.HpChanges[0].Hp != 9 {
		t.Fatalf("hp changes %v, want one with the latest HP 9", wt.HpChanges)
	}
}

func TestJoinDeadPlayerRespawns(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 40.5, 40.5)
	in.HP, in.MP = 0, 3
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	if p.pos != z.area.Respawn || p.cell != z.cellOf(z.area.Respawn) || p.dead {
		t.Fatalf("at %v cell %v dead %v, want alive at the respawn point %v", p.pos, p.cell, p.dead, z.area.Respawn)
	}
	if p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("hp %d/%d mp %d/%d, want full", p.hp, p.maxHP, p.mp, p.stats.MaxMP)
	}
	r := out.take()[0].GetEnterWorldResp()
	if r.X != z.area.Respawn.X || r.Y != z.area.Respawn.Y {
		t.Fatalf("EnterWorldResp at %v,%v, want the respawn point", r.X, r.Y)
	}
}

func TestJoinClampsHP(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	for _, c := range []struct {
		hp, mp         int
		wantHP, wantMP int
	}{{-1, -1, 500, 100}, {9999, 9999, 500, 100}, {300, 40, 300, 40}} {
		in := info(1, 5.5, 5.5)
		in.HP, in.MP = c.hp, c.mp
		if err := z.join(in, &fakeOut{}); err != nil {
			t.Fatal(err)
		}
		p := z.players[1]
		if p.hp != c.wantHP || p.mp != c.wantMP {
			t.Fatalf("join with hp %d mp %d: got %d %d, want %d %d", c.hp, c.mp, p.hp, p.mp, c.wantHP, c.wantMP)
		}
		<-z.leave(1)
	}
}

func TestEnterWorldRespHasStats(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	in := info(1, 5.5, 5.5)
	in.Class = pb.CharClass_CHAR_CLASS_THUY
	in.Level, in.Exp, in.FreePoints, in.HP = 3, 50, 4, 200
	in.Points = combat.Points{Str: 1, Agi: 2, Vit: 3, Int: 4}
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	got := out.take()[0].GetEnterWorldResp().GetStats()
	s := combat.Compute(true, 3, in.Points)
	want := &pb.SelfStats{Level: 3, Exp: 50, ExpNext: testExp.ToNext(3), FreePoints: 4,
		Strength: 1, Agility: 2, Vitality: 3, Intellect: 4, Hp: 200, MaxHp: int32(s.MaxHP), Mp: int32(s.MaxMP), MaxMp: int32(s.MaxMP),
		PhysAttack: int32(s.PhysAttack), MagicAttack: int32(s.MagicAttack), PhysDefense: int32(s.PhysDefense), MagicDefense: int32(s.MagicDefense),
		Accuracy: int32(s.Accuracy), Evasion: int32(s.Evasion), CritRateBp: int32(s.CritRateBP)}
	if got.String() != want.String() {
		t.Fatalf("stats %v, want %v", got, want)
	}
	if s.MaxHP != 500+50*2+16*3 {
		t.Fatalf("magic class max HP %d, want the magic growth", s.MaxHP)
	}
}

func TestStateSavesProgress(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	in := info(1, 5.5, 5.5)
	in.Points, in.FreePoints = combat.Points{Str: 1}, 9
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	p.points, p.freePoints, p.hp, p.mp, p.exp = combat.Points{Str: 2, Agi: 3, Vit: 4, Int: 5}, 1, 77, 33, 88
	t0 := time.Now()
	z.advance(t0)
	z.advance(t0.Add(61 * time.Second))
	saves := sv.all()
	want := store.CharState{ID: 100, MapID: "m1", X: 5.5, Y: 5.5, Level: 1, Exp: 88,
		Points: combat.Points{Str: 2, Agi: 3, Vit: 4, Int: 5}, FreePoints: 1, HP: 77, MP: 33}
	if len(saves) != 1 || saves[0] != want {
		t.Fatalf("saves %+v, want %+v", saves, want)
	}
}

// ______________________TEST ALLOCATE_____________________

// allocateNow queues an allocation by id with seq and runs one tick.
func allocateNow(t *testing.T, z *Zone, id uint64, seq uint32, p combat.Points) {
	t.Helper()
	if !z.Allocate(id, seq, p) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func allocateRespFor(envs []*pb.Envelope, seq uint32) *pb.AllocatePointsResp {
	for _, e := range envs {
		if r := e.GetAllocatePointsResp(); r != nil && e.Seq == seq {
			return r
		}
	}
	return nil
}

func TestAllocate(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	in := info(1, 20.5, 20.5)
	in.Class = pb.CharClass_CHAR_CLASS_VIEM
	in.Level, in.FreePoints, in.HP, in.MP = 2, 10, 300, 50
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 22.5, 20.5)
	z.advance(time.Now())
	a.take()
	v.take()
	p := z.players[1]
	if p.maxHP != 560 || p.stats.MaxMP != 120 {
		t.Fatalf("level-2 Viêm max HP %d MP %d, want 560 and 120", p.maxHP, p.stats.MaxMP)
	}
	sv := z.saver.(*fakeSaver)

	allocateNow(t, z, 1, 9, combat.Points{Vit: 4, Int: 2})
	if p.maxHP != 640 || p.hp != 380 {
		t.Fatalf("hp %d/%d, want 380/640: +4 VIT adds 80 to both", p.hp, p.maxHP)
	}
	if p.stats.MaxMP != 130 || p.mp != 60 {
		t.Fatalf("mp %d/%d, want 60/130: +2 INT adds 10 to both", p.mp, p.stats.MaxMP)
	}
	if p.freePoints != 4 || p.points != (combat.Points{Vit: 4, Int: 2}) {
		t.Fatalf("free %d points %+v, want 4 free and VIT 4 INT 2", p.freePoints, p.points)
	}
	saves := sv.all()
	if len(saves) != 1 || saves[0].Points != p.points || saves[0].FreePoints != 4 || saves[0].HP != 380 || saves[0].MP != 60 {
		t.Fatalf("saves %+v, want one immediate save of the new points", saves)
	}
	envs := a.take()
	r := allocateRespFor(envs, 9)
	if r == nil || r.Stats.String() != z.selfStats(p).String() {
		t.Fatalf("AllocatePointsResp %v, want the new stats %v", r, z.selfStats(p))
	}
	if r.Stats.Vitality != 4 || r.Stats.FreePoints != 4 || r.Stats.MaxHp != 640 {
		t.Fatalf("reply stats %v, want VIT 4, 4 free, max HP 640", r.Stats)
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != 380 || cs[0].MaxHp != 640 {
			t.Fatalf("%s HpChanges %v, want 380/640 for 1", name, cs)
		}
	}

	// STR leaves max HP alone, so no HpChange.
	allocateNow(t, z, 1, 10, combat.Points{Str: 4})
	envs = a.take()
	if allocateRespFor(envs, 10) == nil || p.freePoints != 0 || p.stats.PhysAttack != 25+8 {
		t.Fatalf("STR allocation: free %d attack %d, errors %v", p.freePoints, p.stats.PhysAttack, errorFor(envs, 10))
	}
	if cs := hpChangesIn(envs); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none when max HP is unchanged", cs)
	}
}

func TestAllocateErrors(t *testing.T) {
	for _, c := range []struct {
		name string
		pts  combat.Points
		dead bool
		code pb.ErrorCode
	}{
		{"zero total", combat.Points{}, false, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"negative", combat.Points{Str: 3, Agi: -1}, false, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"too many", combat.Points{Str: 3, Vit: 3}, false, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS},
		{"dead", combat.Points{Str: 1}, true, pb.ErrorCode_ERROR_CODE_DEAD},
		{"dead with zero total", combat.Points{}, true, pb.ErrorCode_ERROR_CODE_DEAD},
		{"sum overflows", combat.Points{Str: math.MaxInt, Agi: 2}, false, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
			in := info(1, 20.5, 20.5)
			in.FreePoints = 5
			a := &fakeOut{}
			if err := z.join(in, a); err != nil {
				t.Fatal(err)
			}
			p := z.players[1]
			if c.dead {
				z.damagePlayer(p, 0, 10000, false)
			}
			before := p.stats
			allocateNow(t, z, 1, 3, c.pts)
			envs := a.take()
			if er := errorFor(envs, 3); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if allocateRespFor(envs, 3) != nil || p.freePoints != 5 || p.points != (combat.Points{}) || p.stats != before {
				t.Fatalf("a rejected allocation changed the player: free %d points %+v", p.freePoints, p.points)
			}
			if saves := z.saver.(*fakeSaver).all(); len(saves) != 0 {
				t.Fatalf("saves %+v, want none", saves)
			}
		})
	}
}

// ______________________TEST DEATH_____________________

func respawnRespFor(envs []*pb.Envelope, seq uint32) (*pb.RespawnResp, int) {
	for i, e := range envs {
		if r := e.GetRespawnResp(); r != nil && e.Seq == seq {
			return r, i
		}
	}
	return nil, -1
}

func indexOfTick(envs []*pb.Envelope) int {
	for i, e := range envs {
		if e.GetWorldTick() != nil {
			return i
		}
	}
	return -1
}

func TestRespawnRequest(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	a := joinNow(t, z, 1, 15.5, 5.5) // cell (1,0); the respawn point is in (0,0)
	v := joinNow(t, z, 2, 12.5, 5.5) // sees both cells
	p := z.players[1]
	p.mp = 7
	z.advance(time.Now())
	a.take()
	v.take()

	if !z.Respawn(1, 4) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if er := errorFor(a.take(), 4); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_NOT_DEAD {
		t.Fatalf("respawn while alive: error %v, want NOT_DEAD", er)
	}

	z.damagePlayer(p, 0, 10000, false)
	z.advance(time.Now())
	a.take()
	v.take()
	if !z.Respawn(1, 5) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if p.dead || p.pos != z.area.Respawn || p.cell != z.cellOf(z.area.Respawn) || p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("after respawn dead %v at %v cell %v hp %d/%d mp %d/%d, want alive and full at %v",
			p.dead, p.pos, p.cell, p.hp, p.maxHP, p.mp, p.stats.MaxMP, z.area.Respawn)
	}
	envs := a.take()
	r, ri := respawnRespFor(envs, 5)
	if r == nil || r.X != z.area.Respawn.X || r.Y != z.area.Respawn.Y {
		t.Fatalf("RespawnResp %v, want the respawn point %v", r, z.area.Respawn)
	}
	if ti := indexOfTick(envs); ti < ri {
		t.Fatalf("RespawnResp at %d after the WorldTick at %d", ri, ti)
	}
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Hp != int32(p.maxHP) || ss[0].Mp != int32(p.stats.MaxMP) {
		t.Fatalf("SelfStats %v, want one push with full HP and MP", ss)
	}
	ev := v.take()
	wt := lastTick(ev)
	mv := moveFor(wt, 1)
	if mv == nil || !mv.Snap || mv.X != z.area.Respawn.X || mv.Y != z.area.Respawn.Y {
		t.Fatalf("viewer move %v, want a snap to the respawn point", mv)
	}
	if cs := hpChangesIn(ev); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(p.maxHP) {
		t.Fatalf("viewer HpChanges %v, want full HP for 1", cs)
	}

	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{0, 1}})
	z.advance(time.Now())
	if p.pos == z.area.Respawn {
		t.Fatal("a respawned player must be able to move")
	}
	if mv := moveFor(lastTick(v.take()), 1); mv == nil || mv.Snap {
		t.Fatalf("viewer move %v, want an ordinary move after the snap", mv)
	}
}

func TestAutoRespawnAfter60s(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	a := joinNow(t, z, 1, 15.5, 5.5)
	v := joinNow(t, z, 2, 12.5, 5.5)
	p := z.players[1]
	z.advance(time.Now())
	z.damagePlayer(p, 0, 10000, false)
	diedAt := z.tickNo
	now := time.Now()
	for z.tickNo < diedAt+599 {
		z.advance(now)
	}
	if !p.dead {
		t.Fatalf("respawned at tick %d, before 60 s had passed", z.tickNo)
	}
	a.take()
	v.take()
	z.advance(now)
	if p.dead || p.pos != z.area.Respawn || p.hp != p.maxHP {
		t.Fatalf("tick %d: dead %v at %v hp %d, want respawned 60 s after death", z.tickNo, p.dead, p.pos, p.hp)
	}
	envs := a.take()
	for _, e := range envs {
		if e.GetRespawnResp() != nil {
			t.Fatal("an automatic respawn must not send RespawnResp")
		}
	}
	if len(selfStatsIn(envs)) != 1 {
		t.Fatal("want a SelfStats push after the automatic respawn")
	}
	if mv := moveFor(lastTick(v.take()), 1); mv == nil || !mv.Snap {
		t.Fatalf("viewer move %v, want a snap", mv)
	}
}

// A monster whose target is killed by its own hits turns on another recent
// attacker, or goes back to wandering when there is none (spec 4.4).
func TestMonsterDropsDeadTarget(t *testing.T) {
	for _, other := range []bool{true, false} {
		z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
		joinHP(t, z, 1, 31.5, 30.5, 20)
		engage(m, 1)
		if other {
			joinNow(t, z, 2, 30.5, 33.5)
			m.attackers[2] = z.tickNo
		}
		now := time.Now()
		for i := 0; i < 30 && !z.players[1].dead; i++ {
			z.advance(now)
		}
		if !z.players[1].dead {
			t.Fatal("the monster should have killed player 1")
		}
		z.advance(now)
		if other && (m.target != 2 || m.state != chase && m.state != attack) {
			t.Fatalf("target %d state %v, want the monster on player 2", m.target, m.state)
		}
		if !other && (m.target != 0 || m.state != wander) {
			t.Fatalf("target %d state %v, want the monster wandering again", m.target, m.state)
		}
	}
}

// A respawned player is not chased by the monster that killed it (spec 3.6):
// even on a map without safe zones, the monster forgets it at death.
func TestRespawnedPlayerIsNotChased(t *testing.T) {
	home := Vec2{30.5, 30.5}
	area := Area{
		Grid:    openGrid(60, 60),
		Respawn: Vec2{24.5, 30.5}, // 6 tiles from home: outside the detect range
		Regions: []Region{{ID: "r1", Tiles: []Vec2{home}, Rects: []Rect{{X: 0, Y: 0, W: 60, H: 60}}, Nest: home,
			Count: 1, MinLevel: 1, MaxLevel: 1, Kind: testKind}},
		Levels: testLevels,
		Exp:    testExp,
	}
	z := New(testConfig(10), area, &fakeSaver{}, quietLog())
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	joinHP(t, z, 1, 31.5, 30.5, 20)
	engage(m, 1)
	z.advance(time.Now())
	p := z.players[1]
	if !p.dead {
		t.Fatal("the monster should have killed player 1")
	}
	if m.target == 1 {
		t.Fatalf("target %d, want the dead player forgotten at death", m.target)
	}
	if !z.Respawn(1, 1) {
		t.Fatal("inbox full")
	}
	now := time.Now()
	for i := 0; i < 20; i++ {
		z.advance(now)
		if m.target == 1 {
			t.Fatalf("tick %d: the monster chases the respawned player", i)
		}
	}
}

// ______________________TEST REST_____________________

// newRestZone is a fight zone with a rest point of radius 2 at (5.5, 5.5),
// far from the monster, and player 1 standing on it with hp.
func newRestZone(t *testing.T, hp int) (*Zone, *player, *fakeOut, *fakeOut) {
	t.Helper()
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	z.area.Rests = []Circle{{C: Vec2{5.5, 5.5}, R: 2}}
	a := joinHP(t, z, 1, 5.5, 5.5, hp)
	v := joinNow(t, z, 2, 8.5, 8.5)
	return z, z.players[1], a, v
}

func TestRestHealsAfterFiveSeconds(t *testing.T) {
	z, p, a, v := newRestZone(t, 100)
	p.mp = 10
	now := time.Now()
	for i := 1; i < 50; i++ {
		z.advance(now)
	}
	if p.hp != 100 || p.mp != 10 {
		t.Fatalf("hp %d mp %d after 49 ticks, want no heal yet", p.hp, p.mp)
	}
	a.take()
	v.take()
	z.advance(now)
	if p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("hp %d/%d mp %d/%d after 50 ticks, want full", p.hp, p.maxHP, p.mp, p.stats.MaxMP)
	}
	envs := a.take()
	if len(selfStatsIn(envs)) != 1 {
		t.Fatal("want a SelfStats push after the rest heal")
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(p.maxHP) {
			t.Fatalf("%s HpChanges %v, want full HP for 1", name, cs)
		}
	}

	// Moving at tick 30 restarts the count, so the heal comes 50 still ticks later.
	z, p, _, _ = newRestZone(t, 100)
	for i := 1; i < 30; i++ {
		z.advance(now)
	}
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{1, 0}})
	z.advance(now) // tick 30, moving
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{}})
	for z.tickNo < 79 {
		z.advance(now)
	}
	if p.hp != 100 {
		t.Fatalf("hp %d at tick 79, want no heal before 50 still ticks", p.hp)
	}
	z.advance(now)
	if p.hp != p.maxHP {
		t.Fatalf("hp %d at tick 80, want healed after 50 still ticks", p.hp)
	}
}

// A player killed on the tick its rest count completes stays dead at 0 HP.
func TestRestDoesNotHealPlayerKilledThatTick(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	z.area.Rests = []Circle{{C: Vec2{31.5, 30.5}, R: 2}} // outside the safe zone, as a bad map might have it
	joinHP(t, z, 1, 31.5, 30.5, 20)
	engage(m, 1)
	p := z.players[1]
	p.restTicks = z.ticks(restSec) - 1 // step 3 completes the count; the monster kills at step 4
	z.advance(time.Now())
	if !p.dead || p.hp != 0 || p.restTicks != 0 {
		t.Fatalf("dead %v hp %d restTicks %d, want dead at 0 HP with the count reset", p.dead, p.hp, p.restTicks)
	}
	if st := z.state(p); st.HP != 0 {
		t.Fatalf("saved HP %d, want 0", st.HP)
	}
}

func TestRestOnlyInsideRadiusAndAlive(t *testing.T) {
	z, p, _, _ := newRestZone(t, 100)
	p.pos = Vec2{7.6, 5.5} // 2.1 tiles from the centre
	now := time.Now()
	for i := 0; i < 60; i++ {
		z.advance(now)
	}
	if p.hp != 100 {
		t.Fatalf("hp %d, want no heal outside the radius", p.hp)
	}

	z, p, _, _ = newRestZone(t, 100)
	z.damagePlayer(p, 0, 10000, false)
	for i := 0; i < 60; i++ {
		z.advance(now)
	}
	if p.hp != 0 || !p.dead {
		t.Fatalf("hp %d dead %v, want a dead player left dead", p.hp, p.dead)
	}
}

func TestRestDoesNothingWhenFull(t *testing.T) {
	z, p, a, v := newRestZone(t, -1)
	now := time.Now()
	for i := 0; i < 120; i++ {
		z.advance(now)
	}
	envs := a.take()
	if cs := append(hpChangesIn(envs), hpChangesIn(v.take())...); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none for a full player", cs)
	}
	if ss := selfStatsIn(envs); len(ss) != 0 {
		t.Fatalf("SelfStats %v, want none for a full player", ss)
	}

	// Full HP but not full MP: MP is restored, with SelfStats but no HpChange.
	p.mp = 1
	for i := 0; i < 50; i++ {
		z.advance(now)
	}
	envs = a.take()
	if p.mp != p.stats.MaxMP || len(selfStatsIn(envs)) != 1 || len(hpChangesIn(envs)) != 0 {
		t.Fatalf("mp %d/%d SelfStats %d HpChanges %v, want MP restored with only a SelfStats push",
			p.mp, p.stats.MaxMP, len(selfStatsIn(envs)), hpChangesIn(envs))
	}
}
