package world

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

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
		Speed: 0.8, AggroRange: 4, LeashRange: 10, RespawnSec: 8}},
	Levels: map[int]gamedata.Level{1: {Level: 1, HP: 56, Attack: 30, Defense: 8}, 2: {Level: 2, HP: 104, Attack: 39, Defense: 16}},
}

// triggerMap is a 20x20 open map with a safe zone around the default spawn,
// a rest point in it and one camp of 3 wanderers east of it.
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
		"camps": []map[string]any{{"id": "c1", "monster": "wanderer", "x": 12, "y": 4, "radius": 1.5,
			"count": 3, "min_level": 1, "max_level": 2}},
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
// must be at least as wide as the largest aggro range (4 in testMonsters).
func TestNewRejectsAggroBeyondAOICell(t *testing.T) {
	for _, c := range []struct {
		cell int
		ok   bool
	}{{3, false}, {4, true}} {
		_, err := New(Config{StartMap: "m1", AOICellSize: c.cell}, testMaps(t), testMonsters, nopSaver{}, slog.Default())
		if (err == nil) != c.ok {
			t.Fatalf("AOI cell %d: err = %v, want ok=%v", c.cell, err, c.ok)
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
	if len(a.Camps) != 1 {
		t.Fatalf("camps = %v", a.Camps)
	}
	c := a.Camps[0]
	var want []zone.Vec2
	for _, p := range m.CampTiles(m.Camps[0]) {
		want = append(want, zone.Vec2{X: p.X, Y: p.Y})
	}
	if c.ID != "c1" || c.Count != 3 || c.MinLevel != 1 || c.MaxLevel != 2 || c.Kind != testMonsters.Kinds["wanderer"] ||
		len(want) < 3 || !slices.Equal(c.Tiles, want) {
		t.Fatalf("camp = %+v, want tiles %v", c, want)
	}
	if len(a.Levels) != 2 || a.Levels[2] != testMonsters.Levels[2] {
		t.Fatalf("levels = %v", a.Levels)
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

func TestMembershipAttackReachesZone(t *testing.T) {
	w := newTestWorld(t, 10)
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 4.5), Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Attack(5, 999999) {
		t.Fatal("Attack: inbox full")
	}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-out.ch:
			if er := e.GetError(); er != nil {
				if e.Seq != 5 || er.Code != pb.ErrorCode_ERROR_CODE_INVALID_TARGET {
					t.Fatalf("got seq %d %v, want seq 5 INVALID_TARGET", e.Seq, er)
				}
				return
			}
		case <-timeout:
			t.Fatal("no reply to the attack")
		}
	}
}
