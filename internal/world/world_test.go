package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
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
		"id": "m1", "name": "M1", "type": "training", "width": 20, "height": 20, "min_zones": 1, "collision": rows,
		// Tiles 1..3 x 1..3, around the default spawn only.
		"safe_zones":   []map[string]any{{"id": "camp", "x": 1, "y": 1, "w": 3, "h": 3}},
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5, "respawn": true}},
	})
	m, err := mapdata.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*mapdata.Map{"m1": m}
}

var testMonsters = &gamedata.Data{
	Kinds: map[string]gamedata.Kind{"wanderer": {ID: "wanderer", Name: "Quỷ lang thang", AttackRange: 2.0, AttackSec: 1.5,
		Speed: 0.8, DetectRange: 4, ChaseRange: 10, ShootSec: 2, ShootWindupSec: 0.5, RegionMargin: 3, ChaseSpeed: 1.2, SafeWaitSec: 5,
		MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8}},
	Levels: map[int]gamedata.Level{1: {Level: 1, HP: 56, Attack: 30, Defense: 8}, 2: {Level: 2, HP: 104, Attack: 39, Defense: 16}},
	Skills: map[string]gamedata.Skill{
		"viem_1": {ID: "viem_1", Class: "viem", Slot: 1, LearnLevel: 10},
		"viem_3": {ID: "viem_3", Class: "viem", Slot: 0, LearnLevel: 30,
			Passive: map[string][]int{gamedata.PassivePhysDefPct: {2, 4, 6, 8, 10, 12, 14, 16, 18, 20}}},
		"nham_3": {ID: "nham_3", Class: "nham", Slot: 0, LearnLevel: 30,
			Passive: map[string][]int{gamedata.PassiveMaxHPPct: {2, 4, 6, 8, 10, 12, 14, 16, 18, 20}}},
	},
	Items: realItems(),
}

// realItems is the committed items.json.
func realItems() *gamedata.Items {
	b, err := os.ReadFile("../../data/items.json")
	if err != nil {
		panic(err)
	}
	it, err := gamedata.ParseItems(b)
	if err != nil {
		panic(err)
	}
	return it
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
		"id": "m1", "name": "M1", "type": "training", "width": 20, "height": 20, "min_zones": 1, "collision": rows,
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5, "respawn": true}},
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
	return startWorldLog(t, capacity, maps, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func startWorldLog(t *testing.T, capacity int, maps map[string]*mapdata.Map, log *slog.Logger) *World {
	t.Helper()
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: capacity,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, maps, testMonsters, nopSaver{}, log)
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
func TestNewRejectsMissingItemData(t *testing.T) {
	d := *testMonsters
	d.Items = nil
	if _, err := New(Config{StartMap: "m1", AOICellSize: 10}, testMaps(t), &d, nopSaver{}, slog.Default()); err == nil {
		t.Fatal("no error without item data")
	}
}

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

// lockedBuf is a log sink safe for the zone goroutines.
type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Stored skills that are not in the skill data, or belong to another class,
// are dropped with one warning each; the rest reach the zone.
func TestEnterDropsUnknownSkills(t *testing.T) {
	var buf lockedBuf
	w := startWorldLog(t, 10, testMaps(t), slog.New(slog.NewTextHandler(&buf, nil)))
	c := char(1, "m1", 3.5, 4.5)
	c.Level, c.SkillPoints = 30, 2
	c.Skills = map[string]int{"viem_1": 3, "viem_3": 2, "nham_3": 4, "gone_9": 1}
	out := newRecOut()
	if _, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out}); err != nil {
		t.Fatal(err)
	}
	r := out.enterResp(t)
	var got []string
	for _, s := range r.Skills {
		got = append(got, fmt.Sprintf("%s:%d", s.SkillId, s.Level))
	}
	if !slices.Equal(got, []string{"viem_1:3", "viem_3:2"}) || r.Stats.SkillPoints != 2 {
		t.Fatalf("skills %v, skill points %d, want viem_1:3 viem_3:2 and 2", got, r.Stats.SkillPoints)
	}
	// viem_3 (phys_def_pct 4 at level 2) counts in the stats.
	if want := combat.Compute(false, 30, combat.Points{}, combat.Bonus{}, combat.Passive{PhysDefPct: 4}); r.Stats.PhysDefense != int32(want.PhysDefense) {
		t.Fatalf("phys defense %d, want %d with the passive", r.Stats.PhysDefense, want.PhysDefense)
	}
	logs := buf.String()
	for _, id := range []string{"nham_3", "gone_9"} {
		if n := strings.Count(logs, "skill="+id); n != 1 {
			t.Fatalf("%d log lines for %s, want 1:\n%s", n, id, logs)
		}
	}
	if strings.Contains(logs, "skill=viem_") {
		t.Fatalf("kept skills were logged:\n%s", logs)
	}
	if c.Skills["nham_3"] != 4 {
		t.Fatal("Enter changed the caller's skill map")
	}
}

// Stored skills whose learn level is above the character's level (a devchar
// level-down) are dropped with one warning each: a level-29 Viêm keeps viem_1
// (learn level 10) but not the passive viem_3 (30), which then no longer
// counts in the stats. TestEnterDropsUnknownSkills keeps viem_3 at level 30.
func TestEnterDropsSkillsAboveLevel(t *testing.T) {
	var buf lockedBuf
	w := startWorldLog(t, 10, testMaps(t), slog.New(slog.NewTextHandler(&buf, nil)))
	c := char(1, "m1", 3.5, 4.5)
	c.Level = 29
	c.Skills = map[string]int{"viem_1": 3, "viem_3": 2}
	out := newRecOut()
	if _, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out}); err != nil {
		t.Fatal(err)
	}
	r := out.enterResp(t)
	var got []string
	for _, s := range r.Skills {
		got = append(got, fmt.Sprintf("%s:%d", s.SkillId, s.Level))
	}
	if !slices.Equal(got, []string{"viem_1:3"}) {
		t.Fatalf("skills %v, want viem_1:3", got)
	}
	if want := combat.Compute(false, 29, combat.Points{}, combat.Bonus{}, combat.Passive{}); r.Stats.PhysDefense != int32(want.PhysDefense) {
		t.Fatalf("phys defense %d, want %d without the passive", r.Stats.PhysDefense, want.PhysDefense)
	}
	logs := buf.String()
	if n := strings.Count(logs, "skill=viem_3"); n != 1 {
		t.Fatalf("%d log lines for viem_3, want 1:\n%s", n, logs)
	}
	if strings.Contains(logs, "skill=viem_1") {
		t.Fatalf("kept skill viem_1 was logged:\n%s", logs)
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
	if len(a.Skills) != 3 || a.Skills["nham_3"].Class != "nham" {
		t.Fatalf("skills = %v", a.Skills)
	}
	if a.Items != testMonsters.Items {
		t.Fatal("the area must share the item data")
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
		// The 2 s must stay in step with zone.restMinSec.
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
		{"enhance", func(m *Membership) bool { return m.Enhance(5, items.Ref{Slot: 0}, false, 1) }, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE},
		{"punch", func(m *Membership) bool { return m.Punch(5, items.Ref{Slot: 0}) }, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE},
		{"socket", func(m *Membership) bool { return m.Socket(5, items.Ref{Slot: 0}, 0, 1) }, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE},
		{"unsocket", func(m *Membership) bool { return m.Unsocket(5, items.Ref{Slot: 0}, 0) }, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE},
		{"refine", func(m *Membership) bool { return m.Refine(5, items.Ref{Slot: 0}, []int{0}) }, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE},
		{"refine choose", func(m *Membership) bool { return m.RefineChoose(5, true) }, pb.ErrorCode_ERROR_CODE_NO_PENDING_REFINE},
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

// ______________________TEST ITEMS_____________________

// goodItemRows are valid rows of a level-10 Viêm: a Xanh boots piece and a
// full stone stack in the bag, a Vàng sword with a hidden pity and three
// holes (an HP gem, an empty hole, a defense gem) worn, and a
// worn ring the character is too low for, which is kept (it just adds
// nothing).
func goodItemRows() []store.ItemRow {
	return []store.ItemRow{
		{Slot: 0, Code: "eq10_boots", Count: 1, Quality: 1, Lines: []store.ItemLine{{Stat: "hp", Value: 40}}, Hidden: []store.ItemLine{}},
		{Slot: 1, Code: "stone_enhance", Count: 999, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
		{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1, Quality: 2, Element: "thuy",
			Lines:  []store.ItemLine{{Stat: "phys_attack", Value: 5}, {Stat: "hp", Value: 43}, {Stat: "magic_defense", Value: 3}},
			Hidden: []store.ItemLine{{Stat: "hp", Value: 86}, {Stat: "phys_defense", Value: 4}},
			Pity:   []int{0, 0, 0, 0, 1, 2, 3, 0, 7, 12, 30, 75, 80, 85, 99},
			Gems:   []string{"gem_hp_1", "", "gem_defense_1"}},
		{Equipped: true, Slot: 8, Code: "eq10_ring_left", Count: 1},
	}
}

// goodInventory is goodItemRows as an inventory.
func goodInventory() *items.Inventory {
	inv := items.NewInventory(60)
	inv.Bag[0] = &items.Item{Code: "eq10_boots", Count: 1, Quality: 1, Lines: []items.Line{{Stat: "hp", Value: 40}}}
	inv.Bag[1] = &items.Item{Code: "stone_enhance", Count: 999}
	inv.Worn[0] = &items.Item{Code: "eq10_weapon_viem", Count: 1, Quality: 2, Element: "thuy",
		Lines:  []items.Line{{Stat: "phys_attack", Value: 5}, {Stat: "hp", Value: 43}, {Stat: "magic_defense", Value: 3}},
		Hidden: []items.Line{{Stat: "hp", Value: 86}, {Stat: "phys_defense", Value: 4}},
		Pity:   [items.MaxEnhance]int{0, 0, 0, 0, 1, 2, 3, 0, 7, 12, 30, 75, 80, 85, 99},
		Gems:   []string{"gem_hp_1", "", "gem_defense_1"}}
	inv.Worn[8] = &items.Item{Code: "eq10_ring_left", Count: 1}
	return inv
}

// A stored pity of the wrong length or with a value outside 0-99 is reset to
// zero with one warning, and the item is kept; so is any pity on a stackable.
func TestKnownItemsResetsBadPity(t *testing.T) {
	good := make([]int, items.MaxEnhance)
	with := func(i, v int) []int {
		p := slices.Clone(good)
		p[i] = v
		return p
	}
	for _, c := range []struct {
		name string
		row  int
		pity []int
	}{
		{"14 entries", 0, good[:14]},
		{"16 entries", 0, append(slices.Clone(good), 0)},
		{"one entry", 0, []int{5}},
		{"100", 0, with(14, 100)},
		{"negative", 0, with(0, -1)},
		{"stone with a pity", 1, with(3, 5)},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf lockedBuf
			w, err := New(Config{StartMap: "m1", AOICellSize: 10}, testMaps(t), testMonsters, nopSaver{},
				slog.New(slog.NewTextHandler(&buf, nil)))
			if err != nil {
				t.Fatal(err)
			}
			ch := char(1, "m1", 3.5, 4.5)
			rows := goodItemRows()
			rows[0].Enhance, rows[c.row].Pity = 3, c.pity
			ch.Level, ch.Items = 10, rows
			got := w.knownItems(ch)
			want := goodInventory()
			want.Bag[0].Enhance = 3
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("inventory\n%+v\nwant\n%+v", got, want)
			}
			if n := strings.Count(buf.String(), "reset stored pity"); n != 1 || strings.Contains(buf.String(), "dropped") {
				t.Fatalf("%d pity warnings, want 1 and nothing dropped:\n%s", n, buf.String())
			}
		})
	}
}

// Stored holes past the maximum are cut, an unknown gem, a stackable that is
// not a gem or a gem above the tier / 10 leaves an empty hole, and holes on a
// stackable are dropped, each fix with one warning; the item is kept.
func TestKnownItemsFixesBadGems(t *testing.T) {
	for _, c := range []struct {
		name  string
		row   int
		gems  []string
		want  []string
		warns int
	}{
		{"four holes", 2, []string{"gem_hp_1", "", "gem_defense_1", "gem_attack_1"}, nil, 1},
		{"unknown gem", 2, []string{"gem_hp_9", "", "gem_defense_1"}, []string{"", "", "gem_defense_1"}, 1},
		{"stone in a hole", 2, []string{"gem_hp_1", "stone_refine", "gem_defense_1"}, []string{"gem_hp_1", "", "gem_defense_1"}, 1},
		{"gem above the tier", 2, []string{"gem_hp_1", "", "gem_defense_2"}, []string{"gem_hp_1", "", ""}, 1},
		{"two bad gems", 2, []string{"gem_hp_2", "", "x"}, []string{"", "", ""}, 2},
		{"five holes and a bad gem", 2, []string{"gem_hp_9", "", "gem_defense_1", "", ""}, []string{"", "", "gem_defense_1"}, 2},
		{"stone with holes", 1, []string{"", "gem_hp_1"}, nil, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf lockedBuf
			w, err := New(Config{StartMap: "m1", AOICellSize: 10}, testMaps(t), testMonsters, nopSaver{},
				slog.New(slog.NewTextHandler(&buf, nil)))
			if err != nil {
				t.Fatal(err)
			}
			ch := char(1, "m1", 3.5, 4.5)
			rows := goodItemRows()
			rows[c.row].Gems = c.gems
			ch.Level, ch.Items = 10, rows
			got := w.knownItems(ch)
			want := goodInventory()
			if c.want != nil {
				want.Worn[0].Gems = c.want
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("inventory\n%+v\nwant\n%+v", got, want)
			}
			if n := strings.Count(buf.String(), "stored gems"); n != c.warns || strings.Contains(buf.String(), "dropped stored item") {
				t.Fatalf("%d gem warnings, want %d and nothing dropped:\n%s", n, c.warns, buf.String())
			}
		})
	}
}

// Each bad stored row is dropped with one warning; the good rows are kept.
func TestKnownItemsDropsBadRows(t *testing.T) {
	stone := func(slot, count int) store.ItemRow {
		return store.ItemRow{Slot: slot, Code: "stone_enhance", Count: count}
	}
	for _, c := range []struct {
		name string
		row  store.ItemRow
	}{
		{"unknown code", store.ItemRow{Slot: 5, Code: "eq99_nothing", Count: 1}},
		{"bag slot 60", stone(60, 1)},
		{"bag slot -1", stone(-1, 1)},
		{"worn slot 10", store.ItemRow{Equipped: true, Slot: 10, Code: "eq10_boots", Count: 1}},
		{"ring in the weapon slot", store.ItemRow{Equipped: true, Slot: 0, Code: "eq10_ring_left", Count: 1}},
		{"left ring in the right ring slot", store.ItemRow{Equipped: true, Slot: 9, Code: "eq10_ring_left", Count: 1}},
		{"boots in the helmet slot", store.ItemRow{Equipped: true, Slot: 4, Code: "eq10_boots", Count: 1}},
		{"stone worn", store.ItemRow{Equipped: true, Slot: 1, Code: "stone_enhance", Count: 1}},
		{"two rows in one slot", store.ItemRow{Slot: 0, Code: "stone_refine", Count: 5}},
		{"two worn in one slot", store.ItemRow{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1}},
		{"stack of 1000", stone(6, 1000)},
		{"stack of 0", stone(6, 0)},
		{"equipment count 2", store.ItemRow{Slot: 7, Code: "eq10_boots", Count: 2}},
		{"stone with quality", store.ItemRow{Slot: 8, Code: "stone_enhance", Count: 1, Quality: 1}},
		{"stone with element", store.ItemRow{Slot: 8, Code: "stone_enhance", Count: 1, Element: "viem"}},
		{"stone with enhance", store.ItemRow{Slot: 8, Code: "stone_enhance", Count: 1, Enhance: 1}},
		{"stone with lines", store.ItemRow{Slot: 8, Code: "stone_enhance", Count: 1, Lines: []store.ItemLine{{Stat: "hp", Value: 1}}}},
		{"stone with hidden lines", store.ItemRow{Slot: 8, Code: "stone_enhance", Count: 1, Hidden: []store.ItemLine{{Stat: "hp", Value: 1}}}},
		{"unknown line stat", store.ItemRow{Slot: 9, Code: "eq10_boots", Count: 1, Lines: []store.ItemLine{{Stat: "mp", Value: 3}}}},
		{"unknown hidden stat", store.ItemRow{Slot: 9, Code: "eq10_boots", Count: 1, Hidden: []store.ItemLine{{Stat: "accuracy", Value: 3}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf lockedBuf
			w, err := New(Config{StartMap: "m1", AOICellSize: 10}, testMaps(t), testMonsters, nopSaver{},
				slog.New(slog.NewTextHandler(&buf, nil)))
			if err != nil {
				t.Fatal(err)
			}
			ch := char(1, "m1", 3.5, 4.5)
			ch.Level, ch.Items = 10, append(goodItemRows(), c.row)
			got := w.knownItems(ch)
			if want := goodInventory(); !reflect.DeepEqual(got, want) {
				t.Fatalf("inventory\n%+v\nwant\n%+v", got, want)
			}
			if n := strings.Count(buf.String(), "dropped stored item"); n != 1 {
				t.Fatalf("%d warnings, want 1:\n%s", n, buf.String())
			}
		})
	}
}

// Enter hands the zone the stored Xu and items: they come back in
// EnterWorldResp, and the worn sword counts in the stats.
func TestEnterCarriesItemsAndXu(t *testing.T) {
	w := newTestWorld(t, 10)
	c := char(1, "m1", 3.5, 4.5)
	c.Level, c.Xu, c.Items = 10, 4321, goodItemRows()
	out := newRecOut()
	if _, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out}); err != nil {
		t.Fatal(err)
	}
	r := out.enterResp(t)
	if r.Stats.Xu != 4321 {
		t.Fatalf("xu %d, want 4321", r.Stats.Xu)
	}
	inv := r.Inventory
	if inv.GetBagSize() != 60 || len(inv.Bag) != 2 || len(inv.Equipped) != 2 ||
		inv.Bag[1].Item.Count != 999 || inv.Equipped[0].Item.Element != pb.CharClass_CHAR_CLASS_THUY || inv.Equipped[1].Slot != 8 {
		t.Fatalf("inventory %v", inv)
	}
	if w := inv.Equipped[0].Item; w.GetHoles() != 3 || !slices.Equal(w.GetGems(), []string{"gem_hp_1", "", "gem_defense_1"}) {
		t.Fatalf("worn sword %v, want 3 holes with gem_hp_1, empty, gem_defense_1", w)
	}
	// The gems add 86 HP and 1 to both defenses.
	gear := combat.Bonus{PhysAttack: 25 + 5, HP: 43 + 86, PhysDefense: 1, MagicDefense: 3 + 1}
	if want := combat.Compute(false, 10, combat.Points{}, gear, combat.Passive{}); r.Stats.PhysAttack != int32(want.PhysAttack) ||
		r.Stats.MaxHp != int32(want.MaxHP) || r.Stats.MagicDefense != int32(want.MagicDefense) ||
		r.Stats.PhysDefense != int32(want.PhysDefense) {
		t.Fatalf("stats %v, want %+v from the sword and its gems", r.Stats, want)
	}
}
