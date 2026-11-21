package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
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

	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/party"
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
		MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8, HPPct: 100, AttackPct: 100, DamageType: gamedata.DamagePhys}},
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

// testMonsters carries the committed quests.json, so that stored quest rows
// of the real quest ids survive Enter.
func init() {
	d, err := gamedata.Load("../../data")
	if err != nil {
		panic(err)
	}
	testMonsters.Quests, testMonsters.QuestByID = d.Quests, d.QuestByID
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

// pvpLeft is the pvp_combat cooldown of r, or 0.
func pvpLeft(r *pb.EnterWorldResp) time.Duration {
	for _, cd := range r.Cooldowns {
		if cd.Key == zone.CooldownPvPCombat {
			return time.Duration(cd.RemainingMs) * time.Millisecond
		}
	}
	return 0
}

// enterResp enters character id at (3.5, 3.5) of m1 and returns its
// EnterWorldResp.
func enterResp(t *testing.T, w *World, id int64) (*Membership, *pb.EnterWorldResp) {
	t.Helper()
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: char(id, "m1", 3.5, 3.5), Out: out})
	if err != nil {
		t.Fatal(err)
	}
	return m, out.enterResp(t)
}

// A PvP lock still running when a character leaves its zone is remembered
// by wall clock and handed to its next Enter (phase 5a spec 4.1), so a
// relogin cannot turn PK off early: the zone refuses a quit while it runs,
// and a disconnect, which removes the character at once, remembers it
// again. The entry is used up by the Enter; an expired one gives nothing
// and the janitor drops it.
func TestPvPLockKeptAcrossRelogin(t *testing.T) {
	w := newTestWorld(t, 10)
	w.keepPvPLock(1, 5*time.Second)
	m, r := enterResp(t, w, 1)
	if left := pvpLeft(r); left <= 4*time.Second || left > 5*time.Second {
		t.Fatalf("pvp_combat on entering %v, want about 5 s", left)
	}
	w.mu.Lock()
	_, still := w.pvpLocks[1]
	w.mu.Unlock()
	if still {
		t.Fatal("the Enter did not use up the remembered lock")
	}
	if saved, code := m.Quit(); code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT || saved != nil {
		t.Fatalf("Quit = %v, %v, want IN_PVP_COMBAT", saved, code)
	}
	if err := <-m.Leave(); err != nil { // a disconnect
		t.Fatal(err)
	}
	_, r = enterResp(t, w, 1)
	if left := pvpLeft(r); left <= 3*time.Second || left > 5*time.Second {
		t.Fatalf("pvp_combat after the relogin %v, want what was left of 5 s", left)
	}

	w.mu.Lock()
	w.pvpLocks[2] = time.Now().Add(-time.Second)
	w.pvpLocks[3] = time.Now().Add(time.Minute)
	w.mu.Unlock()
	if _, r := enterResp(t, w, 2); pvpLeft(r) != 0 {
		t.Fatalf("an expired lock gave pvp_combat %v", pvpLeft(r))
	}
	w.closeIdle(time.Now().Add(2 * time.Minute))
	w.mu.Lock()
	n := len(w.pvpLocks)
	w.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d remembered locks after the janitor, want none", n)
	}
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

// Every respawn point of the map reaches the area, in id order.
func TestAreaOfRespawns(t *testing.T) {
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = strings.Repeat(".", 20)
	}
	data, _ := json.Marshal(map[string]any{
		"id": "m1", "name": "M1", "type": "training", "width": 20, "height": 20, "min_zones": 1, "collision": rows,
		"spawn_points": []map[string]any{
			{"id": "default", "x": 2.5, "y": 2.5, "respawn": true},
			{"id": "arrive", "x": 10.5, "y": 10.5},
			{"id": "camp_b", "x": 16.5, "y": 16.5, "respawn": true},
		},
		"safe_zones": []map[string]any{{"id": "camp", "x": 0, "y": 0, "w": 5, "h": 5}, {"id": "b", "x": 15, "y": 15, "w": 5, "h": 5}},
	})
	m, err := mapdata.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	a := areaOf(m, testMonsters)
	if want := []zone.Vec2{{X: 16.5, Y: 16.5}, {X: 2.5, Y: 2.5}}; !slices.Equal(a.Respawns, want) {
		t.Fatalf("respawns = %v, want %v", a.Respawns, want)
	}
}

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
	if !slices.Equal(a.Respawns, []zone.Vec2{{X: 2.5, Y: 2.5}}) {
		t.Fatalf("respawns = %v", a.Respawns)
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
		if err := z.Join(ctx, zone.PlayerInfo{EntityID: 1, CharID: 1, Name: "p", Level: 1, Pos: a.Respawns[0], HP: -1, MP: -1}, out); err != nil {
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
		{"respawn", func(m *Membership) bool { return m.Respawn(5, false) }, pb.ErrorCode_ERROR_CODE_NOT_DEAD},
		{"enhance", func(m *Membership) bool { return m.Enhance(5, items.Ref{Slot: 0}, false, 1) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		{"punch", func(m *Membership) bool { return m.Punch(5, items.Ref{Slot: 0}) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		{"socket", func(m *Membership) bool { return m.Socket(5, items.Ref{Slot: 0}, 0, 1) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		{"unsocket", func(m *Membership) bool { return m.Unsocket(5, items.Ref{Slot: 0}, 0) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		{"refine", func(m *Membership) bool { return m.Refine(5, items.Ref{Slot: 0}, []int{0}) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
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

// areaOf takes the PK rules from the map type: m1 is a training map, m2 a town.
func TestAreaOfPk(t *testing.T) {
	ms := portalMaps(t)
	if a := areaOf(ms["m1"], testMonsters); !a.PvP || !a.PkPenalty {
		t.Fatalf("training area PvP %v PkPenalty %v, want both", a.PvP, a.PkPenalty)
	}
	if a := areaOf(ms["m2"], testMonsters); a.PvP || a.PkPenalty {
		t.Fatalf("town area PvP %v PkPenalty %v, want neither", a.PvP, a.PkPenalty)
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
			got := w.knownBag(ch)
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
			got := w.knownBag(ch)
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
			got := w.knownBag(ch)
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

// ______________________TEST STASH_____________________

// A stored stash size outside [base, max] is clamped with one warning; a
// stash row at a slot at or past the (clamped) size is dropped with one
// warning, and so is a worn row; the good rows keep their slots.
func TestKnownStash(t *testing.T) {
	good := []store.ItemRow{
		{Slot: 0, Code: "eq10_boots", Count: 1, Quality: 1, Lines: []store.ItemLine{{Stat: "hp", Value: 40}}, Hidden: []store.ItemLine{}},
		{Slot: 29, Code: "stone_enhance", Count: 999},
	}
	for _, c := range []struct {
		name          string
		size, want    int
		extra         []store.ItemRow
		clamps, drops int
	}{
		{"size 30", 30, 30, nil, 0, 0},
		{"slot 35 of 30", 30, 30, []store.ItemRow{{Slot: 35, Code: "stone_enhance", Count: 1}}, 0, 1},
		{"slot 30 of 30", 30, 30, []store.ItemRow{{Slot: 30, Code: "stone_enhance", Count: 1}}, 0, 1},
		{"worn row", 30, 30, []store.ItemRow{{Equipped: true, Slot: 2, Code: "eq10_boots", Count: 1}}, 0, 1},
		{"unknown code", 30, 30, []store.ItemRow{{Slot: 3, Code: "eq99_nothing", Count: 1}}, 0, 1},
		{"size 10", 10, 30, nil, 1, 0},
		{"size 0", 0, 30, nil, 1, 0},
		{"size 150", 150, 120, []store.ItemRow{{Slot: 119, Code: "stone_refine", Count: 2}, {Slot: 125, Code: "stone_refine", Count: 1}}, 1, 1},
		{"size 120", 120, 120, nil, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf lockedBuf
			w, err := New(Config{StartMap: "m1", AOICellSize: 10}, testMaps(t), testMonsters, nopSaver{},
				slog.New(slog.NewTextHandler(&buf, nil)))
			if err != nil {
				t.Fatal(err)
			}
			ch := char(1, "m1", 3.5, 4.5)
			ch.Account = store.AccountState{ID: 1, StashSize: c.size, Stash: append(slices.Clone(good), c.extra...)}
			got := w.knownStash(ch)
			if len(got) != c.want {
				t.Fatalf("stash size %d, want %d", len(got), c.want)
			}
			if got[0] == nil || got[0].Code != "eq10_boots" || got[0].Lines[0].Value != 40 || got[29] == nil || got[29].Count != 999 {
				t.Fatalf("good rows not kept: %+v %+v", got[0], got[29])
			}
			if c.want == 120 && len(c.extra) > 0 && (got[119] == nil || got[119].Count != 2) {
				t.Fatalf("slot 119 %+v, want 2 refine stones", got[119])
			}
			n := 0
			for _, it := range got {
				if it != nil {
					n++
				}
			}
			if want := 2 + len(c.extra) - c.drops; n != want {
				t.Fatalf("%d items kept, want %d", n, want)
			}
			if k := strings.Count(buf.String(), "clamped stored stash size"); k != c.clamps {
				t.Fatalf("%d clamp warnings, want %d:\n%s", k, c.clamps, buf.String())
			}
			if k := strings.Count(buf.String(), "dropped stored item"); k != c.drops {
				t.Fatalf("%d drop warnings, want %d:\n%s", k, c.drops, buf.String())
			}
		})
	}
}

// Enter hands the zone the account part: Lượng shows in the stats, and the
// leave save carries the account id, Lượng, the clamped stash size and the
// kept stash rows.
func TestEnterCarriesAccount(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 10, testMaps(t), sv)

	c := char(1, "m1", 3.5, 4.5)
	c.AccountID = 77
	c.Account = store.AccountState{ID: 77, Luong: 5, StashSize: 10,
		Stash: []store.ItemRow{{Slot: 3, Code: "stone_enhance", Count: 12}, {Slot: 40, Code: "stone_enhance", Count: 1}}}
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if r := out.enterResp(t); r.Stats.GetLuong() != 5 {
		t.Fatalf("luong %d, want 5", r.Stats.GetLuong())
	}
	if err := <-m.Leave(); err != nil {
		t.Fatal(err)
	}
	st, ok := sv.last(1)
	if !ok {
		t.Fatal("nothing saved")
	}
	a := st.Account
	want := &store.AccountState{ID: 77, Luong: 5, StashSize: 30,
		Stash: []store.ItemRow{{Slot: 3, Code: "stone_enhance", Count: 12, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}}}
	if !reflect.DeepEqual(a, want) {
		t.Fatalf("saved account %+v, want %+v", a, want)
	}
}

// ______________________TEST ZONES AND TRANSFER_____________________

// portalMaps is two open 20x20 maps joined by portals: m1 (min_zones 2) has
// the portal "east" on its east edge to m2's "from_m1", and m2 (min_zones 1)
// the portal "west" on its west edge to m1's "from_m2".
func portalMaps(t *testing.T) map[string]*mapdata.Map {
	t.Helper()
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = strings.Repeat(".", 20)
	}
	parse := func(f map[string]any) *mapdata.Map {
		f["type"], f["width"], f["height"], f["collision"] = "training", 20, 20, rows
		f["safe_zones"] = []map[string]any{{"id": "camp", "x": 1, "y": 1, "w": 3, "h": 3}}
		f["town"] = "m2"
		if f["id"] == "m2" { // the town: all walkable tiles safe
			f["type"] = "town"
			f["safe_zones"] = []map[string]any{{"id": "town", "x": 0, "y": 0, "w": 20, "h": 20}}
		}
		data, _ := json.Marshal(f)
		m, err := mapdata.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m1 := parse(map[string]any{"id": "m1", "name": "M1", "min_zones": 2,
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5, "respawn": true}, {"id": "from_m2", "x": 16.5, "y": 10.5}},
		"portals":      []map[string]any{{"id": "east", "x": 19, "y": 8, "w": 1, "h": 4, "to_map": "m2", "to_spawn": "from_m1"}},
	})
	m2 := parse(map[string]any{"id": "m2", "name": "M2", "min_zones": 1,
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5, "respawn": true}, {"id": "from_m1", "x": 3.5, "y": 10.5}},
		"portals":      []map[string]any{{"id": "west", "x": 0, "y": 8, "w": 1, "h": 4, "to_map": "m1", "to_spawn": "from_m2"}},
		"npcs": []map[string]any{
			{"id": "xa_phu", "name": "Xa phu", "role": "carriage", "x": 10.5, "y": 15.5, "routes": []map[string]any{{"map": "m1", "xu": 120}}},
			{"id": "ren", "name": "Tho ren", "role": "blacksmith", "x": 15.5, "y": 15.5},
		},
	})
	maps := map[string]*mapdata.Map{"m1": m1, "m2": m2}
	if err := mapdata.Link(maps); err != nil {
		t.Fatal(err)
	}
	return maps
}

// recSaver records every save and answers it at once.
type recSaver struct {
	mu    sync.Mutex
	saves []store.CharState
}

func (s *recSaver) Save(st store.CharState, done chan<- error) {
	s.mu.Lock()
	s.saves = append(s.saves, st)
	s.mu.Unlock()
	if done != nil {
		done <- nil
	}
}

// last is the latest save of character id.
func (s *recSaver) last(id int64) (store.CharState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.saves) - 1; i >= 0; i-- {
		if s.saves[i].ID == id {
			return s.saves[i], true
		}
	}
	return store.CharState{}, false
}

func startWorldSaver(t *testing.T, capacity int, maps map[string]*mapdata.Map, sv zone.Saver) *World {
	t.Helper()
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: capacity,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, maps, testMonsters, sv, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { cancel(); w.Wait() })
	return w
}

// enterRespSeq is the next EnterWorldResp on o and its seq.
func (o *recOut) enterRespSeq(t *testing.T) (*pb.EnterWorldResp, uint32) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-o.ch:
			if r := e.GetEnterWorldResp(); r != nil {
				return r, e.Seq
			}
		case <-timeout:
			t.Fatal("no EnterWorldResp")
		}
	}
}

// noEnterResp fails if o has an EnterWorldResp queued.
func (o *recOut) noEnterResp(t *testing.T) {
	t.Helper()
	for {
		select {
		case e := <-o.ch:
			if r := e.GetEnterWorldResp(); r != nil {
				t.Fatalf("unexpected EnterWorldResp %v", r)
			}
		default:
			return
		}
	}
}

// zoneNos is the running zones of each map as "map:no", sorted.
func zoneNos(w *World) []string {
	var out []string
	for _, s := range w.Stats() {
		out = append(out, fmt.Sprintf("%s:%d", s.MapID, s.ZoneNo))
	}
	slices.Sort(out)
	return out
}

// enterAt puts character id into the world at (x, y) of mapID.
func enterAt(t *testing.T, w *World, id int64, mapID string, x, y float32) (*Membership, *recOut) {
	t.Helper()
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: char(id, mapID, x, y), Out: out})
	if err != nil {
		t.Fatal(err)
	}
	out.enterResp(t)
	return m, out
}

// slotOf is zone no of mapID, or nil.
func slotOf(w *World, mapID string, no int) *slot {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, s := range w.zones[mapID] {
		if s != nil && s.no == no {
			return s
		}
	}
	return nil
}

// transferCode is the refusal code of err, or 0.
func transferCode(err error) pb.ErrorCode {
	var te *TransferError
	if errors.As(err, &te) {
		return te.Code
	}
	return 0
}

func portal(id string) TransferReq { return TransferReq{Kind: TransferPortal, PortalID: id} }

func zoneChange(no int) TransferReq { return TransferReq{Kind: TransferZone, Zone: no} }

// Start opens min_zones zones per map, and the janitor keeps them: zone 2 of
// m1 stays when it has emptied, an empty zone 3 above it closes.
func TestMinZonesStayOpen(t *testing.T) {
	w := startWorld(t, 1, portalMaps(t))
	if got := zoneNos(w); !slices.Equal(got, []string{"m1:1", "m1:2", "m2:1"}) {
		t.Fatalf("zones at start %v, want m1:1 m1:2 m2:1", got)
	}
	enterAt(t, w, 1, "m1", 5.5, 5.5)
	b, _ := enterAt(t, w, 2, "m1", 5.5, 5.5)
	c, _ := enterAt(t, w, 3, "m1", 5.5, 5.5)
	if b.ZoneNo != 2 || c.ZoneNo != 3 {
		t.Fatalf("zones %d %d, want 2 3", b.ZoneNo, c.ZoneNo)
	}
	<-b.Leave()
	<-c.Leave()
	w.closeIdle(time.Now().Add(10 * time.Minute))
	if got := zoneNos(w); !slices.Equal(got, []string{"m1:1", "m1:2", "m2:1"}) {
		t.Fatalf("zones after the janitor %v, want m1:1 m1:2 m2:1", got)
	}
}

// A portal keeps the zone number when that zone of the target is open with
// room: from m2 zone 2 to m1 zone 2, though m1 zone 1 has room. The player
// arrives at the portal's spawn point, saved there, with the request's seq.
func TestPortalKeepsZoneNumber(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 1, portalMaps(t), sv)
	enterAt(t, w, 1, "m2", 5.5, 5.5)
	m, out := enterAt(t, w, 2, "m2", 1.5, 9.0)
	if m.ZoneNo != 2 {
		t.Fatalf("zone %d, want 2", m.ZoneNo)
	}
	req := portal("west")
	req.RespSeq = 42
	nm, saved, err := w.Transfer(context.Background(), m, req)
	if err != nil || saved != nil {
		t.Fatalf("transfer: %v (saved %v)", err, saved)
	}
	if nm.MapID != "m1" || nm.ZoneNo != 2 || nm.EntityID != m.EntityID {
		t.Fatalf("membership %+v, want m1 zone 2 with entity %d", nm, m.EntityID)
	}
	r, seq := out.enterRespSeq(t)
	if seq != 42 || r.MapId != "m1" || r.Zone != 2 || r.X != 16.5 || r.Y != 10.5 || r.SelfEntityId != m.EntityID {
		t.Fatalf("EnterWorldResp seq %d %v, want seq 42 at m1 zone 2 (16.5, 10.5)", seq, r)
	}
	if st, ok := sv.last(2); !ok || st.MapID != "m1" || st.X != 16.5 || st.Y != 10.5 {
		t.Fatalf("departure save %+v, want m1 (16.5, 10.5)", st)
	}
	if n := slotOf(w, "m2", 2).z.Count(); n != 0 {
		t.Fatalf("m2 zone 2 count %d, want 0", n)
	}
	if s := slotOf(w, "m1", 2); s.z.Count() != 1 || s.reserved.Load() != 0 {
		t.Fatalf("m1 zone 2 count %d reserved %d, want 1 0", s.z.Count(), s.reserved.Load())
	}
	// The new membership talks to the new zone.
	if err := <-nm.Leave(); err != nil {
		t.Fatal(err)
	}
	if n := slotOf(w, "m1", 2).z.Count(); n != 0 {
		t.Fatalf("m1 zone 2 count after leave %d, want 0", n)
	}
}

// Without its zone number open with room in the target, a portal takes the
// first zone with room, and opens a new zone when all are full.
func TestPortalFallsBackToFirstZoneWithRoom(t *testing.T) {
	w := startWorld(t, 1, portalMaps(t))
	a, _ := enterAt(t, w, 1, "m1", 18.5, 10.0)
	b, _ := enterAt(t, w, 2, "m1", 18.5, 10.0)
	if a.ZoneNo != 1 || b.ZoneNo != 2 {
		t.Fatalf("zones %d %d, want 1 2", a.ZoneNo, b.ZoneNo)
	}
	nb, _, err := w.Transfer(context.Background(), b, portal("east"))
	if err != nil || nb.MapID != "m2" || nb.ZoneNo != 1 {
		t.Fatalf("B: %+v %v, want m2 zone 1 (m2 has one zone)", nb, err)
	}
	c, _ := enterAt(t, w, 3, "m1", 19.5, 11.9) // m1 zone 2 is free again
	nc, _, err := w.Transfer(context.Background(), c, portal("east"))
	if err != nil || nc.MapID != "m2" || nc.ZoneNo != 2 {
		t.Fatalf("C: %+v %v, want a new m2 zone 2", nc, err)
	}
	// From zone 1 with m2 zones 1 and 2 full.
	na, _, err := w.Transfer(context.Background(), a, portal("east"))
	if err != nil || na.MapID != "m2" || na.ZoneNo != 3 {
		t.Fatalf("A: %+v %v, want a new m2 zone 3", na, err)
	}
}

// A portal refused by the zone or the world leaves the player where it is
// and releases the seat held in the target; within 1 tile it is taken.
func TestPortalRefused(t *testing.T) {
	w := startWorld(t, 10, portalMaps(t))
	m, out := enterAt(t, w, 1, "m1", 17.9, 10.0) // 1.1 tiles from the east portal
	_, saved, err := w.Transfer(context.Background(), m, portal("east"))
	if code := transferCode(err); code != pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE || saved != nil {
		t.Fatalf("1.1 tiles away: %v (saved %v), want OUT_OF_RANGE", err, saved)
	}
	if _, _, err := w.Transfer(context.Background(), m, portal("west")); transferCode(err) != pb.ErrorCode_ERROR_CODE_BAD_REQUEST {
		t.Fatalf("a portal of another map: %v, want BAD_REQUEST", err)
	}
	if s := slotOf(w, "m2", 1); s.reserved.Load() != 0 || s.z.Count() != 0 {
		t.Fatalf("m2 zone 1 reserved %d count %d, want 0 0", s.reserved.Load(), s.z.Count())
	}
	if n := slotOf(w, "m1", 1).z.Count(); n != 1 {
		t.Fatalf("m1 zone 1 count %d, want 1", n)
	}
	time.Sleep(50 * time.Millisecond)
	out.noEnterResp(t)

	n, _ := enterAt(t, w, 2, "m1", 18.1, 10.0) // 0.9 tiles away
	if nn, _, err := w.Transfer(context.Background(), n, portal("east")); err != nil || nn.MapID != "m2" {
		t.Fatalf("0.9 tiles away: %+v %v, want m2", nn, err)
	}
	// The refused player is still in m1 zone 1 and can leave from there.
	if err := <-m.Leave(); err != nil {
		t.Fatal(err)
	}
	if n := slotOf(w, "m1", 1).z.Count(); n != 0 {
		t.Fatalf("m1 zone 1 count after leave %d, want 0", n)
	}
}

type fakeDeparting struct {
	dead, ready bool
	pos         zone.Vec2
	level       int
	xu          int64
	bag         []*items.Item
	near        pb.ErrorCode // NearNPC's answer
	// What the check asked to take.
	tookXu              int64
	tookSlot, tookCount int
}

func (d *fakeDeparting) Dead() bool            { return d.dead }
func (d *fakeDeparting) Pos() zone.Vec2        { return d.pos }
func (d *fakeDeparting) ZoneChangeReady() bool { return d.ready }
func (d *fakeDeparting) Level() int            { return d.level }
func (d *fakeDeparting) Xu() int64             { return d.xu }
func (d *fakeDeparting) NearNPC(id, role string) pb.ErrorCode {
	if id != "xa_phu" || role != mapdata.RoleCarriage {
		return pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	}
	return d.near
}
func (d *fakeDeparting) ItemAt(i int) (*items.Item, bool) {
	if i < 0 || i >= len(d.bag) {
		return nil, false
	}
	return d.bag[i], true
}
func (d *fakeDeparting) TakeXu(n int64)        { d.tookXu = n }
func (d *fakeDeparting) TakeItem(i, count int) { d.tookSlot, d.tookCount = i, count }

// The zone-side checks in spec 3.3 and 3.4 order: DEAD first.
func TestTransferChecks(t *testing.T) {
	rect := mapdata.Rect{X: 19, Y: 8, W: 1, H: 4}
	in, near, far := zone.Vec2{X: 19.5, Y: 9}, zone.Vec2{X: 18.1, Y: 12.0}, zone.Vec2{X: 19.5, Y: 13.1}
	for _, tc := range []struct {
		d    fakeDeparting
		want pb.ErrorCode
	}{
		{fakeDeparting{dead: true, pos: in}, pb.ErrorCode_ERROR_CODE_DEAD},
		{fakeDeparting{dead: true, pos: far}, pb.ErrorCode_ERROR_CODE_DEAD},
		{fakeDeparting{pos: far}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{fakeDeparting{pos: near}, 0},
		{fakeDeparting{pos: in}, 0},
	} {
		if got := portalCheck(rect)(&tc.d); got != tc.want {
			t.Errorf("portal check %+v = %v, want %v", tc.d, got, tc.want)
		}
	}
	for _, tc := range []struct {
		d    fakeDeparting
		want pb.ErrorCode
	}{
		{fakeDeparting{dead: true}, pb.ErrorCode_ERROR_CODE_DEAD},
		{fakeDeparting{dead: true, ready: true}, pb.ErrorCode_ERROR_CODE_DEAD},
		{fakeDeparting{}, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN},
		{fakeDeparting{ready: true}, 0},
	} {
		if got := zoneChangeCheck(&tc.d); got != tc.want {
			t.Errorf("zone change check %+v = %v, want %v", tc.d, got, tc.want)
		}
	}
}

// Zone change refusals in spec 3.4 order, then a change that keeps the
// position and entity id, starts the cooldown and is reported in
// EnterWorldResp.
func TestZoneChange(t *testing.T) {
	w := startWorld(t, 1, portalMaps(t))
	a, out := enterAt(t, w, 1, "m1", 7.5, 6.5)
	b, _ := enterAt(t, w, 2, "m1", 9.5, 9.5)
	code := func(no int) pb.ErrorCode {
		t.Helper()
		_, saved, err := w.Transfer(context.Background(), a, zoneChange(no))
		if saved != nil {
			t.Fatalf("zone %d: refused with a save channel", no)
		}
		return transferCode(err)
	}
	for _, tc := range []struct {
		no   int
		want pb.ErrorCode
	}{
		{0, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{-1, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{1, pb.ErrorCode_ERROR_CODE_BAD_REQUEST}, // the current zone
		{5, pb.ErrorCode_ERROR_CODE_ZONE_NOT_FOUND},
		{2, pb.ErrorCode_ERROR_CODE_ZONE_FULL}, // B is there
	} {
		if got := code(tc.no); got != tc.want {
			t.Fatalf("zone %d: %v, want %v", tc.no, got, tc.want)
		}
	}
	if s := slotOf(w, "m1", 2); s.reserved.Load() != 0 {
		t.Fatalf("zone 2 reserved %d after ZONE_FULL, want 0", s.reserved.Load())
	}

	<-b.Leave()
	req := zoneChange(2)
	req.RespSeq = 9
	na, saved, err := w.Transfer(context.Background(), a, req)
	if err != nil || saved != nil || na.MapID != "m1" || na.ZoneNo != 2 || na.EntityID != a.EntityID {
		t.Fatalf("zone change: %+v %v, want m1 zone 2 with entity %d", na, err, a.EntityID)
	}
	r, seq := out.enterRespSeq(t)
	if seq != 9 || r.Zone != 2 || r.X != 7.5 || r.Y != 6.5 {
		t.Fatalf("EnterWorldResp seq %d %v, want seq 9 zone 2 at (7.5, 6.5)", seq, r)
	}
	if cd := r.Cooldowns; len(cd) != 1 || cd[0].Key != zone.CooldownZoneChange || cd[0].RemainingMs < 9900 || cd[0].RemainingMs > 10000 {
		t.Fatalf("cooldowns %v, want zone_change about 10000 ms", cd)
	}

	// On cooldown now; the world's checks still come first.
	a = na
	if got := code(3); got != pb.ErrorCode_ERROR_CODE_ZONE_NOT_FOUND {
		t.Fatalf("zone 3 on cooldown: %v, want ZONE_NOT_FOUND", got)
	}
	if got := code(1); got != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("back to zone 1 at once: %v, want ON_COOLDOWN", got)
	}
	enterAt(t, w, 3, "m1", 9.5, 9.5) // fills zone 1
	if got := code(1); got != pb.ErrorCode_ERROR_CODE_ZONE_FULL {
		t.Fatalf("full zone 1 on cooldown: %v, want ZONE_FULL", got)
	}
	if s := slotOf(w, "m1", 2); s.z.Count() != 1 {
		t.Fatalf("zone 2 count %d, want 1 (A stayed)", s.z.Count())
	}
}

// Zones lists the running zones of a map in order with their player counts.
func TestZones(t *testing.T) {
	w := startWorld(t, 1, portalMaps(t))
	enterAt(t, w, 1, "m1", 5.5, 5.5)
	enterAt(t, w, 2, "m1", 5.5, 5.5)
	c, _ := enterAt(t, w, 3, "m1", 5.5, 5.5)
	if got, want := w.Zones("m1"), []ZoneInfo{{1, 1}, {2, 1}, {3, 1}}; !slices.Equal(got, want) {
		t.Fatalf("zones %v, want %v", got, want)
	}
	<-c.Leave()
	if got, want := w.Zones("m1"), []ZoneInfo{{1, 1}, {2, 1}, {3, 0}}; !slices.Equal(got, want) {
		t.Fatalf("zones after leave %v, want %v", got, want)
	}
	w.closeIdle(time.Now().Add(10 * time.Minute))
	if got, want := w.Zones("m1"), []ZoneInfo{{1, 1}, {2, 1}}; !slices.Equal(got, want) {
		t.Fatalf("zones after the janitor %v, want %v", got, want)
	}
	if got, want := w.Zones("m2"), []ZoneInfo{{1, 0}}; !slices.Equal(got, want) {
		t.Fatalf("m2 zones %v, want %v", got, want)
	}
	if got := w.Zones("nope"); got != nil {
		t.Fatalf("unknown map zones %v, want none", got)
	}
	if w.Capacity() != 1 {
		t.Fatalf("capacity %d, want 1", w.Capacity())
	}
}

// After the world stops, a transfer reserves nothing and leaves the player
// where it was saved.
func TestTransferAfterStop(t *testing.T) {
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: 10,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, portalMaps(t), testMonsters, nopSaver{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	m, _ := enterAt(t, w, 1, "m1", 18.5, 10.0)
	cancel()
	w.Wait()
	_, saved, err := w.Transfer(context.Background(), m, portal("east"))
	if !errors.Is(err, ErrStopped) || saved != nil {
		t.Fatalf("transfer after stop: %v (saved %v), want ErrStopped and no save", err, saved)
	}
	if got := zoneNos(w); !slices.Equal(got, []string{"m1:1", "m1:2", "m2:1"}) {
		t.Fatalf("zones %v, want the original ones", got)
	}
}

// hookSaver answers every save at once and calls onSave first.
type hookSaver struct{ onSave func(store.CharState) }

func (s hookSaver) Save(st store.CharState, done chan<- error) {
	s.onSave(st)
	if done != nil {
		done <- nil
	}
}

// The world stops while the player is between zones: either the new zone
// still took it (and evicts it on the way out) or Transfer reports
// ErrStopped with the departure save to wait for.
func TestTransferWorldStopsMidway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sv := hookSaver{onSave: func(st store.CharState) {
		if st.MapID == "m2" { // the departure save, on the old zone's goroutine
			cancel()
		}
	}}
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: 10,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, portalMaps(t), testMonsters, sv, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	w.Start(ctx)
	defer w.Wait()
	m, _ := enterAt(t, w, 1, "m1", 18.5, 10.0)
	nm, saved, err := w.Transfer(context.Background(), m, portal("east"))
	switch {
	case err == nil && saved == nil && nm != nil:
		if e := <-nm.Leave(); e != nil {
			t.Fatal(e)
		}
	case errors.Is(err, ErrStopped) && saved != nil && nm == nil:
		if e := <-saved; e != nil {
			t.Fatal(e)
		}
	default:
		t.Fatalf("transfer: %+v %v (saved %v), want a membership or ErrStopped with the save", nm, err, saved)
	}
}

// The world stops after the departure and before the join: Transfer reports
// ErrStopped with the departure save, which yields once the player is saved
// at the destination.
func TestTransferStoppedAfterDeparture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var w *World
	sv := hookSaver{onSave: func(st store.CharState) {
		if st.MapID == "m2" { // the departure save, on the old zone's goroutine
			cancel()
			<-slotOf(w, "m2", 1).z.Done() // the destination is gone before the join
		}
	}}
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: 10,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, portalMaps(t), testMonsters, sv, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	w.Start(ctx)
	defer w.Wait()
	m, _ := enterAt(t, w, 1, "m1", 18.5, 10.0)
	nm, saved, err := w.Transfer(context.Background(), m, portal("east"))
	if !errors.Is(err, ErrStopped) || saved == nil || nm != nil {
		t.Fatalf("transfer: %+v %v (saved %v), want ErrStopped with the save", nm, err, saved)
	}
	if e := <-saved; e != nil {
		t.Fatal(e)
	}
}

// The destination join fails while the world runs: Transfer reports the join
// error, not ErrStopped, with the departure save, and the player is in no
// zone, saved at the destination.
func TestTransferJoinFailsAfterDeparture(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 10, portalMaps(t), sv)
	m, _ := enterAt(t, w, 1, "m1", 18.5, 10.0)
	// Another player under the same entity id already stands in m2 zone 1, so
	// the zone refuses the join as a duplicate.
	dup := char(2, "m2", 5.5, 5.5)
	info := zone.PlayerInfo{EntityID: m.EntityID, CharID: dup.ID, Name: "dup", Class: pb.CharClass_CHAR_CLASS_VIEM, Level: 1,
		Pos: zone.Vec2{X: 5.5, Y: 5.5}, HP: -1, MP: -1, Skills: w.knownSkills(dup), Inventory: w.knownBag(dup)}
	if err := slotOf(w, "m2", 1).z.Join(context.Background(), info, newRecOut()); err != nil {
		t.Fatal(err)
	}
	nm, saved, err := w.Transfer(context.Background(), m, portal("east"))
	if err == nil || errors.Is(err, ErrStopped) || saved == nil || nm != nil {
		t.Fatalf("transfer: %+v %v (saved %v), want the join error with the save", nm, err, saved)
	}
	if e := <-saved; e != nil {
		t.Fatal(e)
	}
	if st, ok := sv.last(1); !ok || st.MapID != "m2" {
		t.Fatalf("last save of character 1 %+v, want one on m2", st)
	}
	if n := slotOf(w, "m1", 1).z.Count(); n != 0 {
		t.Fatalf("m1 zone 1 has %d players, want the departed one gone", n)
	}
	if s := slotOf(w, "m2", 1); s.reserved.Load() != 0 {
		t.Fatalf("m2 zone 1 holds %d seats, want none", s.reserved.Load())
	}
}

// Enter brings a character's PK mode into the zone, and a portal keeps it,
// in the new zone and in the save.
func TestEnterAndPortalKeepPk(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 10, portalMaps(t), sv)
	c := char(2, "m2", 1.5, 9.0)
	c.PkOn = true
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if r := out.enterResp(t); !r.Stats.GetPkOn() {
		t.Fatalf("EnterWorldResp stats %v, want pk_on", r.Stats)
	}
	nm, _, err := w.Transfer(context.Background(), m, portal("west"))
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := out.enterRespSeq(t); r.MapId != "m1" || !r.Stats.GetPkOn() {
		t.Fatalf("EnterWorldResp after the portal %v, want m1 with pk_on", r)
	}
	if st, ok := sv.last(2); !ok || st.MapID != "m1" || !st.PkOn {
		t.Fatalf("departure save %+v, want m1 with PkOn", st)
	}
	if err := <-nm.Leave(); err != nil {
		t.Fatal(err)
	}
	if st, _ := sv.last(2); !st.PkOn {
		t.Fatalf("leave save %+v, want PkOn", st)
	}
}

// ______________________TEST SCROLL AND CARRIAGE_____________________

func scroll(slot int) TransferReq { return TransferReq{Kind: TransferScroll, BagSlot: slot} }

func travel(npc string, r int) TransferReq {
	return TransferReq{Kind: TransferTravel, NPCID: npc, Route: r}
}

// enterChar puts c into the world and returns its membership after its EnterWorldResp.
func enterChar(t *testing.T, w *World, c store.Character) (*Membership, *recOut) {
	t.Helper()
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	out.enterResp(t)
	return m, out
}

func stackRow(slot int, code string, n int) store.ItemRow {
	return store.ItemRow{Slot: slot, Code: code, Count: n, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}
}

// savedCount is the count of code in a saved bag.
func savedCount(st store.CharState, code string) int {
	n := 0
	for _, r := range st.Items {
		if r.Code == code && !r.Equipped {
			n += r.Count
		}
	}
	return n
}

// The scroll's zone checks in spec 4.5 order (steps 2-6); only a pass asks
// for one scroll.
func TestScrollCheck(t *testing.T) {
	d := realItems()
	high := *d
	high.Stackables = maps.Clone(d.Stackables)
	st := high.Stackables["scroll_return"]
	u := *st.Use
	u.Level = 5
	st.Use = &u
	high.Stackables["scroll_return"] = st
	bag := []*items.Item{
		{Code: "potion_hp_1", Count: 4}, nil, {Code: "scroll_return", Count: 3},
		{Code: "stone_enhance", Count: 2}, {Code: "eq10_boots", Count: 1},
	}
	for _, tc := range []struct {
		name  string
		d     *gamedata.Items
		dead  bool
		slot  int
		level int
		want  pb.ErrorCode
	}{
		{"dead", d, true, 2, 10, pb.ErrorCode_ERROR_CODE_DEAD},
		{"dead with a bad slot", d, true, -1, 10, pb.ErrorCode_ERROR_CODE_DEAD},
		{"slot below 0", d, false, -1, 10, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"slot past the bag", d, false, 5, 10, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"empty slot", d, false, 1, 10, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		{"a potion", d, false, 0, 10, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"a stone", d, false, 3, 10, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"equipment", d, false, 4, 10, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"level too low", &high, false, 2, 4, pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW},
		{"level enough", &high, false, 2, 5, 0},
		{"ok", d, false, 2, 1, 0},
	} {
		fd := &fakeDeparting{dead: tc.dead, level: tc.level, bag: bag}
		got := scrollCheck(tc.d, tc.slot)(fd)
		if got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
		wantSlot, wantCount := 0, 0
		if tc.want == 0 {
			wantSlot, wantCount = tc.slot, 1
		}
		if fd.tookSlot != wantSlot || fd.tookCount != wantCount || fd.tookXu != 0 {
			t.Errorf("%s: took slot %d count %d xu %d, want slot %d count %d", tc.name, fd.tookSlot, fd.tookCount, fd.tookXu, wantSlot, wantCount)
		}
	}
}

// The carriage's zone checks in spec 4.6 order (steps 2-4); only a pass asks
// for the price.
func TestCarriageCheck(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    fakeDeparting
		want pb.ErrorCode
	}{
		{"dead", fakeDeparting{dead: true, near: pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"too far", fakeDeparting{near: pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR}, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		{"xu short", fakeDeparting{xu: 119}, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU},
		{"exact xu", fakeDeparting{xu: 120}, 0},
		{"more xu", fakeDeparting{xu: 5000}, 0},
	} {
		fd := tc.d
		if got := carriageCheck("xa_phu", 120)(&fd); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
		want := int64(0)
		if tc.want == 0 {
			want = 120
		}
		if fd.tookXu != want || fd.tookCount != 0 {
			t.Errorf("%s: took xu %d count %d, want xu %d", tc.name, fd.tookXu, fd.tookCount, want)
		}
	}
}

// A scroll in the town is refused by the world; zone refusals leave the
// player and the stack as they are. A scroll from a training map lands at the
// town's default spawn with one scroll fewer, in the handoff and in the
// departure save.
func TestReturnScroll(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 10, portalMaps(t), sv)
	c := char(1, "m2", 5.5, 5.5)
	c.Items = []store.ItemRow{stackRow(2, "scroll_return", 3)}
	inTown, _ := enterChar(t, w, c)
	if _, saved, err := w.Transfer(context.Background(), inTown, scroll(2)); transferCode(err) != pb.ErrorCode_ERROR_CODE_BAD_REQUEST || saved != nil {
		t.Fatalf("a scroll in the town: %v (saved %v), want BAD_REQUEST", err, saved)
	}

	c = char(2, "m1", 9.5, 9.5)
	c.Items = []store.ItemRow{stackRow(0, "potion_hp_1", 4), stackRow(2, "scroll_return", 3)}
	m, out := enterChar(t, w, c)
	for _, tc := range []struct {
		slot int
		want pb.ErrorCode
	}{
		{-1, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{60, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{1, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		{0, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
	} {
		if _, saved, err := w.Transfer(context.Background(), m, scroll(tc.slot)); transferCode(err) != tc.want || saved != nil {
			t.Fatalf("slot %d: %v (saved %v), want %v", tc.slot, err, saved, tc.want)
		}
	}
	if s := slotOf(w, "m2", 1); s.reserved.Load() != 0 || s.z.Count() != 1 {
		t.Fatalf("m2 zone 1 reserved %d count %d after refusals, want 0 1", s.reserved.Load(), s.z.Count())
	}
	if _, ok := sv.last(2); ok {
		t.Fatal("a refused scroll saved the player")
	}

	req := scroll(2)
	req.RespSeq = 5
	nm, saved, err := w.Transfer(context.Background(), m, req)
	if err != nil || saved != nil || nm.MapID != "m2" || nm.ZoneNo != 1 {
		t.Fatalf("scroll: %+v %v (saved %v), want m2 zone 1", nm, err, saved)
	}
	r, seq := out.enterRespSeq(t)
	if seq != 5 || r.MapId != "m2" || r.X != 2.5 || r.Y != 2.5 {
		t.Fatalf("EnterWorldResp seq %d %v, want seq 5 at m2 default (2.5, 2.5)", seq, r)
	}
	scrolls := 0
	for _, e := range r.Inventory.Bag {
		if e.Slot == 2 && e.Item.Code == "scroll_return" {
			scrolls = int(e.Item.Count)
		}
	}
	if scrolls != 2 {
		t.Fatalf("handoff bag %v, want 2 scrolls in slot 2", r.Inventory.Bag)
	}
	if st, ok := sv.last(2); !ok || st.MapID != "m2" || st.X != 2.5 || st.Y != 2.5 || savedCount(st, "scroll_return") != 2 ||
		savedCount(st, "potion_hp_1") != 4 {
		t.Fatalf("departure save %+v, want m2 (2.5, 2.5) with 2 scrolls and 4 potions", st)
	}
	if n := slotOf(w, "m1", 1).z.Count(); n != 0 {
		t.Fatalf("m1 zone 1 count %d, want 0", n)
	}
}

// Carriage refusals in spec 4.6 order: the world's (no such carriage, route
// out of range) before the zone's (too far, Xu short); a refusal changes
// nothing. A ride takes exactly the route's price and lands at the target's
// default spawn.
func TestCarriage(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 10, portalMaps(t), sv)
	far := char(1, "m2", 3.5, 15.5) // 7 tiles from xa_phu
	far.Xu = 5000
	fm, _ := enterChar(t, w, far)
	poor := char(2, "m2", 10.5, 13.5) // 2 tiles from xa_phu
	poor.Xu = 119
	pm, _ := enterChar(t, w, poor)
	for _, tc := range []struct {
		name string
		m    *Membership
		req  TransferReq
		want pb.ErrorCode
	}{
		{"unknown npc", pm, travel("nope", 0), pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"not a carriage", pm, travel("ren", 0), pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"route past the end", pm, travel("xa_phu", 1), pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"route below 0", pm, travel("xa_phu", -1), pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"far, route past the end", fm, travel("xa_phu", 1), pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"too far", fm, travel("xa_phu", 0), pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		{"xu short", pm, travel("xa_phu", 0), pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU},
	} {
		if _, saved, err := w.Transfer(context.Background(), tc.m, tc.req); transferCode(err) != tc.want || saved != nil {
			t.Fatalf("%s: %v (saved %v), want %v", tc.name, err, saved, tc.want)
		}
	}
	if s := slotOf(w, "m1", 1); s.reserved.Load() != 0 || s.z.Count() != 0 {
		t.Fatalf("m1 zone 1 reserved %d count %d after refusals, want 0 0", s.reserved.Load(), s.z.Count())
	}
	// The poor rider is still in m2 with all its Xu.
	if err := <-pm.Leave(); err != nil {
		t.Fatal(err)
	}
	if st, ok := sv.last(2); !ok || st.MapID != "m2" || st.Xu != 119 {
		t.Fatalf("save after leaving %+v, want m2 with 119 Xu", st)
	}

	rich := char(3, "m2", 11.5, 15.5)
	rich.Xu = 500
	rm, out := enterChar(t, w, rich)
	req := travel("xa_phu", 0)
	req.RespSeq = 8
	nm, saved, err := w.Transfer(context.Background(), rm, req)
	if err != nil || saved != nil || nm.MapID != "m1" || nm.ZoneNo != 1 {
		t.Fatalf("ride: %+v %v (saved %v), want m1 zone 1", nm, err, saved)
	}
	r, seq := out.enterRespSeq(t)
	if seq != 8 || r.MapId != "m1" || r.X != 2.5 || r.Y != 2.5 || r.Stats.Xu != 380 {
		t.Fatalf("EnterWorldResp seq %d %v, want seq 8 at m1 default (2.5, 2.5) with 380 Xu", seq, r)
	}
	if st, ok := sv.last(3); !ok || st.MapID != "m1" || st.X != 2.5 || st.Y != 2.5 || st.Xu != 380 {
		t.Fatalf("departure save %+v, want m1 (2.5, 2.5) with 380 Xu", st)
	}
}

// ____TEST PARTY WIRING____

// drain drops everything queued on o.
func (o *recOut) drain() {
	for {
		select {
		case <-o.ch:
		default:
			return
		}
	}
}

// party is the party pushes queued on o, in order; they are pushed
// synchronously, so they are queued by the time the call returns.
func (o *recOut) party() []*pb.Envelope {
	var out []*pb.Envelope
	for {
		select {
		case e := <-o.ch:
			if e.GetPartyInvite() != nil || e.GetPartyState() != nil || e.GetPartyNotice() != nil {
				out = append(out, e)
			}
		default:
			return out
		}
	}
}

// enterNamed puts character id named name into the world at (x, y) of mapID.
func enterNamed(t *testing.T, w *World, id int64, name, mapID string, x, y float32) (*Membership, *recOut) {
	t.Helper()
	c := char(id, mapID, x, y)
	c.Name = name
	return enterChar(t, w, c)
}

// formParty has leader invite each member, who accepts, then drains every outbox.
func formParty(t *testing.T, w *World, leader *Membership, members []*Membership, outs ...*recOut) {
	t.Helper()
	for _, m := range members {
		if code := w.Party().Invite(leader.EntityID, m.EntityID); code != party.OK {
			t.Fatalf("Invite %d: %v", m.EntityID, code)
		}
		if code := w.Party().Answer(m.EntityID, leader.EntityID, true); code != party.OK {
			t.Fatalf("Answer %d: %v", m.EntityID, code)
		}
	}
	for _, o := range outs {
		o.drain()
	}
}

func member(id uint64, name string, level int32, mapID string, zone int32) *pb.PartyMember {
	return &pb.PartyMember{Id: id, Name: name, CharClass: pb.CharClass_CHAR_CLASS_VIEM, Level: level, MapId: mapID, Zone: zone}
}

func stateEnv(leader uint64, members ...*pb.PartyMember) *pb.Envelope {
	return &pb.Envelope{Payload: &pb.Envelope_PartyState{PartyState: &pb.PartyState{LeaderId: leader, Members: members}}}
}

func noticeEnv(k pb.PartyNoticeKind, name string) *pb.Envelope {
	return &pb.Envelope{Payload: &pb.Envelope_PartyNotice{PartyNotice: &pb.PartyNotice{Kind: k, Name: name}}}
}

func wantParty(t *testing.T, who string, got []*pb.Envelope, want ...*pb.Envelope) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s got %v, want %v", who, got, want)
	}
	for i := range got {
		if !proto.Equal(got[i], want[i]) {
			t.Fatalf("%s push %d = %v, want %v", who, i, got[i], want[i])
		}
	}
}

// Each party push becomes its envelope with seq 0; the shared state is
// not changed by the conversion.
func TestPartyEnvelope(t *testing.T) {
	st := &party.State{LeaderID: 4, Members: []party.Member{
		{ID: 4, Name: "a", Class: pb.CharClass_CHAR_CLASS_NHAM, Level: 12, MapID: "m1", Zone: 2},
		{ID: 9, Name: "b", Class: pb.CharClass_CHAR_CLASS_VIEM, Level: 3, MapID: "m2", Zone: 1},
	}}
	keep := &party.State{LeaderID: st.LeaderID, Members: slices.Clone(st.Members)}
	for _, c := range []struct {
		msg  party.Msg
		want *pb.Envelope
	}{
		{&party.Invite{FromID: 4, FromName: "a", FromLevel: 12, FromClass: pb.CharClass_CHAR_CLASS_NHAM, ExpiresMs: 30000},
			&pb.Envelope{Payload: &pb.Envelope_PartyInvite{PartyInvite: &pb.PartyInvite{
				FromId: 4, FromName: "a", FromLevel: 12, FromClass: pb.CharClass_CHAR_CLASS_NHAM, ExpiresMs: 30000}}}},
		{st, stateEnv(4,
			&pb.PartyMember{Id: 4, Name: "a", CharClass: pb.CharClass_CHAR_CLASS_NHAM, Level: 12, MapId: "m1", Zone: 2},
			member(9, "b", 3, "m2", 1))},
		{&party.State{}, stateEnv(0)},
		{&party.Notice{Kind: party.NoticeNewLeader, Name: "b"}, noticeEnv(pb.PartyNoticeKind_PARTY_NOTICE_KIND_NEW_LEADER, "b")},
	} {
		o := newRecOut()
		if !(partyOutbox{o}).Send(c.msg) {
			t.Fatalf("Send(%T) refused", c.msg)
		}
		got := <-o.ch
		if got.Seq != 0 || !proto.Equal(got, c.want) {
			t.Fatalf("%T became %v, want %v", c.msg, got, c.want)
		}
	}
	if !reflect.DeepEqual(st, keep) {
		t.Fatalf("state changed to %+v", st)
	}
}

// Enter registers the character under its entity id; Leave, a quit and
// a kick's Leave unregister it, and its teammates are told.
func TestEnterRegistersAndLeaveUnregisters(t *testing.T) {
	w := newTestWorld(t, 10)
	a, outA := enterNamed(t, w, 1, "an", "m1", 5.5, 5.5)
	b, outB := enterNamed(t, w, 2, "binh", "m1", 5.5, 5.5)
	c, outC := enterNamed(t, w, 3, "chi", "m1", 5.5, 5.5)
	outB.drain()
	if code := w.Party().Invite(a.EntityID, b.EntityID); code != party.OK {
		t.Fatalf("Invite: %v", code)
	}
	wantParty(t, "B", outB.party(), &pb.Envelope{Payload: &pb.Envelope_PartyInvite{PartyInvite: &pb.PartyInvite{
		FromId: a.EntityID, FromName: "an", FromLevel: 1, FromClass: pb.CharClass_CHAR_CLASS_VIEM, ExpiresMs: 30000}}})
	outA.drain()
	if code := w.Party().Answer(b.EntityID, a.EntityID, true); code != party.OK {
		t.Fatalf("Answer: %v", code)
	}
	st := stateEnv(a.EntityID, member(a.EntityID, "an", 1, "m1", 1), member(b.EntityID, "binh", 1, "m1", 1))
	wantParty(t, "A", outA.party(), noticeEnv(pb.PartyNoticeKind_PARTY_NOTICE_KIND_JOINED, "binh"), st)
	formParty(t, w, a, []*Membership{c}, outA, outB, outC)

	if err := <-c.Leave(); err != nil {
		t.Fatal(err)
	}
	wantParty(t, "A", outA.party(), noticeEnv(pb.PartyNoticeKind_PARTY_NOTICE_KIND_LEFT, "chi"), st)
	if code := w.Party().Invite(a.EntityID, c.EntityID); code != party.InvalidTarget {
		t.Fatalf("Invite the leaver: %v, want INVALID_TARGET", code)
	}

	saved, code := b.Quit()
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("Quit: %v", code)
	}
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	wantParty(t, "A", outA.party(), noticeEnv(pb.PartyNoticeKind_PARTY_NOTICE_KIND_DISBANDED, ""), stateEnv(0))
	if code := w.Party().Invite(a.EntityID, b.EntityID); code != party.InvalidTarget {
		t.Fatalf("Invite the quitter: %v, want INVALID_TARGET", code)
	}
	if a.cell.Load() != 0 {
		t.Fatalf("A's cell %d after the disband, want 0", a.cell.Load())
	}
}

// A quit refused while the PvP lock runs keeps the character registered.
func TestRefusedQuitStaysRegistered(t *testing.T) {
	w := newTestWorld(t, 10)
	w.keepPvPLock(1, 5*time.Second)
	a, _ := enterNamed(t, w, 1, "an", "m1", 5.5, 5.5)
	b, _ := enterNamed(t, w, 2, "binh", "m1", 5.5, 5.5)
	if _, code := a.Quit(); code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT {
		t.Fatalf("Quit: %v, want IN_PVP_COMBAT", code)
	}
	if code := w.Party().Invite(b.EntityID, a.EntityID); code != party.OK {
		t.Fatalf("Invite after a refused quit: %v, want OK", code)
	}
}

// A portal reports the new map and zone to the party, and the cell rides
// the handoff: the new membership holds the same cell, which the service
// still writes.
func TestTransferMovesParty(t *testing.T) {
	w := startWorld(t, 10, portalMaps(t))
	a, outA := enterNamed(t, w, 1, "an", "m1", 5.5, 5.5)
	b, outB := enterNamed(t, w, 2, "binh", "m1", 18.5, 10.0)
	formParty(t, w, a, []*Membership{b}, outA, outB)
	cell := b.cell
	if cell == nil || cell.Load() == 0 || cell.Load() != a.cell.Load() {
		t.Fatalf("cells %v %v, want the same party id", cell, a.cell)
	}
	nb, _, err := w.Transfer(context.Background(), b, portal("east"))
	if err != nil || nb.MapID != "m2" || nb.ZoneNo != 1 {
		t.Fatalf("transfer: %+v %v, want m2 zone 1", nb, err)
	}
	st := stateEnv(a.EntityID, member(a.EntityID, "an", 1, "m1", 1), member(b.EntityID, "binh", 1, "m2", 1))
	wantParty(t, "A", outA.party(), st)
	wantParty(t, "B", outB.party(), st)
	if nb.cell != cell {
		t.Fatalf("new membership cell %p, want the handed-over %p", nb.cell, cell)
	}
	if code := w.Party().Leave(b.EntityID); code != party.OK || cell.Load() != 0 {
		t.Fatalf("Leave after the portal: %v, cell %d", code, cell.Load())
	}
	// The new membership unregisters on Leave.
	formParty(t, w, a, []*Membership{nb}, outA, outB)
	if err := <-nb.Leave(); err != nil {
		t.Fatal(err)
	}
	wantParty(t, "A", outA.party(), noticeEnv(pb.PartyNoticeKind_PARTY_NOTICE_KIND_DISBANDED, ""), stateEnv(0))
}

// A transfer whose join fails after the departure leaves the character in
// no zone, so it is unregistered there.
func TestTransferLostUnregisters(t *testing.T) {
	w := startWorld(t, 10, portalMaps(t))
	a, outA := enterNamed(t, w, 1, "an", "m1", 5.5, 5.5)
	b, outB := enterNamed(t, w, 2, "binh", "m1", 18.5, 10.0)
	formParty(t, w, a, []*Membership{b}, outA, outB)
	dup := char(3, "m2", 5.5, 5.5)
	info := zone.PlayerInfo{EntityID: b.EntityID, CharID: dup.ID, Name: "dup", Class: pb.CharClass_CHAR_CLASS_VIEM, Level: 1,
		Pos: zone.Vec2{X: 5.5, Y: 5.5}, HP: -1, MP: -1, Skills: w.knownSkills(dup), Inventory: w.knownBag(dup)}
	if err := slotOf(w, "m2", 1).z.Join(context.Background(), info, newRecOut()); err != nil {
		t.Fatal(err)
	}
	_, saved, err := w.Transfer(context.Background(), b, portal("east"))
	if err == nil || saved == nil {
		t.Fatalf("transfer: %v (saved %v), want the join error", err, saved)
	}
	<-saved
	wantParty(t, "A", outA.party(), noticeEnv(pb.PartyNoticeKind_PARTY_NOTICE_KIND_DISBANDED, ""), stateEnv(0))
	if code := w.Party().Invite(a.EntityID, b.EntityID); code != party.InvalidTarget {
		t.Fatalf("Invite the lost character: %v, want INVALID_TARGET", code)
	}
}

// Every zone's Leveled reports to the party service, which pushes the new
// level to the party (the zone tests check gainExp calls it).
func TestLevelUpPushesPartyState(t *testing.T) {
	w := startWorld(t, 1, portalMaps(t))
	a, outA := enterNamed(t, w, 1, "an", "m1", 5.5, 5.5)
	b, outB := enterNamed(t, w, 2, "binh", "m1", 5.5, 5.5) // zone 2
	formParty(t, w, a, []*Membership{b}, outA, outB)
	for _, s := range []*slot{slotOf(w, "m1", 1), slotOf(w, "m1", 2), slotOf(w, "m2", 1)} {
		if s.z.Config().Leveled == nil {
			t.Fatalf("zone %d has no Leveled hook", s.no)
		}
	}
	slotOf(w, "m1", 2).z.Config().Leveled(b.EntityID, 7)
	st := stateEnv(a.EntityID, member(a.EntityID, "an", 1, "m1", 1), member(b.EntityID, "binh", 7, "m1", 2))
	wantParty(t, "A", outA.party(), st)
	wantParty(t, "B", outB.party(), st)
}

// An Enter that never joins a zone (every zone full) leaves nothing
// registered.
func TestFailedEnterRegistersNothing(t *testing.T) {
	w := newTestWorld(t, 0)
	_, err := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 5.5, 5.5), Out: newRecOut()})
	if !errors.Is(err, zone.ErrZoneFull) {
		t.Fatalf("Enter: %v, want ErrZoneFull", err)
	}
	id := w.nextEntity.Load() // the map has no monsters, so this is the character's id
	probe := uint64(1 << 40)
	w.Party().Register(party.Info{ID: probe, Name: "probe"}, partyOutbox{newRecOut()})
	if code := w.Party().Invite(probe, id); code != party.InvalidTarget {
		t.Fatalf("Invite the failed entry %d: %v, want INVALID_TARGET", id, code)
	}
}

// ______________________TEST PARTY OUTBOX_____________________

// fullOut is an outbox whose queue is always full.
type fullOut struct{ kicked []pb.KickReason }

func (o *fullOut) Send(*pb.Envelope) bool { return false }
func (o *fullOut) Kick(r pb.KickReason)   { o.kicked = append(o.kicked, r) }

func TestPartyOutboxKicksWhenFull(t *testing.T) {
	o := &fullOut{}
	if (partyOutbox{out: o}).Send(&party.Notice{Kind: party.NoticeDisbanded}) {
		t.Fatal("Send into a full outbox reported success")
	}
	if !slices.Equal(o.kicked, []pb.KickReason{pb.KickReason_KICK_REASON_SLOW_CONSUMER}) {
		t.Fatalf("kicked = %v, want one SLOW_CONSUMER", o.kicked)
	}
}

// ______________________TEST QUESTS KEPT_____________________

// A character's quest rows and book waiver survive the departure save of a
// portal and the leave save.
func TestQuestsAndWaiverSurviveSaves(t *testing.T) {
	sv := &recSaver{}
	w := startWorldSaver(t, 10, portalMaps(t), sv)
	c := char(2, "m2", 1.5, 9.0)
	c.SkillBookWaiver = 25
	c.Quests = []store.QuestRow{{QuestID: "m01", State: 2, Progress: 5}, {QuestID: "m02", State: 1, Progress: 3}}
	m, _ := enterChar(t, w, c)
	nm, _, err := w.Transfer(context.Background(), m, portal("west"))
	if err != nil {
		t.Fatal(err)
	}
	check := func(when string) {
		t.Helper()
		st, ok := sv.last(2)
		if !ok || st.SkillBookWaiver != 25 || !reflect.DeepEqual(st.Quests, c.Quests) {
			t.Fatalf("%s save %+v, want waiver 25 and both quest rows", when, st)
		}
	}
	check("departure")
	if err := <-nm.Leave(); err != nil {
		t.Fatal(err)
	}
	check("leave")
}

// Enter drops stored quests unknown to quests.json with a warning, clamps
// progress to the objective and resets the day of main-line quests; the
// zone saves the cleaned log.
func TestEnterCleansQuests(t *testing.T) {
	var logBuf lockedBuf
	sv := &recSaver{}
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: 10,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, testMaps(t), testMonsters, sv, slog.New(slog.NewTextHandler(&logBuf, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { cancel(); w.Wait() })
	c := char(3, "m1", 2.5, 2.5)
	c.Quests = []store.QuestRow{
		{QuestID: "gone", State: 1, Progress: 3},
		{QuestID: "m01", State: 2, Progress: 99},
		{QuestID: "m02", State: 1, Progress: 99},
		{QuestID: "m03", State: 1, Progress: 4, Day: 5},
	}
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	want := []*pb.QuestEntry{
		{QuestId: "m01", State: pb.QuestState_QUEST_STATE_DONE, Progress: 5},
		{QuestId: "m02", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 6},
		{QuestId: "m03", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 4},
	}
	got := out.enterResp(t).GetQuests().GetQuests()
	if len(got) != len(want) {
		t.Fatalf("EnterWorldResp quests %v, want %v", got, want)
	}
	for i := range want {
		if !proto.Equal(got[i], want[i]) {
			t.Fatalf("EnterWorldResp quests %v, want %v", got, want)
		}
	}
	if err := <-m.Leave(); err != nil {
		t.Fatal(err)
	}
	st, _ := sv.last(3)
	rows := []store.QuestRow{{QuestID: "m01", State: 2, Progress: 5}, {QuestID: "m02", State: 1, Progress: 6}, {QuestID: "m03", State: 1, Progress: 4}}
	if !reflect.DeepEqual(st.Quests, rows) {
		t.Fatalf("saved quests %+v, want %+v", st.Quests, rows)
	}
	if s := logBuf.String(); !strings.Contains(s, "dropped stored quest") || !strings.Contains(s, "quest=gone") {
		t.Fatalf("log %q, want a warning for quest gone", s)
	}
}

// ____TEST DAILY CLOCK____

// Every zone the world opens takes the quest day from Config.Now.
func TestZonesUseWorldClock(t *testing.T) {
	fake := time.Date(2030, 3, 14, 17, 0, 0, 0, time.UTC)
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: 1,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
		Now: func() time.Time { return fake },
	}, testMaps(t), testMonsters, nopSaver{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { cancel(); w.Wait() })
	enterNamed(t, w, 1, "an", "m1", 5.5, 5.5)
	enterNamed(t, w, 2, "binh", "m1", 5.5, 5.5) // zone 2
	for _, no := range []int{1, 2} {
		if now := slotOf(w, "m1", no).z.Config().Now; now == nil || !now().Equal(fake) {
			t.Fatalf("zone %d clock is not the world clock", no)
		}
	}
}
