package zone

import (
	"maps"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
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
		Grid:     openGrid(60, 60),
		Safe:     []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Respawns: []Vec2{{5.5, 5.5}},
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
// damage and landing after its flight at shot_speed. The monster stands still
// and never chases (spec 4.3).
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
	var shots, fired []uint32
	for z.tickNo < stepAt+100 {
		z.advance(now)
		envs := a.take()
		for _, c := range castsBy(envs, m.id) {
			if !c.Ranged || c.TargetId != 1 || c.WindupMs != 0 {
				t.Fatalf("tick %d: cast %v, want a ranged shot at 1 with no windup", z.tickNo, c)
			}
			fired = append(fired, z.tickNo)
		}
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
	windup, every, flight := z.ticks(testKind.ShootWindupSec), z.ticks(testKind.ShootSec), z.ticks(4.9/testKind.ShotSpeed)
	var wantFired, wantShots []uint32
	for at := stepAt + windup; at < stepAt+100; at += every {
		wantFired = append(wantFired, at)
		if at+flight <= stepAt+100 {
			wantShots = append(wantShots, at+flight)
		}
	}
	if !slices.Equal(fired, wantFired) || !slices.Equal(shots, wantShots) {
		t.Fatalf("shots fired on ticks %v landed on %v, want %v and %v (windup %d, then every %d, flight %d)",
			fired, shots, wantFired, wantShots, windup, every, flight)
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
	if !z.Respawn(1, 3, false) {
		t.Fatal("inbox full")
	}
	z.advance(now)
	if r, _ := respawnRespFor(a.take(), 3); r == nil || p.dead || p.pos != z.area.Respawns[0] || p.hp != p.maxHP {
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
	attackLand(t, z, 1, 5, m.id)
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
	var hitTicks, swingTicks []int
	for i := 1; i <= 33; i++ {
		z.advance(now)
		ea, ev := a.take(), v.take()
		if ca, cv := castsBy(ea, m.id), castsBy(ev, m.id); len(ca) != 0 {
			want := &pb.Cast{CasterId: m.id, TargetId: 1, DirX: 1, WindupMs: 200, Impacts: []*pb.CastImpact{{TargetId: 1, ImpactMs: 200}}}
			if len(ca) != 1 || ca[0].String() != want.String() || len(cv) != 1 || cv[0].String() != want.String() {
				t.Fatalf("tick %d: casts %v viewer %v, want %v for both", i, ca, cv, want)
			}
			swingTicks = append(swingTicks, i)
		}
		ha, hv := hitsIn(ea), hitsIn(ev)
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
	// Each swing lands its 200 ms windup (2 ticks) later; the cadence counts
	// from the swing.
	if !slices.Equal(swingTicks, []int{1, 16, 31}) || !slices.Equal(hitTicks, []int{3, 18, 33}) {
		t.Fatalf("swings on ticks %v hits on %v, want 1, 16 and 31 (every 15 ticks), each landing 2 ticks later", swingTicks, hitTicks)
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
		if len(z.events.casts) == 0 {
			continue
		}
		hitAt = i
		if lineBlocked(z.area.Grid, m.pos, p.pos) {
			t.Fatalf("tick %d: swung from %v through the wall", i, m.pos)
		}
	}
	settle(z)
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
	if p.dead || p.hp != 20 {
		t.Fatalf("hp %d dead %v in the swing tick, want the hit still to come", p.hp, p.dead)
	}
	settle(z)
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
	settle(z)
	if ea := a.take(); len(hitsIn(ea)) != 1 || len(castsBy(ea, m.id)) != 1 {
		t.Fatal("the target must see the swing and its own hit")
	}
	ef := f.take()
	if cs, hs, ds := castsIn(ef), hitsIn(ef), deathsIn(ef); len(cs) != 0 || len(hs) != 0 || len(ds) != 0 {
		t.Fatalf("a far viewer got casts %v hits %v deaths %v", cs, hs, ds)
	}
}

// A viewer who sees only the attacker still gets the Cast and the Hit; the
// client ignores ids it has no EntityEnter for.
func TestHitWithOnlyAttackerVisible(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{39.5, 30.5}) // AOI cell (3,3)
	joinNow(t, z, 1, 40.5, 30.5)                 // target in cell (4,3)
	v := joinNow(t, z, 2, 20.5, 30.5)            // cell (2,3): sees (3,3), not (4,3)
	engage(m, 1)
	z.advance(time.Now())
	settle(z)
	envs := v.take()
	if slices.Contains(enters(envs), 1) || !slices.Contains(enters(envs), m.id) {
		t.Fatalf("viewer enters %v, want the monster but not the target", enters(envs))
	}
	if cs := castsIn(envs); len(cs) != 1 || cs[0].CasterId != m.id || cs[0].TargetId != 1 {
		t.Fatalf("viewer casts %v, want the monster's swing at 1", cs)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	swings        int // monster attacks started at it, landed or not
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
	return b.z.tickNo+1 >= p.slashReady
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

// tickBots runs one tick and counts its hits and monster swings for the bots.
func tickBots(z *Zone, bots ...*bot) {
	z.advance(time.Now())
	for _, c := range z.events.casts {
		for _, b := range bots {
			if c.TargetId == b.id && z.monsters[c.CasterId] != nil {
				b.swings++
			}
		}
	}
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

// slashAndLand has b slash m heading along dir, then runs ticks with b still
// heading along dir until the slash lands. others are counted too.
func (b *bot) slashAndLand(dir Vec2, m uint64, others ...*bot) {
	all := append([]*bot{b}, others...)
	b.act(dir, m)
	tickBots(b.z, all...)
	for b.slashInFlight() {
		b.act(dir, 0)
		tickBots(b.z, all...)
	}
}

// slashInFlight reports whether an attack by b has yet to land.
func (b *bot) slashInFlight() bool {
	return slices.ContainsFunc(b.z.impacts, func(im impact) bool { return !im.byMonster && im.attacker == b.id })
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
			b.slashAndLand(Vec2{1, 0}, m.id) // slash and run east
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
	b.slashAndLand(Vec2{}, m.id, a)
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
			attackLand(t, z, 1, 1, m.id)
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
// player who steps out in sight to poke it is chased and swung at at once,
// before the slash and the step back in, and the swing lands although the
// player is back in the safe zone by then (spec 4.5, attack phases spec 3.3).
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
	hits, swings := b.melee, b.swings
	b.act(Vec2{1, 0}, 0) // step out, in sight
	tickBots(z, b)
	if z.area.InSafe(b.p().pos) {
		t.Fatalf("setup: the player did not step out, at %v", b.p().pos)
	}
	b.act(Vec2{-1, 0}, m.id) // slash and step back in
	tickBots(z, b)
	if b.swings == swings {
		t.Fatalf("poked a monster going back without being swung at: state %v target %d at %v", m.state, m.target, m.pos)
	}
	for b.melee == hits && len(z.impacts) > 0 {
		tickBots(z, b)
	}
	if b.melee == hits || !z.area.InSafe(b.p().pos) {
		t.Fatalf("hits %d, player at %v: want the swing landed on the player back in the safe zone", b.melee-hits, b.p().pos)
	}
}

// Once chasing, a monster keeps its target when someone else hits it, but
// remembers when each one last hit it.
func TestChaseKeepsTargetWhenOthersHit(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	joinNow(t, z, 1, 31.5, 30.5)
	joinNow(t, z, 2, 29.5, 30.5)
	attackLand(t, z, 1, 1, m.id)
	first := z.tickNo
	z.advance(time.Now())
	attackLand(t, z, 2, 1, m.id)
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
			attackNow(t, z, 1, 1, m.id)  // A first: the target
			attackLand(t, z, 2, 1, m.id) // both land, B last
			for i := 0; i < c.wait; i++ {
				z.advance(time.Now())
			}
			if m.target != 1 {
				t.Fatalf("target %d, want A", m.target)
			}
			z.damagePlayer(z.players[1], killer{}, 10000, false) // A dies: the monster lets it go
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
	z.area.Respawns = []Vec2{{28.5, 30.5}} // inside the region, outside any safe zone
	joinNow(t, z, 1, 31.5, 30.5)
	joinNow(t, z, 2, 29.5, 30.5)
	attackNow(t, z, 1, 1, m.id)  // A first: the target
	attackLand(t, z, 2, 1, m.id) // both land
	z.damagePlayer(z.players[2], killer{}, 10000, false)
	if !z.Respawn(2, 2, false) {
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
	z.damagePlayer(z.players[1], killer{}, 10000, false) // the target dies: the monster lets it go
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
	if chasing(m) {
		t.Fatalf("state %v, want the monster still going back until the slash lands", m.state)
	}
	var hitAt Vec2 // where it stands when the slash lands, before it moves that tick
	for slices.ContainsFunc(z.impacts, func(im impact) bool { return !im.byMonster }) {
		hitAt = m.pos
		z.advance(time.Now())
	}
	if hitAt == (Vec2{33.5, 30.5}) {
		t.Fatal("setup: the monster did not move during the windup")
	}
	if !chasing(m) || m.target != 1 || m.chaseFrom != hitAt {
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

// settle runs ticks until every impact in flight now has landed and returns
// how many it ran. Impacts started meanwhile, such as a monster hitting back,
// may still be in flight.
func settle(z *Zone) int {
	if len(z.impacts) == 0 {
		return 0
	}
	last := z.impacts[len(z.impacts)-1].due
	n := 0
	for ; z.tickNo < last; n++ {
		z.advance(time.Now())
	}
	return n
}

// attackLand is attackNow followed by settle: the slash is sent and lands.
func attackLand(t *testing.T, z *Zone, id uint64, seq uint32, target uint64) {
	t.Helper()
	attackNow(t, z, id, seq, target)
	settle(z)
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
		// A player id goes through the PK checks (TEST PVP ATTACKS); this
		// area allows no PvP.
		{"a player id", func(z *Zone, m *monster) uint64 { joinNow(t, z, 2, 31.5, 31.5); return 2 },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
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
			hp, last := m.hp, z.players[1].slashReady
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
			if got := z.players[1].slashReady; got != last {
				t.Fatalf("slashReady %d, want %d: a rejected attack must not start the cooldown", got, last)
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
	if hs := hitsIn(envs); len(hs) != 0 || m.hp != 56 || chasing(m) {
		t.Fatalf("hits %v hp %d state %v in the press tick, want nothing landed yet", hs, m.hp, m.state)
	}
	v.take()
	if n := settle(z); n != 2 {
		t.Fatalf("the slash landed %d ticks after the press, want 2 (150 ms)", n)
	}
	envs = a.take()
	// Level-1 slash (phys attack 20) on a level-1 monster (defense 8): 20*(1-8/28) = 14.
	if m.hp != 56-14 {
		t.Fatalf("monster hp %d, want 42", m.hp)
	}
	// The slash, when it lands, turns the monster on its attacker, who stands
	// within its reach, so it swings back on the same tick; the swing lands
	// 2 ticks later for 30*(1-10/30) = 20.
	slashHit := &pb.Hit{AttackerId: 1, TargetId: m.id, Damage: 14, HpAfter: 42}
	ev := v.take()
	for name, envs := range map[string][]*pb.Envelope{"attacker": envs, "viewer": ev} {
		if got := hitsIn(envs); len(got) != 1 || got[0].String() != slashHit.String() {
			t.Fatalf("%s hits %v, want %v", name, got, slashHit)
		}
		if cs := castsBy(envs, m.id); len(cs) != 1 || cs[0].TargetId != 1 {
			t.Fatalf("%s monster casts %v, want its swing at 1", name, cs)
		}
	}
	if !chasing(m) || m.target != 1 {
		t.Fatalf("state %v target %d, want a wandering monster to chase its attacker", m.state, m.target)
	}
	if at, ok := m.attackers[1]; !ok || at != z.tickNo || m.chaseFrom != (Vec2{30.5, 30.5}) {
		t.Fatalf("attackers %v chaseFrom %v, want the hit's tick recorded and the chase starting where it stood", m.attackers, m.chaseFrom)
	}
	if n := settle(z); n != 2 {
		t.Fatalf("the swing landed %d ticks after it started, want 2 (200 ms)", n)
	}
	back := &pb.Hit{AttackerId: m.id, TargetId: 1, Damage: 20, HpAfter: 480}
	for name, got := range map[string][]*pb.Hit{"attacker": hitsIn(a.take()), "viewer": hitsIn(v.take())} {
		if len(got) != 1 || got[0].String() != back.String() {
			t.Fatalf("%s hits %v, want %v", name, got, back)
		}
	}

	// The cooldown is 8 ticks from the press: an attack 8 ticks after the
	// last is accepted.
	for i := 0; i < 3; i++ {
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
	attackLand(t, z, 1, 6, m.id)
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

// Each slash pays its EXP share as it lands: the share of 14, 28, 42 and 56 of
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
			for j := 0; j < 5; j++ { // 8 ticks from the last press, 2 of them its windup
				z.advance(now)
			}
		}
		a.take()
		attackLand(t, z, 1, uint32(10+i), m.id)
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
	in.SkillPoints = 2
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 30.5, 36.5)
	sv := z.saver.(*fakeSaver)
	m.nextAttack = 1 << 30 // it survives the slash; keep it from hitting back
	attackLand(t, z, 1, 1, m.id)
	p := z.players[1]
	if m.dead || m.hp != 42 {
		t.Fatalf("monster dead=%v hp %d, want it alive at 42", m.dead, m.hp)
	}
	if p.level != 2 || p.exp != 1 || p.freePoints != 10 || p.skillPoints != 3 {
		t.Fatalf("level %d exp %d free %d skill points %d, want 2, 1, 10, 3", p.level, p.exp, p.freePoints, p.skillPoints)
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
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Level != 2 || ss[0].FreePoints != 10 || ss[0].SkillPoints != 3 || ss[0].MaxHp != 560 {
		t.Fatalf("SelfStats %v, want level 2 with 10 free points and 3 skill points", ss)
	}
	saves := sv.all()
	if len(saves) != 1 || saves[0].Level != 2 || saves[0].Exp != 1 || saves[0].FreePoints != 10 || saves[0].SkillPoints != 3 ||
		saves[0].HP != 560 || saves[0].MP != 120 {
		t.Fatalf("saves %+v, want one immediate save at level 2", saves)
	}
}

// HpChange carries end-of-tick HP: a level-up heal from a slash's impact
// followed by a monster's impact on the same tick must leave viewers with the
// real HP.
func TestLevelUpThenHitSameTick(t *testing.T) {
	home := Vec2{30.5, 30.5}
	area := Area{
		Grid:     openGrid(60, 60),
		Respawns: []Vec2{{5.5, 5.5}},
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
	// The other monster swings in the press tick, after the slash: both land
	// 2 ticks later, the slash first.
	engage(z.monsters[ids[1]], 1)
	attackLand(t, z, 1, 1, victim.id)
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
	attackLand(t, z, 2, 1, m.id)
	if want1, want2 := combat.ExpShare(total, hp, 14), combat.ExpShare(total, hp, 21); m.hp != 21 || p1.exp != want1 || p2.exp != want2 {
		t.Fatalf("hp %d exp %d/%d, want 21 and %d/%d", m.hp, p1.exp, p2.exp, want1, want2)
	}
	// 1 hits twice more: 14 (7 left) and the kill, which counts only 7.
	for seq := uint32(2); seq <= 3; seq++ {
		for range 7 {
			z.advance(now)
		}
		attackLand(t, z, 1, seq, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
	attackLand(t, z, 1, 1, m.id)
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
		attackLand(t, z, 1, seq, m.id)
	}
	if !m.dead || p.exp != testLevels[1].Exp {
		t.Fatalf("dead %v exp %d, want the kill to bring the total to the monster's %d EXP", m.dead, p.exp, testLevels[1].Exp)
	}
}

// A player at the level cap gains nothing and gets no SelfStats push, even
// from a monster close enough in level to pay EXP.
// One gain that crosses two levels pays the points of both.
func TestMultiLevelGain(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	in := info(1, 5.5, 5.5)
	in.SkillPoints, in.FreePoints = 1, 3
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	z.gainExp(p, testExp.ToNext(1)+testExp.ToNext(2)+5)
	if p.level != 3 || p.exp != 5 {
		t.Fatalf("level %d exp %d, want 3 and 5", p.level, p.exp)
	}
	if p.skillPoints != 3 || p.freePoints != 23 {
		t.Fatalf("skill points %d free points %d, want 1+2 and 3+20", p.skillPoints, p.freePoints)
	}
	if saves := sv.all(); len(saves) != 1 || saves[0].Level != 3 || saves[0].SkillPoints != 3 || saves[0].FreePoints != 23 {
		t.Fatalf("saves %+v, want one save at level 3", saves)
	}
}

func TestNoExpAtMaxLevel(t *testing.T) {
	z, m := newFightZone(t, 85, Vec2{30.5, 30.5})
	if !combat.ExpEligible(combat.MaxLevel, m.level) {
		t.Fatal("setup: the monster must be inside the EXP window of a level-90 player")
	}
	in := info(1, 31.5, 30.5)
	in.Level = combat.MaxLevel
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	a.take()
	attackLand(t, z, 1, 1, m.id)
	p := z.players[1]
	if _, counted := m.dmgBy[1]; !counted || m.hp == m.maxHP {
		t.Fatalf("dmgBy %v hp %d/%d, want the slash landed and counted", m.dmgBy, m.hp, m.maxHP)
	}
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

// Only a player within ExpLevelWindow levels of the monster, above or below
// it, earns EXP from a hit. A hit outside the window still lands but pays
// nothing and is not counted towards the monster's EXP shares (phase 2
// spec 2.4).
func TestExpOnlyWithinLevelWindow(t *testing.T) {
	for _, c := range []struct {
		level, monster int
		want           int64
	}{{12, 1, 0}, {11, 1, testLevels[1].Exp}, {1, 12, 0}} {
		z, m := newFightZone(t, c.monster, Vec2{30.5, 30.5})
		in := info(1, 31.5, 30.5)
		in.Level = c.level
		a := &fakeOut{}
		if err := z.join(in, a); err != nil {
			t.Fatal(err)
		}
		a.take()
		m.nextAttack = 1 << 30 // keep a stronger monster from hitting back
		attackLand(t, z, 1, 1, m.id)
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

// A monster pays the EXP of its own level: killing a level-2 monster in one
// slash pays its whole Levels[2].Exp.
func TestExpByMonsterLevel(t *testing.T) {
	z, m := newFightZone(t, 2, Vec2{30.5, 30.5})
	in := info(1, 31.5, 30.5)
	in.Points = combat.Points{Str: 200} // one slash kills it
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	attackLand(t, z, 1, 1, m.id)
	if p := z.players[1]; !m.dead || p.level != 1 || p.exp != testLevels[2].Exp || testLevels[2].Exp == testLevels[1].Exp {
		t.Fatalf("dead %v level %d exp %d, want level 1 with the level-2 monster's %d EXP", m.dead, p.level, p.exp, testLevels[2].Exp)
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
	if p.pos != z.area.Respawns[0] || p.cell != z.cellOf(z.area.Respawns[0]) || p.dead {
		t.Fatalf("at %v cell %v dead %v, want alive at the respawn point %v", p.pos, p.cell, p.dead, z.area.Respawns[0])
	}
	if p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("hp %d/%d mp %d/%d, want full", p.hp, p.maxHP, p.mp, p.stats.MaxMP)
	}
	r := out.take()[0].GetEnterWorldResp()
	if r.X != z.area.Respawns[0].X || r.Y != z.area.Respawns[0].Y {
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
	s := combat.Compute(true, 3, in.Points, combat.Bonus{}, combat.Passive{})
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
	in.Skills = map[string]int{"viem_1": 1}
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	p.points, p.freePoints, p.hp, p.mp, p.exp = combat.Points{Str: 2, Agi: 3, Vit: 4, Int: 5}, 1, 77, 33, 88
	p.skillPoints, p.skills["viem_1"] = 4, 2
	in.Skills["viem_1"] = 9 // the zone owns its own copy
	t0 := time.Now()
	z.advance(t0)
	z.advance(t0.Add(61 * time.Second))
	saves := sv.all()
	want := store.CharState{ID: 100, MapID: "m1", X: 5.5, Y: 5.5, Level: 1, Exp: 88,
		Points: combat.Points{Str: 2, Agi: 3, Vit: 4, Int: 5}, FreePoints: 1, HP: 77, MP: 33,
		SkillPoints: 4, Skills: map[string]int{"viem_1": 2}}
	if len(saves) != 1 || !reflect.DeepEqual(saves[0], want) {
		t.Fatalf("saves %+v, want %+v", saves, want)
	}
	// The saver runs on another goroutine: the saved map is a copy.
	p.skills["viem_1"] = 3
	if saves[0].Skills["viem_1"] != 2 {
		t.Fatal("the saved state shares the player's skill map")
	}
}

// ______________________TEST SKILL STATE_____________________

var testSkills = map[string]gamedata.Skill{
	"nham_1": {ID: "nham_1", Class: "nham", Slot: 1, LearnLevel: 10},
	"viem_1": {ID: "viem_1", Class: "viem", Slot: 1, LearnLevel: 10},
	"nham_3": {ID: "nham_3", Class: "nham", Slot: 0, LearnLevel: 30, Passive: map[string][]int{
		gamedata.PassiveMaxHPPct:   {2, 4, 6, 8, 10, 12, 14, 16, 18, 20},
		gamedata.PassivePhysDefPct: {2, 4, 6, 8, 10, 12, 14, 16, 18, 20},
	}},
}

// A learned passive counts in the stats at join, after a level-up and after
// allocating points; the join reply carries the skills and skill points.
func TestPassiveInStats(t *testing.T) {
	sv := &fakeSaver{}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Skills: testSkills}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.SkillPoints, in.FreePoints = pb.CharClass_CHAR_CLASS_NHAM, 30, 2, 3
	in.Skills = map[string]int{"nham_3": 5, "nham_1": 2}
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	pas := combat.Passive{MaxHPPct: 10, PhysDefPct: 10}
	with, without := combat.Compute(false, 30, combat.Points{}, combat.Bonus{}, pas), combat.Compute(false, 30, combat.Points{}, combat.Bonus{}, combat.Passive{})
	if with.MaxHP <= without.MaxHP {
		t.Fatalf("passive max HP %d, base %d", with.MaxHP, without.MaxHP)
	}
	r := out.take()[0].GetEnterWorldResp()
	if r.Stats.MaxHp != int32(with.MaxHP) || r.Stats.Hp != int32(with.MaxHP) || r.Stats.PhysDefense != int32(with.PhysDefense) {
		t.Fatalf("stats %v, want max HP %d and phys defense %d", r.Stats, with.MaxHP, with.PhysDefense)
	}
	if r.Stats.SkillPoints != 2 {
		t.Fatalf("skill points %d, want 2", r.Stats.SkillPoints)
	}
	want := []*pb.SkillLevel{{SkillId: "nham_1", Level: 2}, {SkillId: "nham_3", Level: 5}}
	if len(r.Skills) != len(want) || r.Skills[0].String() != want[0].String() || r.Skills[1].String() != want[1].String() {
		t.Fatalf("skills %v, want %v sorted by id", r.Skills, want)
	}

	p := z.players[1]
	z.gainExp(p, testExp.ToNext(30))
	if lv := combat.Compute(false, 31, combat.Points{}, combat.Bonus{}, pas); p.level != 31 || p.skillPoints != 3 || p.maxHP != lv.MaxHP || p.hp != lv.MaxHP {
		t.Fatalf("after level-up: level %d skill points %d hp %d/%d, want 31, 3, %d", p.level, p.skillPoints, p.hp, p.maxHP, lv.MaxHP)
	}

	allocateNow(t, z, 1, 4, combat.Points{Vit: 3})
	lv := combat.Compute(false, 31, combat.Points{Vit: 3}, combat.Bonus{}, pas)
	if p.maxHP != lv.MaxHP || p.hp != lv.MaxHP {
		t.Fatalf("after allocate: hp %d/%d, want %d", p.hp, p.maxHP, lv.MaxHP)
	}
	if ss := allocateRespFor(out.take(), 4).GetStats(); ss.SkillPoints != 3 || ss.MaxHp != int32(lv.MaxHP) {
		t.Fatalf("allocate stats %v", ss)
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
				z.damagePlayer(p, killer{}, 10000, false)
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

	if !z.Respawn(1, 4, false) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if er := errorFor(a.take(), 4); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_NOT_DEAD {
		t.Fatalf("respawn while alive: error %v, want NOT_DEAD", er)
	}

	z.damagePlayer(p, killer{}, 10000, false)
	z.advance(time.Now())
	a.take()
	v.take()
	if !z.Respawn(1, 5, false) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if p.dead || p.pos != z.area.Respawns[0] || p.cell != z.cellOf(z.area.Respawns[0]) || p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("after respawn dead %v at %v cell %v hp %d/%d mp %d/%d, want alive and full at %v",
			p.dead, p.pos, p.cell, p.hp, p.maxHP, p.mp, p.stats.MaxMP, z.area.Respawns[0])
	}
	envs := a.take()
	r, ri := respawnRespFor(envs, 5)
	if r == nil || r.X != z.area.Respawns[0].X || r.Y != z.area.Respawns[0].Y {
		t.Fatalf("RespawnResp %v, want the respawn point %v", r, z.area.Respawns[0])
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
	if mv == nil || !mv.Snap || mv.X != z.area.Respawns[0].X || mv.Y != z.area.Respawns[0].Y {
		t.Fatalf("viewer move %v, want a snap to the respawn point", mv)
	}
	if cs := hpChangesIn(ev); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(p.maxHP) {
		t.Fatalf("viewer HpChanges %v, want full HP for 1", cs)
	}

	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{0, 1}})
	z.advance(time.Now())
	if p.pos == z.area.Respawns[0] {
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
	z.damagePlayer(p, killer{}, 10000, false)
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
	if p.dead || p.pos != z.area.Respawns[0] || p.hp != p.maxHP {
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
		Grid:     openGrid(60, 60),
		Respawns: []Vec2{{24.5, 30.5}}, // 6 tiles from home: outside the detect range
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
	settle(z)
	p := z.players[1]
	if !p.dead {
		t.Fatal("the monster should have killed player 1")
	}
	if m.target == 1 {
		t.Fatalf("target %d, want the dead player forgotten at death", m.target)
	}
	if !z.Respawn(1, 1, false) {
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

// ______________________TEST RESPAWN POINTS_____________________

// newTwoCampZone is newFightZone with a second safe camp at (50,50) and the
// respawn points (5.5, 5.5) and (55.5, 55.5), in that order.
func newTwoCampZone(t *testing.T) *Zone {
	t.Helper()
	z, _ := newFightZone(t, 1, Vec2{30.5, 5.5})
	z.area.Safe = append(z.area.Safe, Rect{X: 50, Y: 50, W: 10, H: 10})
	z.area.Respawns = []Vec2{{5.5, 5.5}, {55.5, 55.5}}
	return z
}

func TestNearestRespawn(t *testing.T) {
	a := Area{Respawns: []Vec2{{5.5, 5.5}, {55.5, 55.5}}}
	for _, c := range []struct{ at, want Vec2 }{
		{Vec2{10, 10}, Vec2{5.5, 5.5}},
		{Vec2{45.5, 45.5}, Vec2{55.5, 55.5}},
		{Vec2{30.5, 30.5}, Vec2{5.5, 5.5}}, // a tie goes to the earlier point
		{Vec2{55.5, 5.5}, Vec2{5.5, 5.5}},  // a tie the other way round too
	} {
		if got := a.nearestRespawn(c.at); got != c.want {
			t.Errorf("nearestRespawn(%v) = %v, want %v", c.at, got, c.want)
		}
	}
	if got := (&Area{}).nearestRespawn(Vec2{3, 3}); got != (Vec2{}) {
		t.Errorf("no respawn points: %v, want the origin", got)
	}
}

// A player dying nearer the second camp comes back there, both by request and
// by the automatic respawn after 60 s; one dying halfway comes back at the first.
func TestRespawnAtNearestPoint(t *testing.T) {
	for _, c := range []struct {
		name string
		at   Vec2
		auto bool
		want Vec2
	}{
		{"request near the second", Vec2{45.5, 45.5}, false, Vec2{55.5, 55.5}},
		{"automatic near the second", Vec2{45.5, 45.5}, true, Vec2{55.5, 55.5}},
		{"request halfway", Vec2{30.5, 30.5}, false, Vec2{5.5, 5.5}},
	} {
		t.Run(c.name, func(t *testing.T) {
			z := newTwoCampZone(t)
			a := joinNow(t, z, 1, c.at.X, c.at.Y)
			p := z.players[1]
			now := time.Now()
			z.advance(now)
			z.damagePlayer(p, killer{}, 10000, false)
			if !p.dead {
				t.Fatal("the player must be dead")
			}
			a.take()
			if c.auto {
				diedAt := z.tickNo
				for z.tickNo < diedAt+z.ticks(autoRespawnSec) {
					z.advance(now)
				}
			} else {
				if !z.Respawn(1, 7, false) {
					t.Fatal("inbox full")
				}
				z.advance(now)
				if r, _ := respawnRespFor(a.take(), 7); r == nil || r.X != c.want.X || r.Y != c.want.Y {
					t.Fatalf("RespawnResp %v, want %v", r, c.want)
				}
			}
			if p.dead || p.pos != c.want || p.cell != z.cellOf(c.want) || p.hp != p.maxHP {
				t.Fatalf("dead %v at %v hp %d/%d, want alive and full at %v", p.dead, p.pos, p.hp, p.maxHP, c.want)
			}
		})
	}
}

// Joining with HP 0 from a saved position near the second camp puts the
// player at the second camp with full HP and MP.
func TestJoinDeadAtNearestRespawn(t *testing.T) {
	z := newTwoCampZone(t)
	in := info(1, 50.5, 40.5)
	in.HP, in.MP = 0, 3
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	p, want := z.players[1], Vec2{55.5, 55.5}
	if p.pos != want || p.cell != z.cellOf(want) || p.dead || p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("at %v dead %v hp %d/%d mp %d/%d, want alive and full at %v",
			p.pos, p.dead, p.hp, p.maxHP, p.mp, p.stats.MaxMP, want)
	}
	if r := out.take()[0].GetEnterWorldResp(); r.X != want.X || r.Y != want.Y {
		t.Fatalf("EnterWorldResp at %v,%v, want %v", r.X, r.Y, want)
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
	now := time.Now()
	z.advance(now) // the swing
	for z.impacts[0].due > z.tickNo+1 {
		z.advance(now)
	}
	p.restTicks = z.ticks(restSec) - 1 // the landing tick would complete the count; the swing kills first
	z.advance(now)
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
	z.damagePlayer(p, killer{}, 10000, false)
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

// ______________________TEST CAST_____________________

// repoSkills is the real skill data, data/skills.json.
var repoSkills = func() map[string]gamedata.Skill {
	b, err := os.ReadFile("../../data/skills.json")
	if err != nil {
		panic(err)
	}
	s, err := gamedata.ParseSkills(b)
	if err != nil {
		panic(err)
	}
	return s
}()

// castSkills are what castZone's player has learned, all at level 1: a
// single-target skill (thuy_1), a cone (thuy_2), the passive (thuy_3) and a
// self-centred circle (thuy_4).
var castSkills = map[string]int{"thuy_1": 1, "thuy_2": 1, "thuy_3": 1, "thuy_4": 1}

// castZone is newFightZone with the repo skills and a level-12 monster at
// home that does not notice players by itself, so only hits make it chase.
// Player 1, a level-12 Thủy with castSkills, joins at pos.
func castZone(t *testing.T, home, pos Vec2) (*Zone, *monster, *player, *fakeOut) {
	t.Helper()
	z, m := newFightZone(t, 12, home)
	z.area.Skills = repoSkills
	m.region.Kind.DetectRange = 0.5
	a := joinCaster(t, z, 1, pos)
	a.take()
	return z, m, z.players[1], a
}

// joinCaster joins a level-12 Thủy with castSkills at pos.
func joinCaster(t *testing.T, z *Zone, id uint64, pos Vec2) *fakeOut {
	t.Helper()
	in := info(id, pos.X, pos.Y)
	in.Class, in.Level, in.Skills = pb.CharClass_CHAR_CLASS_THUY, 12, castSkills
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	return out
}

// castNow queues a cast by id with seq and runs one tick.
func castNow(t *testing.T, z *Zone, id uint64, seq uint32, skillID string, target uint64, dir Vec2) {
	t.Helper()
	if !z.Cast(id, seq, skillID, target, dir) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

// castLand is castNow followed by settle: the cast is sent and lands.
func castLand(t *testing.T, z *Zone, id uint64, seq uint32, skillID string, target uint64, dir Vec2) {
	t.Helper()
	castNow(t, z, id, seq, skillID, target, dir)
	settle(z)
}

func castRespAt(envs []*pb.Envelope, seq uint32) int {
	for i, e := range envs {
		if e.GetCastResp() != nil && e.Seq == seq {
			return i
		}
	}
	return -1
}

func castsIn(envs []*pb.Envelope) []*pb.Cast {
	var cs []*pb.Cast
	for _, wt := range ticksIn(envs) {
		cs = append(cs, wt.Casts...)
	}
	return cs
}

// castsBy is the casts in envs by caster; monsters hitting back cast too.
func castsBy(envs []*pb.Envelope, caster uint64) []*pb.Cast {
	var cs []*pb.Cast
	for _, c := range castsIn(envs) {
		if c.CasterId == caster {
			cs = append(cs, c)
		}
	}
	return cs
}

// untimed is c without its timing fields, for tests about what a cast aims
// at; TEST IMPACTS checks the timing.
func untimed(c *pb.Cast) *pb.Cast {
	return &pb.Cast{CasterId: c.CasterId, SkillId: c.SkillId, TargetId: c.TargetId, DirX: c.DirX, DirY: c.DirY, Level: c.Level, Ranged: c.Ranged}
}

// impactIDs are the targets of c's impacts, in hit order (phase 2 spec 4.4).
func impactIDs(c *pb.Cast) []uint64 {
	var ids []uint64
	for _, im := range c.Impacts {
		ids = append(ids, im.TargetId)
	}
	return ids
}

// hitsBy are the hits by attacker in envs.
func hitsBy(envs []*pb.Envelope, attacker uint64) []*pb.Hit {
	var hs []*pb.Hit
	for _, h := range hitsIn(envs) {
		if h.AttackerId == attacker {
			hs = append(hs, h)
		}
	}
	return hs
}

// skillDamage is what p's skill s at level 1 does to a monster of level lv.
func skillDamage(p *player, s gamedata.Skill, lv int) int {
	base := combat.SkillBase(p.stats, s.Damage == gamedata.DamageMagic, s.CoefPct[0])
	return combat.Damage(base, testLevels[lv].Defense, p.level)
}

// Each check of phase 2 spec 4.2 in order: a case breaks every later check
// too, so the first failing one is answered. No error spends MP, starts a
// cooldown or lands.
func TestCastValidation(t *testing.T) {
	home := Vec2{30.5, 30.5}
	near := Vec2{32.5, 30.5} // 2 tiles from home: within thuy_1's 5 + 0.5
	for _, c := range []struct {
		name   string
		player Vec2
		skill  string
		setup  func(z *Zone, m *monster, p *player) uint64 // returns the target id
		code   pb.ErrorCode
	}{
		{"dead in a safe zone, no MP, unknown skill", Vec2{5.5, 5.5}, "nope", func(z *Zone, m *monster, p *player) uint64 {
			p.dead, p.deadUntil, p.mp = true, z.tickNo+100, 0
			return 999999
		}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"in a safe zone, no MP, unknown skill", Vec2{5.5, 5.5}, "nope", func(z *Zone, m *monster, p *player) uint64 {
			p.mp = 0
			return 999999
		}, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"unknown skill", near, "nope", func(z *Zone, m *monster, p *player) uint64 { return m.id },
			pb.ErrorCode_ERROR_CODE_SKILL_NOT_LEARNED},
		{"another class's skill", near, "viem_1", func(z *Zone, m *monster, p *player) uint64 {
			p.skills["viem_1"] = 1 // even if somehow learned
			return m.id
		}, pb.ErrorCode_ERROR_CODE_SKILL_NOT_LEARNED},
		{"not learned", near, "thuy_5", func(z *Zone, m *monster, p *player) uint64 { return m.id },
			pb.ErrorCode_ERROR_CODE_SKILL_NOT_LEARNED},
		{"a passive", near, "thuy_3", func(z *Zone, m *monster, p *player) uint64 { return m.id },
			pb.ErrorCode_ERROR_CODE_SKILL_NOT_LEARNED},
		{"on cooldown, no MP, bad target", near, "thuy_1", func(z *Zone, m *monster, p *player) uint64 {
			p.skillReady["thuy_1"] = z.tickNo + 5
			p.mp = 0
			return 999999
		}, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN},
		{"one MP short, bad target", near, "thuy_1", func(z *Zone, m *monster, p *player) uint64 {
			p.mp = repoSkills["thuy_1"].MP[0] - 1
			return 999999
		}, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_MP},
		{"unknown target", near, "thuy_1", func(z *Zone, m *monster, p *player) uint64 { return 999999 },
			pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"a player target in an area without PvP", near, "thuy_1", func(z *Zone, m *monster, p *player) uint64 {
			joinNow(t, z, 2, 31.5, 31.5)
			return 2
		}, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
		{"a dead monster", near, "thuy_1", func(z *Zone, m *monster, p *player) uint64 { m.dead = true; return m.id },
			pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"5.51 tiles away", Vec2{36.01, 30.5}, "thuy_1", func(z *Zone, m *monster, p *player) uint64 { return m.id },
			pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"a wall in between", Vec2{28.5, 30.5}, "thuy_1", func(z *Zone, m *monster, p *player) uint64 {
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
		}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m, p, a := castZone(t, home, c.player)
			target := c.setup(z, m, p)
			mp, ready, hp := p.mp, maps.Clone(p.skillReady), m.hp
			castNow(t, z, 1, 42, c.skill, target, Vec2{1, 0})
			envs := a.take()
			if er := errorFor(envs, 42); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v with seq 42", er, c.code)
			}
			if castRespAt(envs, 42) >= 0 || len(castsIn(envs)) != 0 || m.hp != hp {
				t.Fatalf("a rejected cast must not land: hp %d want %d, casts %v", m.hp, hp, castsIn(envs))
			}
			if p.mp != mp || !maps.Equal(p.skillReady, ready) {
				t.Fatalf("mp %d ready %v, want %d %v: a rejected cast spends nothing", p.mp, p.skillReady, mp, ready)
			}
		})
	}
}

// A single-target skill spends MP and starts its cooldown at the press, with
// the reply ahead of the WorldTick, which carries the Cast, and a SelfStats
// push after it. When it lands it hits for its own damage, names itself on
// the Hit, pays EXP and turns the monster on the caster.
func TestCastSingleHitsAndAggro(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m, p, a := castZone(t, home, Vec2{35.99, 30.5}) // 5.49 tiles: in range with the slack
	s := repoSkills["thuy_1"]
	mp, exp := p.mp, p.exp
	castNow(t, z, 1, 9, "thuy_1", m.id, Vec2{})
	envs := a.take()
	ri, wi := castRespAt(envs, 9), indexOfTick(envs)
	if ri < 0 || wi < 0 || ri > wi {
		t.Fatalf("CastResp at %d, WorldTick at %d (error %v): the reply must come first", ri, wi, errorFor(envs, 9))
	}
	// 0.25 s of windup and 5.49/20 s of flight: 6 ticks.
	wantCast := &pb.Cast{CasterId: 1, SkillId: "thuy_1", TargetId: m.id, DirX: -1, Level: 1, WindupMs: 250,
		Impacts: []*pb.CastImpact{{TargetId: m.id, ImpactMs: 600}}}
	if cs := castsIn(envs); len(cs) != 1 || cs[0].String() != wantCast.String() {
		t.Fatalf("casts %v, want %v: dir points at the target", cs, wantCast)
	}
	if p.mp != mp-s.MP[0] || p.skillReady["thuy_1"] != z.tickNo+z.ticks(1.5) {
		t.Fatalf("mp %d ready %d, want %d and %d", p.mp, p.skillReady["thuy_1"], mp-s.MP[0], z.tickNo+z.ticks(1.5))
	}
	si := slices.IndexFunc(envs, func(e *pb.Envelope) bool { return e.GetSelfStats() != nil })
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Mp != int32(p.mp) || si < wi {
		t.Fatalf("self stats %v, want one after the tick with mp %d", ss, p.mp)
	}
	if hs := hitsIn(envs); len(hs) != 0 || m.hp != m.maxHP || p.exp != exp || chasing(m) {
		t.Fatalf("hits %v hp %d exp %d state %v in the press tick, want nothing landed", hs, m.hp, p.exp, m.state)
	}

	if n := settle(z); n != 6 {
		t.Fatalf("landed %d ticks after the press, want 6", n)
	}
	envs = a.take()
	dmg := skillDamage(p, s, 12)
	if dmg <= 1 || m.hp != testLevels[12].HP-dmg {
		t.Fatalf("monster hp %d, want %d - %d", m.hp, testLevels[12].HP, dmg)
	}
	want := &pb.Hit{AttackerId: 1, TargetId: m.id, Damage: int32(dmg), HpAfter: int32(m.hp), SkillId: "thuy_1"}
	if hs := hitsBy(envs, 1); len(hs) != 1 || hs[0].String() != want.String() {
		t.Fatalf("hits %v, want %v", hs, want)
	}
	if p.exp <= exp {
		t.Fatalf("exp %d, want more than %d: a skill hit pays its EXP share", p.exp, exp)
	}
	if !chasing(m) || m.target != 1 || m.attackers[1] != z.tickNo {
		t.Fatalf("state %v target %d attackers %v, want the monster chasing its caster", m.state, m.target, m.attackers)
	}
}

// A single-target skill reaches range[n-1] of its learned level n plus
// attackRangeSlack: the repo's thuy_1 (5 tiles at level 1) lands at 5.49
// and not at 5.51, and with a range table that grows by level the learned
// level picks the entry.
func TestCastRangeByLevel(t *testing.T) {
	home := Vec2{30.5, 30.5}
	grown := maps.Clone(repoSkills)
	s := grown["thuy_1"]
	s.Range = []float64{5, 7, 9, 9, 9, 9, 9, 9, 9, 9}
	grown["thuy_1"] = s
	for _, c := range []struct {
		name   string
		skills map[string]gamedata.Skill
		level  int
		dist   float32
		ok     bool
	}{
		{"repo level 1 at 5.49", repoSkills, 1, 5.49, true},
		{"repo level 1 at 5.51", repoSkills, 1, 5.51, false},
		{"grown level 1 at 5.51", grown, 1, 5.51, false},
		{"grown level 2 at 7.49", grown, 2, 7.49, true},
		{"grown level 2 at 7.51", grown, 2, 7.51, false},
		{"grown level 3 at 9.49", grown, 3, 9.49, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m, p, a := castZone(t, home, Vec2{home.X + c.dist, home.Y})
			z.area.Skills = c.skills
			p.skills["thuy_1"] = c.level
			hp := m.hp
			castLand(t, z, 1, 7, "thuy_1", m.id, Vec2{})
			envs := a.take()
			if c.ok {
				if castRespAt(envs, 7) < 0 || m.hp >= hp {
					t.Fatalf("error %v, hp %d of %d: want the cast to land", errorFor(envs, 7), m.hp, hp)
				}
				return
			}
			if er := errorFor(envs, 7); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE || m.hp != hp {
				t.Fatalf("error %v, hp %d of %d: want OUT_OF_RANGE and no hit", er, m.hp, hp)
			}
		})
	}
}

// A slash and a skill are both accepted in one tick: their cooldowns are
// separate. The slash resolves first, and lands first.
func TestSlashAndCastSameTick(t *testing.T) {
	z, m, p, a := castZone(t, Vec2{30.5, 30.5}, Vec2{31.5, 30.5})
	z.Attack(1, 1, m.id)
	castNow(t, z, 1, 2, "thuy_1", m.id, Vec2{})
	t0 := z.tickNo
	settle(z)
	envs := a.take()
	if attackRespAt(envs, 1) < 0 || castRespAt(envs, 2) < 0 {
		t.Fatalf("attack %v cast %v, want both accepted", errorFor(envs, 1), errorFor(envs, 2))
	}
	hs := hitsBy(envs, 1)
	if len(hs) != 2 || hs[0].SkillId != "" || hs[1].SkillId != "thuy_1" {
		t.Fatalf("hits %v, want the slash then the skill", hs)
	}
	if p.slashReady != t0+z.ticks(combat.SlashCooldown.Seconds()) || p.skillReady["thuy_1"] <= t0 {
		t.Fatalf("slashReady %d ready %v, want both cooldowns started", p.slashReady, p.skillReady)
	}
}

// A skill is on cooldown for z.ticks(cooldown_ms/1000) ticks, and other
// skills are not held up by it.
func TestCastCooldown(t *testing.T) {
	z, m, p, a := castZone(t, Vec2{30.5, 30.5}, Vec2{31.5, 30.5})
	castNow(t, z, 1, 1, "thuy_1", m.id, Vec2{})
	cd := z.ticks(float64(repoSkills["thuy_1"].CooldownMS[0]) / 1000)
	for i := uint32(1); i < cd-1; i++ {
		z.advance(time.Now())
	}
	a.take()
	castNow(t, z, 1, 2, "thuy_1", m.id, Vec2{}) // cd-1 ticks after the first
	if er := errorFor(a.take(), 2); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("cast %d ticks later got %v, want ON_COOLDOWN", cd-1, er)
	}
	p.hp = p.maxHP                              // the monster has been hitting back
	castNow(t, z, 1, 3, "thuy_1", m.id, Vec2{}) // cd ticks after the first
	if envs := a.take(); castRespAt(envs, 3) < 0 {
		t.Fatalf("cast %d ticks later refused: %v", cd, errorFor(envs, 3))
	}
	castNow(t, z, 1, 4, "thuy_4", 0, Vec2{})
	if envs := a.take(); castRespAt(envs, 4) < 0 {
		t.Fatalf("another skill refused: %v", errorFor(envs, 4))
	}
}

func TestNewerCastReplacesPending(t *testing.T) {
	z, m, _, a := castZone(t, Vec2{30.5, 30.5}, Vec2{31.5, 30.5})
	z.Cast(1, 5, "thuy_1", m.id, Vec2{})
	castLand(t, z, 1, 6, "thuy_1", m.id, Vec2{})
	envs := a.take()
	if er := errorFor(envs, 5); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("the replaced cast got %v, want ON_COOLDOWN", er)
	}
	if castRespAt(envs, 6) < 0 || len(hitsBy(envs, 1)) != 1 {
		t.Fatalf("the newer cast must land once: %v", hitsBy(envs, 1))
	}
}

// Liệt Oa (radius 2.5) hits a monster 2.4 tiles from the caster and not one
// 2.6 away; it needs no target and its Cast has none and no direction.
func TestCastCircleSelf(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m, p, a := castZone(t, home, Vec2{28.1, 30.5}) // m is 2.4 tiles east
	if !z.spawn(m.region, Vec2{28.1, 33.1}) {         // 2.6 tiles south
		t.Fatal("spawn failed")
	}
	var far *monster
	for _, o := range z.monsters {
		if o != m {
			far = o
		}
	}
	s := repoSkills["thuy_4"]
	mp := p.mp
	castLand(t, z, 1, 1, "thuy_4", 0, Vec2{0, 1})
	envs := a.take()
	if castRespAt(envs, 1) < 0 {
		t.Fatalf("cast refused: %v", errorFor(envs, 1))
	}
	hs := hitsBy(envs, 1)
	if len(hs) != 1 || hs[0].TargetId != m.id || hs[0].SkillId != "thuy_4" || int(hs[0].Damage) != skillDamage(p, s, 12) {
		t.Fatalf("hits %v, want one thuy_4 hit on %d only", hs, m.id)
	}
	if far.hp != far.maxHP {
		t.Fatal("the monster 2.6 tiles away was hit")
	}
	want := &pb.Cast{CasterId: 1, SkillId: "thuy_4", Level: 1, WindupMs: 300, Impacts: []*pb.CastImpact{{TargetId: m.id, ImpactMs: 300}}}
	if cs := castsBy(envs, 1); len(cs) != 1 || cs[0].String() != want.String() {
		t.Fatalf("casts %v, want %v", cs, want)
	}
	if p.mp != mp-s.MP[0] {
		t.Fatalf("mp %d, want %d", p.mp, mp-s.MP[0])
	}
}

// A self-centred skill with nobody in reach is still a cast: MP, cooldown
// and the Cast event, but no hits.
func TestCastCircleSelfAlone(t *testing.T) {
	z, _, p, a := castZone(t, Vec2{30.5, 30.5}, Vec2{40.5, 40.5})
	mp := p.mp
	castNow(t, z, 1, 1, "thuy_4", 0, Vec2{})
	envs := a.take()
	if castRespAt(envs, 1) < 0 || len(hitsIn(envs)) != 0 || len(castsIn(envs)) != 1 {
		t.Fatalf("resp %d hits %v casts %v, want an accepted cast with no hits", castRespAt(envs, 1), hitsIn(envs), castsIn(envs))
	}
	if p.mp != mp-repoSkills["thuy_4"].MP[0] || p.skillReady["thuy_4"] != z.tickNo+z.ticks(4) {
		t.Fatalf("mp %d ready %v, want MP spent and the cooldown started", p.mp, p.skillReady)
	}
	// No EXP was paid, so only the MP spend can have pushed the stats.
	wi := indexOfTick(envs)
	si := slices.IndexFunc(envs, func(e *pb.Envelope) bool { return e.GetSelfStats() != nil })
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Mp != int32(p.mp) || si < wi {
		t.Fatalf("self stats %v at %d (tick at %d), want one after the tick with mp %d", ss, si, wi, p.mp)
	}
}

// A magic skill hits from magic attack, not the higher physical attack:
// thuy_1 level 1 has coef 150%, so base = floor(magic attack * 1.5), and
// damage = floor(base * (1 - def/(def + 20*level))) on a level-12 monster
// (defense 96).
func TestCastUsesSkillDamageKind(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 12, home)
	z.area.Skills = repoSkills
	m.region.Kind.DetectRange = 0.5
	in := info(1, 32.5, 30.5)
	in.Class, in.Level, in.Skills = pb.CharClass_CHAR_CLASS_THUY, 12, castSkills
	in.Points = combat.Points{Str: 200}
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	if p.stats.PhysAttack <= p.stats.MagicAttack {
		t.Fatalf("phys %d magic %d: the test needs phys attack above magic attack", p.stats.PhysAttack, p.stats.MagicAttack)
	}
	base := p.stats.MagicAttack * 150 / 100
	want := int(math.Floor(float64(base) * (1 - 96.0/(96.0+20*12))))
	wrong := int(math.Floor(float64(p.stats.PhysAttack*150/100) * (1 - 96.0/(96.0+20*12))))
	if want == wrong {
		t.Fatalf("phys and magic give the same damage %d", want)
	}
	a.take()
	castLand(t, z, 1, 1, "thuy_1", m.id, Vec2{})
	if hs := hitsBy(a.take(), 1); len(hs) != 1 || int(hs[0].Damage) != want {
		t.Fatalf("hits %v, want one hit of %d (phys attack would give %d)", hs, want, wrong)
	}
}

// A monster that is dead but still lying inside the area is not hit.
func TestCastSkipsDeadMonsterInArea(t *testing.T) {
	z, m, _, a := castZone(t, Vec2{30.5, 30.5}, Vec2{29.5, 30.5}) // m is 1 tile east
	if !z.spawn(m.region, Vec2{29.5, 31.5}) {                     // 1 tile south
		t.Fatal("spawn failed")
	}
	var alive *monster
	for _, o := range z.monsters {
		if o != m {
			alive = o
		}
	}
	z.killMonster(m, nil)
	castLand(t, z, 1, 1, "thuy_4", m.id, Vec2{})
	envs := a.take()
	if castRespAt(envs, 1) < 0 {
		t.Fatalf("cast refused: %v", errorFor(envs, 1))
	}
	if hs := hitsBy(envs, 1); len(hs) != 1 || hs[0].TargetId != alive.id {
		t.Fatalf("hits %v, want only the living monster %d hit", hs, alive.id)
	}
}

// The Cast goes to the caster and to whoever sees the caster, not further.
func TestCastSeenByViewers(t *testing.T) {
	z, m, _, a := castZone(t, Vec2{30.5, 30.5}, Vec2{31.5, 30.5})
	v := joinNow(t, z, 2, 30.5, 36.5) // same AOI cell
	f := joinNow(t, z, 3, 5.5, 5.5)   // three cells away
	v.take()
	f.take()
	castNow(t, z, 1, 1, "thuy_1", m.id, Vec2{})
	for name, o := range map[string]*fakeOut{"caster": a, "viewer": v} {
		if cs := castsIn(o.take()); len(cs) != 1 || cs[0].CasterId != 1 {
			t.Fatalf("%s casts %v, want the cast", name, cs)
		}
	}
	if cs := castsIn(f.take()); len(cs) != 0 {
		t.Fatalf("a far player got casts %v", cs)
	}
}

// A non-finite cast direction reaches the zone as zero.
func TestCastSanitisesDir(t *testing.T) {
	z, m, p, _ := castZone(t, Vec2{30.5, 30.5}, Vec2{31.5, 30.5})
	z.Cast(1, 1, "thuy_1", m.id, Vec2{float32(math.NaN()), 1})
	z.drain()
	if p.pendingCast == nil || p.pendingCast.dir != (Vec2{}) {
		t.Fatalf("pending cast %+v, want a zero dir", p.pendingCast)
	}
	z.Cast(1, 2, "thuy_1", m.id, Vec2{3, 4})
	z.drain()
	if p.pendingCast.dir != (Vec2{0.6, 0.8}) {
		t.Fatalf("dir %v, want (0.6, 0.8)", p.pendingCast.dir)
	}
}

// monstersInBox finds the monsters inside a box across every AOI cell it
// overlaps, not only the 3x3 around one cell.
func TestMonstersInBox(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{9, 9})
	for _, pos := range []Vec2{{41, 11}, {43, 11}, {25, 13}} {
		if !z.spawn(m.region, pos) {
			t.Fatal("spawn failed")
		}
	}
	var got []Vec2
	for _, o := range z.monstersInBox(Vec2{8, 8}, Vec2{42, 12}) {
		got = append(got, o.pos)
	}
	slices.SortFunc(got, func(a, b Vec2) int { return int(a.X - b.X) })
	if want := []Vec2{{9, 9}, {41, 11}}; !slices.Equal(got, want) {
		t.Fatalf("in box %v, want %v", got, want)
	}
}

// ______________________TEST AREAS_____________________

// areaCaster is where areaZone's caster stands.
var areaCaster = Vec2{30.5, 30.5}

// areaZone is a zone like castZone whose player 1 is a level-12 caster of
// skill's class who knows only skill, at skill level lv, standing at
// areaCaster. One level-12 monster stands at each areaCaster+offs[i],
// returned in that order, which is increasing id order. None of them
// notices players by itself.
func areaZone(t *testing.T, skill string, lv int, offs ...Vec2) (*Zone, *player, *fakeOut, []*monster) {
	t.Helper()
	at := func(o Vec2) Vec2 { return Vec2{areaCaster.X + o.X, areaCaster.Y + o.Y} }
	z, first := newFightZone(t, 12, at(offs[0]))
	z.area.Skills = repoSkills
	first.region.Kind.DetectRange = 0.5
	ms := []*monster{first}
	for _, o := range offs[1:] {
		before := maps.Clone(z.monsters)
		if !z.spawn(first.region, at(o)) {
			t.Fatal("spawn failed")
		}
		for id, m := range z.monsters {
			if before[id] == nil {
				ms = append(ms, m)
			}
		}
		if n := len(ms); n != len(z.monsters) || ms[n-1].id <= ms[n-2].id {
			t.Fatal("spawned monsters must get increasing ids")
		}
	}
	in := info(1, areaCaster.X, areaCaster.Y)
	in.Class, in.Level, in.Skills = classes.ToPB(repoSkills[skill].Class), 12, map[string]int{skill: lv}
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	a.take()
	return z, z.players[1], a, ms
}

// wallAt turns tile (x, y) of a 60x60 zone into a wall.
func wallAt(z *Zone, x, y int) {
	rows := make([]string, 60)
	for ty := range rows {
		r := []byte(strings.Repeat(".", 60))
		if ty == y {
			r[x] = '#'
		}
		rows[ty] = string(r)
	}
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
}

// polar is the offset of length d at deg degrees from the +x axis.
func polar(d, deg float64) Vec2 {
	r := deg * math.Pi / 180
	return Vec2{float32(d * math.Cos(r)), float32(d * math.Sin(r))}
}

// targetIDs are the targets of hs in order.
func targetIDs(hs []*pb.Hit) []uint64 {
	var ids []uint64
	for _, h := range hs {
		ids = append(ids, h.TargetId)
	}
	return ids
}

func idsOf(ms ...*monster) []uint64 {
	var ids []uint64
	for _, m := range ms {
		ids = append(ids, m.id)
	}
	return ids
}

// Each shape's membership rule (phase 2 spec 4.4), one monster at a time so
// max_targets plays no part. A cone (thuy_2, 120 degrees) and a line
// (phong_1, width 1.5) reach range[n-1] with no attackRangeSlack: 4 tiles at
// skill level 1, 6 at level 10. Every cast is accepted; the question is only
// whether the monster is hit. A wall between the origin and the monster
// keeps it out.
func TestCastAreaShapes(t *testing.T) {
	east := Vec2{1, 0}
	for _, c := range []struct {
		name  string
		skill string
		lv    int
		off   Vec2
		dir   Vec2
		wall  *[2]int
		hit   bool
	}{
		{"cone 3.9 ahead", "thuy_2", 1, Vec2{3.9, 0}, east, nil, true},
		{"cone 59 degrees left", "thuy_2", 1, polar(3, 59), east, nil, true},
		{"cone 59 degrees right", "thuy_2", 1, polar(3, -59), east, nil, true},
		{"cone 61 degrees left", "thuy_2", 1, polar(3, 61), east, nil, false},
		{"cone 61 degrees right", "thuy_2", 1, polar(3, -61), east, nil, false},
		{"cone behind", "thuy_2", 1, Vec2{-2, 0}, east, nil, false},
		{"cone 4.1 ahead at level 1", "thuy_2", 1, Vec2{4.1, 0}, east, nil, false},
		{"cone 5.9 ahead at level 10", "thuy_2", 10, Vec2{5.9, 0}, east, nil, true},
		{"cone 6.1 ahead at level 10", "thuy_2", 10, Vec2{6.1, 0}, east, nil, false},
		{"cone aimed north", "thuy_2", 1, Vec2{0, -3}, Vec2{0, -1}, nil, true},
		{"cone aimed north, monster east", "thuy_2", 1, Vec2{3, 0}, Vec2{0, -1}, nil, false},
		{"cone on the caster", "thuy_2", 1, Vec2{}, east, nil, true},
		{"cone behind a wall", "thuy_2", 1, Vec2{3, 0}, east, &[2]int{32, 30}, false},
		{"line at (5.9, 0.7) at level 10", "phong_1", 10, Vec2{5.9, 0.7}, east, nil, true},
		{"line at (5.9, -0.7) at level 10", "phong_1", 10, Vec2{5.9, -0.7}, east, nil, true},
		{"line at (6.1, 0) at level 10", "phong_1", 10, Vec2{6.1, 0}, east, nil, false},
		{"line at (3, 0.8)", "phong_1", 10, Vec2{3, 0.8}, east, nil, false},
		{"line behind", "phong_1", 10, Vec2{-1, 0}, east, nil, false},
		{"line at (3.9, 0) at level 1", "phong_1", 1, Vec2{3.9, 0}, east, nil, true},
		{"line at (4.1, 0) at level 1", "phong_1", 1, Vec2{4.1, 0}, east, nil, false},
		{"line on a diagonal, 0.71 off", "phong_1", 10, Vec2{2, 3}, Vec2{1, 1}, nil, true},
		{"line on a diagonal, 0.85 off", "phong_1", 10, Vec2{2, 3.2}, Vec2{1, 1}, nil, false},
		{"line on the caster", "phong_1", 1, Vec2{}, east, nil, true},
		{"line behind a wall", "phong_1", 1, Vec2{3, 0}, east, &[2]int{32, 30}, false},
		{"circle_self 2.4 away", "thuy_4", 1, Vec2{2.4, 0}, Vec2{}, nil, true},
		{"circle_self 2.6 away", "thuy_4", 1, Vec2{0, 2.6}, Vec2{}, nil, false},
		{"circle_self behind a wall", "thuy_4", 1, Vec2{2.4, 0}, Vec2{}, &[2]int{31, 30}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, a, ms := areaZone(t, c.skill, c.lv, c.off)
			if c.wall != nil {
				wallAt(z, c.wall[0], c.wall[1])
			}
			m, mp := ms[0], p.mp
			castLand(t, z, 1, 1, c.skill, 0, c.dir)
			envs := a.take()
			if castRespAt(envs, 1) < 0 || p.mp != mp-repoSkills[c.skill].MP[c.lv-1] {
				t.Fatalf("cast refused (%v) or mp %d of %d not spent", errorFor(envs, 1), p.mp, mp)
			}
			hs := hitsBy(envs, 1)
			if got := len(hs) == 1 && hs[0].TargetId == m.id && m.hp < m.maxHP; got != c.hit || (!c.hit && len(hs) != 0) {
				t.Fatalf("hits %v (monster %d at %v), want hit=%v", hs, m.id, m.pos, c.hit)
			}
		})
	}
}

// Cone and line aim (phase 2 spec 4.3): at a living named monster at any
// range, else along dir scaled to length 1; with neither the cast is a
// BAD_REQUEST that spends nothing. A dir too short to point anywhere counts
// as none.
func TestCastConeLineAim(t *testing.T) {
	for _, skill := range []string{"thuy_2", "phong_1"} {
		for _, c := range []struct {
			name   string
			target func(m *monster) uint64
			dir    Vec2
		}{
			{"no target, no dir", func(m *monster) uint64 { return 0 }, Vec2{}},
			{"unknown target, no dir", func(m *monster) uint64 { return 999999 }, Vec2{}},
			{"no target, a tiny dir", func(m *monster) uint64 { return 0 }, Vec2{1e-4, 0}},
			{"dead target, no dir", func(m *monster) uint64 { m.dead = true; return m.id }, Vec2{}},
		} {
			t.Run(skill+" "+c.name, func(t *testing.T) {
				z, p, a, ms := areaZone(t, skill, 1, Vec2{2, 0})
				target := c.target(ms[0])
				mp, ready := p.mp, maps.Clone(p.skillReady)
				castLand(t, z, 1, 3, skill, target, c.dir)
				envs := a.take()
				if er := errorFor(envs, 3); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_BAD_REQUEST {
					t.Fatalf("error %v, want BAD_REQUEST", er)
				}
				if p.mp != mp || !maps.Equal(p.skillReady, ready) || len(castsIn(envs)) != 0 || len(hitsIn(envs)) != 0 {
					t.Fatalf("mp %d ready %v casts %v: a refused cast spends and shows nothing", p.mp, p.skillReady, castsIn(envs))
				}
			})
		}

		// Only a short dir: it is scaled up, the area lies along it and the
		// Cast carries it with no target.
		t.Run(skill+" along a short dir", func(t *testing.T) {
			z, _, a, ms := areaZone(t, skill, 1, Vec2{0, 3}, Vec2{3, 0})
			castLand(t, z, 1, 4, skill, 999999, Vec2{0, 0.5})
			envs := a.take()
			if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), idsOf(ms[0])) {
				t.Fatalf("hits %v, want only %d to the south", hs, ms[0].id)
			}
			want := &pb.Cast{CasterId: 1, SkillId: skill, DirY: 1, Level: 1}
			if cs := castsBy(envs, 1); len(cs) != 1 || untimed(cs[0]).String() != want.String() {
				t.Fatalf("casts %v, want %v", cs, want)
			}
		})

		// A named monster sets the direction even out of reach and against
		// dir; it is the Cast's target but is not hit from 10 tiles.
		t.Run(skill+" at a far target", func(t *testing.T) {
			z, _, a, ms := areaZone(t, skill, 1, Vec2{0, -10}, Vec2{0, -3}, Vec2{3, 0})
			castLand(t, z, 1, 5, skill, ms[0].id, Vec2{1, 0})
			envs := a.take()
			if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), idsOf(ms[1])) {
				t.Fatalf("hits %v, want only %d, towards the target", hs, ms[1].id)
			}
			want := &pb.Cast{CasterId: 1, SkillId: skill, TargetId: ms[0].id, DirY: -1, Level: 1}
			if cs := castsBy(envs, 1); len(cs) != 1 || untimed(cs[0]).String() != want.String() {
				t.Fatalf("casts %v, want %v", cs, want)
			}
		})

		// A named monster standing on the caster gives no direction; with
		// no dir either the cast still lands on it, and on nothing else.
		t.Run(skill+" at a target on the caster", func(t *testing.T) {
			z, _, a, ms := areaZone(t, skill, 1, Vec2{}, Vec2{1, 0}, Vec2{0, 1})
			castLand(t, z, 1, 7, skill, ms[0].id, Vec2{})
			envs := a.take()
			if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), idsOf(ms[0])) {
				t.Fatalf("hits %v, want only %d", targetIDs(hs), ms[0].id)
			}
			want := &pb.Cast{CasterId: 1, SkillId: skill, TargetId: ms[0].id, Level: 1}
			if cs := castsBy(envs, 1); len(cs) != 1 || untimed(cs[0]).String() != want.String() {
				t.Fatalf("casts %v, want %v", cs, want)
			}
		})

		// The named monster is hit first even when it is the farthest, and
		// max_targets (2 at level 1) keeps only the nearest of the rest.
		t.Run(skill+" named target first", func(t *testing.T) {
			z, _, a, ms := areaZone(t, skill, 1, Vec2{3.5, 0}, Vec2{2, 0}, Vec2{1, 0})
			castLand(t, z, 1, 6, skill, ms[0].id, Vec2{})
			envs := a.take()
			// The wave reaches the nearer one first, so the hits come in
			// distance order; the Cast lists them in hit order.
			if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), idsOf(ms[2], ms[0])) {
				t.Fatalf("hits %v, want %v", targetIDs(hs), idsOf(ms[2], ms[0]))
			}
			if cs := castsBy(envs, 1); len(cs) != 1 || !slices.Equal(impactIDs(cs[0]), idsOf(ms[0], ms[2])) {
				t.Fatalf("casts %v, want impacts on %v", cs, idsOf(ms[0], ms[2]))
			}
		})
	}
}

// A target circle (phase 2 spec 4.3-4.4) needs a living monster within
// range[n-1] plus attackRangeSlack and in sight of the caster, like a
// single-target skill; a refusal spends nothing.
func TestCastCircleTargetAim(t *testing.T) {
	for _, c := range []struct {
		name  string
		lv    int
		off   Vec2
		setup func(z *Zone, m *monster) uint64
		code  pb.ErrorCode
	}{
		{"unknown target", 1, Vec2{2, 0}, func(z *Zone, m *monster) uint64 { return 999999 }, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"no target", 1, Vec2{2, 0}, func(z *Zone, m *monster) uint64 { return 0 }, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"dead target", 1, Vec2{2, 0}, func(z *Zone, m *monster) uint64 { m.dead = true; return m.id }, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"4.49 away at level 1", 1, Vec2{4.49, 0}, func(z *Zone, m *monster) uint64 { return m.id }, pb.ErrorCode_ERROR_CODE_UNSPECIFIED},
		{"4.51 away at level 1", 1, Vec2{4.51, 0}, func(z *Zone, m *monster) uint64 { return m.id }, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"5.49 away at level 4", 4, Vec2{5.49, 0}, func(z *Zone, m *monster) uint64 { return m.id }, pb.ErrorCode_ERROR_CODE_UNSPECIFIED},
		{"5.51 away at level 4", 4, Vec2{5.51, 0}, func(z *Zone, m *monster) uint64 { return m.id }, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"behind a wall", 1, Vec2{3, 0}, func(z *Zone, m *monster) uint64 { wallAt(z, 32, 30); return m.id }, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, a, ms := areaZone(t, "viem_4", c.lv, c.off)
			target := c.setup(z, ms[0])
			mp := p.mp
			castLand(t, z, 1, 2, "viem_4", target, Vec2{1, 0})
			envs := a.take()
			if c.code == pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
				if castRespAt(envs, 2) < 0 || len(hitsBy(envs, 1)) != 1 {
					t.Fatalf("error %v, hits %v: want the cast to land", errorFor(envs, 2), hitsBy(envs, 1))
				}
				return
			}
			if er := errorFor(envs, 2); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if p.mp != mp || len(castsIn(envs)) != 0 || len(hitsIn(envs)) != 0 {
				t.Fatalf("mp %d of %d, casts %v: a refused cast spends and shows nothing", p.mp, mp, castsIn(envs))
			}
		})
	}
}

// A target circle is centred on the target: Viêm's level-4 viem_4 (radius
// 1.5, 3 targets) hits a monster 1.4 from the target and not one 1.6 from
// it, nor one next to the caster. Its Cast names the target and points at it.
func TestCastCircleTargetArea(t *testing.T) {
	z, _, a, ms := areaZone(t, "viem_4", 4, Vec2{3, 0}, Vec2{3, 1.4}, Vec2{3, -1.6}, Vec2{0.5, 0})
	castLand(t, z, 1, 1, "viem_4", ms[0].id, Vec2{0, 1})
	envs := a.take()
	if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), idsOf(ms[0], ms[1])) {
		t.Fatalf("hits %v, want %v", targetIDs(hs), idsOf(ms[0], ms[1]))
	}
	want := &pb.Cast{CasterId: 1, SkillId: "viem_4", TargetId: ms[0].id, DirX: 1, Level: 4}
	if cs := castsBy(envs, 1); len(cs) != 1 || untimed(cs[0]).String() != want.String() {
		t.Fatalf("casts %v, want %v", cs, want)
	}
}

// A target circle checks line of sight from the target, not the caster:
// with a wall between the target and a monster 1.9 from it, phong_4 (radius
// 2) misses that monster although the caster sees it.
func TestCastCircleTargetSight(t *testing.T) {
	for _, wall := range []bool{false, true} {
		z, _, a, ms := areaZone(t, "phong_4", 4, Vec2{3, 0}, Vec2{3, 1.9})
		if wall {
			wallAt(z, 33, 31)
			if lineBlocked(z.area.Grid, areaCaster, ms[1].pos) || !lineBlocked(z.area.Grid, ms[0].pos, ms[1].pos) {
				t.Fatal("the wall must block target to monster only")
			}
		}
		castLand(t, z, 1, 1, "phong_4", ms[0].id, Vec2{})
		want := idsOf(ms[0], ms[1])
		if wall {
			want = idsOf(ms[0])
		}
		if hs := hitsBy(a.take(), 1); !slices.Equal(targetIDs(hs), want) {
			t.Fatalf("wall %v: hits %v, want %v", wall, targetIDs(hs), want)
		}
	}
}

// Hit order and max_targets (phase 2 spec 4.4) on Liệt Oa (radius 2.5): the
// named target first even when it is the farthest, then the nearest to the
// centre, cut at max_targets (2 at level 1, 4 at level 7). The monsters
// spawn farthest first, so id order is not distance order. The Cast names
// the target only when it was hit.
func TestCastCircleSelfOrder(t *testing.T) {
	// Distances 2.3, 2, 1.70, 1.5, 1, 0.5.
	offs := []Vec2{{2.3, 0}, {-2, 0}, {1.2, 1.2}, {0, -1.5}, {-1, 0}, {0, 0.5}}
	for _, c := range []struct {
		name   string
		lv     int
		named  bool
		far    Vec2 // where the named monster stands
		want   []int
		castTo bool
	}{
		{"level 1, named", 1, true, offs[0], []int{0, 5}, true},
		{"level 1, no target", 1, false, offs[0], []int{5, 4}, false},
		{"level 7, named", 7, true, offs[0], []int{0, 5, 4, 3}, true},
		{"level 1, named out of reach", 1, true, Vec2{2.6, 0}, []int{5, 4}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _, a, ms := areaZone(t, "thuy_4", c.lv, append([]Vec2{c.far}, offs[1:]...)...)
			var target uint64
			if c.named {
				target = ms[0].id
			}
			castLand(t, z, 1, 1, "thuy_4", target, Vec2{1, 0})
			envs := a.take()
			var want []uint64
			for _, i := range c.want {
				want = append(want, ms[i].id)
			}
			if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), want) {
				t.Fatalf("hits %v, want %v", targetIDs(hs), want)
			}
			cast := &pb.Cast{CasterId: 1, SkillId: "thuy_4", Level: int32(c.lv)}
			if c.castTo {
				cast.TargetId = ms[0].id
			}
			if cs := castsBy(envs, 1); len(cs) != 1 || untimed(cs[0]).String() != cast.String() {
				t.Fatalf("casts %v, want %v", cs, cast)
			}
		})
	}
}

// Monsters at the same distance from the centre are hit lower id first: of
// three at 1 tile, a level-1 Liệt Oa (2 targets) hits the two lowest ids in
// that order, whatever order the AOI scan finds them in.
func TestCastAreaTieByID(t *testing.T) {
	for range 20 {
		z, _, a, ms := areaZone(t, "thuy_4", 1, Vec2{1, 0}, Vec2{0, 1}, Vec2{-1, 0})
		castLand(t, z, 1, 1, "thuy_4", 0, Vec2{})
		if hs := hitsBy(a.take(), 1); !slices.Equal(targetIDs(hs), idsOf(ms[0], ms[1])) {
			t.Fatalf("hits %v, want %v", targetIDs(hs), idsOf(ms[0], ms[1]))
		}
	}
}

// ______________________TEST DASH_____________________

// dashZone is a zone like castZone with a level-12 monster at target and a
// level-12 Viêm, player 1, at caster who knows only viem_1 (Bất Tri Hỏa, a
// dash) at skill level 1. One tick has run, so the caster has seen itself
// enter and its later jumps reach it as moves.
func dashZone(t *testing.T, caster, target Vec2) (*Zone, *monster, *player, *fakeOut) {
	t.Helper()
	z, m := newFightZone(t, 12, target)
	z.area.Skills = repoSkills
	m.region.Kind.DetectRange = 0.5
	in := info(1, caster.X, caster.Y)
	in.Class, in.Level, in.Skills = pb.CharClass_CHAR_CLASS_VIEM, 12, map[string]int{"viem_1": 1}
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	z.advance(time.Now())
	a.take()
	return z, m, z.players[1], a
}

func near32(a, b float32) bool { return abs32(a-b) < 1e-4 }

// Bất Tri Hỏa from 3.5 tiles lands the caster on the line to the monster,
// exactly 1 tile short of it, at the press; the caster and a viewer both
// get the jump as a snapping EntityMove, and the Cast points at the monster.
func TestDashLandsShortOfTarget(t *testing.T) {
	start := Vec2{30.5, 30.5}
	off := polar(3.5, 30)
	z, m, p, a := dashZone(t, start, Vec2{start.X + off.X, start.Y + off.Y})
	v := joinNow(t, z, 2, 30.5, 36.5) // same AOI cell
	z.advance(time.Now())             // the viewer sees the caster enter
	v.take()
	a.take()
	mp := p.mp
	castNow(t, z, 1, 5, "viem_1", m.id, Vec2{})
	if d := distance(p.pos, m.pos); !near32(d, 1) {
		t.Fatalf("landed %v, %v from the monster, want 1", p.pos, d)
	}
	if d := distance(start, p.pos); !near32(d, 2.5) {
		t.Fatalf("dashed %v, want 2.5 along the line", d)
	}
	if u, w := dirTo(start, p.pos), dirTo(start, m.pos); !near32(u.X, w.X) || !near32(u.Y, w.Y) {
		t.Fatalf("dashed along %v, want towards the monster %v", u, w)
	}
	if p.cell != z.cellOf(p.pos) || z.cells[p.cell][1] == nil {
		t.Fatal("the caster's AOI cell is stale")
	}
	envs := a.take()
	for name, es := range map[string][]*pb.Envelope{"caster": envs, "viewer": v.take()} {
		mv := moveFor(lastTick(es), 1)
		if mv == nil || !mv.Snap || mv.X != p.pos.X || mv.Y != p.pos.Y {
			t.Fatalf("%s got move %v, want a snap to %v", name, mv, p.pos)
		}
	}
	if castRespAt(envs, 5) < 0 || p.mp != mp-repoSkills["viem_1"].MP[0] {
		t.Fatalf("the dash was not accepted: mp %d from %d", p.mp, mp)
	}
	cs, w := castsIn(envs), dirTo(start, m.pos)
	if len(cs) != 1 || cs[0].TargetId != m.id || !near32(cs[0].DirX, w.X) || !near32(cs[0].DirY, w.Y) {
		t.Fatalf("casts %v, want one at the monster along %v", cs, w)
	}
	if hs := hitsIn(envs); len(hs) != 0 {
		t.Fatalf("hits %v in the press tick, want none", hs)
	}
	settle(z)
	if hs := hitsBy(a.take(), 1); len(hs) != 1 || hs[0].TargetId != m.id || hs[0].SkillId != "viem_1" {
		t.Fatalf("hits %v, want one viem_1 hit on the monster", hs)
	}
	z.advance(time.Now())
	if mv := moveFor(lastTick(a.take()), 1); mv != nil {
		t.Fatalf("the next tick moved the caster again: %v", mv)
	}
}

// The caster's direction survives the dash: that tick's input moves it on
// from the landing point, and the snap carries where it ends up.
func TestDashKeepsMoving(t *testing.T) {
	z, m, p, a := dashZone(t, Vec2{30.5, 30.5}, Vec2{34.5, 30.5})
	z.Move(1, 1, 0, 1)
	castNow(t, z, 1, 5, "viem_1", m.id, Vec2{})
	want := Step(z.area.Grid, Vec2{33.5, 30.5}, Vec2{0, 1}, z.cfg.MoveSpeed/float32(z.cfg.TickRate))
	if !near32(p.pos.X, want.X) || !near32(p.pos.Y, want.Y) || p.dir != (Vec2{0, 1}) {
		t.Fatalf("pos %v dir %v, want %v still moving (0, 1)", p.pos, p.dir, want)
	}
	if mv := moveFor(lastTick(a.take()), 1); mv == nil || !mv.Snap || mv.X != p.pos.X || mv.Y != p.pos.Y {
		t.Fatalf("move %v, want a snap to %v", mv, p.pos)
	}
}

// A monster within 1 tile is hit where the caster stands: no move, no snap.
func TestDashWithinOneTileStays(t *testing.T) {
	start := Vec2{30.5, 30.5}
	z, m, p, a := dashZone(t, start, Vec2{31.3, 30.5})
	castNow(t, z, 1, 5, "viem_1", m.id, Vec2{})
	envs := a.take()
	if p.pos != start || p.snap {
		t.Fatalf("pos %v snap %v, want %v unmoved", p.pos, p.snap, start)
	}
	if mv := moveFor(lastTick(envs), 1); mv != nil {
		t.Fatalf("move %v, want none", mv)
	}
	settle(z)
	if hs := hitsBy(a.take(), 1); len(hs) != 1 || hs[0].TargetId != m.id {
		t.Fatalf("hits %v, want one on the monster", hs)
	}
}

// A landing point in a safe zone, or one where the caster's box would
// overlap a wall, is OUT_OF_RANGE even though the monster is in range and in
// sight; the caster stays and spends nothing.
func TestDashBadLanding(t *testing.T) {
	for _, c := range []struct {
		name           string
		caster, target Vec2
		setup          func(z *Zone)
	}{
		// Both ends are outside the safe rect (0,0,10,10); the landing
		// point (8.2, 9.8) cuts its corner.
		{"safe zone", Vec2{10.5, 7.5}, Vec2{7.5, 10.5}, func(*Zone) {}},
		// The landing point (33, 30.25) is on an open tile, but its box
		// reaches into the wall above; the line along y = 30.25 is clear.
		{"wall beside the landing tile", Vec2{30.5, 30.25}, Vec2{34, 30.25}, func(z *Zone) { wallAt(z, 32, 29) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m, p, a := dashZone(t, c.caster, c.target)
			c.setup(z)
			mp, ready, hp := p.mp, maps.Clone(p.skillReady), m.hp
			castNow(t, z, 1, 42, "viem_1", m.id, Vec2{})
			envs := a.take()
			if er := errorFor(envs, 42); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE {
				t.Fatalf("error %v, want OUT_OF_RANGE", er)
			}
			if p.pos != c.caster || moveFor(lastTick(envs), 1) != nil {
				t.Fatalf("pos %v, want %v unmoved", p.pos, c.caster)
			}
			if p.mp != mp || !maps.Equal(p.skillReady, ready) || m.hp != hp || len(castsIn(envs)) != 0 {
				t.Fatalf("mp %d ready %v hp %d casts %v: a rejected dash spends and lands nothing", p.mp, p.skillReady, m.hp, castsIn(envs))
			}
		})
	}
}

// A dash across an AOI cell border moves the caster's cell, so a player who
// is only near the new cell now sees the caster enter.
func TestDashCrossesAOICell(t *testing.T) {
	z, m, p, a := dashZone(t, Vec2{29.5, 30.5}, Vec2{33.5, 30.5}) // cell (2, 3) to (3, 3)
	v := joinNow(t, z, 2, 45.5, 30.5)                             // cell (4, 3)
	if slices.Contains(enters(v.take()), 1) {
		t.Fatal("the viewer already sees the caster")
	}
	castNow(t, z, 1, 5, "viem_1", m.id, Vec2{})
	if p.cell != (cellKey{3, 3}) {
		t.Fatalf("cell %v, want (3, 3)", p.cell)
	}
	if !slices.Contains(enters(v.take()), 1) {
		t.Fatal("the viewer did not see the caster enter")
	}
	if !slices.Contains(enters(a.take()), 2) {
		t.Fatal("the caster did not see the viewer enter")
	}
}

// ______________________TEST IMPACTS_____________________

// landTicks runs ticks until every impact in flight has landed and returns
// the tick each hit by attacker landed on, by target, and the targets in the
// order they were hit.
func landTicks(z *Zone, attacker uint64) (map[uint64]uint32, []uint64) {
	at := map[uint64]uint32{}
	var order []uint64
	for len(z.impacts) > 0 {
		z.advance(time.Now())
		for _, h := range z.events.hits {
			if h.AttackerId == attacker {
				at[h.TargetId] = z.tickNo
				order = append(order, h.TargetId)
			}
		}
	}
	return at, order
}

// The queue lands impacts by due tick, then by the order they were made, and
// only those due by now.
func TestImpactQueueOrder(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{30.5, 30.5})
	for _, due := range []uint32{5, 3, 5, 3, 4} {
		z.schedule(impact{due: due, target: 999999}) // no such monster: landing does nothing
	}
	type key struct{ due, seq uint32 }
	var got []key
	for _, im := range z.impacts {
		got = append(got, key{im.due, im.seq})
	}
	if want := []key{{3, 2}, {3, 4}, {4, 5}, {5, 1}, {5, 3}}; !slices.Equal(got, want) {
		t.Fatalf("queue %v, want %v", got, want)
	}
	z.tickNo = 3
	z.runImpacts()
	if len(z.impacts) != 3 || z.impacts[0].due != 4 {
		t.Fatalf("after tick 3: %d left, first due %d; want the two due at 3 gone", len(z.impacts), z.impacts[0].due)
	}
}

// The slash changes nothing in the press tick but announces itself with a
// Cast: no skill, 150 ms of windup, one impact. It lands ticks(0.15) later,
// and only then does the monster take damage and turn on the attacker.
func TestSlashLandsAtImpactTick(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.region.Kind.DetectRange = 0.5
	a := joinNow(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	t0, p := z.tickNo, z.players[1]
	envs := a.take()
	if attackRespAt(envs, 1) < 0 {
		t.Fatalf("slash rejected: %v", errorFor(envs, 1))
	}
	if hs := hitsIn(envs); len(hs) != 0 || m.hp != m.maxHP || p.exp != 0 || len(m.attackers) != 0 || chasing(m) {
		t.Fatalf("press tick: hits %v hp %d exp %d attackers %v state %v, want nothing changed", hs, m.hp, p.exp, m.attackers, m.state)
	}
	want := &pb.Cast{CasterId: 1, TargetId: m.id, DirX: -1, WindupMs: 150, Impacts: []*pb.CastImpact{{TargetId: m.id, ImpactMs: 200}}}
	if cs := castsIn(envs); len(cs) != 1 || cs[0].String() != want.String() {
		t.Fatalf("casts %v, want %v", cs, want)
	}
	at, _ := landTicks(z, 1)
	if at[m.id] != t0+z.ticks(0.15) || m.hp != 56-14 {
		t.Fatalf("landed at tick %d with hp %d, want tick %d and hp 42", at[m.id], m.hp, t0+z.ticks(0.15))
	}
	if !chasing(m) || m.target != 1 || m.attackers[1] != at[m.id] || p.exp == 0 {
		t.Fatalf("state %v target %d attackers %v exp %d, want aggro and EXP at the impact", m.state, m.target, m.attackers, p.exp)
	}
}

// Each victim's impact is windup + flight after the press (spec 2.2): a
// single target by its distance, a cone or line wave by each victim's own
// distance, a target circle all at once by the primary's distance, and a
// self circle all at once after its windup.
func TestSkillImpactTicks(t *testing.T) {
	for _, c := range []struct {
		name   string
		skill  string
		offs   []Vec2
		target bool     // name ms[0]
		ticks  []uint32 // after the press, per monster
	}{
		{"single at 5 tiles", "thuy_1", []Vec2{{5, 0}}, true, []uint32{5}},           // 0.25 + 5/20
		{"cone at 2 and 4", "thuy_2", []Vec2{{2, 0}, {4, 0}}, false, []uint32{4, 5}}, // 0.25 + 2/20, 0.25 + 4/20
		{"line at 2 and 4", "phong_1", []Vec2{{4, 0}, {2, 0}}, false, []uint32{5, 4}},
		{"target circle", "viem_4", []Vec2{{3, 0}, {4.4, 0}}, true, []uint32{5, 5}}, // 0.3 + 3/20 for both
		{"self circle", "thuy_4", []Vec2{{1, 0}, {-2.4, 0}}, false, []uint32{3, 3}}, // 0.3
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _, a, ms := areaZone(t, c.skill, 1, c.offs...)
			var target uint64
			if c.target {
				target = ms[0].id
			}
			castNow(t, z, 1, 1, c.skill, target, Vec2{1, 0})
			t0 := z.tickNo
			envs := a.take()
			if hs := hitsIn(envs); len(hs) != 0 {
				t.Fatalf("hits %v in the press tick, want none", hs)
			}
			for _, m := range ms {
				if m.hp != m.maxHP {
					t.Fatalf("monster %d hp %d in the press tick, want full", m.id, m.hp)
				}
			}
			cs := castsIn(envs)
			if len(cs) != 1 || cs[0].WindupMs != int32(repoSkills[c.skill].WindupMS) || len(cs[0].Impacts) != len(ms) {
				t.Fatalf("casts %v, want one with %d ms of windup and %d impacts", cs, repoSkills[c.skill].WindupMS, len(ms))
			}
			at, _ := landTicks(z, 1)
			for i, m := range ms {
				if at[m.id] != t0+c.ticks[i] {
					t.Errorf("monster %d (%v) hit at +%d, want +%d", i, c.offs[i], at[m.id]-t0, c.ticks[i])
				}
			}
			for _, im := range cs[0].Impacts {
				if want := int32((at[im.TargetId] - t0) * 100); im.ImpactMs != want {
					t.Errorf("impact %v, want %d ms", im, want)
				}
			}
		})
	}
}

// A dash moves the caster at the press and flies its own travel, not the
// distance to the target: from 3.5 tiles it travels 2.5, so 0.1 + 2.5/25 =
// 0.2 s (2 ticks), not 0.24 s (3).
func TestDashImpactTick(t *testing.T) {
	start := Vec2{30.5, 30.5}
	z, m, p, a := dashZone(t, start, Vec2{34, 30.5})
	castNow(t, z, 1, 1, "viem_1", m.id, Vec2{})
	t0 := z.tickNo
	envs := a.take()
	if d := distance(start, p.pos); !near32(d, 2.5) || m.hp != m.maxHP {
		t.Fatalf("dashed %v hp %d, want the caster moved 2.5 at the press and no damage", d, m.hp)
	}
	if mv := moveFor(lastTick(envs), 1); mv == nil || !mv.Snap {
		t.Fatalf("move %v, want a snap at the press", mv)
	}
	if cs := castsIn(envs); len(cs) != 1 || len(cs[0].Impacts) != 1 || cs[0].Impacts[0].ImpactMs != 200 || cs[0].WindupMs != 100 {
		t.Fatalf("casts %v, want 100 ms windup and one impact at 200 ms", cs)
	}
	if at, _ := landTicks(z, 1); at[m.id] != t0+2 {
		t.Fatalf("hit at +%d, want +2", at[m.id]-t0)
	}
}

// Impacts due on one tick land in the order they were made; the one that
// brings the monster to 0 is the killing blow, and the rest, that tick or
// later, are dropped: no Hit, no EXP.
func TestImpactOnKilledMonsterIsDropped(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.region.Kind.DetectRange = 0.5
	m.hp = 10 // one slash kills it
	v := joinNow(t, z, 9, 30.5, 36.5)
	joinNow(t, z, 1, 31.5, 30.5)
	joinNow(t, z, 2, 29.5, 30.5)
	joinNow(t, z, 3, 30.5, 31.5)
	z.Attack(1, 1, m.id)
	attackNow(t, z, 2, 1, m.id) // 1 and 2 land on the same tick, 1 first
	attackNow(t, z, 3, 1, m.id) // 3 lands one tick later
	v.take()
	settle(z)
	envs := v.take()
	hs := hitsIn(envs)
	if len(hs) != 1 || hs[0].AttackerId != 1 || hs[0].HpAfter != 0 || !m.dead || !slices.Equal(deathsIn(envs), []uint64{m.id}) {
		t.Fatalf("hits %v dead %v deaths %v, want player 1's slash the only hit and the kill", hs, m.dead, deathsIn(envs))
	}
	if p1, p2, p3 := z.players[1], z.players[2], z.players[3]; p1.exp != combat.ExpShare(testLevels[1].Exp, m.maxHP, 10) || p2.exp != 0 || p3.exp != 0 {
		t.Fatalf("exp %d/%d/%d, want only the killer paid", p1.exp, p2.exp, p3.exp)
	}
}

// An attacker who leaves before its impact still lands it, but earns no
// EXP, and the monster neither records nor chases it.
func TestImpactAfterAttackerLeaves(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.region.Kind.DetectRange = 0.5
	v := joinNow(t, z, 2, 30.5, 36.5)
	joinNow(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	v.take()
	settle(z)
	want := &pb.Hit{AttackerId: 1, TargetId: m.id, Damage: 14, HpAfter: 42}
	if hs := hitsIn(v.take()); len(hs) != 1 || hs[0].String() != want.String() || m.hp != 42 {
		t.Fatalf("hits %v hp %d, want %v", hs, m.hp, want)
	}
	if len(m.attackers) != 0 || len(m.dmgBy) != 0 || chasing(m) || m.target != 0 {
		t.Fatalf("attackers %v dmgBy %v state %v target %d, want the departed attacker ignored", m.attackers, m.dmgBy, m.state, m.target)
	}
}

// An attacker who dies before its impact still lands it and, being in the
// zone, earns its EXP; the monster does not record it, so it is not chased
// once respawned.
func TestImpactAfterAttackerDies(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.region.Kind.DetectRange = 0.5
	joinNow(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	z.damagePlayer(p, killer{}, 10000, false)
	settle(z)
	if m.hp != 42 || p.exp != combat.ExpShare(testLevels[1].Exp, m.maxHP, 14) {
		t.Fatalf("hp %d exp %d, want the slash landed and paid", m.hp, p.exp)
	}
	if len(m.attackers) != 0 || chasing(m) {
		t.Fatalf("attackers %v state %v, want the dead attacker not recorded or chased", m.attackers, m.state)
	}
}

// A player's impact can pay EXP to a player who died after the attack. If it
// levels the corpse up, the level-up must not refill its HP and MP: the
// corpse stays at 0 HP and respawns full anyway.
func TestImpactLevelsUpDeadAttacker(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	m.region.Kind.DetectRange = 0.5
	in := info(1, 31.5, 30.5)
	in.Exp = testExp.ToNext(1) - combat.ExpShare(testLevels[1].Exp, testLevels[1].HP, 14) // the slash's share reaches level 2
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	p.mp = 7
	z.damagePlayer(p, killer{}, 10000, false)
	a.take()
	settle(z)
	if p.level != 2 || !p.dead || p.hp != 0 || p.mp != 7 {
		t.Fatalf("level %d dead %v hp %d mp %d, want level 2 with the corpse at 0 HP and 7 MP", p.level, p.dead, p.hp, p.mp)
	}
	for _, c := range hpChangesIn(a.take()) {
		if c.Id == 1 && c.Hp != 0 {
			t.Fatalf("HpChange %v, want the corpse shown at 0 HP", c)
		}
	}
	if saves := z.saver.(*fakeSaver).all(); len(saves) != 1 || saves[0].Level != 2 || saves[0].HP != 0 {
		t.Fatalf("saves %+v, want the level-up saved with 0 HP", saves)
	}
	if !z.Respawn(1, 2, false) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if p.dead || p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("dead %v hp %d/%d mp %d/%d, want respawned full", p.dead, p.hp, p.maxHP, p.mp, p.stats.MaxMP)
	}
}

// ______________________TEST MONSTER IMPACTS_____________________

// A melee swing is decided when the monster's attack is due: its Cast goes
// out then, with the kind's 200 ms windup, and the Hit lands 2 ticks later.
// The cadence counts from the swing.
func TestMonsterMeleeLandsAtImpactTick(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 30.5, 31.5)
	v := joinNow(t, z, 2, 30.5, 36.5)
	engage(m, 1)
	z.advance(time.Now())
	t0 := z.tickNo
	want := &pb.Cast{CasterId: m.id, TargetId: 1, DirY: 1, WindupMs: 200, Impacts: []*pb.CastImpact{{TargetId: 1, ImpactMs: 200}}}
	for name, envs := range map[string][]*pb.Envelope{"target": a.take(), "viewer": v.take()} {
		if cs := castsIn(envs); len(cs) != 1 || cs[0].String() != want.String() {
			t.Fatalf("%s casts %v, want %v", name, cs, want)
		}
		if hs := hitsIn(envs); len(hs) != 0 {
			t.Fatalf("%s hits %v in the swing tick, want none", name, hs)
		}
	}
	p := z.players[1]
	if p.hp != 500 || m.nextAttack != t0+z.ticks(testKind.AttackSec) {
		t.Fatalf("hp %d nextAttack %d, want 500 and %d", p.hp, m.nextAttack, t0+z.ticks(testKind.AttackSec))
	}
	if n := settle(z); z.tickNo != t0+z.ticks(0.2) || n != 2 {
		t.Fatalf("landed on tick %d, want %d", z.tickNo, t0+z.ticks(0.2))
	}
	hit := &pb.Hit{AttackerId: m.id, TargetId: 1, Damage: 20, HpAfter: 480}
	for name, envs := range map[string][]*pb.Envelope{"target": a.take(), "viewer": v.take()} {
		if hs := hitsIn(envs); len(hs) != 1 || hs[0].String() != hit.String() {
			t.Fatalf("%s hits %v, want %v", name, hs, hit)
		}
	}
}

// shotInFlight has m shoot player 1, 5 tiles east of it, and returns once
// the shot is fired, with the tick it was fired on and player 1's outbox.
func shotInFlight(t *testing.T) (*Zone, *monster, *fakeOut, uint32) {
	t.Helper()
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 35.5, 30.5)
	z.startShoot(m)
	m.nextShot = z.tickNo + 1
	z.advance(time.Now())
	if len(z.impacts) != 1 {
		t.Fatalf("impacts %v, want the shot in flight", z.impacts)
	}
	return z, m, a, z.tickNo
}

// A shot's damage is fixed when it is fired; it flies at shot_speed and lands
// distance / speed later: 5 tiles at 15 tiles/s is 4 ticks.
func TestMonsterShotLandsAfterFlight(t *testing.T) {
	z, m, a, t0 := shotInFlight(t)
	envs := a.take()
	want := &pb.Cast{CasterId: m.id, TargetId: 1, DirX: 1, Ranged: true, Impacts: []*pb.CastImpact{{TargetId: 1, ImpactMs: 400}}}
	if cs := castsIn(envs); len(cs) != 1 || cs[0].String() != want.String() {
		t.Fatalf("casts %v, want %v", cs, want)
	}
	if hs := hitsIn(envs); len(hs) != 0 {
		t.Fatalf("hits %v in the firing tick, want none", hs)
	}
	p := z.players[1]
	p.stats.PhysDefense = 1000 // fixed at the shot: a later defense changes nothing
	settle(z)
	hit := &pb.Hit{AttackerId: m.id, TargetId: 1, Damage: 10, HpAfter: 490, Ranged: true}
	if hs := hitsIn(a.take()); z.tickNo != t0+z.ticks(5.0/15) || len(hs) != 1 || hs[0].String() != hit.String() {
		t.Fatalf("tick %d hits %v, want %v on tick %d", z.tickNo, hs, hit, t0+z.ticks(5.0/15))
	}
	if m.state != shoot || m.nextShot != t0+z.ticks(testKind.ShootSec) {
		t.Fatalf("state %v nextShot %d, want shooting again %d ticks after the shot", m.state, m.nextShot, z.ticks(testKind.ShootSec))
	}
}

// A shot on a player who dies or leaves before it lands is dropped.
func TestMonsterShotOnGonePlayerIsDropped(t *testing.T) {
	for _, gone := range []string{"dies", "leaves"} {
		t.Run(gone, func(t *testing.T) {
			z, m, a, _ := shotInFlight(t)
			v := joinNow(t, z, 2, 30.5, 36.5)
			p := z.players[1]
			if gone == "dies" {
				z.damagePlayer(p, killer{}, 10000, false)
			} else if err := <-z.leave(1); err != nil {
				t.Fatal(err)
			}
			a.take()
			settle(z)
			if len(z.impacts) != 0 {
				t.Fatalf("impacts %v, want the shot gone", z.impacts)
			}
			for _, envs := range [][]*pb.Envelope{a.take(), v.take()} {
				for _, h := range hitsIn(envs) {
					if h.AttackerId == m.id {
						t.Fatalf("hit %v, want the shot dropped", h)
					}
				}
			}
		})
	}
}

// A shot already fired lands even if the monster dies meanwhile, or the
// player steps into a safe zone: the safe-zone check was made when it fired.
func TestMonsterShotLandsAnyway(t *testing.T) {
	for _, c := range []string{"monster dies", "player in a safe zone"} {
		t.Run(c, func(t *testing.T) {
			z, m, a, _ := shotInFlight(t)
			if c == "monster dies" {
				m.hp = 0
				z.killMonster(m, nil)
			} else {
				teleport(z, 1, Vec2{5.5, 5.5})
				if !z.area.InSafe(z.players[1].pos) {
					t.Fatal("setup: not in the safe zone")
				}
			}
			a.take()
			settle(z)
			hit := &pb.Hit{AttackerId: m.id, TargetId: 1, Damage: 10, HpAfter: 490, Ranged: true}
			if hs := hitsIn(a.take()); len(hs) != 1 || hs[0].String() != hit.String() || z.players[1].hp != 490 {
				t.Fatalf("hits %v hp %d, want %v", hs, z.players[1].hp, hit)
			}
		})
	}
}

// ______________________TEST LOOT_____________________

// sureDrops is the real item data with the normal rank dropping one
// equipment piece and every stack on every kill.
var sureDrops = func() *gamedata.Items {
	d := *testItemData
	d.Drops = maps.Clone(d.Drops)
	n := d.Drops["normal"]
	n.Equipment.ChanceBP = 10000
	n.Stacks = slices.Clone(n.Stacks)
	for i := range n.Stacks {
		n.Stacks[i].ChanceBP = 10000
	}
	d.Drops["normal"] = n
	return &d
}()

// lootZone is newFightZone with item data d and a level-1 normal monster at
// (30.5, 30.5) that one slash kills and that never chases or respawns.
func lootZone(t *testing.T, d *gamedata.Items) (*Zone, *monster) {
	t.Helper()
	k := testKind
	k.Rank, k.DetectRange = "normal", 0.5
	z, m := newFightZoneOf(t, k, 1, Vec2{30.5, 30.5})
	z.area.Items = d
	m.hp = 10
	return z, m
}

// joinViem joins a level-1 Viêm, so that weapon drops have a class.
func joinViem(t *testing.T, z *Zone, id uint64, x, y float32) *fakeOut {
	t.Helper()
	in := info(id, x, y)
	in.Class = pb.CharClass_CHAR_CLASS_VIEM
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	return out
}

// groundOf is the zone's ground items by id.
func groundOf(z *Zone) []*groundItem {
	var gs []*groundItem
	for _, id := range slices.Sorted(maps.Keys(z.ground)) {
		gs = append(gs, z.ground[id])
	}
	return gs
}

// itemEnters are the EntityEnter messages of ITEM entities in envs by id.
func itemEnters(envs []*pb.Envelope) map[uint64]*pb.EntityEnter {
	got := map[uint64]*pb.EntityEnter{}
	for _, e := range envs {
		if en := e.GetEntityEnter(); en != nil && en.Kind == pb.EntityKind_ENTITY_KIND_ITEM {
			got[en.Id] = en
		}
	}
	return got
}

// The slash that kills pays the killer Xu in [3L, 5L] at once, and the
// next SelfStats carries it.
func TestKillPaysXu(t *testing.T) {
	z, m := lootZone(t, testItemData)
	a := joinViem(t, z, 1, 31.5, 30.5)
	attackLand(t, z, 1, 1, m.id)
	p := z.players[1]
	if !m.dead || p.xu < 3 || p.xu > 5 {
		t.Fatalf("dead %v xu %d, want a kill paying 3-5 Xu", m.dead, p.xu)
	}
	if ss := selfStatsIn(a.take()); len(ss) == 0 || ss[len(ss)-1].Xu != p.xu {
		t.Fatalf("SelfStats %v, want the last one carrying %d Xu", ss, p.xu)
	}
	// Each kill sets statsDirty and pays within the monster level's range.
	for _, lv := range []int{15, 90} {
		for range 200 {
			before := p.xu
			p.statsDirty = false
			z.loot(&monster{entity: entity{level: lv, pos: m.pos}, region: m.region}, p)
			if gain := p.xu - before; gain < int64(3*lv) || gain > int64(5*lv) || !p.statsDirty {
				t.Fatalf("level %d: gain %d dirty %v, want %d-%d Xu and statsDirty", lv, gain, p.statsDirty, 3*lv, 5*lv)
			}
		}
	}
}

// With sure drops a kill leaves one equipment piece and one stone stack
// 0.6 tiles from the corpse. Only the killer gets their EntityEnter, with
// the item filled; a player next to the corpse never does.
func TestDropsOnlyForTheKiller(t *testing.T) {
	z, m := lootZone(t, sureDrops)
	a := joinViem(t, z, 1, 31.5, 30.5)
	b := joinViem(t, z, 2, 29.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	a.take()
	b.take()
	settle(z)
	gs := groundOf(z)
	if !m.dead || len(gs) != 2 || !sureDrops.IsEquipment(gs[0].item.Code) || !reflect.DeepEqual(gs[1].item, items.Item{Code: "stone_enhance", Count: 1}) {
		t.Fatalf("dead %v ground %+v, want an equipment piece and one stone", m.dead, gs)
	}
	enA := itemEnters(a.take())
	for _, g := range gs {
		if g.kind != pb.EntityKind_ENTITY_KIND_ITEM || g.owner != 100 || g.level != 0 || g.hp != 0 ||
			math.Abs(dist(g.pos, m.pos)-0.6) > 1e-4 || g.expires != z.tickNo+z.ticks(60) {
			t.Fatalf("ground item %+v, want an ITEM of character 100, 0.6 from the corpse, gone in 60 s", g)
		}
		en := enA[g.id]
		if en == nil || en.Name != itemName(sureDrops, g.item.Code) || en.X != g.pos.X || en.Y != g.pos.Y ||
			!proto.Equal(en.Item, itemInfo(&g.item)) {
			t.Fatalf("killer's EntityEnter %v for %+v, want it with the item", en, g)
		}
	}
	if en := itemEnters(b.take()); len(en) != 0 {
		t.Fatalf("bystander got %v, want no item entities", en)
	}
	for range 5 {
		z.advance(time.Now())
	}
	if en := itemEnters(b.take()); len(en) != 0 || len(itemEnters(a.take())) != 0 {
		t.Fatalf("bystander got %v later, want nothing (and the killer no repeat)", en)
	}
}

// The killer is whoever lands the killing blow, not the first or the
// biggest attacker; the others get no Xu and no items.
func TestKillerIsTheFinishingBlow(t *testing.T) {
	z, m := lootZone(t, sureDrops)
	m.hp = 20
	joinViem(t, z, 1, 31.5, 30.5)
	joinViem(t, z, 2, 29.5, 30.5)
	attackLand(t, z, 2, 1, m.id) // 14 of 20
	attackLand(t, z, 1, 1, m.id) // the last 6
	p1, p2 := z.players[1], z.players[2]
	if !m.dead || p1.xu == 0 || p2.xu != 0 {
		t.Fatalf("dead %v xu %d/%d, want only player 1 paid", m.dead, p1.xu, p2.xu)
	}
	gs := groundOf(z)
	if len(gs) != 2 || gs[0].owner != 100 || gs[1].owner != 100 {
		t.Fatalf("ground %+v, want two items of character 100", gs)
	}
}

// A killer who has left before the killing impact lands gets nothing, and
// nothing is dropped.
func TestKillerGoneGetsNothing(t *testing.T) {
	z, m := lootZone(t, sureDrops)
	joinViem(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	settle(z)
	if !m.dead || len(z.ground) != 0 {
		t.Fatalf("dead %v ground %d, want the kill with no drop", m.dead, len(z.ground))
	}
}

// A killer who died before the killing impact lands still gets the Xu and
// the drop.
func TestDeadKillerGetsLoot(t *testing.T) {
	z, m := lootZone(t, sureDrops)
	joinViem(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	z.damagePlayer(p, killer{}, 10000, false)
	settle(z)
	if gs := groundOf(z); !p.dead || !m.dead || p.xu < 3 || p.xu > 5 || len(gs) != 2 || gs[0].owner != 100 {
		t.Fatalf("dead %v/%v xu %d ground %+v, want the dead killer paid and two drops", p.dead, m.dead, p.xu, gs)
	}
}

// Ground items last exactly z.ticks(60) ticks; then the owner gets
// EntityLeave for them.
func TestGroundItemsExpire(t *testing.T) {
	z, m := lootZone(t, sureDrops)
	a := joinViem(t, z, 1, 31.5, 30.5)
	attackNow(t, z, 1, 1, m.id)
	settle(z)
	z.regions[0].pending = nil // no respawn to fight meanwhile
	gs := groundOf(z)
	if len(gs) != 2 {
		t.Fatalf("ground %d, want 2", len(gs))
	}
	end := z.tickNo + z.ticks(60)
	a.take()
	for z.tickNo < end-1 {
		z.advance(time.Now())
	}
	if l := leaves(a.take()); len(z.ground) != 2 || slices.Contains(l, gs[0].id) || slices.Contains(l, gs[1].id) {
		t.Fatalf("ground %d leaves %v one tick early, want both items still there", len(z.ground), l)
	}
	z.advance(time.Now())
	l := leaves(a.take())
	if len(z.ground) != 0 || !slices.Contains(l, gs[0].id) || !slices.Contains(l, gs[1].id) {
		t.Fatalf("ground %d leaves %v at 60 s, want both gone with EntityLeave", len(z.ground), l)
	}
	if c := z.cells[gs[0].cell]; c[gs[0].id] != nil || c[gs[1].id] != nil {
		t.Fatal("expired items are still in the AOI grid")
	}
}

// Ownership is by character: the owner who leaves and comes back as a new
// entity within 60 s sees the items again.
func TestOwnerSeesItemsAfterRejoin(t *testing.T) {
	z, m := lootZone(t, sureDrops)
	joinViem(t, z, 1, 31.5, 30.5)
	attackLand(t, z, 1, 1, m.id)
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	in := info(5, 31.5, 30.5)
	in.CharID = 100
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	z.advance(time.Now())
	en := itemEnters(a.take())
	for _, g := range groundOf(z) {
		if en[g.id] == nil {
			t.Fatalf("enters %v, want item %d shown to its owner's new entity", en, g.id)
		}
	}
	if len(en) != 2 {
		t.Fatalf("enters %v, want two items", en)
	}
}

// n items go 0.6 tiles from the corpse at angles 2πi/n; one item, or one
// whose spot is a wall, goes on the corpse.
func TestDropAt(t *testing.T) {
	z, _ := lootZone(t, sureDrops)
	pos := Vec2{30.5, 30.5}
	near := func(a, b Vec2) bool { return dist(a, b) < 1e-4 }
	if got := z.dropAt(pos, 0, 1); got != pos {
		t.Fatalf("one item at %v, want the corpse %v", got, pos)
	}
	for _, c := range []struct {
		i, n int
		want Vec2
	}{{0, 2, Vec2{31.1, 30.5}}, {1, 2, Vec2{29.9, 30.5}}, {1, 4, Vec2{30.5, 31.1}}, {3, 4, Vec2{30.5, 29.9}}} {
		if got := z.dropAt(pos, c.i, c.n); !near(got, c.want) {
			t.Fatalf("item %d of %d at %v, want %v", c.i, c.n, got, c.want)
		}
	}
	wallAt(z, 31, 30)
	if got := z.dropAt(pos, 0, 2); got != pos {
		t.Fatalf("item on a wall at %v, want the corpse %v", got, pos)
	}
	if got := z.dropAt(pos, 1, 2); !near(got, Vec2{29.9, 30.5}) {
		t.Fatalf("item off the wall at %v, want (29.9, 30.5)", got)
	}
}

// ______________________TEST PVP ATTACKS_____________________

// pvpHome is where pvpZone's monster stands, away from the fights.
var pvpHome = Vec2{50.5, 50.5}

// pvpZone is newFightZone with PvP allowed (a training map, phase 5a spec 2),
// the repo skills and a level-12 monster at pvpHome that notices nobody.
func pvpZone(t *testing.T) (*Zone, *monster) {
	t.Helper()
	z, m := newFightZone(t, 12, pvpHome)
	z.area.Skills = repoSkills
	z.area.PvP = true
	m.region.Kind.DetectRange = 0.5
	return z, m
}

// joinFighter joins a level-12 player id of class at pos with PK on or off
// and skills learned.
func joinFighter(t *testing.T, z *Zone, id uint64, pos Vec2, class pb.CharClass, pk bool, skills map[string]int) *fakeOut {
	t.Helper()
	in := info(id, pos.X, pos.Y)
	in.Class, in.Level, in.PkOn, in.Skills = class, 12, pk, skills
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Each check of phase 5a spec 4.3 with a player target, in order: a case
// that breaks several checks is answered by the first. A refused slash
// starts no cooldown, lands nothing and locks nobody.
func TestPvPAttackValidation(t *testing.T) {
	a0, b0 := Vec2{30.5, 30.5}, Vec2{31.5, 30.5}
	aSafe, bSafe := Vec2{10.5, 5.5}, Vec2{9.5, 5.5} // tile 10 is outside the safe rect, tile 9 inside
	two := func(z *Zone) uint64 { return 2 }
	noPvP := func(z *Zone) uint64 { z.area.PvP = false; return 2 }
	cooling := func(z *Zone) uint64 { z.players[1].slashReady = z.tickNo + 5; return 2 }
	for _, c := range []struct {
		name     string
		aPK, bPK bool
		a, b     Vec2
		setup    func(z *Zone) uint64 // returns the target id
		code     pb.ErrorCode
	}{
		{"self", true, true, a0, b0, func(z *Zone) uint64 { return 1 }, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"a dead player", true, true, a0, b0, func(z *Zone) uint64 { z.players[2].dead = true; return 2 }, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"an area without PvP", true, true, a0, b0, noPvP, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
		{"target in a safe zone", true, true, aSafe, bSafe, two, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"both PK off", false, false, a0, b0, two, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
		{"2.1 tiles away", true, false, a0, Vec2{32.6, 30.5}, two, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"on cooldown", true, false, a0, b0, cooling, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN},
		{"attacker in a safe zone", true, true, bSafe, aSafe, two, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"no PvP, target in a safe zone, both PK off", false, false, aSafe, bSafe, noPvP, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
		{"target in a safe zone, both PK off", false, false, aSafe, bSafe, two, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"both PK off, far, on cooldown", false, false, a0, Vec2{33.5, 30.5}, cooling, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
		{"far and on cooldown", true, false, a0, Vec2{33.5, 30.5}, cooling, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _ := pvpZone(t)
			a := joinFighter(t, z, 1, c.a, pb.CharClass_CHAR_CLASS_VIEM, c.aPK, nil)
			joinFighter(t, z, 2, c.b, pb.CharClass_CHAR_CLASS_THUY, c.bPK, nil)
			target := c.setup(z)
			b, last := z.players[2], z.players[1].slashReady
			hp := b.hp
			a.take()
			attackNow(t, z, 1, 42, target)
			settle(z)
			envs := a.take()
			if er := errorFor(envs, 42); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v with seq 42", er, c.code)
			}
			if attackRespAt(envs, 42) >= 0 || b.hp != hp || len(hitsIn(envs)) != 0 {
				t.Fatalf("a refused slash landed: hp %d, want %d", b.hp, hp)
			}
			if got := z.players[1].slashReady; got != last {
				t.Fatalf("slashReady %d, want %d", got, last)
			}
			if z.players[1].pvpUntil != 0 || b.pvpUntil != 0 {
				t.Fatal("a refused slash locked PK")
			}
		})
	}
}

// A (PK on) slashes B (PK off): the damage is PvPDamage against B's physical
// defense, fixed at the press and landing at the impact tick; both see the
// Hit and both get the 10 s PvP lock from that tick. B, PK off, may hit
// back.
func TestPvPSlashHits(t *testing.T) {
	z, _ := pvpZone(t)
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	b := joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, false, nil)
	z.advance(time.Now())
	a.take()
	b.take()
	A, B := z.players[1], z.players[2]
	B.stats.MagicDefense += 200 // so that hitting the wrong defense shows
	want := combat.PvPDamage(combat.SlashBase(A.stats), B.stats.PhysDefense, A.level)
	if want == combat.PvPDamage(combat.SlashBase(A.stats), B.stats.MagicDefense, A.level) {
		t.Fatal("setup: physical and magic defense give the same damage")
	}
	hp := B.hp
	attackNow(t, z, 1, 5, 2)
	envs := a.take()
	if attackRespAt(envs, 5) < 0 {
		t.Fatalf("slash refused: %v", errorFor(envs, 5))
	}
	if cs := castsBy(envs, 1); len(cs) != 1 || cs[0].TargetId != 2 || !slices.Equal(impactIDs(cs[0]), []uint64{2}) {
		t.Fatalf("casts %v, want one at player 2", cs)
	}
	if B.hp != hp || len(hitsIn(envs)) != 0 {
		t.Fatal("the slash landed at the press")
	}
	settle(z)
	if B.hp != hp-want {
		t.Fatalf("hp %d, want %d - %d", B.hp, hp, want)
	}
	lock := z.tickNo + z.ticks(pvpLockSec)
	if A.pvpUntil != lock || B.pvpUntil != lock {
		t.Fatalf("pvpUntil %d and %d, want %d", A.pvpUntil, B.pvpUntil, lock)
	}
	h := &pb.Hit{AttackerId: 1, TargetId: 2, Damage: int32(want), HpAfter: int32(B.hp)}
	for name, out := range map[string]*fakeOut{"attacker": a, "target": b} {
		if hs := hitsIn(out.take()); len(hs) != 1 || hs[0].String() != h.String() {
			t.Fatalf("%s got hits %v, want %v", name, hs, h)
		}
	}
	ahp := A.hp
	attackLand(t, z, 2, 6, 1)
	if back := combat.PvPDamage(combat.SlashBase(B.stats), A.stats.PhysDefense, B.level); A.hp != ahp-back {
		t.Fatalf("A hp %d, want %d - %d from B hitting back", A.hp, ahp, back)
	}
}

// The PK rules are checked at the press only (phase 5a spec 1.3): a target
// who walks into a safe zone or turns PK off before the impact is still
// hit. One who leaves the zone is not, but the attacker keeps the lock it got
// at the press. An attacker who leaves still lands its hit, which locks the
// target.
func TestPvPImpactAfterPress(t *testing.T) {
	for _, c := range []struct {
		name     string
		aPK, bPK bool
		a, b     Vec2
		after    func(t *testing.T, z *Zone)
		hit      bool
	}{
		{"target walks into a safe zone", true, false, Vec2{11.5, 5.5}, Vec2{10.5, 5.5},
			func(t *testing.T, z *Zone) { teleport(z, 2, Vec2{9.5, 5.5}) }, true},
		{"target turns PK off", false, true, Vec2{30.5, 30.5}, Vec2{31.5, 30.5},
			func(t *testing.T, z *Zone) { z.players[2].pkOn = false }, true},
		{"attacker leaves", true, false, Vec2{30.5, 30.5}, Vec2{31.5, 30.5},
			func(t *testing.T, z *Zone) {
				if err := <-z.leave(1); err != nil {
					t.Fatal(err)
				}
			}, true},
		{"target leaves", true, false, Vec2{30.5, 30.5}, Vec2{31.5, 30.5},
			func(t *testing.T, z *Zone) {
				if err := <-z.leave(2); err != nil {
					t.Fatal(err)
				}
			}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _ := pvpZone(t)
			joinFighter(t, z, 1, c.a, pb.CharClass_CHAR_CLASS_VIEM, c.aPK, nil)
			v := joinFighter(t, z, 3, Vec2{c.b.X, c.b.Y + 3}, pb.CharClass_CHAR_CLASS_VIEM, false, nil) // a viewer
			joinFighter(t, z, 2, c.b, pb.CharClass_CHAR_CLASS_THUY, c.bPK, nil)
			A, B := z.players[1], z.players[2]
			hp := B.hp
			attackNow(t, z, 1, 5, 2)
			c.after(t, z)
			v.take()
			settle(z)
			hs := hitsIn(v.take())
			if c.hit != (len(hs) == 1 && hs[0].TargetId == 2 && B.hp < hp) || (!c.hit && len(hs) != 0) {
				t.Fatalf("hits %v hp %d of %d, want hit=%v", hs, B.hp, hp, c.hit)
			}
			lock := z.tickNo + z.ticks(pvpLockSec)
			if c.hit && B.pvpUntil != lock {
				t.Fatalf("target pvpUntil %d, want %d", B.pvpUntil, lock)
			}
			if z.players[1] != nil && A.pvpUntil != lock {
				t.Fatalf("attacker pvpUntil %d, want %d", A.pvpUntil, lock)
			}
		})
	}
}

// Two slashes on B due the same tick: the first brings it to 0 HP and kills
// it, the second is dropped.
func TestPvPSecondImpactOnKilledIsDropped(t *testing.T) {
	z, _ := pvpZone(t)
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	joinFighter(t, z, 3, Vec2{32.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, false, nil)
	B := z.players[2]
	B.hp = 1
	z.Attack(1, 5, 2)
	z.Attack(3, 5, 2)
	z.advance(time.Now())
	if len(z.impacts) != 2 || z.impacts[0].due != z.impacts[1].due {
		t.Fatalf("impacts %v, want two due the same tick", z.impacts)
	}
	settle(z)
	envs := a.take()
	if hs := hitsIn(envs); len(hs) != 1 || hs[0].AttackerId != 1 || hs[0].HpAfter != 0 {
		t.Fatalf("hits %v, want only player 1's killing hit", hs)
	}
	if !B.dead || !slices.Equal(deathsIn(envs), []uint64{2}) {
		t.Fatalf("dead %v deaths %v, want B dead once", B.dead, deathsIn(envs))
	}
}

// The PK-off lock starts when a PvP hit is accepted at the press (phase 5a
// spec 1.3): an attacker cannot press and turn PK off before the impact
// lands (hit-and-hide), nor can a PK-on target during the windup. The lock
// runs to pvpLockSec after the impact is due, and landing keeps it there.
func TestPvPLockStartsAtPress(t *testing.T) {
	z, _ := pvpZone(t)
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	b := joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, true, nil)
	A, B := z.players[1], z.players[2]
	hp := B.hp
	attackNow(t, z, 1, 5, 2)
	if attackRespAt(a.take(), 5) < 0 || len(z.impacts) != 1 {
		t.Fatal("slash refused")
	}
	due := z.impacts[0].due
	lock := due + z.ticks(pvpLockSec)
	if A.pvpUntil != lock || B.pvpUntil != lock {
		t.Fatalf("pvpUntil %d and %d at the press, want %d", A.pvpUntil, B.pvpUntil, lock)
	}
	b.take()
	if !z.SetPk(1, 6, false) || !z.SetPk(2, 7, false) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now()) // the slash is still in flight
	if B.hp != hp {
		t.Fatal("setup: the slash landed before the switches")
	}
	for name, c := range map[string]struct {
		out *fakeOut
		seq uint32
		p   *player
	}{"attacker": {a, 6, A}, "target": {b, 7, B}} {
		if er := errorFor(c.out.take(), c.seq); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT || !c.p.pkOn {
			t.Fatalf("%s: error %v pk %v, want IN_PVP_COMBAT and still on", name, er, c.p.pkOn)
		}
	}
	settle(z)
	if B.hp >= hp || A.pvpUntil != lock || B.pvpUntil != lock {
		t.Fatalf("hp %d of %d, pvpUntil %d and %d, want the hit and %d", B.hp, hp, A.pvpUntil, B.pvpUntil, lock)
	}
}

// PvP leaves monsters alone (phase 5a spec 4.6): a monster chasing A keeps
// A when B hits A, and records nothing about B.
func TestPvPDoesNotTurnMonster(t *testing.T) {
	z, m := newFightZone(t, 12, Vec2{30.5, 33.5})
	z.area.PvP = true
	m.region.Kind.DetectRange = 0.5
	joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, false, nil)
	joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	engage(m, 1)
	hp := z.players[1].hp
	attackLand(t, z, 2, 5, 1)
	for range 5 {
		z.advance(time.Now())
	}
	if z.players[1].hp >= hp {
		t.Fatal("B's slash did not land")
	}
	if _, ok := m.attackers[2]; ok || m.target != 1 || m.dmgBy[2] != 0 {
		t.Fatalf("target %d attackers %v dmgBy %v, want the monster still on A and B unrecorded", m.target, m.attackers, m.dmgBy)
	}
}

// ______________________TEST PVP SKILLS_____________________

// A single magic skill on a player hits against its magic defense, with the
// skill's id on the Hit and the player listed in the Cast.
func TestPvPCastSingleUsesMagicDefense(t *testing.T) {
	z, _ := pvpZone(t)
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, true, castSkills)
	joinFighter(t, z, 2, Vec2{33.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, false, nil)
	A, B := z.players[1], z.players[2]
	B.stats.PhysDefense += 200 // so that hitting the wrong defense shows
	s := repoSkills["thuy_1"]
	base := combat.SkillBase(A.stats, true, s.CoefPct[0])
	want := combat.PvPDamage(base, B.stats.MagicDefense, A.level)
	if want == combat.PvPDamage(base, B.stats.PhysDefense, A.level) {
		t.Fatal("setup: physical and magic defense give the same damage")
	}
	hp := B.hp
	a.take()
	castNow(t, z, 1, 5, "thuy_1", 2, Vec2{})
	envs := a.take()
	if castRespAt(envs, 5) < 0 {
		t.Fatalf("cast refused: %v", errorFor(envs, 5))
	}
	if cs := castsBy(envs, 1); len(cs) != 1 || cs[0].TargetId != 2 || !slices.Equal(impactIDs(cs[0]), []uint64{2}) {
		t.Fatalf("casts %v, want one at player 2", cs)
	}
	settle(z)
	if hs := hitsBy(a.take(), 1); len(hs) != 1 || hs[0].TargetId != 2 || hs[0].SkillId != "thuy_1" || int(hs[0].Damage) != want || B.hp != hp-want {
		t.Fatalf("hits %v hp %d, want one thuy_1 hit of %d", hs, B.hp, want)
	}
	if lock := z.tickNo + z.ticks(pvpLockSec); A.pvpUntil != lock || B.pvpUntil != lock {
		t.Fatalf("pvpUntil %d and %d, want %d", A.pvpUntil, B.pvpUntil, lock)
	}
}

// single and circle_target need a valid primary target, so a player who may
// not be hit is answered with the error of phase 5a spec 4.2; nothing is
// spent.
func TestPvPCastTargetErrors(t *testing.T) {
	a0, b0 := Vec2{30.5, 30.5}, Vec2{33.5, 30.5}
	aSafe, bSafe := Vec2{12.5, 5.5}, Vec2{9.5, 5.5}
	for _, skill := range []string{"thuy_1", "phong_4"} {
		for _, c := range []struct {
			name     string
			aPK, bPK bool
			a, b     Vec2
			target   uint64
			noPvP    bool
			code     pb.ErrorCode
		}{
			{"self", true, true, a0, b0, 1, false, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
			{"both PK off", false, false, a0, b0, 2, false, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
			{"no PvP", true, true, a0, b0, 2, true, pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN},
			{"target in a safe zone", true, false, aSafe, bSafe, 2, false, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
			{"4.6 tiles away", true, false, a0, Vec2{35.1, 30.5}, 2, false, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		} {
			t.Run(skill+" "+c.name, func(t *testing.T) {
				if skill == "thuy_1" && c.code == pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE {
					t.Skip("thuy_1 reaches 5.5 tiles")
				}
				z, _ := pvpZone(t)
				z.area.PvP = !c.noPvP
				class := classes.ToPB(repoSkills[skill].Class)
				a := joinFighter(t, z, 1, c.a, class, c.aPK, map[string]int{skill: 1})
				joinFighter(t, z, 2, c.b, pb.CharClass_CHAR_CLASS_VIEM, c.bPK, nil)
				A, B := z.players[1], z.players[2]
				mp, hp := A.mp, B.hp
				a.take()
				castNow(t, z, 1, 5, skill, c.target, Vec2{1, 0})
				settle(z)
				envs := a.take()
				if er := errorFor(envs, 5); er == nil || er.Code != c.code {
					t.Fatalf("error %v, want %v", er, c.code)
				}
				if A.mp != mp || A.skillReady[skill] != 0 || B.hp != hp || len(hitsIn(envs)) != 0 {
					t.Fatalf("mp %d of %d ready %d hp %d of %d: a refused cast spent or landed", A.mp, mp, A.skillReady[skill], B.hp, hp)
				}
			})
		}
	}
}

// A cone naming a player who may not be hit fires along dir as if no target
// were named and does not hit that player; naming one who may be hit aims
// at them.
func TestPvPConeNamedPlayer(t *testing.T) {
	for _, pk := range []bool{false, true} {
		z, _, a, ms := areaZone(t, "thuy_2", 1, Vec2{0, -3}) // a monster 3 tiles north
		z.area.PvP = true
		joinFighter(t, z, 2, Vec2{areaCaster.X + 2, areaCaster.Y}, pb.CharClass_CHAR_CLASS_VIEM, pk, nil) // 2 tiles east
		B := z.players[2]
		hp := B.hp
		castLand(t, z, 1, 5, "thuy_2", 2, Vec2{0, -1})
		envs := a.take()
		if castRespAt(envs, 5) < 0 {
			t.Fatalf("pk %v: cast refused: %v", pk, errorFor(envs, 5))
		}
		want, cast := idsOf(ms[0]), &pb.Cast{CasterId: 1, SkillId: "thuy_2", DirY: -1, Level: 1}
		if pk {
			want, cast = []uint64{2}, &pb.Cast{CasterId: 1, SkillId: "thuy_2", TargetId: 2, DirX: 1, Level: 1}
		}
		if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), want) || (B.hp < hp) != pk {
			t.Fatalf("pk %v: hits %v hp %d of %d, want %v", pk, targetIDs(hs), B.hp, hp, want)
		}
		if cs := castsBy(envs, 1); len(cs) != 1 || untimed(cs[0]).String() != cast.String() {
			t.Fatalf("pk %v: casts %v, want %v", pk, cs, cast)
		}
	}
}

// A self-centred circle by a PK-off caster hits monsters and PK-on players,
// never PK-off ones, all under one target cap and one order: nearest to the
// centre first, whatever their kind. Player damage is PvPDamage against
// magic defense.
func TestPvPCircleSelfMixed(t *testing.T) {
	for _, c := range []struct {
		name     string
		lv       int
		monsters []Vec2
		on, off  Vec2  // where the PK-on and PK-off players stand
		want     []int // indexes into the monsters, -1 for the PK-on player
	}{
		{"level 7, cap 4", 7, []Vec2{{2, 0}, {-2, 0}}, Vec2{0, 1.5}, Vec2{0, -1}, []int{-1, 0, 1}},
		{"level 1, cap 2", 1, []Vec2{{1, 0}, {-2, 0}}, Vec2{0, 1.5}, Vec2{0, -0.5}, []int{0, -1}},
		{"level 1, player nearest", 1, []Vec2{{1, 0}, {-2, 0}}, Vec2{0, 0.8}, Vec2{0, -0.5}, []int{-1, 0}},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, a, ms := areaZone(t, "thuy_4", c.lv, c.monsters...)
			z.area.PvP = true
			at := func(o Vec2) Vec2 { return Vec2{areaCaster.X + o.X, areaCaster.Y + o.Y} }
			joinFighter(t, z, 2, at(c.on), pb.CharClass_CHAR_CLASS_VIEM, true, nil)
			joinFighter(t, z, 3, at(c.off), pb.CharClass_CHAR_CLASS_VIEM, false, nil)
			C, D := z.players[2], z.players[3]
			C.stats.PhysDefense += 200 // so that hitting the wrong defense shows
			chp, dhp := C.hp, D.hp
			castNow(t, z, 1, 5, "thuy_4", 0, Vec2{})
			var want []uint64
			for _, i := range c.want {
				if i < 0 {
					want = append(want, 2)
				} else {
					want = append(want, ms[i].id)
				}
			}
			envs := a.take()
			if cs := castsBy(envs, 1); len(cs) != 1 || !slices.Equal(impactIDs(cs[0]), want) {
				t.Fatalf("casts %v, want impacts on %v", cs, want)
			}
			settle(z)
			if hs := hitsBy(a.take(), 1); !slices.Equal(targetIDs(hs), want) {
				t.Fatalf("hits %v, want %v", targetIDs(hs), want)
			}
			s := repoSkills["thuy_4"]
			dmg := combat.PvPDamage(combat.SkillBase(p.stats, true, s.CoefPct[c.lv-1]), C.stats.MagicDefense, p.level)
			if C.hp != chp-dmg || D.hp != dhp {
				t.Fatalf("PK-on hp %d of %d (want -%d), PK-off hp %d of %d (want untouched)", C.hp, chp, dmg, D.hp, dhp)
			}
		})
	}
}

// A circle never hits a player standing in a safe zone, nor anyone in an
// area without PvP.
func TestPvPCircleSelfSkipsSafeAndNoPvP(t *testing.T) {
	for _, noPvP := range []bool{false, true} {
		z, _, a, _ := areaZone(t, "thuy_4", 1, Vec2{20, 20})
		z.area.PvP = !noPvP
		teleport(z, 1, Vec2{11.5, 5.5})
		joinFighter(t, z, 2, Vec2{9.6, 5.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)  // in the safe zone, 1.9 away
		joinFighter(t, z, 3, Vec2{12.5, 6.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil) // outside, 1.4 away
		castLand(t, z, 1, 5, "thuy_4", 2, Vec2{})
		envs := a.take()
		if castRespAt(envs, 5) < 0 {
			t.Fatalf("noPvP %v: cast refused: %v", noPvP, errorFor(envs, 5))
		}
		var want []uint64
		if !noPvP {
			want = []uint64{3}
		}
		if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), want) {
			t.Fatalf("noPvP %v: hits %v, want %v", noPvP, targetIDs(hs), want)
		}
	}
}

// A dash at a player lands 1 tile short of them, as at a monster, and hits
// against physical defense.
func TestPvPDashOnPlayer(t *testing.T) {
	start := Vec2{30.5, 30.5}
	z, _, p, a := dashZone(t, start, pvpHome)
	z.area.PvP = true
	p.pkOn = true
	off := polar(3.5, 30)
	joinFighter(t, z, 2, Vec2{start.X + off.X, start.Y + off.Y}, pb.CharClass_CHAR_CLASS_THUY, false, nil)
	B := z.players[2]
	B.stats.MagicDefense += 200 // so that hitting the wrong defense shows
	hp := B.hp
	castNow(t, z, 1, 5, "viem_1", 2, Vec2{})
	if d := distance(p.pos, B.pos); !near32(d, 1) {
		t.Fatalf("landed %v, %v from the player, want 1", p.pos, d)
	}
	settle(z)
	want := combat.PvPDamage(combat.SkillBase(p.stats, false, repoSkills["viem_1"].CoefPct[0]), B.stats.PhysDefense, p.level)
	if hs := hitsBy(a.take(), 1); len(hs) != 1 || hs[0].TargetId != 2 || hs[0].SkillId != "viem_1" || B.hp != hp-want {
		t.Fatalf("hits %v hp %d of %d, want one viem_1 hit of %d", hs, B.hp, hp, want)
	}
}

// ______________________TEST PK DEATH_____________________

// deathNoticesIn lists the DeathNotice envelopes in envs and the index of
// each, so a test can check the order against WorldTick and SelfStats.
func deathNoticesIn(envs []*pb.Envelope) (ns []*pb.DeathNotice, at []int) {
	for i, e := range envs {
		if d := e.GetDeathNotice(); d != nil {
			ns, at = append(ns, d), append(at, i)
		}
	}
	return
}

// pkKill has player 1 (PK on, level 12) kill player 2 with one slash in a
// PvP zone: B stands at the given level and EXP, with PK on or off, and the
// area has the PK penalty or not. It returns the zone, both outboxes and the
// killer's name.
func pkKill(t *testing.T, level int, exp int64, bPK, penalty bool) (*Zone, *fakeOut, *fakeOut) {
	t.Helper()
	z, _ := pvpZone(t)
	z.area.PkPenalty = penalty
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	b := joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, bPK, nil)
	B := z.players[2]
	B.level, B.exp, B.hp = level, exp, 1
	z.players[1].name = "Kaito"
	a.take()
	b.take()
	attackLand(t, z, 1, 5, 2)
	if !B.dead {
		t.Fatal("B did not die")
	}
	return z, a, b
}

// pkLoss10 is 5% of the EXP to leave level 10 (spec 4.7).
var pkLoss10 = testExp.ToNext(10) * 5 / 100

func TestPkDeathPenalty(t *testing.T) {
	for _, c := range []struct {
		name     string
		level    int
		bPK, pen bool
		want     int64 // exp after, from 1000
		saves    int
	}{
		{"PK on, level 10, training", 10, true, true, 1000 - pkLoss10, 1},
		{"level 90 loses nothing and saves nothing", 90, true, true, 1000, 0},
		{"PK off", 10, false, true, 1000, 0},
		{"PvP area without the penalty", 10, true, false, 1000, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _, _ := pkKill(t, c.level, 1000, c.bPK, c.pen)
			saves := z.saver.(*fakeSaver).all()
			if got := z.players[2].exp; got != c.want {
				t.Fatalf("exp %d, want %d", got, c.want)
			}
			if len(saves) != c.saves || (c.saves == 1 && saves[0].Exp != c.want) {
				t.Fatalf("saves %+v, want %d with exp %d", saves, c.saves, c.want)
			}
		})
	}
}

// The penalty needs a player's blow: a monster's kills PK-on B without cost
// and tells B the monster's name.
func TestPkDeathByMonsterCostsNothing(t *testing.T) {
	z, m := pvpZone(t)
	z.area.PkPenalty = true
	b := joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, true, nil)
	B := z.players[2]
	B.level, B.exp = 10, 1000
	b.take()
	z.monsterAttack(m, B, 100000, 0, 0, false)
	settle(z)
	if !B.dead || B.exp != 1000 || len(z.saver.(*fakeSaver).all()) != 0 {
		t.Fatalf("dead %v exp %d, want dead with the EXP kept and no save", B.dead, B.exp)
	}
	ns, _ := deathNoticesIn(b.take())
	if len(ns) != 1 || ns[0].KillerId != m.id || ns[0].KillerName != testKind.Name || ns[0].ByPlayer || ns[0].ExpLost != 0 {
		t.Fatalf("notices %v, want the monster's id and name, not by a player, nothing lost", ns)
	}
}

// The killer's name comes from the impact: A may have left the zone before it
// lands, and the kill is still a player's.
func TestPkDeathKillerGone(t *testing.T) {
	z, _ := pvpZone(t)
	z.area.PkPenalty = true
	joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	b := joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, true, nil)
	B := z.players[2]
	B.level, B.exp, B.hp = 10, 1000, 1
	z.players[1].name = "Kaito"
	attackNow(t, z, 1, 5, 2)
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	b.take()
	settle(z)
	if !B.dead || B.exp != 1000-pkLoss10 {
		t.Fatalf("dead %v exp %d, want dead and the penalty charged", B.dead, B.exp)
	}
	ns, _ := deathNoticesIn(b.take())
	if len(ns) != 1 || ns[0].KillerId != 1 || ns[0].KillerName != "Kaito" || !ns[0].ByPlayer || ns[0].ExpLost != pkLoss10 {
		t.Fatalf("notices %v, want Kaito's, by a player, the penalty lost", ns)
	}
}

// The notice goes to B alone, between B's WorldTick and its SelfStats of the
// tick, once.
func TestPkDeathNotice(t *testing.T) {
	z, _ := pvpZone(t)
	z.area.PkPenalty = true
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	b := joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, true, nil)
	B := z.players[2]
	B.level, B.exp, B.hp = 10, 1000, 1
	z.players[1].name = "Kaito"
	attackNow(t, z, 1, 5, 2)
	a.take()
	b.take()
	settle(z)
	if ns, _ := deathNoticesIn(a.take()); len(ns) != 0 {
		t.Fatalf("the killer got %v", ns)
	}
	envs := b.take()
	ns, at := deathNoticesIn(envs)
	if len(ns) != 1 || ns[0].KillerId != 1 || ns[0].KillerName != "Kaito" || !ns[0].ByPlayer || ns[0].ExpLost != pkLoss10 {
		t.Fatalf("notices %v, want Kaito's, by a player, the penalty lost", ns)
	}
	i := at[0]
	if envs[i-1].GetWorldTick() == nil || len(deathsIn(envs[i-1:i])) != 1 || envs[i+1].GetSelfStats().GetExp() != 1000-pkLoss10 || envs[i].Seq != 0 {
		t.Fatalf("envelopes around the notice %v, want the death's WorldTick, the notice, then SelfStats with the new EXP", envs)
	}
	z.advance(time.Now())
	if ns, _ := deathNoticesIn(b.take()); len(ns) != 0 {
		t.Fatal("the notice was sent twice")
	}
}

// Respawning, in place or at the village, is free and gives the EXP back
// nothing.
func TestPkDeathRespawnKeepsLoss(t *testing.T) {
	z, _, _ := pkKill(t, 10, 1000, true, true)
	z.respawn(respawnCmd{id: 2, seq: 9})
	B := z.players[2]
	if B.dead || B.exp != 1000-pkLoss10 {
		t.Fatalf("dead %v exp %d, want alive at %d", B.dead, B.exp, 1000-pkLoss10)
	}
}

// ____TEST PARTY EXP____

// partied puts the players of ids into one party cell with id pid.
func partied(z *Zone, pid uint64, ids ...uint64) {
	cell := new(atomic.Uint64)
	cell.Store(pid)
	for _, id := range ids {
		z.players[id].party = cell
	}
}

// partyExpZone is pvpZone with three level-12 fighters: 1 and 2 in party 7,
// 3 in no party. Their EXP is cleared.
func partyExpZone(t *testing.T) (*Zone, *monster) {
	z, m := pvpZone(t)
	for id := uint64(1); id <= 3; id++ {
		joinFighter(t, z, id, Vec2{30.5 + float32(id), 30.5}, pb.CharClass_CHAR_CLASS_VIEM, false, nil)
	}
	partied(z, 7, 1, 2)
	return z, m
}

// A teammate who never attacked gets 5% of the whole EXP, and the killer all
// of it; the bonus is not counted against the monster.
func TestPartyExpBonus(t *testing.T) {
	z, m := partyExpZone(t)
	total := testLevels[12].Exp
	z.payExpShare(z.players[1], m, m.maxHP)
	if p := z.players[1]; p.exp != total {
		t.Fatalf("killer exp %d, want %d", p.exp, total)
	}
	if want := 5 * total / 100; z.players[2].exp != want || want == 0 {
		t.Fatalf("teammate exp %d, want %d", z.players[2].exp, want)
	}
	if z.players[3].exp != 0 {
		t.Fatalf("player with no party got %d", z.players[3].exp)
	}
	if _, ok := m.dmgBy[2]; ok || len(m.expPaid) != 1 || m.expPaid[1] != total {
		t.Fatalf("bonus touched the tables: dmgBy %v expPaid %v", m.dmgBy, m.expPaid)
	}
}

// Another party, no party, and a teammate 11 levels from the monster get
// nothing.
func TestPartyExpWhoGetsNothing(t *testing.T) {
	z, m := partyExpZone(t)
	joinFighter(t, z, 4, Vec2{40.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, false, nil)
	joinFighter(t, z, 5, Vec2{41.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, false, nil)
	partied(z, 7, 4)
	partied(z, 8, 5)
	z.players[4].level = 1 // 11 levels from the monster
	z.payExpShare(z.players[1], m, m.maxHP)
	for _, id := range []uint64{3, 4, 5} {
		if z.players[id].exp != 0 || z.players[id].level != map[uint64]int{3: 12, 4: 1, 5: 12}[id] {
			t.Fatalf("player %d level %d exp %d, want nothing", id, z.players[id].level, z.players[id].exp)
		}
	}
}

// A dead teammate still gets the bonus and stays at 0 HP.
func TestPartyExpDeadTeammate(t *testing.T) {
	z, m := partyExpZone(t)
	T := z.players[2]
	T.dead, T.hp = true, 0
	z.payExpShare(z.players[1], m, m.maxHP)
	if T.exp != 5*testLevels[12].Exp/100 || T.hp != 0 || !T.dead {
		t.Fatalf("exp %d hp %d dead %v, want the bonus at 0 HP", T.exp, T.hp, T.dead)
	}
}

// A hit outside the level window pays nobody, teammates included.
func TestPartyExpNoGainNoBonus(t *testing.T) {
	z, m := partyExpZone(t)
	z.players[1].level = 1
	z.payExpShare(z.players[1], m, m.maxHP)
	if z.players[1].exp != 0 || z.players[2].exp != 0 {
		t.Fatalf("exp %d and %d, want 0 and 0", z.players[1].exp, z.players[2].exp)
	}
}

// Two teammates who both hit each get their own share plus 5% of the
// other's, with the remainders carried.
func TestPartyExpBothHit(t *testing.T) {
	z, m := partyExpZone(t)
	A, T := z.players[1], z.players[2]
	var carryA, carryT, wantA, wantT int64
	var paidA, paidT int64
	for _, c := range []struct {
		p       *player
		counted int
	}{{A, 100}, {T, 60}, {A, 50}, {T, 70}} {
		z.payExpShare(c.p, m, c.counted)
		share := combat.ExpShare(testLevels[12].Exp, m.maxHP, m.dmgBy[c.p.id])
		var gain, bonus int64
		if c.p == A {
			gain, paidA = share-paidA, share
			wantA += gain
			bonus, carryT = combat.PartyBonus(carryT, gain)
			wantT += bonus
		} else {
			gain, paidT = share-paidT, share
			wantT += gain
			bonus, carryA = combat.PartyBonus(carryA, gain)
			wantA += bonus
		}
	}
	if A.exp != wantA || T.exp != wantT || A.partyCarry != carryA || T.partyCarry != carryT {
		t.Fatalf("exp %d/%d carry %d/%d, want exp %d/%d carry %d/%d", A.exp, T.exp, A.partyCarry, T.partyCarry, wantA, wantT, carryA, carryT)
	}
	if A.exp <= m.expPaid[1] || T.exp <= m.expPaid[2] {
		t.Fatalf("exp %d/%d not above own shares %d/%d", A.exp, T.exp, m.expPaid[1], m.expPaid[2])
	}
}

// ____TEST PARTY PK____

// Two teammates with PK on cannot hit each other with a slash or a single
// skill; a self circle skips the teammate and hits a PK-on stranger.
func TestPartyPKForbidden(t *testing.T) {
	z, _ := pvpZone(t)
	a := joinFighter(t, z, 1, Vec2{30.5, 30.5}, pb.CharClass_CHAR_CLASS_THUY, true, castSkills)
	joinFighter(t, z, 2, Vec2{31.5, 30.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	joinFighter(t, z, 3, Vec2{30.5, 31.5}, pb.CharClass_CHAR_CLASS_VIEM, true, nil)
	partied(z, 7, 1, 2)
	hp2, hp3 := z.players[2].hp, z.players[3].hp
	a.take()
	attackNow(t, z, 1, 5, 2)
	if er := errorFor(a.take(), 5); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN {
		t.Fatalf("slash error %v, want PK_FORBIDDEN", er)
	}
	castNow(t, z, 1, 6, "thuy_1", 2, Vec2{})
	if er := errorFor(a.take(), 6); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN {
		t.Fatalf("single error %v, want PK_FORBIDDEN", er)
	}
	castNow(t, z, 1, 7, "thuy_4", 0, Vec2{})
	envs := a.take()
	settle(z)
	envs = append(envs, a.take()...)
	if castRespAt(envs, 7) < 0 {
		t.Fatalf("circle refused: %v", errorFor(envs, 7))
	}
	if hs := hitsBy(envs, 1); !slices.Equal(targetIDs(hs), []uint64{3}) || z.players[2].hp != hp2 || z.players[3].hp >= hp3 {
		t.Fatalf("hits %v hp %d/%d, want only player 3 hit", targetIDs(hs), z.players[2].hp, z.players[3].hp)
	}
}
