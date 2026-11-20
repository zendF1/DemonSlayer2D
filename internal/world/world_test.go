package world

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

type nopSaver struct{}

func (nopSaver) Save(_ store.CharState, done chan<- error) {
	if done != nil {
		done <- nil
	}
}

type recOut struct{ ch chan *pb.Envelope }

func newRecOut() *recOut { return &recOut{ch: make(chan *pb.Envelope, 256)} }

func (o *recOut) Send(e *pb.Envelope) bool {
	select {
	case o.ch <- e:
	default:
	}
	return true
}

func (o *recOut) Kick(pb.KickReason) {}

func (o *recOut) enterResp(t *testing.T) *pb.EnterWorldResp {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-o.ch:
			if r := e.GetEnterWorldResp(); r != nil {
				return r
			}
		case <-timeout:
			t.Fatal("no EnterWorldResp")
		}
	}
}

func testMaps(t *testing.T) map[string]*mapdata.Map {
	t.Helper()
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = strings.Repeat(".", 20)
	}
	rows[10] = strings.Repeat("#", 20)
	data, _ := json.Marshal(map[string]any{
		"id": "m1", "name": "M1", "type": "training", "width": 20, "height": 20, "collision": rows,
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5}},
	})
	m, err := mapdata.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*mapdata.Map{"m1": m}
}

var testMonsters = &gamedata.Data{
	Kinds: map[string]gamedata.Kind{"wanderer": {ID: "wanderer", Name: "Quỷ lang thang", AttackRange: 1.5, AttackSec: 1.5,
		Speed: 0.8, DetectRange: 4, ChaseRange: 10, ShootSec: 2, ShootWindupSec: 0.5, RegionMargin: 3, RespawnSec: 8}},
	Levels: map[int]gamedata.Level{1: {Level: 1, HP: 56, Attack: 30, Defense: 8}, 2: {Level: 2, HP: 104, Attack: 39, Defense: 16}},
}

// triggerMap is a 20x20 open map with a safe zone around the default spawn,
// a rest point in it and one region of 3 wanderers east of it.
func triggerMap(t *testing.T) *mapdata.Map {
	t.Helper()
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = strings.Repeat(".", 20)
	}
	data, _ := json.Marshal(map[string]any{
		"id": "m1", "name": "M1", "type": "training", "width": 20, "height": 20, "collision": rows,
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5}},
		"safe_zones":   []map[string]any{{"id": "camp", "x": 0, "y": 0, "w": 5, "h": 5}},
		"rest_points":  []map[string]any{{"id": "fire", "x": 2.5, "y": 2.5, "radius": 1.5}},
		"regions": []map[string]any{{"id": "r1", "monster": "wanderer", "min_level": 1, "max_level": 2, "count": 3,
			"rects": []map[string]any{{"x": 11, "y": 3, "w": 3, "h": 3}}, "nest": map[string]any{"x": 12.5, "y": 4.5}}},
	})
	m, err := mapdata.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newTestWorld(t *testing.T, capacity int) *World {
	t.Helper()
	return startWorld(t, capacity, testMaps(t))
}

func startWorld(t *testing.T, capacity int, maps map[string]*mapdata.Map) *World {
	t.Helper()
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: capacity,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, maps, testMonsters, nopSaver{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { cancel(); w.Wait() })
	return w
}

func char(id int64, mapID string, x, y float32) store.Character {
	// HP and MP -1 (full) is what the store returns for NULL; 0 would mean logged out dead.
	return store.Character{ID: id, AccountID: id, Slot: 1, Name: "c", Class: "viem", Level: 1, MapID: mapID, X: x, Y: y, HP: -1, MP: -1}
}

func TestNewRejectsUnknownStartMap(t *testing.T) {
	_, err := New(Config{StartMap: "nope"}, testMaps(t), testMonsters, nopSaver{}, slog.Default())
	if err == nil {
		t.Fatal("want error for unknown start map")
	}
}

// Monsters look for players only in the 3x3 AOI cells around them, so a cell
// must be at least as wide as the largest detect range (4 in testMonsters).
func TestNewRejectsDetectBeyondAOICell(t *testing.T) {
	for _, c := range []struct {
		cell int
		ok   bool
	}{{3, false}, {4, true}} {
		_, err := New(Config{StartMap: "m1", AOICellSize: c.cell}, testMaps(t), testMonsters, nopSaver{}, slog.Default())
		if (err == nil) != c.ok {
			t.Fatalf("AOI cell %d: err = %v, want ok=%v", c.cell, err, c.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "detect range") {
			t.Fatalf("AOI cell %d: err %q does not say detect range", c.cell, err)
		}
	}
}

func TestEnterUsesSavedPosition(t *testing.T) {
	w := newTestWorld(t, 10)
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 4.5), Out: out, RespSeq: 9})
	if err != nil {
		t.Fatal(err)
	}
	r := out.enterResp(t)
	if r.X != 3.5 || r.Y != 4.5 || m.ZoneNo != 1 || m.MapID != "m1" || r.SelfEntityId != m.EntityID {
		t.Fatalf("resp %v membership %+v", r, m)
	}
}

func TestEnterFallsBackToStartSpawn(t *testing.T) {
	w := newTestWorld(t, 10)
	for _, c := range []store.Character{char(1, "m1", 5.5, 10.5), char(2, "gone", 3.5, 3.5)} {
		out := newRecOut()
		if _, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out}); err != nil {
			t.Fatal(err)
		}
		if r := out.enterResp(t); r.MapId != "m1" || r.X != 2.5 || r.Y != 2.5 {
			t.Fatalf("char %d: want start spawn, got %v", c.ID, r)
		}
	}
}

func TestEnterOpensNewZoneWhenFull(t *testing.T) {
	w := newTestWorld(t, 2)
	var zones []int
	for i := int64(1); i <= 3; i++ {
		m, err := w.Enter(context.Background(), EnterRequest{Char: char(i, "m1", 3.5, 3.5), Out: newRecOut()})
		if err != nil {
			t.Fatal(err)
		}
		zones = append(zones, m.ZoneNo)
	}
	if zones[0] != 1 || zones[1] != 1 || zones[2] != 2 {
		t.Fatalf("zones = %v, want [1 1 2]", zones)
	}
}

func TestLeaveAndCloseIdleZone(t *testing.T) {
	w := newTestWorld(t, 1)
	m1, _ := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 3.5), Out: newRecOut()})
	m2, err := w.Enter(context.Background(), EnterRequest{Char: char(2, "m1", 3.5, 3.5), Out: newRecOut()})
	if err != nil || m2.ZoneNo != 2 {
		t.Fatalf("second player zone %d err %v", m2.ZoneNo, err)
	}
	if err := <-m2.Leave(); err != nil {
		t.Fatal(err)
	}
	w.closeIdle(time.Now().Add(10 * time.Minute))
	if n := len(w.Stats()); n != 1 {
		t.Fatalf("zones after janitor = %d, want 1 (zone 1 is never closed)", n)
	}
	_ = m1
}

type sess struct{ done chan struct{} }

func (s *sess) Kick(pb.KickReason)    {}
func (s *sess) Done() <-chan struct{} { return s.done }

func TestClaimRelease(t *testing.T) {
	w := newTestWorld(t, 10)
	a, b := &sess{}, &sess{}
	if prev := w.Claim(7, a); prev != nil {
		t.Fatal("first claim should have no previous session")
	}
	if prev := w.Claim(7, b); prev != a {
		t.Fatal("second claim should return the first session")
	}
	w.Release(7, a) // stale release must not remove b
	if prev := w.Claim(7, a); prev != b {
		t.Fatal("b should still be registered")
	}
	w.Release(7, a)
	if w.Online() != 0 {
		t.Fatalf("online = %d", w.Online())
	}
}

func TestEnterAfterStopOpensNoZones(t *testing.T) {
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: 10,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, testMaps(t), testMonsters, nopSaver{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	cancel()
	w.Wait()

	_, err = w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 3.5), Out: newRecOut()})
	if !errors.Is(err, ErrStopped) {
		t.Fatalf("Enter after stop = %v, want ErrStopped", err)
	}
	if n := len(w.Stats()); n != 1 {
		t.Fatalf("zones after stop = %d, want only the original zone 1", n)
	}
	w.Wait() // must not have gained new goroutines
}

type kickSess struct {
	done   chan struct{}
	kicked chan pb.KickReason
}

func newKickSess() *kickSess {
	return &kickSess{done: make(chan struct{}), kicked: make(chan pb.KickReason, 1)}
}

func (s *kickSess) Kick(r pb.KickReason) {
	select {
	case s.kicked <- r:
	default:
	}
}

func (s *kickSess) Done() <-chan struct{} { return s.done }

func TestTakeoverWaitsForPreviousSession(t *testing.T) {
	w := newTestWorld(t, 10)
	prev, next := newKickSess(), newKickSess()
	w.Claim(7, prev)
	go func() {
		if r := <-prev.kicked; r == pb.KickReason_KICK_REASON_DUPLICATE_LOGIN {
			time.Sleep(30 * time.Millisecond) // leaving the world and saving
			close(prev.done)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := w.Takeover(ctx, 7, next); err != nil {
		t.Fatal(err)
	}
	select {
	case <-prev.done:
	default:
		t.Fatal("Takeover returned before the previous session finished")
	}
	if cur := w.Claim(7, prev); cur != next {
		t.Fatalf("online session = %v, want next", cur)
	}
}

func TestTakeoverTimeoutKeepsPreviousSession(t *testing.T) {
	w := newTestWorld(t, 10)
	prev, next := newKickSess(), newKickSess() // prev never finishes
	w.Claim(7, prev)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := w.Takeover(ctx, 7, next); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Takeover = %v, want DeadlineExceeded", err)
	}
	if cur := w.Claim(7, next); cur != prev {
		t.Fatalf("after a failed takeover the previous session must stay online, got %v", cur)
	}
}

func TestTakeoverWithoutPreviousSession(t *testing.T) {
	w := newTestWorld(t, 10)
	s := newKickSess()
	if err := w.Takeover(context.Background(), 7, s); err != nil {
		t.Fatal(err)
	}
	if w.Online() != 1 {
		t.Fatalf("online = %d", w.Online())
	}
}

// A is online and saving slowly. B logs in and waits for A, C logs in while B is
// still waiting, then B gives up and closes. C must still wait for A's save.
func TestTakeoverChainWaitsForOldestSession(t *testing.T) {
	w := newTestWorld(t, 10)
	a, b, c := newKickSess(), newKickSess(), newKickSess()
	w.Claim(7, a)

	bErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := w.Takeover(ctx, 7, b)
		close(b.done) // B's connection closes after its login fails
		bErr <- err
	}()
	<-a.kicked // B is now waiting for A
	go func() {
		time.Sleep(200 * time.Millisecond) // A's save is slow
		close(a.done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := w.Takeover(ctx, 7, c); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.done:
	default:
		t.Fatal("C logged in before A's save landed")
	}
	if err := <-bErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("B's takeover = %v, want DeadlineExceeded", err)
	}
	if cur := w.Claim(7, a); cur != c {
		t.Fatalf("online session = %v, want C", cur)
	}
	w.mu.Lock()
	n := len(w.handoffs)
	w.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d account locks left after every takeover finished", n)
	}
}

func TestTakeoverGivesUpWhileQueued(t *testing.T) {
	w := newTestWorld(t, 10)
	a, b, c := newKickSess(), newKickSess(), newKickSess() // a never finishes
	w.Claim(7, a)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = w.Takeover(ctx, 7, b)
	}()
	<-a.kicked
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := w.Takeover(ctx, 7, c); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Takeover = %v, want DeadlineExceeded", err)
	}
	if cur := w.Claim(7, b); cur != b {
		t.Fatalf("a queued takeover that gave up must not change the online session, got %v", cur)
	}
}

// ______________________TEST AREA_____________________

func TestAreaOf(t *testing.T) {
	m := triggerMap(t)
	a := areaOf(m, testMonsters)
	if a.Grid != zone.Grid(m) {
		t.Fatal("the area grid must be the map")
	}
	if !slices.Equal(a.Safe, []zone.Rect{{X: 0, Y: 0, W: 5, H: 5}}) {
		t.Fatalf("safe = %v", a.Safe)
	}
	if !slices.Equal(a.Rests, []zone.Circle{{C: zone.Vec2{X: 2.5, Y: 2.5}, R: 1.5}}) {
		t.Fatalf("rests = %v", a.Rests)
	}
	if a.Respawn != (zone.Vec2{X: 2.5, Y: 2.5}) {
		t.Fatalf("respawn = %v", a.Respawn)
	}
	if len(a.Regions) != 1 {
		t.Fatalf("regions = %v", a.Regions)
	}
	r := a.Regions[0]
	var want []zone.Vec2
	for _, p := range m.RegionTiles(m.Regions[0]) {
		want = append(want, zone.Vec2{X: p.X, Y: p.Y})
	}
	if r.ID != "r1" || r.Count != 3 || r.MinLevel != 1 || r.MaxLevel != 2 || r.Kind != testMonsters.Kinds["wanderer"] ||
		r.Nest != (zone.Vec2{X: 12.5, Y: 4.5}) || !slices.Equal(r.Rects, []zone.Rect{{X: 11, Y: 3, W: 3, H: 3}}) ||
		len(want) != 9 || !slices.Equal(r.Tiles, want) {
		t.Fatalf("region = %+v, want tiles %v", r, want)
	}
	if len(a.Levels) != 2 || a.Levels[2] != testMonsters.Levels[2] {
		t.Fatalf("levels = %v", a.Levels)
	}
}

// kumotoriArea is the zone's view of the real kumotori_1 with the real monster data.
func kumotoriArea(t *testing.T) zone.Area {
	t.Helper()
	maps, err := mapdata.LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	d, err := gamedata.Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(maps); err != nil {
		t.Fatal(err)
	}
	return areaOf(maps["kumotori_1"], d)
}

func TestAreaOfKumotori(t *testing.T) {
	a := kumotoriArea(t)
	wants := []struct {
		id, kind string
		count    int
		nest     zone.Vec2
	}{
		{"r1", "wanderer", 8, zone.Vec2{X: 22.5, Y: 10.5}},
		{"r2", "wanderer", 7, zone.Vec2{X: 47.5, Y: 9.5}},
		{"r3", "snow_ghoul", 7, zone.Vec2{X: 15.5, Y: 34.5}},
		{"r4", "snow_ghoul", 8, zone.Vec2{X: 52.5, Y: 40.5}},
	}
	if len(a.Regions) != len(wants) {
		t.Fatalf("%d regions, want %d", len(a.Regions), len(wants))
	}
	for i, w := range wants {
		r := a.Regions[i]
		if r.ID != w.id || r.Kind.ID != w.kind || r.Count != w.count || r.Nest != w.nest || len(r.Rects) == 0 {
			t.Errorf("region %d = %s %s count %d nest %v rects %v, want %+v", i, r.ID, r.Kind.ID, r.Count, r.Nest, r.Rects, w)
		}
		if len(r.Tiles) < r.Count || !slices.Contains(r.Tiles, r.Nest) {
			t.Errorf("region %s: %d tiles, nest on one: %v", r.ID, len(r.Tiles), slices.Contains(r.Tiles, r.Nest))
		}
	}
}

// The real map's regions keep their first monsters at least 2 tiles apart.
// A zone on kumotori_1 is joined by a player who sees the whole map, and the
// monsters it is shown are sorted into regions by their exact tile centre.
func TestKumotoriSpawnSpreadsOut(t *testing.T) {
	a := kumotoriArea(t)
	region := map[zone.Vec2]string{}
	for _, r := range a.Regions {
		for _, p := range r.Tiles {
			region[p] = r.ID
		}
	}
	worst := map[string]float64{}
	for seed := uint64(1); seed <= 10; seed++ {
		var next atomic.Uint64
		next.Store(1000)
		const tickRate = 20
		z := zone.New(zone.Config{MapID: "kumotori_1", ZoneNo: 1, TickRate: tickRate, MoveSpeed: 4, AOICellSize: 64, Capacity: 10,
			SaveInterval: time.Minute, NewID: func() uint64 { return next.Add(1) }, Rand: rand.New(rand.NewPCG(seed, 7))},
			a, nopSaver{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		ctx, cancel := context.WithCancel(context.Background())
		go z.Run(ctx)
		out := newRecOut()
		if err := z.Join(ctx, zone.PlayerInfo{EntityID: 1, CharID: 1, Name: "p", Level: 1, Pos: a.Respawn, HP: -1, MP: -1}, out); err != nil {
			t.Fatal(err)
		}
		// The join's tick sends the EntityEnters. Before the first rest of
		// 2 s (in ticks) ends, no monster has taken a step, so they show the
		// spawn positions. A join any later would make the test meaningless.
		if ew := out.enterResp(t); ew.ServerTick >= uint32(2*tickRate) {
			t.Fatalf("seed %d: joined at tick %d, after the monsters may have started walking", seed, ew.ServerTick)
		}
		pos := map[string][]zone.Vec2{}
		seen := 0
		timeout := time.After(2 * time.Second)
		for seen < 30 {
			select {
			case e := <-out.ch:
				if en := e.GetEntityEnter(); en != nil && en.Kind == pb.EntityKind_ENTITY_KIND_MONSTER {
					p := zone.Vec2{X: en.X, Y: en.Y}
					id, ok := region[p]
					if !ok {
						t.Fatalf("seed %d: monster %d at %v is on no region tile", seed, en.Id, p)
					}
					pos[id] = append(pos[id], p)
					seen++
				}
			case <-timeout:
				t.Fatalf("seed %d: saw %d monsters, want 30", seed, seen)
			}
		}
		cancel()
		<-z.Done()
		for _, r := range a.Regions {
			ps := pos[r.ID]
			if len(ps) != r.Count {
				t.Fatalf("seed %d: region %s has %d monsters, want %d", seed, r.ID, len(ps), r.Count)
			}
			d := math.Inf(1)
			for i := range ps {
				for j := i + 1; j < len(ps); j++ {
					d = min(d, math.Hypot(float64(ps[i].X-ps[j].X), float64(ps[i].Y-ps[j].Y)))
				}
			}
			if d < 2 {
				t.Errorf("seed %d: region %s monsters %v only %.2f tiles apart, want at least 2", seed, r.ID, ps, d)
			}
			if w, ok := worst[r.ID]; !ok || d < w {
				worst[r.ID] = d
			}
			if seed == 1 {
				t.Logf("seed 1: region %s min pairwise distance %.2f", r.ID, d)
			}
		}
	}
	for _, r := range a.Regions {
		t.Logf("seeds 1-10: region %s min pairwise distance %.2f", r.ID, worst[r.ID])
	}
}

func TestMonsterAndPlayerIDsShareCounter(t *testing.T) {
	w := startWorld(t, 10, map[string]*mapdata.Map{"m1": triggerMap(t)})
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 3.5), Out: out})
	if err != nil {
		t.Fatal(err)
	}
	monsters := map[uint64]bool{}
	timeout := time.After(2 * time.Second)
	for len(monsters) < 3 {
		select {
		case e := <-out.ch:
			if en := e.GetEntityEnter(); en != nil && en.Kind == pb.EntityKind_ENTITY_KIND_MONSTER {
				monsters[en.Id] = true
			}
		case <-timeout:
			t.Fatalf("saw %d monsters, want 3", len(monsters))
		}
	}
	if monsters[m.EntityID] {
		t.Fatalf("player id %d clashes with a monster id %v", m.EntityID, monsters)
	}
	for id := range monsters {
		if id > w.nextEntity.Load() {
			t.Fatalf("monster id %d was not drawn from the world counter (at %d)", id, w.nextEntity.Load())
		}
	}
}

// Each in-world request reaches the zone, which answers its seq on the outbox.
func TestMembershipRequestsReachZone(t *testing.T) {
	for _, c := range []struct {
		name string
		send func(m *Membership) bool
		code pb.ErrorCode
	}{
		{"attack", func(m *Membership) bool { return m.Attack(5, 999999) }, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"allocate", func(m *Membership) bool { return m.Allocate(5, combat.Points{}) }, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"respawn", func(m *Membership) bool { return m.Respawn(5) }, pb.ErrorCode_ERROR_CODE_NOT_DEAD},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newTestWorld(t, 10)
			out := newRecOut()
			m, err := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 4.5), Out: out})
			if err != nil {
				t.Fatal(err)
			}
			if !c.send(m) {
				t.Fatal("inbox full")
			}
			timeout := time.After(2 * time.Second)
			for {
				select {
				case e := <-out.ch:
					if er := e.GetError(); er != nil {
						if e.Seq != 5 || er.Code != c.code {
							t.Fatalf("got seq %d %v, want seq 5 %v", e.Seq, er, c.code)
						}
						return
					}
				case <-timeout:
					t.Fatal("no reply to the request")
				}
			}
		})
	}
}
