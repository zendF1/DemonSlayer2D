package zone

import (
	"context"
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
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/quest"
	"demonslayer2d/internal/store"
)

type fakeOut struct {
	mu    sync.Mutex
	envs  []*pb.Envelope
	kicks []pb.KickReason
	full  bool
}

func (o *fakeOut) Send(e *pb.Envelope) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.full {
		return false
	}
	o.envs = append(o.envs, e)
	return true
}

func (o *fakeOut) Kick(r pb.KickReason) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.kicks = append(o.kicks, r)
}

func (o *fakeOut) take() []*pb.Envelope {
	o.mu.Lock()
	defer o.mu.Unlock()
	e := o.envs
	o.envs = nil
	return e
}

func (o *fakeOut) kicked() []pb.KickReason {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]pb.KickReason(nil), o.kicks...)
}

type fakeSaver struct {
	mu    sync.Mutex
	saves []store.CharState
}

func (s *fakeSaver) Save(st store.CharState, done chan<- error) {
	s.mu.Lock()
	s.saves = append(s.saves, st)
	s.mu.Unlock()
	if done != nil {
		done <- nil
	}
}

func (s *fakeSaver) all() []store.CharState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.CharState(nil), s.saves...)
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// counterID hands out entity ids from start+1 upwards and remembers them.
type counterID struct {
	next   uint64
	issued []uint64
}

func (c *counterID) id() uint64 {
	c.next++
	c.issued = append(c.issued, c.next)
	return c.next
}

func testConfig(capacity int) Config {
	ids := &counterID{next: 1000} // far above the player ids the tests use
	return Config{MapID: "m1", ZoneNo: 1, TickRate: 10, MoveSpeed: 4, AOICellSize: 10, Capacity: capacity, SaveInterval: time.Minute,
		NewID: ids.id, Rand: rand.New(rand.NewPCG(1, 2))}
}

func newTestZone(capacity int, g Grid) (*Zone, *fakeSaver) {
	sv := &fakeSaver{}
	return New(testConfig(capacity), Area{Grid: g, Exp: testExp}, sv, quietLog()), sv
}

func info(id uint64, x, y float32) PlayerInfo {
	return PlayerInfo{EntityID: id, CharID: int64(id) * 100, Name: fmt.Sprint("p", id), Level: 1, Pos: Vec2{x, y}, HP: -1, MP: -1, RespSeq: 7}
}

func joinNow(t *testing.T, z *Zone, id uint64, x, y float32) *fakeOut {
	t.Helper()
	out := &fakeOut{}
	if err := z.join(info(id, x, y), out); err != nil {
		t.Fatal(err)
	}
	return out
}

func enters(envs []*pb.Envelope) []uint64 {
	var ids []uint64
	for _, e := range envs {
		if en := e.GetEntityEnter(); en != nil {
			ids = append(ids, en.Id)
		}
	}
	return ids
}

func leaves(envs []*pb.Envelope) []uint64 {
	var ids []uint64
	for _, e := range envs {
		if l := e.GetEntityLeave(); l != nil {
			ids = append(ids, l.Id)
		}
	}
	return ids
}

func lastTick(envs []*pb.Envelope) *pb.WorldTick {
	for i := len(envs) - 1; i >= 0; i-- {
		if wt := envs[i].GetWorldTick(); wt != nil {
			return wt
		}
	}
	return nil
}

func moveFor(wt *pb.WorldTick, id uint64) *pb.EntityMove {
	for _, m := range wt.GetMoves() {
		if m.Id == id {
			return m
		}
	}
	return nil
}

func TestJoinSendsEnterWorldRespFirst(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	out := joinNow(t, z, 1, 5.5, 5.5)
	envs := out.take()
	if len(envs) != 1 || envs[0].Seq != 7 {
		t.Fatalf("want exactly the EnterWorldResp with seq 7, got %v", envs)
	}
	r := envs[0].GetEnterWorldResp()
	if r == nil || r.SelfEntityId != 1 || r.MapId != "m1" || r.Zone != 1 || r.X != 5.5 || r.Y != 5.5 || r.MoveSpeed != 4 || r.TickRate != 10 {
		t.Fatalf("bad EnterWorldResp %v", r)
	}
	z.advance(time.Now())
	envs = out.take()
	if !slices.Equal(enters(envs), []uint64{1}) || lastTick(envs) == nil {
		t.Fatalf("after first tick want own EntityEnter then WorldTick, got %v", envs)
	}
}

func TestNearbyPlayersSeeEachOther(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	b := joinNow(t, z, 2, 8.5, 5.5)
	z.advance(time.Now())
	if got := enters(a.take()); !slices.Contains(got, 2) {
		t.Fatalf("a should see b, enters=%v", got)
	}
	if got := enters(b.take()); !slices.Contains(got, 1) {
		t.Fatalf("b should see a, enters=%v", got)
	}
}

func TestMoveIsBroadcast(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	b := joinNow(t, z, 2, 8.5, 5.5)
	z.advance(time.Now())
	a.take()
	b.take()

	z.move(moveCmd{id: 1, seq: 3, dir: Vec2{1, 0}})
	z.advance(time.Now())
	m := moveFor(lastTick(b.take()), 1)
	if m == nil || !near(m.X, 5.9) || m.DirX != 1 {
		t.Fatalf("b should get a's move to x=5.9, got %v", m)
	}
	ta := lastTick(a.take())
	if ta.AckInputSeq != 3 || moveFor(ta, 1) == nil {
		t.Fatalf("a should get its own move and ack seq 3, got %v", ta)
	}

	z.move(moveCmd{id: 1, seq: 4, dir: Vec2{}})
	z.advance(time.Now())
	if m := moveFor(lastTick(b.take()), 1); m == nil || m.DirX != 0 {
		t.Fatalf("stopping must be broadcast once, got %v", m)
	}
	z.advance(time.Now())
	if m := moveFor(lastTick(b.take()), 1); m != nil {
		t.Fatalf("an idle entity must not be resent, got %v", m)
	}
}

func TestOldInputSeqIgnored(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	z.advance(time.Now()) // flush the player's own EntityEnter
	a.take()
	z.move(moveCmd{id: 1, seq: 5, dir: Vec2{1, 0}})
	z.move(moveCmd{id: 1, seq: 4, dir: Vec2{-1, 0}})
	z.advance(time.Now())
	if m := moveFor(lastTick(a.take()), 1); m == nil || m.DirX != 1 {
		t.Fatalf("older input must not override newer, got %v", m)
	}
}

func TestFarPlayersAreInvisible(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 35.5, 5.5)
	z.advance(time.Now())
	if slices.Contains(enters(a.take()), 2) {
		t.Fatal("b is 3 cells away and must not be visible")
	}
}

func TestWalkingIntoAndOutOfView(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 35.5, 5.5)
	z.move(moveCmd{id: 2, seq: 1, dir: Vec2{-1, 0}})
	seen := false
	for i := 0; i < 60 && !seen; i++ {
		z.advance(time.Now())
		seen = slices.Contains(enters(a.take()), 2)
	}
	if !seen {
		t.Fatal("b never came into view")
	}
	z.move(moveCmd{id: 2, seq: 2, dir: Vec2{1, 0}})
	gone := false
	for i := 0; i < 60 && !gone; i++ {
		z.advance(time.Now())
		gone = slices.Contains(leaves(a.take()), 2)
	}
	if !gone {
		t.Fatal("b never left view")
	}
}

func TestCapacity(t *testing.T) {
	z, _ := newTestZone(2, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 5.5, 5.5)
	if err := z.join(info(3, 5.5, 5.5), &fakeOut{}); !errors.Is(err, ErrZoneFull) {
		t.Fatalf("err = %v, want ErrZoneFull", err)
	}
	if z.Count() != 2 {
		t.Fatalf("count = %d", z.Count())
	}
}

func TestLeaveSavesAndOthersSeeLeave(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 6.5, 5.5)
	z.advance(time.Now())
	a.take()

	if err := <-z.leave(2); err != nil {
		t.Fatal(err)
	}
	if saves := sv.all(); len(saves) != 1 || saves[0].ID != 200 || saves[0].MapID != "m1" || saves[0].X != 6.5 {
		t.Fatalf("saves = %+v", saves)
	}
	z.advance(time.Now())
	if !slices.Contains(leaves(a.take()), 2) {
		t.Fatal("a should get EntityLeave for b")
	}
	if z.Count() != 1 {
		t.Fatalf("count = %d", z.Count())
	}
	if err := <-z.leave(99); err != nil {
		t.Fatalf("leaving an unknown entity must succeed, got %v", err)
	}
}

func TestSlowConsumerIsKickedAndSaved(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5)
	b := joinNow(t, z, 2, 6.5, 5.5)
	b.full = true
	z.advance(time.Now())
	if k := b.kicked(); !slices.Equal(k, []pb.KickReason{pb.KickReason_KICK_REASON_SLOW_CONSUMER}) {
		t.Fatalf("kicks = %v", k)
	}
	if z.Count() != 1 || len(sv.all()) != 1 {
		t.Fatalf("count=%d saves=%v", z.Count(), sv.all())
	}
	if err := <-z.leave(2); err != nil {
		t.Fatalf("leave after eviction should return the eviction save result, got %v", err)
	}
}

// holdSaver keeps each save pending until release, like a slow database.
type holdSaver struct {
	mu   sync.Mutex
	held []chan<- error
}

func (s *holdSaver) Save(_ store.CharState, done chan<- error) {
	if done == nil {
		return
	}
	s.mu.Lock()
	s.held = append(s.held, done)
	s.mu.Unlock()
}

func (s *holdSaver) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.held {
		d <- nil
	}
	s.held = nil
}

// A zone emptied by a slow-consumer kick still holds that player's save; when
// the zone stops (closeIdle) before the connection's Leave arrives, Leave must
// still hand back that save instead of an already-closed nil, or the session
// ends before the save commits.
func TestLeaveAfterStopReturnsTheDepartedSave(t *testing.T) {
	sv := &holdSaver{}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp}, sv, quietLog())
	b := joinNow(t, z, 2, 6.5, 5.5)
	b.full = true
	z.advance(time.Now())
	if z.Count() != 0 {
		t.Fatalf("count = %d, want the kicked player gone", z.Count())
	}
	ctx, cancel := context.WithCancel(context.Background())
	go z.Run(ctx)
	cancel()
	<-z.Done()

	ch := z.Leave(2)
	select {
	case err := <-ch:
		t.Fatalf("Leave returned %v before the departed save committed", err)
	default:
	}
	sv.release()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Leave never yielded the departed save")
	}
}

func TestStateSavesEntityHPAndLevel(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5) // HP -1: full
	joinNow(t, z, 2, 6.5, 5.5)
	p := z.players[2]
	p.hp, p.level = 120, 3
	<-z.leave(1)
	<-z.leave(2)
	saves := sv.all()
	if len(saves) != 2 || saves[0].HP != 500 || saves[0].Level != 1 {
		t.Fatalf("a full player saves its real HP 500, got %+v", saves)
	}
	if saves[1].HP != 120 || saves[1].Level != 3 {
		t.Fatalf("saves must come from the entity's hp and level, got %+v", saves[1])
	}
}

func TestPeriodicSave(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5)
	t0 := time.Now()
	z.advance(t0)
	z.advance(t0.Add(30 * time.Second))
	if n := len(sv.all()); n != 0 {
		t.Fatalf("saved too early: %d", n)
	}
	z.advance(t0.Add(61 * time.Second))
	if n := len(sv.all()); n != 1 {
		t.Fatalf("want 1 periodic save, got %d", n)
	}
}

type panicGrid struct{ on bool }

func (g *panicGrid) Blocked(int, int) bool {
	if g.on {
		panic("boom")
	}
	return false
}

func TestPanicEvictsPlayers(t *testing.T) {
	g := &panicGrid{}
	z, sv := newTestZone(10, g)
	a := joinNow(t, z, 1, 5.5, 5.5)
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{1, 0}})
	g.on = true
	z.safeAdvance(time.Now())
	if k := a.kicked(); !slices.Equal(k, []pb.KickReason{pb.KickReason_KICK_REASON_ZONE_RESTART}) {
		t.Fatalf("kicks = %v", k)
	}
	if z.Count() != 0 || len(sv.all()) != 1 {
		t.Fatalf("count=%d saves=%d", z.Count(), len(sv.all()))
	}
	g.on = false
	joinNow(t, z, 2, 5.5, 5.5) // zone keeps working
	z.safeAdvance(time.Now())
}

func TestRunJoinLeaveAndShutdown(t *testing.T) {
	sv := &fakeSaver{}
	cfg := testConfig(10)
	cfg.TickRate = 50
	z := New(cfg, Area{Grid: openGrid(64, 64), Exp: testExp}, sv, quietLog())
	ctx, cancel := context.WithCancel(context.Background())
	go z.Run(ctx)

	o1, o2 := &fakeOut{}, &fakeOut{}
	if err := z.Join(ctx, info(1, 5.5, 5.5), o1); err != nil {
		t.Fatal(err)
	}
	if err := z.Join(ctx, info(2, 6.5, 5.5), o2); err != nil {
		t.Fatal(err)
	}
	z.Move(1, 1, 1, 0)
	time.Sleep(100 * time.Millisecond)
	if err := <-z.Leave(1); err != nil {
		t.Fatal(err)
	}
	if st := z.TakeStats(); st.Ticks == 0 {
		t.Fatal("no ticks recorded")
	}

	cancel()
	select {
	case <-z.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("zone did not stop")
	}
	if k := o2.kicked(); !slices.Equal(k, []pb.KickReason{pb.KickReason_KICK_REASON_SERVER_SHUTDOWN}) {
		t.Fatalf("o2 kicks = %v", k)
	}
	if n := len(sv.all()); n != 2 {
		t.Fatalf("saves = %d, want 2 (leave + shutdown)", n)
	}
	if err := z.Join(context.Background(), info(3, 5.5, 5.5), &fakeOut{}); !errors.Is(err, ErrZoneClosed) {
		t.Fatalf("join after stop err = %v", err)
	}
	if err := <-z.Leave(2); err != nil {
		t.Fatalf("leave after stop err = %v", err)
	}
}

// Once a join is queued the zone will apply it and send EnterWorldResp, so Join must
// report the zone's answer rather than the caller's expired context.
func TestJoinWaitsForReplyOnceQueued(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	out := &fakeOut{}
	errc := make(chan error, 1)
	go func() { errc <- z.Join(ctx, info(1, 5.5, 5.5), out) }()

	time.Sleep(60 * time.Millisecond) // ctx has expired; the join is still in the inbox
	z.advance(time.Now())
	if err := <-errc; err != nil {
		t.Fatalf("Join = %v, want nil because the zone applied the join", err)
	}
	if z.Count() != 1 || out.take()[0].GetEnterWorldResp() == nil {
		t.Fatal("player should be in the zone with EnterWorldResp sent")
	}
}

func TestJoinHonoursContextBeforeQueueing(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	for range cap(z.inbox) {
		z.inbox <- moveCmd{} // fill the inbox so the join cannot be queued
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := z.Join(ctx, info(1, 5.5, 5.5), &fakeOut{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Join = %v, want context.DeadlineExceeded", err)
	}
}

// ______________________TEST MONSTERS_____________________

var testKind = gamedata.Kind{ID: "wanderer", Name: "Quỷ lang thang", AttackRange: 2.0, AttackSec: 1.5, Speed: 0.8,
	DetectRange: 5, ChaseRange: 10, ShootSec: 2, ShootWindupSec: 0.5, RegionMargin: 3, ChaseSpeed: 1.2, SafeWaitSec: 5,
	MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8, MeleeWindupMS: 200, ShotSpeed: 15,
	HPPct: 100, AttackPct: 100, DamageType: gamedata.DamagePhys}

var testLevels = map[int]gamedata.Level{
	1: {Level: 1, HP: 56, Attack: 30, Defense: 8, Exp: 6},
	2: {Level: 2, HP: 104, Attack: 39, Defense: 16, Exp: 11},
	// Copied from data/monsters.json for the EXP level window and the cap.
	12: {Level: 12, HP: 880, Attack: 110, Defense: 96, Exp: 185},
	85: {Level: 85, HP: 8578, Attack: 640, Defense: 680, Exp: 8063},
}

// testExp is the real EXP table, data/exp.json.
var testExp = func() combat.ExpTable {
	b, err := os.ReadFile("../../data/exp.json")
	if err != nil {
		panic(err)
	}
	t, err := gamedata.ParseExp(b)
	if err != nil {
		panic(err)
	}
	return t
}()

// newMonsterZone builds a zone on a 60x60 open grid with the safe rect (0,0,10,10)
// and one region of 3 monsters on the given tiles, with its nest on the first.
func newMonsterZone(t *testing.T, tiles ...Vec2) (*Zone, *counterID) {
	t.Helper()
	ids := &counterID{next: 100}
	cfg := testConfig(10)
	cfg.NewID = ids.id
	area := Area{
		Grid:     openGrid(60, 60),
		Safe:     []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Rests:    []Circle{{C: Vec2{3.5, 3.5}, R: 1.5}},
		Respawns: []Vec2{{5.5, 5.5}},
		Regions:  []Region{{ID: "r1", Tiles: tiles, Nest: tiles[0], Count: 3, MinLevel: 1, MaxLevel: 2, Kind: testKind}},
		Levels:   testLevels,
		Exp:      testExp,
	}
	return New(cfg, area, &fakeSaver{}, quietLog()), ids
}

var fewTiles = []Vec2{{30.5, 30.5}, {31.5, 30.5}, {30.5, 31.5}}

func dist(a, b Vec2) float64 { return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) }

func TestPlayerSeesMonsters(t *testing.T) {
	z, _ := newMonsterZone(t, fewTiles...)
	a := joinNow(t, z, 1, 28.5, 30.5)
	z.advance(time.Now())
	seen := 0
	for _, e := range a.take() {
		en := e.GetEntityEnter()
		if en == nil || en.Kind != pb.EntityKind_ENTITY_KIND_MONSTER {
			continue
		}
		m := z.monsters[en.Id]
		if m == nil {
			t.Fatalf("enter for unknown monster %d", en.Id)
		}
		if en.Name != testKind.Name || en.CharClass != pb.CharClass_CHAR_CLASS_UNSPECIFIED || int(en.Level) != m.level ||
			en.Hp != en.MaxHp || int(en.MaxHp) != testLevels[m.level].HP || en.Dead {
			t.Fatalf("bad monster enter %v", en)
		}
		seen++
	}
	if seen != 3 {
		t.Fatalf("saw %d monsters, want 3", seen)
	}
}

func TestPlayerEnterHasHP(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	hurt := info(2, 6.5, 5.5)
	hurt.HP = 120
	if err := z.join(hurt, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	z.advance(time.Now())
	got := map[uint64]*pb.EntityEnter{}
	for _, e := range a.take() {
		if en := e.GetEntityEnter(); en != nil {
			got[en.Id] = en
		}
	}
	// A level-1 Viêm (physical) has 500 max HP; HP -1 means full.
	if en := got[1]; en == nil || en.Kind != pb.EntityKind_ENTITY_KIND_PLAYER || en.MaxHp != 500 || en.Hp != 500 {
		t.Fatalf("full player: got %v", en)
	}
	if en := got[2]; en == nil || en.MaxHp != 500 || en.Hp != 120 {
		t.Fatalf("hurt player: got %v", en)
	}
}

func TestMonstersNeverEnterSafeZone(t *testing.T) {
	// Region tiles border the safe rect (0,0,10,10) on its right edge.
	z, _ := newMonsterZone(t, Vec2{10.5, 5.5}, Vec2{10.5, 6.5}, Vec2{11.5, 5.5})
	now := time.Now()
	for i := 0; i < 1000; i++ {
		z.advance(now)
		for id, m := range z.monsters {
			if z.area.InSafe(m.pos) {
				t.Fatalf("tick %d: monster %d at %v is in the safe zone", i, id, m.pos)
			}
		}
	}
}

// A wall splits the region in two. A wander target behind the wall is never
// picked, so every walk arrives at its target instead of sticking at the
// wall, a walking monster really moves every tick, and no monster ever gets
// to the other side.
func TestWanderDoesNotWalkIntoWalls(t *testing.T) {
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		if y >= 25 && y <= 36 {
			r[32] = '#'
		}
		rows[y] = string(r)
	}
	var tiles []Vec2
	for y := 26; y <= 35; y++ {
		tiles = append(tiles, Vec2{31.5, float32(y) + 0.5}, Vec2{33.5, float32(y) + 0.5})
	}
	z, _ := newMonsterZone(t, tiles...)
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	step := 0.5 * testKind.Speed * 4 / 10
	prev := map[uint64]Vec2{}
	west := map[uint64]bool{}
	walking := map[uint64]bool{}
	walks := 0
	now := time.Now()
	for id, m := range z.monsters {
		prev[id], west[id] = m.pos, m.pos.X < 32
	}
	for i := 0; i < 2000; i++ {
		z.advance(now)
		for id, m := range z.monsters {
			if (m.pos.X < 32) != west[id] {
				t.Fatalf("tick %d: monster %d got to the other side of the wall, at %v", i, id, m.pos)
			}
			if m.dir == (Vec2{}) {
				if walking[id] {
					walks++
					if m.pos != m.wanderTo {
						t.Fatalf("tick %d: monster %d stopped at %v short of its target %v", i, id, m.pos, m.wanderTo)
					}
				}
				walking[id] = false
			} else {
				if d := dist(m.pos, prev[id]); d < 0.1*step-1e-4 {
					t.Fatalf("tick %d: monster %d walks (dir %v) but moved only %.5f tiles", i, id, m.dir, d)
				}
				walking[id] = true
			}
			prev[id] = m.pos
		}
	}
	if walks < 20 {
		t.Fatalf("only %d wander walks finished", walks)
	}
}

func TestWalkToResults(t *testing.T) {
	z, _ := newMonsterZone(t, fewTiles...)
	z.area.Grid = gridFrom(
		"......",
		"...#..",
		"......",
	)
	z.area.Safe = nil
	z.mgrid = z.area.monsterGrid()
	m := &monster{entity: entity{pos: Vec2{1.5, 1.5}}}
	if r := z.walkTo(m, Vec2{2.5, 1.5}, 0.4); r != walkMoving || m.dir != (Vec2{1, 0}) {
		t.Fatalf("first step: result %v dir %v, want moving east", r, m.dir)
	}
	for i := 0; i < 5 && z.walkTo(m, Vec2{2.5, 1.5}, 0.4) == walkMoving; i++ {
	}
	if m.pos != (Vec2{2.5, 1.5}) || m.dir != (Vec2{}) {
		t.Fatalf("want arrived and stopped at the target, at %v dir %v", m.pos, m.dir)
	}
	if r := z.walkTo(m, Vec2{2.5, 1.5}, 0.4); r != walkArrived {
		t.Fatalf("standing on the target: result %v, want arrived", r)
	}
	// Straight into the wall at x=3: it reaches the wall, then is stuck.
	var r walkResult
	for i := 0; i < 10; i++ {
		if r = z.walkTo(m, Vec2{4.5, 1.5}, 0.4); r != walkMoving {
			break
		}
	}
	if r != walkStuck || m.dir != (Vec2{}) || m.pos.X != 3-halfSize {
		t.Fatalf("result %v at %v dir %v, want stuck and stopped at the wall", r, m.pos, m.dir)
	}
}

func TestMonsterGridBlocksSafeTiles(t *testing.T) {
	z, _ := newMonsterZone(t, Vec2{10.5, 5.5}, Vec2{10.5, 6.5}, Vec2{11.5, 5.5})
	g := z.area.monsterGrid()
	if !g.Blocked(5, 5) || !g.Blocked(9, 0) || z.area.Grid.Blocked(5, 5) {
		t.Fatal("safe tiles must block monsters but not the map grid")
	}
	if g.Blocked(10, 5) || !g.Blocked(-1, 5) {
		t.Fatal("outside the safe rect the monster grid follows the map grid")
	}
	if (&Area{Grid: z.area.Grid}).monsterGrid() != z.area.Grid {
		t.Fatal("without safe zones the monster grid is the map grid")
	}
	m := &monster{entity: entity{pos: Vec2{10.5, 5.5}}, region: z.regions[0]}
	for i := 0; i < 1000; i++ {
		to, ok := z.wanderTarget(m)
		if !ok || z.area.InSafe(to) || !slices.Contains(m.region.Tiles, to) || to == m.pos {
			t.Fatalf("wander target %v ok=%v, want another region tile", to, ok)
		}
	}
	if p := Step(z.mgrid, Vec2{11.5, 5.5}, Vec2{-1, 0}, 5); p.X != 10+halfSize {
		t.Fatalf("a monster walking into the safe rect stops at %v, want x=%v", p, 10+halfSize)
	}
}

func TestDeadMonsterIsNotNewlyEntered(t *testing.T) {
	z, _ := newMonsterZone(t, fewTiles...)
	a := joinNow(t, z, 1, 28.5, 30.5)
	z.advance(time.Now())
	a.take()
	var id uint64
	for id = range z.monsters {
		break
	}
	z.monsters[id].dead = true
	b := joinNow(t, z, 2, 28.5, 31.5)
	z.advance(time.Now())
	if slices.Contains(enters(b.take()), id) {
		t.Fatal("a newcomer must not get EntityEnter for a corpse")
	}
	if _, still := z.players[2].visible[id]; still {
		t.Fatal("a corpse must not be added to a newcomer's visible set")
	}
	if slices.Contains(leaves(a.take()), id) {
		t.Fatal("a viewer who already saw the monster keeps it")
	}
}

func TestTicksRoundsUp(t *testing.T) {
	z, _ := newTestZone(10, openGrid(8, 8))
	for _, c := range []struct {
		sec  float64
		want uint32
	}{{0.8, 8}, {0.05, 1}, {1.5, 15}, {0.7, 7}, {0, 1}} {
		if got := z.ticks(c.sec); got != c.want {
			t.Errorf("ticks(%v) = %d, want %d", c.sec, got, c.want)
		}
	}
}

// ______________________TEST REGIONS_____________________

// rectTiles lists the tile centres of r, sorted by (y, x).
func rectTiles(r Rect) []Vec2 {
	var out []Vec2
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			out = append(out, Vec2{float32(x) + 0.5, float32(y) + 0.5})
		}
	}
	return out
}

// newRegionZone builds a zone on a 60x60 open grid with the safe rect
// (0,0,10,10) and the given regions, its randomness seeded with seed.
func newRegionZone(t *testing.T, seed uint64, regions ...Region) (*Zone, *counterID) {
	t.Helper()
	ids := &counterID{next: 100}
	cfg := testConfig(10)
	cfg.NewID = ids.id
	cfg.Rand = rand.New(rand.NewPCG(seed, 2))
	area := Area{
		Grid:     openGrid(60, 60),
		Safe:     []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Respawns: []Vec2{{5.5, 5.5}},
		Regions:  regions,
		Levels:   testLevels,
		Exp:      testExp,
	}
	return New(cfg, area, &fakeSaver{}, quietLog()), ids
}

// minPairwise is the smallest distance between two of ps.
func minPairwise(ps []Vec2) float64 {
	best := math.Inf(1)
	for i := range ps {
		for j := i + 1; j < len(ps); j++ {
			best = min(best, dist(ps[i], ps[j]))
		}
	}
	return best
}

func TestSpawnFillsRegions(t *testing.T) {
	west := []Rect{{X: 12, Y: 12, W: 6, H: 5}}
	east := []Rect{{X: 40, Y: 12, W: 5, H: 8}, {X: 45, Y: 12, W: 2, H: 2}}
	eastTiles := append(rectTiles(east[0]), rectTiles(east[1])...)
	z, ids := newRegionZone(t, 1,
		Region{ID: "west", Tiles: rectTiles(west[0]), Rects: west, Nest: Vec2{14.5, 14.5}, Count: 5, MinLevel: 1, MaxLevel: 2, Kind: testKind},
		Region{ID: "east", Tiles: eastTiles, Rects: east, Nest: Vec2{42.5, 15.5}, Count: 3, MinLevel: 2, MaxLevel: 2, Kind: testKind},
	)
	if len(z.regions) != 2 || z.regions[0].ID != "west" || z.regions[1].ID != "east" {
		t.Fatalf("regions %v, want west and east in map order", z.regions)
	}
	if got := slices.Sorted(maps.Keys(z.monsters)); len(got) != 8 || !slices.Equal(got, ids.issued) {
		t.Fatalf("monster ids %v, want the 8 ids NewID issued %v", got, ids.issued)
	}
	count := map[string]int{}
	for id, m := range z.monsters {
		r := m.region
		if r == nil {
			t.Fatalf("monster %d has no region", id)
		}
		count[r.ID]++
		if m.id != id || m.kind != pb.EntityKind_ENTITY_KIND_MONSTER || m.name != testKind.Name {
			t.Fatalf("monster %d: bad identity %+v", id, m.entity)
		}
		if !slices.Contains(r.Tiles, m.pos) || !r.hasTile(floor(m.pos.X), floor(m.pos.Y)) {
			t.Fatalf("monster %d at %v, want on a tile of region %s", id, m.pos, r.ID)
		}
		if m.level < r.MinLevel || m.level > r.MaxLevel {
			t.Fatalf("monster %d level %d outside region %s's %d-%d", id, m.level, r.ID, r.MinLevel, r.MaxLevel)
		}
		if m.maxHP != testLevels[m.level].HP || m.hp != m.maxHP || m.dead {
			t.Fatalf("monster %d hp %d/%d dead=%v, want full %d", id, m.hp, m.maxHP, m.dead, testLevels[m.level].HP)
		}
		if _, in := z.cells[m.cell][id]; !in {
			t.Fatalf("monster %d is not in its AOI cell", id)
		}
		if m.attackers == nil || m.dmgBy == nil || m.expPaid == nil {
			t.Fatalf("monster %d has nil maps", id)
		}
	}
	for _, r := range z.regions {
		if count[r.ID] != r.Count || r.present != r.Count || len(r.pending) != 0 {
			t.Fatalf("region %s: %d monsters, present %d, pending %v, want %d", r.ID, count[r.ID], r.present, r.pending, r.Count)
		}
	}
	// The tile set is the region's own: east's second rect counts, west's tiles do not.
	if !z.regions[1].hasTile(46, 13) || z.regions[1].hasTile(14, 14) || z.regions[0].hasTile(46, 13) || z.regions[0].hasTile(18, 12) {
		t.Fatal("hasTile does not match the region tiles")
	}
}

// Best-candidate placement spreads a region's monsters out: on a 20x20 open
// region 8 monsters stay at least 2 tiles apart, for every seed tried.
// Uniform random placement (one sample) fails it for 6 of these 20 seeds, so
// the seeds together catch a placement that does not spread.
func TestSpawnSpreadsOut(t *testing.T) {
	open := Rect{X: 20, Y: 20, W: 20, H: 20}
	for seed := uint64(1); seed <= 20; seed++ {
		z, _ := newRegionZone(t, seed, Region{ID: "open", Tiles: rectTiles(open), Rects: []Rect{open}, Nest: Vec2{30.5, 30.5},
			Count: 8, MinLevel: 1, MaxLevel: 2, Kind: testKind})
		var ps []Vec2
		for _, m := range z.monsters {
			ps = append(ps, m.pos)
		}
		if len(ps) != 8 {
			t.Fatalf("seed %d: %d monsters, want 8", seed, len(ps))
		}
		if d := minPairwise(ps); d < 2 {
			t.Errorf("seed %d: monsters %v only %.2f tiles apart, want at least 2", seed, ps, d)
		}
	}
}

// A dead monster comes back respawn_sec after its death at the region's nest,
// not where it first spawned, with full HP and fresh EXP tracking (spec 4.1).
func TestRespawnAtNest(t *testing.T) {
	area := Rect{X: 20, Y: 20, W: 10, H: 10}
	nest := Vec2{29.5, 29.5}
	z, _ := newRegionZone(t, 1, Region{ID: "r", Tiles: rectTiles(area), Rects: []Rect{area}, Nest: nest,
		Count: 1, MinLevel: 1, MaxLevel: 2, Kind: testKind})
	if len(z.monsters) != 1 {
		t.Fatalf("monsters = %d, want 1", len(z.monsters))
	}
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	if m.pos == nest {
		t.Fatal("the first spawn landed on the nest; pick another seed so the test means something")
	}
	r := m.region
	m.dmgBy[1], m.expPaid[1] = 20, 3
	z.killMonster(m, nil)
	diedAt := z.tickNo
	due := diedAt + z.ticks(testKind.RespawnSec)
	now := time.Now()
	for z.tickNo < due {
		for id := range z.monsters {
			if id != m.id {
				t.Fatalf("tick %d: monster %d spawned before tick %d", z.tickNo, id, due)
			}
		}
		z.advance(now)
	}
	if len(z.monsters) != 1 {
		t.Fatalf("at tick %d: %d monsters, want only the new one", z.tickNo, len(z.monsters))
	}
	for id, nm := range z.monsters {
		if id == m.id || nm.dead || nm.region != r {
			t.Fatalf("monster %d dead=%v region %v, want a new live monster of region r", id, nm.dead, nm.region)
		}
		if nm.pos != nest {
			t.Fatalf("respawned at %v, want the nest %v", nm.pos, nest)
		}
		if nm.level < 1 || nm.level > 2 || nm.hp != testLevels[nm.level].HP || nm.maxHP != nm.hp {
			t.Fatalf("respawned level %d hp %d/%d, want full HP at level 1-2", nm.level, nm.hp, nm.maxHP)
		}
		if len(nm.dmgBy) != 0 || len(nm.expPaid) != 0 || len(nm.attackers) != 0 {
			t.Fatalf("respawned monster keeps damage %v, exp %v, attackers %v", nm.dmgBy, nm.expPaid, nm.attackers)
		}
	}
	if r.present != 1 || len(r.pending) != 0 {
		t.Fatalf("region present %d pending %v, want 1 and none", r.present, r.pending)
	}
}

// nearestTile is the region tile nearest a point, the first in (y, x) order
// on a tie; nearMargin is whether a point lies within margin tiles of the
// region's rects.
func TestRegionNearestTileAndMargin(t *testing.T) {
	rects := []Rect{{X: 10, Y: 10, W: 5, H: 5}, {X: 20, Y: 10, W: 2, H: 2}}
	r := newRegionState(Region{ID: "r", Tiles: append(rectTiles(rects[0]), rectTiles(rects[1])...), Rects: rects})
	for p, want := range map[Vec2]Vec2{
		{12.2, 12.7}: {12.5, 12.5}, // on a tile
		{5.0, 5.0}:   {10.5, 10.5},
		{17.5, 10.5}: {14.5, 10.5}, // tie between 14.5 and 20.5: the first
		{19.0, 11.9}: {20.5, 11.5},
	} {
		if got := r.nearestTile(p); got != want {
			t.Errorf("nearestTile(%v) = %v, want %v", p, got, want)
		}
	}
	for _, c := range []struct {
		p    Vec2
		want bool
	}{
		{Vec2{12, 12}, true}, {Vec2{7.01, 12}, true}, {Vec2{6.99, 12}, false}, // x edge 10 - 3 = 7
		{Vec2{17.99, 16}, true}, {Vec2{18.0, 16}, false}, // x edge 15 + 3 = 18, exclusive (y 16 is past the second rect's margin)
		{Vec2{24.9, 12}, true}, {Vec2{12, 17.9}, true}, {Vec2{12, 18.1}, false},
	} {
		if got := r.nearMargin(c.p, 3); got != c.want {
			t.Errorf("nearMargin(%v, 3) = %v, want %v", c.p, got, c.want)
		}
	}
}

// ______________________TEST LEARN_____________________

// learnZone joins a Nham player 1 of the given level and skill points, with
// skills learned, into a zone that has testSkills.
func learnZone(t *testing.T, level, points int, skills map[string]int) (*Zone, *player, *fakeOut, *fakeSaver) {
	t.Helper()
	sv := &fakeSaver{}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Skills: testSkills}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.SkillPoints, in.Skills = pb.CharClass_CHAR_CLASS_NHAM, level, points, skills
	in.SkillBookWaiver = combat.MaxLevel // these tests are not about books
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	out.take()
	return z, z.players[1], out, sv
}

// learnNow queues learning skillID by id with seq and runs one tick.
func learnNow(t *testing.T, z *Zone, id uint64, seq uint32, skillID string) {
	t.Helper()
	if !z.Learn(id, seq, skillID) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func learnRespFor(envs []*pb.Envelope, seq uint32) *pb.LearnSkillResp {
	for _, e := range envs {
		if r := e.GetLearnSkillResp(); r != nil && e.Seq == seq {
			return r
		}
	}
	return nil
}

// Each check of phase 2 spec 3.1 in order: a case breaks every later check
// too, so the first failing check is the one answered.
func TestLearnErrors(t *testing.T) {
	for _, c := range []struct {
		name   string
		level  int
		points int
		skills map[string]int
		dead   bool
		learn  []string // all but the last must succeed
		code   pb.ErrorCode
	}{
		{"dead with a bad id", combat.MaxLevel, 5, nil, true, []string{"nope"}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"unknown id", 1, 0, nil, false, []string{"nope"}, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"other class", 1, 0, nil, false, []string{"viem_1"}, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"maxed", 10, 0, map[string]int{"nham_1": gamedata.SkillLevels}, false, []string{"nham_1"}, pb.ErrorCode_ERROR_CODE_SKILL_MAXED},
		{"level too low to learn", 9, 0, nil, false, []string{"nham_1"}, pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW},
		{"level too low to raise", 10, 2, nil, false, []string{"nham_1", "nham_1"}, pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW},
		{"no points", 10, 0, nil, false, []string{"nham_1"}, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, out, sv := learnZone(t, c.level, c.points, c.skills)
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
				z.advance(time.Now())
				out.take()
			}
			for i, id := range c.learn[:len(c.learn)-1] {
				learnNow(t, z, 1, uint32(i+1), id)
				if learnRespFor(out.take(), uint32(i+1)) == nil {
					t.Fatalf("setup learn %d of %s failed", i+1, id)
				}
			}
			points, skills, stats, saves := p.skillPoints, maps.Clone(p.skills), p.stats, len(sv.all())
			learnNow(t, z, 1, 9, c.learn[len(c.learn)-1])
			envs := out.take()
			if er := errorFor(envs, 9); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if learnRespFor(envs, 9) != nil || p.skillPoints != points || !maps.Equal(p.skills, skills) || p.stats != stats {
				t.Fatalf("a rejected learn changed the player: points %d skills %v", p.skillPoints, p.skills)
			}
			if n := len(sv.all()); n != saves {
				t.Fatalf("%d saves, want %d", n, saves)
			}
		})
	}
}

// Learning spends a point, raises the skill a level, saves at once and
// replies with the new stats and skills.
func TestLearn(t *testing.T) {
	z, p, out, sv := learnZone(t, 11, 3, map[string]int{"nham_3": 1})
	learnNow(t, z, 1, 4, "nham_1")
	learnNow(t, z, 1, 5, "nham_1")
	if p.skillPoints != 1 || !maps.Equal(p.skills, map[string]int{"nham_1": 2, "nham_3": 1}) {
		t.Fatalf("points %d skills %v, want 1 and nham_1 at 2", p.skillPoints, p.skills)
	}
	envs := out.take()
	r := learnRespFor(envs, 5)
	if r == nil || r.Stats.String() != z.selfStats(p).String() || r.Stats.SkillPoints != 1 {
		t.Fatalf("LearnSkillResp %v, want the new stats %v", r, z.selfStats(p))
	}
	want := []*pb.SkillLevel{{SkillId: "nham_1", Level: 2}, {SkillId: "nham_3", Level: 1}}
	if len(r.Skills) != len(want) || r.Skills[0].String() != want[0].String() || r.Skills[1].String() != want[1].String() {
		t.Fatalf("skills %v, want %v", r.Skills, want)
	}
	if r1 := learnRespFor(envs, 4); r1 == nil || len(r1.Skills) != 2 || r1.Skills[0].Level != 1 {
		t.Fatalf("first LearnSkillResp %v, want nham_1 at 1", r1)
	}
	saves := sv.all()
	if len(saves) != 2 || saves[1].SkillPoints != 1 || !maps.Equal(saves[1].Skills, p.skills) {
		t.Fatalf("saves %+v, want one save per learn with the new skills", saves)
	}
	if cs := hpChangesIn(envs); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none for an active skill", cs)
	}
}

// Learning the Nham passive raises max HP and current HP by the same amount,
// and viewers see the new bar.
func TestLearnPassiveRaisesHP(t *testing.T) {
	z, p, out, _ := learnZone(t, 31, 2, map[string]int{"nham_3": 1})
	p.hp, p.mp = 100, 7
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	before := combat.Compute(false, 31, combat.Points{}, combat.Bonus{}, combat.Passive{MaxHPPct: 2, PhysDefPct: 2})
	after := combat.Compute(false, 31, combat.Points{}, combat.Bonus{}, combat.Passive{MaxHPPct: 4, PhysDefPct: 4})
	if p.maxHP != before.MaxHP || after.MaxHP <= before.MaxHP {
		t.Fatalf("setup: max HP %d, want %d below %d", p.maxHP, before.MaxHP, after.MaxHP)
	}
	learnNow(t, z, 1, 3, "nham_3")
	wantHP := 100 + after.MaxHP - before.MaxHP
	if p.maxHP != after.MaxHP || p.hp != wantHP || p.mp != 7 || p.stats != after {
		t.Fatalf("hp %d/%d mp %d, want %d/%d and mp 7", p.hp, p.maxHP, p.mp, wantHP, after.MaxHP)
	}
	envs := out.take()
	if r := learnRespFor(envs, 3); r == nil || r.Stats.MaxHp != int32(after.MaxHP) || r.Stats.Hp != int32(wantHP) {
		t.Fatalf("LearnSkillResp %v", r)
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(wantHP) || cs[0].MaxHp != int32(after.MaxHP) {
			t.Fatalf("%s HpChanges %v, want %d/%d for 1", name, cs, wantHP, after.MaxHP)
		}
	}
}

// ______________________TEST ITEMS ZONE_____________________

// testItemData is the real item data, data/items.json.
var testItemData = func() *gamedata.Items {
	b, err := os.ReadFile("../../data/items.json")
	if err != nil {
		panic(err)
	}
	it, err := gamedata.ParseItems(b)
	if err != nil {
		panic(err)
	}
	return it
}()

// gearInventory has the white Lữ Hành sword worn, a Vàng boots piece with
// lines and hidden lines in bag slot 0 and 12 stones in bag slot 3.
func gearInventory() *items.Inventory {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Worn[0] = &items.Item{Code: "eq10_weapon_viem", Count: 1}
	inv.Bag[0] = &items.Item{Code: "eq10_boots", Count: 1, Quality: 2, Element: "thuy",
		Lines:  []items.Line{{Stat: "hp", Value: 43}, {Stat: "phys_defense", Value: 3}, {Stat: "magic_attack", Value: 4}},
		Hidden: []items.Line{{Stat: "hp", Value: 86}, {Stat: "phys_attack", Value: 5}}}
	inv.Bag[3] = &items.Item{Code: "stone_enhance", Count: 12}
	return inv
}

// gearZone joins a level-10 Viêm with 60 STR and 30 VIT, 77 Xu and inv as
// player 1 into a zone with the real item data, and returns its
// EnterWorldResp.
func gearZone(t *testing.T, inv *items.Inventory) (*Zone, *player, *pb.EnterWorldResp, *fakeSaver) {
	t.Helper()
	return gearZoneAt(t, 10, inv)
}

// gearZoneAt is gearZone with a player of the given level.
func gearZoneAt(t *testing.T, level int, inv *items.Inventory) (*Zone, *player, *pb.EnterWorldResp, *fakeSaver) {
	t.Helper()
	sv := &fakeSaver{}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Items: testItemData}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.Points = pb.CharClass_CHAR_CLASS_VIEM, level, combat.Points{Str: 60, Vit: 30}
	in.Xu, in.Inventory = 77, inv
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	envs := out.take()
	if len(envs) == 0 || envs[0].GetEnterWorldResp() == nil {
		t.Fatalf("first message %v, want EnterWorldResp", envs)
	}
	return z, z.players[1], envs[0].GetEnterWorldResp(), sv
}

// The worn sword counts in the stats from the join on (spec 4.2 example:
// 185 + 25 = 210 physical attack, 1,640 HP), and EnterWorldResp carries the
// inventory and the Xu.
func TestJoinWithItems(t *testing.T) {
	inv := gearInventory()
	_, p, r, _ := gearZone(t, inv)
	if r.Stats.PhysAttack != 210 || r.Stats.MaxHp != 1640 || p.stats.PhysAttack != 210 || r.Stats.Xu != 77 {
		t.Fatalf("stats %v, want 210 physical attack, 1640 HP and 77 Xu", r.Stats)
	}
	got := r.Inventory
	want := &pb.Inventory{BagSize: 60,
		Bag: []*pb.ItemSlot{
			{Slot: 0, Item: &pb.ItemInfo{Code: "eq10_boots", Count: 1, Quality: 2, Element: pb.CharClass_CHAR_CLASS_THUY,
				Lines:  []*pb.ItemLine{{Stat: "hp", Value: 43}, {Stat: "phys_defense", Value: 3}, {Stat: "magic_attack", Value: 4}},
				Hidden: []*pb.ItemLine{{Stat: "hp", Value: 86}, {Stat: "phys_attack", Value: 5}}}},
			{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 12}},
		},
		Equipped: []*pb.ItemSlot{{Slot: 0, Item: &pb.ItemInfo{Code: "eq10_weapon_viem", Count: 1}}},
	}
	if !proto.Equal(got, want) {
		t.Fatalf("inventory\n%v\nwant\n%v", got, want)
	}
	// Join cloned the inventory: the caller's copy is not the player's.
	inv.Worn[0].Code, inv.Bag[0].Lines[0].Value = "eq10_weapon_thuy", 1
	if p.inv.Worn[0].Code != "eq10_weapon_viem" || p.inv.Bag[0].Lines[0].Value != 43 {
		t.Fatal("the player shares items with PlayerInfo.Inventory")
	}
}

// Without the sword the same character has 185 physical attack.
func TestJoinWithoutItems(t *testing.T) {
	_, p, r, _ := gearZone(t, nil)
	if r.Stats.PhysAttack != 185 || len(p.inv.Bag) != 60 || r.Inventory.BagSize != 60 ||
		len(r.Inventory.Bag) != 0 || len(r.Inventory.Equipped) != 0 {
		t.Fatalf("stats %v, inventory %v, want 185 and an empty 60-slot bag", r.Stats, r.Inventory)
	}
}

// state saves Xu and the items as rows that share nothing with the player;
// an all-zero pity and no holes save as none.
func TestStateSavesItemsAndXu(t *testing.T) {
	z, p, _, _ := gearZone(t, gearInventory())
	p.xu = 1234
	p.inv.Worn[0].Enhance, p.inv.Worn[0].Pity[7], p.inv.Worn[0].Pity[14] = 8, 4, 90
	p.inv.Bag[0].Gems = []string{"", "gem_hp_1"}
	st := z.state(p)
	want := []store.ItemRow{
		{Slot: 0, Code: "eq10_boots", Count: 1, Quality: 2, Element: "thuy",
			Lines:  []store.ItemLine{{Stat: "hp", Value: 43}, {Stat: "phys_defense", Value: 3}, {Stat: "magic_attack", Value: 4}},
			Hidden: []store.ItemLine{{Stat: "hp", Value: 86}, {Stat: "phys_attack", Value: 5}},
			Gems:   []string{"", "gem_hp_1"}},
		{Slot: 3, Code: "stone_enhance", Count: 12, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
		{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1, Enhance: 8, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{},
			Pity: []int{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 90}},
	}
	if st.Xu != 1234 || !reflect.DeepEqual(st.Items, want) {
		t.Fatalf("saved xu %d items %+v\nwant %+v", st.Xu, st.Items, want)
	}
	// Changing the saved copy leaves the player alone, and the other way round.
	st.Items[0].Lines[0].Value, st.Items[0].Hidden[1].Stat, st.Items[2].Pity[7], st.Items[0].Gems[0] = 1, "mp", 5, "gem_hp_1"
	if p.inv.Bag[0].Lines[0].Value != 43 || p.inv.Bag[0].Hidden[1].Stat != "phys_attack" || p.inv.Worn[0].Pity[7] != 4 ||
		p.inv.Bag[0].Gems[0] != "" {
		t.Fatal("the saved rows share lines with the player")
	}
	st = z.state(p)
	p.inv.Bag[0].Lines[1].Value, p.inv.Bag[3].Count, p.inv.Bag[0].Gems[1] = 9, 13, ""
	if st.Items[0].Lines[1].Value != 3 || st.Items[1].Count != 12 || st.Items[0].Gems[1] != "gem_hp_1" {
		t.Fatal("the player shares lines or items with the saved rows")
	}
}

// inventoryUpdate lists the asked slots as they are, holes and gems
// included; an empty one has no item.
func TestInventoryUpdate(t *testing.T) {
	inv := gearInventory()
	inv.Worn[0].Gems = []string{"gem_attack_1", ""}
	got := inventoryUpdate(inv, []int{3, 1}, []int{0, 9})
	want := &pb.InventoryUpdate{
		Bag: []*pb.ItemSlot{{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 12}}, {Slot: 1}},
		Equipped: []*pb.ItemSlot{{Slot: 0, Item: &pb.ItemInfo{Code: "eq10_weapon_viem", Count: 1,
			Holes: 2, Gems: []string{"gem_attack_1", ""}}}, {Slot: 9}},
	}
	if !proto.Equal(got, want) {
		t.Fatalf("update %v, want %v", got, want)
	}
}

// Gear stays in the stats when they are recomputed on a level-up.
func TestLevelUpKeepsGear(t *testing.T) {
	z, p, _, _ := gearZone(t, gearInventory())
	z.gainExp(p, z.area.Exp.ToNext(p.level))
	want := combat.Compute(false, 11, combat.Points{Str: 60, Vit: 30}, combat.Bonus{PhysAttack: 25}, combat.Passive{})
	if p.level != 11 || p.stats != want {
		t.Fatalf("level %d stats %+v, want %+v", p.level, p.stats, want)
	}
}

// ______________________TEST PICKUP_____________________

// putGround lays it on the ground at pos for character owner, as loot does,
// for 60 seconds.
func putGround(z *Zone, owner int64, pos Vec2, it items.Item) *groundItem {
	g := &groundItem{
		entity: entity{id: z.cfg.NewID(), kind: pb.EntityKind_ENTITY_KIND_ITEM, name: itemName(z.area.Items, it.Code), pos: pos},
		owner:  owner, item: it, expires: z.tickNo + z.ticks(60),
	}
	g.cell = z.cellOf(g.pos)
	z.ground[g.id] = g
	z.addToCell(&g.entity)
	return g
}

// fillBag puts a white Lữ Hành helmet in every empty bag slot of inv.
func fillBag(inv *items.Inventory) {
	for i, it := range inv.Bag {
		if it == nil {
			inv.Bag[i] = &items.Item{Code: "eq10_helmet", Count: 1}
		}
	}
}

// pickupNow queues picking up itemID by id with seq and runs one tick.
func pickupNow(t *testing.T, z *Zone, id uint64, seq uint32, itemID uint64) {
	t.Helper()
	if !z.Pickup(id, seq, itemID) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func pickupRespFor(envs []*pb.Envelope, seq uint32) *pb.PickupResp {
	for _, e := range envs {
		if r := e.GetPickupResp(); r != nil && e.Seq == seq {
			return r
		}
	}
	return nil
}

// onGround reports whether g is still in the zone's ground items and its
// AOI cell.
func onGround(z *Zone, g *groundItem) bool {
	return z.ground[g.id] == g && z.cells[g.cell][g.id] == &g.entity
}

// Each check of spec 4.6 in order. Every case also breaks the checks after
// its own, so only the first failing check can answer it.
func TestPickupErrors(t *testing.T) {
	sword := items.Item{Code: "eq10_weapon_viem", Count: 1}
	for _, c := range []struct {
		name  string
		dead  bool
		owner int64   // the item's owner; 100 is player 1
		dx    float32 // tiles east of the player
		gone  bool    // ask for an id that is not on the ground
		full  bool    // fill the bag first
		item  items.Item
		code  pb.ErrorCode
	}{
		{"dead next to their item", true, 100, 0.5, false, false, sword, pb.ErrorCode_ERROR_CODE_DEAD},
		{"dead, missing, far and full", true, 200, 5, true, true, sword, pb.ErrorCode_ERROR_CODE_DEAD},
		{"missing id", false, 100, 0.5, true, true, sword, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		{"another owner's item", false, 200, 5, false, true, sword, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		{"another owner's item in reach", false, 200, 0.5, false, false, sword, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		{"2.1 tiles away", false, 100, 2.1, false, true, sword, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"full bag", false, 100, 1.9, false, true, sword, pb.ErrorCode_ERROR_CODE_BAG_FULL},
		// 12 stones in slot 3 leave room for 987, not for the whole stack.
		{"no room for the whole stack", false, 100, 1, false, true, items.Item{Code: "stone_enhance", Count: 988},
			pb.ErrorCode_ERROR_CODE_BAG_FULL},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, _, sv := gearZone(t, gearInventory())
			out := p.out.(*fakeOut)
			if c.full {
				fillBag(p.inv)
			}
			g := putGround(z, c.owner, Vec2{p.pos.X + c.dx, p.pos.Y}, c.item)
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			out.take()
			id := g.id
			if c.gone {
				id = g.id + 1000
			}
			before, saves := p.inv.Clone(), len(sv.all())
			pickupNow(t, z, 1, 9, id)
			envs := out.take()
			if er := errorFor(envs, 9); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if pickupRespFor(envs, 9) != nil || !reflect.DeepEqual(p.inv, before) {
				t.Fatal("a rejected pickup changed the bag")
			}
			if !onGround(z, g) || slices.Contains(leaves(envs), g.id) {
				t.Fatal("a rejected pickup took the item off the ground")
			}
			if n := len(sv.all()); n != saves {
				t.Fatalf("%d saves, want %d", n, saves)
			}
		})
	}
}

// A piece 1.9 tiles away goes into the first empty slot; the item leaves the
// ground and the owner's view, the bag is saved at once and the reply lists
// the changed slot.
func TestPickup(t *testing.T) {
	z, p, _, sv := gearZone(t, gearInventory())
	out := p.out.(*fakeOut)
	it := items.Item{Code: "eq10_armor", Count: 1, Quality: 1, Lines: []items.Line{{Stat: "hp", Value: 40}}}
	g := putGround(z, 100, Vec2{p.pos.X, p.pos.Y + 1.9}, it)
	z.advance(time.Now())
	if en := itemEnters(out.take()); en[g.id] == nil {
		t.Fatal("setup: the owner does not see the item")
	}
	saves := len(sv.all())
	pickupNow(t, z, 1, 9, g.id)
	envs := out.take()
	r := pickupRespFor(envs, 9)
	want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 1, Item: itemInfo(&it)}}}
	if r == nil || !proto.Equal(r.Update, want) {
		t.Fatalf("PickupResp %v, want %v", r, want)
	}
	if !reflect.DeepEqual(*p.inv.Bag[1], it) {
		t.Fatalf("bag slot 1 %+v, want %+v", p.inv.Bag[1], it)
	}
	if z.ground[g.id] != nil || z.cells[g.cell][g.id] != nil || !slices.Contains(leaves(envs), g.id) {
		t.Fatal("the picked item is still on the ground or in the owner's view")
	}
	all := sv.all()
	if len(all) != saves+1 || !slices.ContainsFunc(all[len(all)-1].Items, func(r store.ItemRow) bool {
		return !r.Equipped && r.Slot == 1 && r.Code == "eq10_armor" && r.Quality == 1
	}) {
		t.Fatalf("saves %d, want one more holding the armor in slot 1", len(all))
	}
	// The bag shares nothing with what lay on the ground.
	g.item.Lines[0].Value = 1
	if p.inv.Bag[1].Lines[0].Value != 40 {
		t.Fatal("the bag shares lines with the ground item")
	}
	// The same id again is gone.
	pickupNow(t, z, 1, 10, g.id)
	if er := errorFor(out.take(), 10); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND {
		t.Fatalf("second pickup %v, want ITEM_NOT_FOUND", er)
	}
}

// A stone stack tops up the stack already in the bag; the reply lists only
// that slot.
func TestPickupMergesStack(t *testing.T) {
	z, p, _, _ := gearZone(t, gearInventory())
	out := p.out.(*fakeOut)
	g := putGround(z, 100, p.pos, items.Item{Code: "stone_enhance", Count: 5})
	pickupNow(t, z, 1, 9, g.id)
	r := pickupRespFor(out.take(), 9)
	want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 17}}}}
	if r == nil || !proto.Equal(r.Update, want) || p.inv.Bag[3].Count != 17 || p.inv.Bag[1] != nil {
		t.Fatalf("PickupResp %v, want %v", r, want)
	}
}

// ______________________TEST EQUIP_____________________

// itemNow runs one tick after an item command was queued.
func itemNow(t *testing.T, z *Zone, queued bool) {
	t.Helper()
	if !queued {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

// respFor is the reply to seq that get finds in envs, or nil.
func respFor[T any](envs []*pb.Envelope, seq uint32, get func(*pb.Envelope) *T) *T {
	for _, e := range envs {
		if r := get(e); r != nil && e.Seq == seq {
			return r
		}
	}
	return nil
}

// errorInventory is gearInventory with a level-20 Thủy weapon in bag slot 5
// and a level-10 one in slot 6.
func errorInventory() *items.Inventory {
	inv := gearInventory()
	inv.Bag[5] = &items.Item{Code: "eq20_weapon_thuy", Count: 1}
	inv.Bag[6] = &items.Item{Code: "eq10_weapon_thuy", Count: 1}
	return inv
}

// Each check of spec 4.7 in order, for every item command, on a level-10
// Viêm wearing the Lữ Hành sword (errorInventory: level-11 boots in slot 0,
// 12 stones in slot 3, Thủy weapons in 5 and 6). Every case also breaks
// the checks after its own where it can.
func TestItemCommandErrors(t *testing.T) {
	equip := func(slot int) func(*Zone) bool { return func(z *Zone) bool { return z.Equip(1, 9, slot) } }
	unequip := func(slot int) func(*Zone) bool { return func(z *Zone) bool { return z.Unequip(1, 9, slot) } }
	discard := func(slot, n int) func(*Zone) bool { return func(z *Zone) bool { return z.Discard(1, 9, slot, n) } }
	sortBag := func(z *Zone) bool { return z.SortBag(1, 9) }
	const (
		dead       = pb.ErrorCode_ERROR_CODE_DEAD
		bad        = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		notFound   = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
		wrongClass = pb.ErrorCode_ERROR_CODE_WRONG_CLASS
		tooLow     = pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW
		bagFull    = pb.ErrorCode_ERROR_CODE_BAG_FULL
	)
	for _, c := range []struct {
		name string
		dead bool
		full bool
		cmd  func(*Zone) bool
		code pb.ErrorCode
	}{
		{"equip dead, bad slot", true, false, equip(60), dead},
		{"equip dead, other class", true, false, equip(6), dead},
		{"equip slot -1", false, false, equip(-1), bad},
		{"equip slot 60", false, false, equip(60), bad},
		{"equip empty slot", false, false, equip(7), notFound},
		{"equip stones", false, false, equip(3), bad},
		{"equip other class and level", false, false, equip(5), wrongClass},
		{"equip other class", false, false, equip(6), wrongClass},
		{"equip level too low", false, false, equip(0), tooLow},
		{"unequip dead, bad slot", true, true, unequip(10), dead},
		{"unequip slot -1", false, true, unequip(-1), bad},
		{"unequip slot 10", false, true, unequip(10), bad},
		{"unequip empty slot", false, true, unequip(9), notFound},
		{"unequip into a full bag", false, true, unequip(0), bagFull},
		{"discard dead, bad slot", true, false, discard(60, -1), dead},
		{"discard slot -1", false, false, discard(-1, -1), bad},
		{"discard slot 60", false, false, discard(60, 0), bad},
		{"discard empty slot", false, false, discard(7, -1), notFound},
		{"discard count -1", false, false, discard(3, -1), bad},
		{"discard more than there is", false, false, discard(3, 13), bad},
		{"sort dead", true, false, sortBag, dead},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, _, sv := gearZone(t, errorInventory())
			out := p.out.(*fakeOut)
			if c.full {
				fillBag(p.inv)
			}
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			out.take()
			before, stats, hp, saves := p.inv.Clone(), p.stats, p.hp, len(sv.all())
			itemNow(t, z, c.cmd(z))
			envs := out.take()
			if er := errorFor(envs, 9); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if n := len(slices.DeleteFunc(slices.Clone(envs), func(e *pb.Envelope) bool { return e.Seq != 9 })); n != 1 {
				t.Fatalf("%d replies to seq 9, want only the error", n)
			}
			if !reflect.DeepEqual(p.inv, before) || p.stats != stats || p.hp != hp {
				t.Fatal("a rejected command changed the inventory or stats")
			}
			if n := len(sv.all()); n != saves {
				t.Fatalf("%d saves, want %d", n, saves)
			}
		})
	}
}

// swapInventory has the white Lữ Hành sword worn, the Vàng boots of
// gearInventory in bag slot 0 and a Xanh sword with +5 physical attack in
// bag slot 1.
func swapInventory() *items.Inventory {
	inv := gearInventory()
	inv.Bag[1] = &items.Item{Code: "eq10_weapon_viem", Count: 1, Quality: 1, Lines: []items.Line{{Stat: "phys_attack", Value: 5}}}
	return inv
}

// Equipping into an occupied slot swaps the two pieces, the stats include
// the new piece, the reply lists both slots with the new stats, and the
// inventory is saved at once.
func TestEquipSwaps(t *testing.T) {
	z, p, r, sv := gearZoneAt(t, 19, swapInventory())
	out := p.out.(*fakeOut)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	white, blue := *p.inv.Worn[0], *p.inv.Bag[1]
	itemNow(t, z, z.Equip(1, 9, 1))
	envs := out.take()
	resp := respFor(envs, 9, (*pb.Envelope).GetEquipResp)
	want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{itemSlot(1, &white)}, Equipped: []*pb.ItemSlot{itemSlot(0, &blue)}}
	if resp == nil || !proto.Equal(resp.Update, want) {
		t.Fatalf("EquipResp %v, want update %v", resp, want)
	}
	if !reflect.DeepEqual(*p.inv.Worn[0], blue) || !reflect.DeepEqual(*p.inv.Bag[1], white) {
		t.Fatalf("worn %+v bag %+v, want the pieces swapped", p.inv.Worn[0], p.inv.Bag[1])
	}
	if p.stats.PhysAttack != int(r.Stats.PhysAttack)+5 || !proto.Equal(resp.Stats, z.selfStats(p)) {
		t.Fatalf("stats %v, want 5 more physical attack than %d", resp.Stats, r.Stats.PhysAttack)
	}
	all := sv.all()
	if len(all) != 1 || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
		t.Fatalf("saves %+v, want one with the swapped pieces", all)
	}
	// No max HP change, so no HpChange.
	if cs := hpChangesIn(append(envs, v.take()...)); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none", cs)
	}
}

// Equipping into an empty slot empties the bag slot; full HP stays full at
// the new max, MP is unchanged, and viewers see the new bar.
func TestEquipRaisesMaxHP(t *testing.T) {
	z, p, r, _ := gearZoneAt(t, 19, swapInventory())
	out := p.out.(*fakeOut)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	boots := testItemData.Equipment["eq10_boots"]
	mp := p.mp
	itemNow(t, z, z.Equip(1, 9, 0))
	envs := out.take()
	resp := respFor(envs, 9, (*pb.Envelope).GetEquipResp)
	want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 0}}, Equipped: []*pb.ItemSlot{itemSlot(boots.SlotIndex, p.inv.Worn[boots.SlotIndex])}}
	if resp == nil || !proto.Equal(resp.Update, want) || p.inv.Bag[0] != nil {
		t.Fatalf("EquipResp %v, want update %v", resp, want)
	}
	wantMax := int(r.Stats.MaxHp) + boots.HP[0] + 43
	if p.maxHP != wantMax || p.hp != wantMax || p.mp != mp || resp.Stats.Hp != int32(wantMax) || resp.Stats.MaxHp != int32(wantMax) {
		t.Fatalf("hp %d/%d mp %d, want full at %d and mp %d", p.hp, p.maxHP, p.mp, wantMax, mp)
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(wantMax) || cs[0].MaxHp != int32(wantMax) {
			t.Fatalf("%s HpChanges %v, want %d/%d for 1", name, cs, wantMax, wantMax)
		}
	}
}

// The HP rule of spec 4.7: full stays full, otherwise HP keeps its share of
// max HP, rounded down, and never drops below 1.
func TestKeepHPRatio(t *testing.T) {
	for _, c := range []struct{ hp, oldMax, newMax, want int }{
		{1000, 1000, 1500, 1500},
		{1500, 1500, 1000, 1000},
		{500, 1000, 1500, 750},
		{999, 1000, 1500, 1498},
		{750, 1500, 1000, 500},
		{1, 1000, 500, 1},
		{2, 1000, 500, 1},
	} {
		if got := keepHPRatio(c.hp, c.oldMax, c.newMax); got != c.want {
			t.Fatalf("keepHPRatio(%d, %d, %d) = %d, want %d", c.hp, c.oldMax, c.newMax, got, c.want)
		}
	}
}

// Taking off and wearing again the boots, at any HP, keeps the HP share and
// never ends above the HP it started at; at full HP it stays full.
func TestGearSwapNeverHeals(t *testing.T) {
	z, p, _, _ := gearZoneAt(t, 19, swapInventory())
	out := p.out.(*fakeOut)
	slot := testItemData.Equipment["eq10_boots"].SlotIndex
	itemNow(t, z, z.Equip(1, 1, 0))
	high := p.maxHP
	itemNow(t, z, z.Unequip(1, 2, slot))
	low := p.maxHP
	if p.hp != low || high <= low {
		t.Fatalf("hp %d/%d after taking off at full, want full below %d", p.hp, low, high)
	}
	itemNow(t, z, z.Equip(1, 3, 0))
	if p.hp != high {
		t.Fatalf("hp %d/%d after wearing again at full, want full", p.hp, p.maxHP)
	}
	for _, start := range []int{1, 2, high / 3, high / 2, high - 1} {
		p.hp = start
		itemNow(t, z, z.Unequip(1, 4, slot))
		if want := max(1, start*low/high); p.hp != want {
			t.Fatalf("from %d/%d: hp %d after taking off, want %d", start, high, p.hp, want)
		}
		mid := p.hp
		itemNow(t, z, z.Equip(1, 5, 0))
		if want := max(1, mid*high/low); p.hp != want || p.hp > start {
			t.Fatalf("from %d/%d: hp %d after wearing again, want %d and at most %d", start, high, p.hp, want, start)
		}
		if r := respFor(out.take(), 5, (*pb.Envelope).GetEquipResp); r == nil || r.Stats.Hp != int32(p.hp) {
			t.Fatalf("EquipResp %v, want hp %d", r, p.hp)
		}
	}
}

// Taking off moves the piece to the first empty bag slot, drops its stats,
// replies with both slots and the stats, and saves at once.
func TestUnequip(t *testing.T) {
	z, p, r, sv := gearZoneAt(t, 19, swapInventory())
	out := p.out.(*fakeOut)
	sword := *p.inv.Worn[0]
	itemNow(t, z, z.Unequip(1, 9, 0))
	resp := respFor(out.take(), 9, (*pb.Envelope).GetUnequipResp)
	want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{itemSlot(2, &sword)}, Equipped: []*pb.ItemSlot{{Slot: 0}}}
	if resp == nil || !proto.Equal(resp.Update, want) || p.inv.Worn[0] != nil {
		t.Fatalf("UnequipResp %v, want update %v", resp, want)
	}
	if p.stats.PhysAttack != int(r.Stats.PhysAttack)-25 || !proto.Equal(resp.Stats, z.selfStats(p)) {
		t.Fatalf("stats %v, want 25 less physical attack than %d", resp.Stats, r.Stats.PhysAttack)
	}
	if all := sv.all(); len(all) != 1 || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
		t.Fatalf("saves %+v, want one with the sword in the bag", all)
	}
}

// Discarding part of a stack leaves the rest; count 0, or the whole count,
// empties the slot. Each discard replies with the slot and saves at once.
func TestDiscard(t *testing.T) {
	z, p, _, sv := gearZone(t, gearInventory())
	out := p.out.(*fakeOut)
	for i, c := range []struct {
		slot, count int
		want        *pb.ItemSlot
	}{
		{3, 5, &pb.ItemSlot{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 7}}},
		{0, 0, &pb.ItemSlot{Slot: 0}},
		{3, 7, &pb.ItemSlot{Slot: 3}},
	} {
		seq := uint32(10 + i)
		itemNow(t, z, z.Discard(1, seq, c.slot, c.count))
		resp := respFor(out.take(), seq, (*pb.Envelope).GetDiscardResp)
		if resp == nil || !proto.Equal(resp.Update, &pb.InventoryUpdate{Bag: []*pb.ItemSlot{c.want}}) {
			t.Fatalf("discard %d of slot %d: %v, want %v", c.count, c.slot, resp, c.want)
		}
		if all := sv.all(); len(all) != i+1 || !reflect.DeepEqual(all[i].Items, itemRows(p.inv)) {
			t.Fatalf("discard %d: %d saves, want one more with the new bag", i, len(all))
		}
	}
	if p.inv.Bag[0] != nil || p.inv.Bag[3] != nil || p.inv.Worn[0] == nil {
		t.Fatalf("bag %v, want slots 0 and 3 empty and the sword still worn", p.inv.Bag[:4])
	}
}

// Sorting merges the stacks and lays the bag out as items.Sort does; the
// reply lists every bag slot and the bag is saved at once.
func TestSortBag(t *testing.T) {
	inv := gearInventory()
	inv.Bag[0], inv.Bag[3] = nil, nil
	inv.Bag[10] = &items.Item{Code: "stone_enhance", Count: 500}
	inv.Bag[20] = &items.Item{Code: "stone_enhance", Count: 600}
	inv.Bag[5] = &items.Item{Code: "eq10_helmet", Count: 1}
	inv.Bag[30] = &items.Item{Code: "eq20_boots", Count: 1, Quality: 3}
	z, p, _, sv := gearZone(t, inv)
	out := p.out.(*fakeOut)
	sorted := inv.Clone()
	sorted.Sort(testItemData)
	itemNow(t, z, z.SortBag(1, 9))
	resp := respFor(out.take(), 9, (*pb.Envelope).GetSortBagResp)
	if resp == nil || len(resp.Update.Bag) != 60 || len(resp.Update.Equipped) != 0 {
		t.Fatalf("SortBagResp %v, want all 60 bag slots", resp)
	}
	for i, s := range resp.Update.Bag {
		if !proto.Equal(s, itemSlot(i, sorted.Bag[i])) {
			t.Fatalf("slot %d: %v, want %v", i, s, itemSlot(i, sorted.Bag[i]))
		}
	}
	if !reflect.DeepEqual(p.inv, sorted) || p.inv.Bag[0].Code != "eq20_boots" || p.inv.Bag[2].Count != 999 || p.inv.Bag[3].Count != 101 {
		t.Fatalf("bag %v, want boots first, then 999 and 101 stones after the helmet", p.inv.Bag[:5])
	}
	if all := sv.all(); len(all) != 1 || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
		t.Fatalf("saves %+v, want one with the sorted bag", all)
	}
}

// ______________________TEST ENHANCE ZONE_____________________

// forgeInventory is gearInventory (the white sword worn, the Vàng boots in
// bag slot 0, 12 stones in slot 3) with the white Lữ Hành armour worn, a +15
// helmet in bag slot 8 and a +7 one in slot 9.
func forgeInventory() *items.Inventory {
	inv := gearInventory()
	inv.Worn[testItemData.Equipment["eq10_armor"].SlotIndex] = &items.Item{Code: "eq10_armor", Count: 1}
	inv.Bag[8] = &items.Item{Code: "eq10_helmet", Count: 1, Enhance: 15}
	inv.Bag[9] = &items.Item{Code: "eq10_helmet", Count: 1, Enhance: 7}
	return inv
}

// forgeZone is gearZoneAt for a level-19 Viêm with inv and 100,000 Xu,
// standing 3 tiles from a blacksmith NPC unless far is set (then 15 tiles).
func forgeZone(t *testing.T, inv *items.Inventory, far bool) (*Zone, *player, *fakeSaver) {
	t.Helper()
	z, p, _, sv := gearZoneAt(t, 19, inv)
	x := float32(8.5)
	if far {
		x = 20.5
	}
	z.area.NPCs = map[string]mapdata.NPC{
		"smith": {ID: "smith", Role: mapdata.RoleBlacksmith, Pos: mapdata.Point{X: x, Y: 5.5}},
	}
	p.xu = 100000
	return z, p, sv
}

// failSource makes every rng.IntN(100) roll 99, so every attempt below 100%
// fails.
type failSource struct{}

func (failSource) Uint64() uint64 { return math.MaxUint64 }

// Each check of spec 4.1 in order, on forgeInventory. Every case also breaks
// the checks after its own where it can, and none costs anything.
func TestEnhanceErrors(t *testing.T) {
	armour := testItemData.Equipment["eq10_armor"].SlotIndex
	bag := func(slot int) items.Ref { return items.Ref{Slot: slot} }
	worn := func(slot int) items.Ref { return items.Ref{Equipped: true, Slot: slot} }
	broke := func(p *player) { p.xu = 0 }
	const (
		dead      = pb.ErrorCode_ERROR_CODE_DEAD
		tooFar    = pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
		bad       = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		notFound  = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
		maxed     = pb.ErrorCode_ERROR_CODE_ENHANCE_MAXED
		materials = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_MATERIALS
		noXu      = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU
	)
	for _, c := range []struct {
		name      string
		dead, far bool
		setup     func(*player)
		ref       items.Ref
		protect   bool
		times     int
		code      pb.ErrorCode
	}{
		{"dead far from the blacksmith, bad slot", true, true, broke, bag(60), false, 101, dead},
		{"far from the blacksmith, bad slot", false, true, broke, bag(60), false, 101, tooFar},
		{"bag slot 60", false, false, broke, bag(60), false, 101, bad},
		{"bag slot -1", false, false, broke, bag(-1), false, 1, bad},
		{"worn slot 10", false, false, broke, worn(10), false, 1, bad},
		{"worn slot -1", false, false, broke, worn(-1), false, 1, bad},
		{"times -1", false, false, broke, bag(7), false, -1, bad},
		{"times 101", false, false, broke, bag(7), false, 101, bad},
		{"empty bag slot", false, false, broke, bag(7), false, 1, notFound},
		{"empty worn slot", false, false, broke, worn(9), false, 1, notFound},
		{"stones", false, false, broke, bag(3), false, 1, bad},
		{"already +15", false, false, broke, bag(8), true, 1, maxed},
		{"no stones", false, false, func(p *player) { p.xu, p.inv.Bag[3] = 0, nil }, bag(0), false, 1, materials},
		{"2 of 3 stones for +8", false, false, func(p *player) { p.xu, p.inv.Bag[3].Count = 0, 2 }, bag(9), false, 1, materials},
		{"protect at +8 without a charm", false, false, broke, bag(9), true, 1, materials},
		{"1 Xu short", false, false, func(p *player) { p.xu = 99 }, bag(0), false, 1, noXu},
		{"1 Xu short for the worn armour", false, false, func(p *player) { p.xu = 99 }, worn(armour), false, 100, noXu},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := forgeZone(t, forgeInventory(), c.far)
			out := p.out.(*fakeOut)
			c.setup(p)
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			out.take()
			before, stats, hp, xu, saves := p.inv.Clone(), p.stats, p.hp, p.xu, len(sv.all())
			itemNow(t, z, z.Enhance(1, 9, c.ref, c.protect, c.times))
			envs := out.take()
			if er := errorFor(envs, 9); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if n := len(slices.DeleteFunc(slices.Clone(envs), func(e *pb.Envelope) bool { return e.Seq != 9 })); n != 1 {
				t.Fatalf("%d replies to seq 9, want only the error", n)
			}
			if !reflect.DeepEqual(p.inv, before) || p.stats != stats || p.hp != hp || p.xu != xu {
				t.Fatal("a rejected enhancement changed the inventory, stats or Xu")
			}
			if n := len(sv.all()); n != saves {
				t.Fatalf("%d saves, want %d", n, saves)
			}
		})
	}
}

// Enhancing the worn armour +0 -> +1 raises its stats per the data, keeps
// the HP share (phase 3a spec 4.7), shows the new bar to viewers, replies
// with the armour and the stones taken, and saves at once.
func TestEnhanceWornArmour(t *testing.T) {
	z, p, sv := forgeZone(t, forgeInventory(), false)
	out := p.out.(*fakeOut)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	armour := testItemData.Equipment["eq10_armor"]
	slot := armour.SlotIndex
	p.hp = p.maxHP / 3
	hp, oldMax, attack := p.hp, p.maxHP, p.stats.PhysAttack
	itemNow(t, z, z.Enhance(1, 9, items.Ref{Equipped: true, Slot: slot}, false, 0))
	envs := out.take()
	resp := respFor(envs, 9, (*pb.Envelope).GetEnhanceResp)
	if resp == nil || resp.Attempts != 1 || !resp.Success || resp.Level != 1 || p.inv.Worn[slot].Enhance != 1 {
		t.Fatalf("EnhanceResp %v, want one successful attempt to +1", resp)
	}
	wantMax := oldMax + armour.HP[1] - armour.HP[0]
	if p.maxHP != wantMax || p.hp != keepHPRatio(hp, oldMax, wantMax) || p.hp == hp ||
		p.stats.PhysAttack != attack+armour.PhysAttack[1]-armour.PhysAttack[0] {
		t.Fatalf("hp %d/%d attack %d, want %d/%d and %d", p.hp, p.maxHP, p.stats.PhysAttack,
			keepHPRatio(hp, oldMax, wantMax), wantMax, attack+armour.PhysAttack[1]-armour.PhysAttack[0])
	}
	want := &pb.InventoryUpdate{
		Bag:      []*pb.ItemSlot{{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 11}}},
		Equipped: []*pb.ItemSlot{itemSlot(slot, p.inv.Worn[slot])},
	}
	if !proto.Equal(resp.Update, want) || resp.Update.Equipped[0].Item.Enhance != 1 {
		t.Fatalf("update %v, want %v", resp.Update, want)
	}
	if cost := int64(testItemData.Enhance.Levels[0].XuPerTier * armour.Tier); p.xu != 100000-cost ||
		resp.Stats.Xu != p.xu || !proto.Equal(resp.Stats, z.selfStats(p)) {
		t.Fatalf("stats %v xu %d, want the new stats and %d Xu", resp.Stats, p.xu, 100000-cost)
	}
	if all := sv.all(); len(all) != 1 || all[0].Xu != p.xu || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
		t.Fatalf("saves %+v, want one with the +1 armour and the Xu paid", all)
	}
	z.advance(time.Now())
	for name, es := range map[string][]*pb.Envelope{"player": append(envs, out.take()...), "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(p.hp) || cs[0].MaxHp != int32(wantMax) {
			t.Fatalf("%s HpChanges %v, want %d/%d for 1", name, cs, p.hp, wantMax)
		}
	}
}

// Enhancing a bag item changes no stats; protect below +8 takes no charm.
func TestEnhanceBagItem(t *testing.T) {
	z, p, sv := forgeZone(t, forgeInventory(), false)
	out := p.out.(*fakeOut)
	z.advance(time.Now())
	out.take()
	stats, hp, maxHP := p.stats, p.hp, p.maxHP
	itemNow(t, z, z.Enhance(1, 9, items.Ref{Slot: 0}, true, 5))
	envs := out.take()
	resp := respFor(envs, 9, (*pb.Envelope).GetEnhanceResp)
	if resp == nil || resp.Attempts != 1 || !resp.Success || resp.Level != 1 || p.inv.Bag[0].Enhance != 1 {
		t.Fatalf("EnhanceResp %v, want one successful attempt to +1", resp)
	}
	want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{itemSlot(0, p.inv.Bag[0]),
		{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 11}}}}
	if !proto.Equal(resp.Update, want) {
		t.Fatalf("update %v, want %v", resp.Update, want)
	}
	if p.stats != stats || p.hp != hp || p.maxHP != maxHP || p.xu != 100000-100 || !proto.Equal(resp.Stats, z.selfStats(p)) {
		t.Fatalf("stats %+v hp %d/%d xu %d, want them unchanged but 100 Xu less", p.stats, p.hp, p.maxHP, p.xu)
	}
	if all := sv.all(); len(all) != 1 || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
		t.Fatalf("saves %+v, want one with the +1 boots", all)
	}
	if cs := hpChangesIn(envs); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none", cs)
	}
}

// Failures go through the item package: without a charm a failed +8 drops to
// +6 after one attempt; with charms the attempts go on until the charms run
// out. The pity reaches the save.
func TestEnhanceFailures(t *testing.T) {
	lvl := testItemData.Enhance.Levels[7]
	cost := int64(lvl.XuPerTier * testItemData.Equipment["eq10_helmet"].Tier)
	for _, c := range []struct {
		name            string
		protect         bool
		charms          int
		attempts, level int
		bag             []*pb.ItemSlot
	}{
		{"no charm", false, 0, 1, 6, []*pb.ItemSlot{{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: int32(12 - lvl.Stones)}}}},
		{"two charms", true, 2, 2, 7, []*pb.ItemSlot{{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: int32(12 - 2*lvl.Stones)}}, {Slot: 10}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			inv := forgeInventory()
			if c.charms > 0 {
				inv.Bag[10] = &items.Item{Code: gamedata.CharmProtect, Count: c.charms}
			}
			z, p, sv := forgeZone(t, inv, false)
			z.rng = rand.New(failSource{})
			out := p.out.(*fakeOut)
			itemNow(t, z, z.Enhance(1, 9, items.Ref{Slot: 9}, c.protect, 100))
			resp := respFor(out.take(), 9, (*pb.Envelope).GetEnhanceResp)
			if resp == nil || int(resp.Attempts) != c.attempts || resp.Success || int(resp.Level) != c.level {
				t.Fatalf("EnhanceResp %v, want %d failed attempts ending at +%d", resp, c.attempts, c.level)
			}
			want := &pb.InventoryUpdate{Bag: append([]*pb.ItemSlot{}, c.bag[0], itemSlot(9, p.inv.Bag[9]))}
			want.Bag = append(want.Bag, c.bag[1:]...)
			if !proto.Equal(resp.Update, want) || p.xu != 100000-int64(c.attempts)*cost {
				t.Fatalf("update %v xu %d, want %v and %d Xu", resp.Update, p.xu, want, 100000-int64(c.attempts)*cost)
			}
			all := sv.all()
			i := slices.IndexFunc(all[0].Items, func(r store.ItemRow) bool { return !r.Equipped && r.Slot == 9 })
			if len(all) != 1 || i < 0 || all[0].Items[i].Enhance != c.level || len(all[0].Items[i].Pity) != items.MaxEnhance ||
				all[0].Items[i].Pity[7] != c.attempts*testItemData.Enhance.PityPct {
				t.Fatalf("saves %+v, want one with the helmet at +%d and its pity", all, c.level)
			}
		})
	}
}

// ______________________TEST REGEN ZONE_____________________

// regenZone joins a player of class and level with VIT vit, wearing worn
// (codes at the given enhance levels, in their own slots), as player 1 into
// a zone with the real item data and no rest point.
func regenZone(t *testing.T, class pb.CharClass, level, vit int, worn map[string]int) (*Zone, *player, *fakeOut, *pb.EnterWorldResp) {
	t.Helper()
	inv := items.NewInventory(testItemData.BagSize)
	for code, n := range worn {
		inv.Worn[testItemData.Equipment[code].SlotIndex] = &items.Item{Code: code, Count: 1, Enhance: n}
	}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Items: testItemData}, &fakeSaver{}, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.Points, in.Inventory = class, level, combat.Points{Vit: vit}, inv
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	r := out.take()[0].GetEnterWorldResp()
	return z, z.players[1], out, r
}

// fullForge is one tier-10 piece in every slot at +15, with the Thủy weapon.
func fullForge() map[string]int {
	worn := map[string]int{"eq10_weapon_thuy": 15}
	for _, s := range []string{"boots", "bracers", "pants", "helmet", "armor", "necklace", "pendant", "ring_left", "ring_right"} {
		worn["eq10_"+s] = 15
	}
	return worn
}

// advanceSec runs n seconds of ticks.
func advanceSec(z *Zone, n int) {
	for range n * int(z.ticks(1)) {
		z.advance(time.Now())
	}
}

// The spec 8 example: 1,640 max HP with one worn +7 boots (5 bp) regains 0 HP
// in the first second, 1 in the second and 8 after 10 seconds, the fraction
// carried over. Only a second that changes HP tells the player and viewers.
func TestRegenCarriesFractions(t *testing.T) {
	// A level-11 Thủy with 32 VIT: 500 + 50*10 + 16*32 + 128 (the boots) = 1,640.
	z, p, out, r := regenZone(t, pb.CharClass_CHAR_CLASS_THUY, 11, 32, map[string]int{"eq10_boots": 7})
	if p.maxHP != 1640 || r.Stats.HpRegenBp != 5 || r.Stats.MpRegen != 0 {
		t.Fatalf("max HP %d, regen %d bp %d MP, want 1640, 5 and 0", p.maxHP, r.Stats.HpRegenBp, r.Stats.MpRegen)
	}
	v := joinNow(t, z, 2, 7.5, 5.5)
	p.hp = 1000
	z.advance(time.Now())
	out.take()
	v.take()
	for range z.ticks(1) - 1 {
		z.advance(time.Now()) // to the end of the first second
	}
	if envs := out.take(); p.hp != 1000 || len(hpChangesIn(envs)) != 0 || len(selfStatsIn(envs)) != 0 {
		t.Fatalf("after 1 s HP %d, envs %v; want 1000 and nothing sent", p.hp, envs)
	}
	advanceSec(z, 1)
	envs := out.take()
	if p.hp != 1001 || len(selfStatsIn(envs)) != 1 || selfStatsIn(envs)[0].Hp != 1001 {
		t.Fatalf("after 2 s HP %d, SelfStats %v; want 1001", p.hp, selfStatsIn(envs))
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != 1001 {
			t.Fatalf("%s HpChanges %v, want one with 1001", name, cs)
		}
	}
	advanceSec(z, 8)
	if p.hp != 1008 {
		t.Fatalf("after 10 s HP %d, want 1008", p.hp)
	}
}

// Ten worn +15 pieces give 150 bp of max HP and 25 MP a second, shown in
// SelfStats; neither goes above its max, and full HP drops the carry.
func TestRegenFullForge(t *testing.T) {
	z, p, out, r := regenZone(t, pb.CharClass_CHAR_CLASS_THUY, 19, 0, fullForge())
	if p.regenHPBP != 150 || p.regenMP != 25 || r.Stats.HpRegenBp != 150 || r.Stats.MpRegen != 25 {
		t.Fatalf("regen %d bp %d MP, SelfStats %v; want 150 and 25", p.regenHPBP, p.regenMP, r.Stats)
	}
	p.hp, p.mp = 100, 0
	advanceSec(z, 1)
	gain := p.maxHP * 150 / 10000
	if p.hp != 100+gain || p.mp != 25 || p.hpCarry != p.maxHP*150%10000 {
		t.Fatalf("after 1 s HP %d MP %d carry %d, want %d, 25 and %d", p.hp, p.mp, p.hpCarry, 100+gain, p.maxHP*150%10000)
	}
	ss := selfStatsIn(out.take())
	if len(ss) != 1 || ss[0].Hp != int32(p.hp) || ss[0].Mp != 25 || ss[0].HpRegenBp != 150 || ss[0].MpRegen != 25 {
		t.Fatalf("SelfStats %v, want the new HP and MP and the regen sums", ss)
	}
	p.hp, p.mp = p.maxHP-1, p.stats.MaxMP-3
	advanceSec(z, 1)
	if p.hp != p.maxHP || p.mp != p.stats.MaxMP || p.hpCarry != 0 {
		t.Fatalf("HP %d/%d MP %d/%d carry %d, want both full and no carry", p.hp, p.maxHP, p.mp, p.stats.MaxMP, p.hpCarry)
	}
	out.take()
	advanceSec(z, 1)
	if envs := out.take(); len(hpChangesIn(envs)) != 0 || len(selfStatsIn(envs)) != 0 {
		t.Fatalf("at full HP and MP envs %v, want nothing sent", envs)
	}
}

// The dead regenerate nothing; the living keep regenerating while hit.
func TestRegenDeadAndInCombat(t *testing.T) {
	z, p, _, _ := regenZone(t, pb.CharClass_CHAR_CLASS_THUY, 19, 0, fullForge())
	p.mp = 0
	z.damagePlayer(p, killer{}, 1<<30, false)
	advanceSec(z, 3)
	if !p.dead || p.hp != 0 || p.mp != 0 {
		t.Fatalf("dead player at HP %d MP %d, want 0 and 0", p.hp, p.mp)
	}

	z, p, _, _ = regenZone(t, pb.CharClass_CHAR_CLASS_THUY, 19, 0, fullForge())
	p.hp, p.mp = 1000, 0
	n := 2 * int(z.ticks(1))
	for range n {
		z.damagePlayer(p, killer{}, 1, false)
		z.advance(time.Now())
	}
	gain := 2 * p.maxHP * 150 / 10000
	if p.hp != 1000-n+gain || p.mp != 50 {
		t.Fatalf("HP %d MP %d after 2 s of hits, want %d and 50", p.hp, p.mp, 1000-n+gain)
	}
}

// ______________________TEST GLOW_____________________

// looksIn collects the LookChanges of the WorldTicks in envs.
func looksIn(envs []*pb.Envelope) []*pb.LookChange {
	var ls []*pb.LookChange
	for _, wt := range ticksIn(envs) {
		ls = append(ls, wt.Looks...)
	}
	return ls
}

// enterOf is the EntityEnter for id in envs, or nil.
func enterOf(envs []*pb.Envelope, id uint64) *pb.EntityEnter {
	for _, e := range envs {
		if en := e.GetEntityEnter(); en != nil && en.Id == id {
			return en
		}
	}
	return nil
}

// winSource makes every rng.IntN(100) roll 0, so every attempt succeeds. A
// source of 0 would spin in the unbiasing loop of IntN.
type winSource struct{}

func (winSource) Uint64() uint64 { return 1 << 56 }

// A viewer coming into view of a player with a worn +12 weapon gets
// weapon_enhance 12; a weapon of another class shows nothing. Joining sends
// no LookChange.
func TestGlowOnEnter(t *testing.T) {
	for _, c := range []struct {
		name  string
		class pb.CharClass
		want  int32
	}{
		{"own class", pb.CharClass_CHAR_CLASS_THUY, 12},
		{"another class", pb.CharClass_CHAR_CLASS_VIEM, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _, out, _ := regenZone(t, c.class, 19, 0, map[string]int{"eq10_weapon_thuy": 12})
			v := joinNow(t, z, 2, 7.5, 5.5)
			z.advance(time.Now())
			venvs, envs := v.take(), out.take()
			if en := enterOf(venvs, 1); en == nil || en.WeaponEnhance != c.want {
				t.Fatalf("viewer's EntityEnter %v, want weapon_enhance %d", en, c.want)
			}
			if en := enterOf(envs, 2); en == nil || en.WeaponEnhance != 0 {
				t.Fatalf("player's EntityEnter for the viewer %v, want weapon_enhance 0", en)
			}
			if ls := append(looksIn(envs), looksIn(venvs)...); len(ls) != 0 {
				t.Fatalf("LookChanges %v on joining, want none", ls)
			}
		})
	}
}

// Equipping a +10 weapon over the +0 one sends one LookChange to the player
// and a viewer in view, none to one out of view; taking it off sends 0.
func TestGlowOnEquip(t *testing.T) {
	inv := gearInventory()
	inv.Bag[5] = &items.Item{Code: "eq10_weapon_viem", Count: 1, Enhance: 10}
	z, _, _, _ := gearZone(t, inv)
	out := z.players[1].out.(*fakeOut)
	near := joinNow(t, z, 2, 7.5, 5.5)
	far := joinNow(t, z, 3, 60.5, 60.5)
	z.advance(time.Now())
	for _, o := range []*fakeOut{out, near, far} {
		o.take()
	}
	itemNow(t, z, z.Equip(1, 9, 5))
	envs := out.take()
	if respFor(envs, 9, (*pb.Envelope).GetEquipResp) == nil {
		t.Fatalf("envs %v, want an EquipResp", envs)
	}
	want := &pb.LookChange{Id: 1, WeaponEnhance: 10}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": near.take()} {
		if ls := looksIn(es); len(ls) != 1 || !proto.Equal(ls[0], want) {
			t.Fatalf("%s LookChanges %v, want %v", name, ls, want)
		}
	}
	if ls := looksIn(far.take()); len(ls) != 0 {
		t.Fatalf("out-of-view LookChanges %v, want none", ls)
	}
	itemNow(t, z, z.Unequip(1, 10, 0))
	if ls := looksIn(near.take()); len(ls) != 1 || ls[0].Id != 1 || ls[0].WeaponEnhance != 0 {
		t.Fatalf("viewer LookChanges %v after unequip, want one with 0", ls)
	}
	z.advance(time.Now())
	if en := enterOf(far.take(), 1); en != nil {
		t.Fatalf("far viewer got %v", en)
	}
}

// Enhancing the worn weapon +6 -> +7 sends a LookChange; enhancing a worn
// helmet sends none.
func TestGlowOnEnhance(t *testing.T) {
	inv := forgeInventory()
	inv.Worn[0].Enhance = 6
	helmet := testItemData.Equipment["eq10_helmet"].SlotIndex
	inv.Worn[helmet], inv.Bag[8] = inv.Bag[8], nil
	inv.Worn[helmet].Enhance = 3
	z, p, _ := forgeZone(t, inv, false)
	z.rng = rand.New(winSource{})
	out := p.out.(*fakeOut)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	itemNow(t, z, z.Enhance(1, 9, items.Ref{Equipped: true, Slot: helmet}, false, 1))
	envs := out.take()
	if r := respFor(envs, 9, (*pb.Envelope).GetEnhanceResp); r == nil || !r.Success || r.Level != 4 {
		t.Fatalf("helmet EnhanceResp %v, want a success to +4", r)
	}
	if ls := append(looksIn(envs), looksIn(v.take())...); len(ls) != 0 {
		t.Fatalf("LookChanges %v for the helmet, want none", ls)
	}
	itemNow(t, z, z.Enhance(1, 10, items.Ref{Equipped: true, Slot: 0}, false, 1))
	envs = out.take()
	if r := respFor(envs, 10, (*pb.Envelope).GetEnhanceResp); r == nil || !r.Success || r.Level != 7 {
		t.Fatalf("weapon EnhanceResp %v, want a success to +7", r)
	}
	want := &pb.LookChange{Id: 1, WeaponEnhance: 7}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if ls := looksIn(es); len(ls) != 1 || !proto.Equal(ls[0], want) {
			t.Fatalf("%s LookChanges %v, want %v", name, ls, want)
		}
	}
}

// A level-up that makes the worn +8 weapon wearable sends a LookChange.
func TestGlowOnLevelUp(t *testing.T) {
	inv := gearInventory()
	inv.Worn[0].Enhance = 8
	z, p, _, _ := gearZoneAt(t, 9, inv)
	out := p.out.(*fakeOut)
	z.advance(time.Now())
	out.take()
	if p.weaponEnhance != 0 {
		t.Fatalf("weapon enhance %d at level 9, want 0", p.weaponEnhance)
	}
	z.gainExp(p, z.area.Exp.ToNext(p.level))
	z.runPlayerTimers()
	if !z.syncView(p) {
		t.Fatal("outbox full")
	}
	if ls := looksIn(out.take()); p.level != 10 || len(ls) != 1 || ls[0].Id != 1 || ls[0].WeaponEnhance != 8 {
		t.Fatalf("level %d LookChanges %v, want 10 and one with 8", p.level, ls)
	}
}

// ______________________TEST FORGE ZONE_____________________

// socketInventory is forgeInventory (the white sword and armour worn, the
// Vàng boots in bag slot 0, 12 stones in slot 3, a +15 helmet in slot 8 and a
// +7 one in slot 9) with two holes in the worn armour (an empty one, then an
// HP gem), three empty holes in the +15 helmet, three level-1 HP gems in bag
// slot 4, a level-2 attack gem in slot 5 and two refine stones in slot 6.
func socketInventory() *items.Inventory {
	inv := forgeInventory()
	inv.Bag[6] = &items.Item{Code: gamedata.StoneRefine, Count: 2}
	inv.Worn[testItemData.Equipment["eq10_armor"].SlotIndex].Gems = []string{"", "gem_hp_1"}
	inv.Bag[8].Gems = []string{"", "", ""}
	inv.Bag[4] = &items.Item{Code: "gem_hp_1", Count: 3}
	inv.Bag[5] = &items.Item{Code: "gem_attack_2", Count: 1}
	return inv
}

// forgeCase is one rejected forge command: the setup, whether the player is
// dead or far from the blacksmith, and the code it must get.
type forgeCase struct {
	name      string
	dead, far bool
	setup     func(*player)
	send      func(z *Zone) bool
	code      pb.ErrorCode
}

// runForgeErrors runs each case on socketInventory: the reply is only the
// error, and nothing changes or is saved.
func runForgeErrors(t *testing.T, cases []forgeCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := forgeZone(t, socketInventory(), c.far)
			out := p.out.(*fakeOut)
			if c.setup != nil {
				c.setup(p)
			}
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			out.take()
			before, stats, hp, xu, saves := p.inv.Clone(), p.stats, p.hp, p.xu, len(sv.all())
			itemNow(t, z, c.send(z))
			envs := out.take()
			if er := errorFor(envs, 9); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if n := len(slices.DeleteFunc(slices.Clone(envs), func(e *pb.Envelope) bool { return e.Seq != 9 })); n != 1 {
				t.Fatalf("%d replies to seq 9, want only the error", n)
			}
			if !reflect.DeepEqual(p.inv, before) || p.stats != stats || p.hp != hp || p.xu != xu {
				t.Fatal("a rejected forge command changed the inventory, stats or Xu")
			}
			if n := len(sv.all()); n != saves {
				t.Fatalf("%d saves, want %d", n, saves)
			}
		})
	}
}

const (
	codeDead      = pb.ErrorCode_ERROR_CODE_DEAD
	codeTooFar    = pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
	codeBad       = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	codeNotFound  = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
	codeNoXu      = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU
	codeBagFull   = pb.ErrorCode_ERROR_CODE_BAG_FULL
	codeNoRefine  = pb.ErrorCode_ERROR_CODE_NO_PENDING_REFINE
	codeMaterials = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_MATERIALS
)

func bagRef(slot int) items.Ref  { return items.Ref{Slot: slot} }
func wornRef(slot int) items.Ref { return items.Ref{Equipped: true, Slot: slot} }

// Each punch check of spec 4.2 in order; every case also breaks the checks
// after its own where it can.
func TestPunchErrors(t *testing.T) {
	armour := testItemData.Equipment["eq10_armor"].SlotIndex
	broke := func(p *player) { p.xu = 0 }
	punch := func(r items.Ref) func(*Zone) bool { return func(z *Zone) bool { return z.Punch(1, 9, r) } }
	runForgeErrors(t, []forgeCase{
		{"dead far from the blacksmith, bad slot", true, true, broke, punch(bagRef(60)), codeDead},
		{"far from the blacksmith, bad slot", false, true, broke, punch(bagRef(60)), codeTooFar},
		{"bag slot 60", false, false, broke, punch(bagRef(60)), codeBad},
		{"worn slot 10", false, false, broke, punch(wornRef(10)), codeBad},
		{"empty bag slot", false, false, broke, punch(bagRef(7)), codeNotFound},
		{"stones", false, false, broke, punch(bagRef(3)), codeBad},
		{"three holes", false, false, broke, punch(bagRef(8)), pb.ErrorCode_ERROR_CODE_HOLES_MAXED},
		// The third hole costs 400 x tier 10.
		{"1 Xu short", false, false, func(p *player) { p.xu = 3999 }, punch(wornRef(armour)), codeNoXu},
	})
}

// Each socket check of spec 4.3 in order.
func TestSocketErrors(t *testing.T) {
	armour := testItemData.Equipment["eq10_armor"].SlotIndex
	socket := func(r items.Ref, hole, gemSlot int) func(*Zone) bool {
		return func(z *Zone) bool { return z.Socket(1, 9, r, hole, gemSlot) }
	}
	runForgeErrors(t, []forgeCase{
		{"dead far from the blacksmith, bad slot", true, true, nil, socket(bagRef(60), 9, 60), codeDead},
		{"far from the blacksmith, bad slot", false, true, nil, socket(bagRef(60), 9, 60), codeTooFar},
		{"bag slot 60", false, false, nil, socket(bagRef(60), 0, 4), codeBad},
		{"gem slot 60", false, false, nil, socket(wornRef(armour), 0, 60), codeBad},
		{"gem slot -1", false, false, nil, socket(wornRef(armour), 0, -1), codeBad},
		{"empty item slot", false, false, nil, socket(bagRef(7), 0, 4), codeNotFound},
		{"empty gem slot", false, false, nil, socket(wornRef(armour), 9, 7), codeNotFound},
		{"stones as the item", false, false, nil, socket(bagRef(3), 0, 4), codeBad},
		{"stones as the gem", false, false, nil, socket(wornRef(armour), 0, 3), codeBad},
		{"hole 2 of 2", false, false, nil, socket(wornRef(armour), 2, 4), codeBad},
		{"hole -1", false, false, nil, socket(wornRef(armour), -1, 4), codeBad},
		{"filled hole", false, false, nil, socket(wornRef(armour), 1, 5), codeBad},
		{"level-2 gem in a tier-10 piece", false, false, nil, socket(wornRef(armour), 0, 5), pb.ErrorCode_ERROR_CODE_GEM_TOO_HIGH},
	})
}

// Each unsocket check of spec 4.4 in order.
func TestUnsocketErrors(t *testing.T) {
	armour := testItemData.Equipment["eq10_armor"].SlotIndex
	broke := func(p *player) { p.xu = 0 }
	// fullBag leaves no stack the gem can join and no empty slot.
	fullBag := func(p *player) {
		p.xu, p.inv.Bag[4] = 0, nil
		for i, it := range p.inv.Bag {
			if it == nil {
				p.inv.Bag[i] = &items.Item{Code: "eq10_boots", Count: 1}
			}
		}
	}
	unsocket := func(r items.Ref, hole int) func(*Zone) bool {
		return func(z *Zone) bool { return z.Unsocket(1, 9, r, hole) }
	}
	runForgeErrors(t, []forgeCase{
		{"dead far from the blacksmith, bad slot", true, true, broke, unsocket(bagRef(60), 9), codeDead},
		{"far from the blacksmith, bad slot", false, true, broke, unsocket(bagRef(60), 9), codeTooFar},
		{"bag slot 60", false, false, broke, unsocket(bagRef(60), 1), codeBad},
		{"empty bag slot", false, false, broke, unsocket(bagRef(7), 1), codeNotFound},
		{"stones", false, false, broke, unsocket(bagRef(3), 0), codeBad},
		{"hole 2 of 2", false, false, broke, unsocket(wornRef(armour), 2), codeBad},
		{"hole -1", false, false, broke, unsocket(wornRef(armour), -1), codeBad},
		{"empty hole", false, false, broke, unsocket(wornRef(armour), 0), codeBad},
		{"bag full", false, false, fullBag, unsocket(wornRef(armour), 1), codeBagFull},
		// A level-1 gem costs 500.
		{"1 Xu short", false, false, func(p *player) { p.xu = 499 }, unsocket(wornRef(armour), 1), codeNoXu},
	})
}

// Socketing an HP gem into the worn armour raises max HP by the gem's value
// with the HP share kept, shows the new bar, replies with the gem's slot and
// the armour, and saves at once; it costs no Xu.
func TestSocketWornArmour(t *testing.T) {
	z, p, sv := forgeZone(t, socketInventory(), false)
	out := p.out.(*fakeOut)
	z.advance(time.Now())
	out.take()
	slot := testItemData.Equipment["eq10_armor"].SlotIndex
	p.hp = p.maxHP / 2
	hp, oldMax := p.hp, p.maxHP
	itemNow(t, z, z.Socket(1, 9, wornRef(slot), 0, 4))
	envs := out.take()
	resp := respFor(envs, 9, (*pb.Envelope).GetSocketResp)
	if resp == nil || !slices.Equal(p.inv.Worn[slot].Gems, []string{"gem_hp_1", "gem_hp_1"}) {
		t.Fatalf("SocketResp %v gems %v, want the gem in hole 0", resp, p.inv.Worn[slot].Gems)
	}
	gem := testItemData.Stackables["gem_hp_1"].Gem.Stats["hp"]
	if wantMax := oldMax + gem; p.maxHP != wantMax || p.hp != keepHPRatio(hp, oldMax, wantMax) || p.xu != 100000 {
		t.Fatalf("hp %d/%d xu %d, want %d/%d and no Xu spent", p.hp, p.maxHP, p.xu, keepHPRatio(hp, oldMax, wantMax), wantMax)
	}
	want := &pb.InventoryUpdate{
		Bag:      []*pb.ItemSlot{{Slot: 4, Item: &pb.ItemInfo{Code: "gem_hp_1", Count: 2}}},
		Equipped: []*pb.ItemSlot{itemSlot(slot, p.inv.Worn[slot])},
	}
	if !proto.Equal(resp.Update, want) || resp.Update.Equipped[0].Item.Holes != 2 || !proto.Equal(resp.Stats, z.selfStats(p)) {
		t.Fatalf("update %v stats %v, want %v and the new stats", resp.Update, resp.Stats, want)
	}
	if all := sv.all(); len(all) != 1 || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
		t.Fatalf("saves %+v, want one with the gem socketed", all)
	}
	z.advance(time.Now())
	if cs := hpChangesIn(append(envs, out.take()...)); len(cs) != 1 || cs[0].MaxHp != int32(p.maxHP) {
		t.Fatalf("HpChanges %v, want one to %d", cs, p.maxHP)
	}
}

// Punching a bag piece pays 100 x tier and adds an empty hole without
// touching the stats; unsocketing from the worn armour pays 500, returns the
// gem onto its stack and lowers max HP again.
func TestPunchAndUnsocket(t *testing.T) {
	z, p, sv := forgeZone(t, socketInventory(), false)
	out := p.out.(*fakeOut)
	z.advance(time.Now())
	out.take()
	stats, maxHP := p.stats, p.maxHP
	itemNow(t, z, z.Punch(1, 9, bagRef(0)))
	resp := respFor(out.take(), 9, (*pb.Envelope).GetPunchResp)
	if resp == nil || !slices.Equal(p.inv.Bag[0].Gems, []string{""}) || p.xu != 100000-1000 || p.stats != stats ||
		!proto.Equal(resp.Update, &pb.InventoryUpdate{Bag: []*pb.ItemSlot{itemSlot(0, p.inv.Bag[0])}}) ||
		resp.Update.Bag[0].Item.Holes != 1 || resp.Stats.Xu != p.xu {
		t.Fatalf("PunchResp %v gems %v xu %d, want one empty hole for 1000 Xu", resp, p.inv.Bag[0].Gems, p.xu)
	}

	slot := testItemData.Equipment["eq10_armor"].SlotIndex
	itemNow(t, z, z.Unsocket(1, 10, wornRef(slot), 1))
	un := respFor(out.take(), 10, (*pb.Envelope).GetUnsocketResp)
	gem := testItemData.Stackables["gem_hp_1"].Gem.Stats["hp"]
	if un == nil || !slices.Equal(p.inv.Worn[slot].Gems, []string{"", ""}) || p.inv.Bag[4].Count != 4 ||
		p.xu != 100000-1000-500 || p.maxHP != maxHP-gem {
		t.Fatalf("UnsocketResp %v gems %v xu %d max HP %d, want the hole emptied, 4 gems, %d Xu, %d HP",
			un, p.inv.Worn[slot].Gems, p.xu, p.maxHP, 100000-1500, maxHP-gem)
	}
	want := &pb.InventoryUpdate{
		Bag:      []*pb.ItemSlot{{Slot: 4, Item: &pb.ItemInfo{Code: "gem_hp_1", Count: 4}}},
		Equipped: []*pb.ItemSlot{itemSlot(slot, p.inv.Worn[slot])},
	}
	if !proto.Equal(un.Update, want) || !proto.Equal(un.Stats, z.selfStats(p)) {
		t.Fatalf("update %v, want %v", un.Update, want)
	}
	if all := sv.all(); len(all) != 2 || !reflect.DeepEqual(all[1].Items, itemRows(p.inv)) || all[1].Xu != p.xu {
		t.Fatalf("saves %+v, want one per command", all)
	}
}

// Wearing a Phong helmet lights line 1 of the worn Viêm sword's hidden lines
// (spec 4.1): the stats rise by that line on top of the helmet's own; a Nham
// helmet does not light it, and taking the helmet off removes it.
func TestResonanceInZone(t *testing.T) {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Worn[0] = &items.Item{Code: "eq10_weapon_viem", Count: 1, Quality: 2, Element: "viem",
		Hidden: []items.Line{{Stat: "hp", Value: 86}, {Stat: "phys_attack", Value: 5}}}
	inv.Bag[0] = &items.Item{Code: "eq10_helmet", Count: 1, Element: "phong"}
	inv.Bag[1] = &items.Item{Code: "eq10_helmet", Count: 1, Element: "nham"}
	z, p, _ := forgeZone(t, inv, false)
	out := p.out.(*fakeOut)
	base := p.maxHP
	helmet := testItemData.Equipment["eq10_helmet"].SlotIndex

	itemNow(t, z, z.Equip(1, 9, 1))
	nham := p.maxHP
	itemNow(t, z, z.Equip(1, 10, 0)) // swaps the Nham helmet back into slot 0
	phong := p.maxHP
	if p.inv.Worn[helmet].Element != "phong" || phong-nham != 86 || nham <= base {
		t.Fatalf("max HP %d without the helmet, %d with Nham, %d with Phong; want Phong 86 above Nham", base, nham, phong)
	}
	itemNow(t, z, z.Unequip(1, 11, helmet))
	if p.maxHP != base {
		t.Fatalf("max HP %d after taking the helmet off, want %d", p.maxHP, base)
	}
	if es := out.take(); errorFor(es, 9) != nil || errorFor(es, 10) != nil || errorFor(es, 11) != nil {
		t.Fatalf("errors in %v", es)
	}
}

// ______________________TEST REFINE ZONE_____________________

// Each refine check of spec 4.5 in order, on socketInventory (the Vàng boots
// in bag slot 0 have three lines; tier 10 costs 200 Xu and a stone, plus as
// much per lock).
func TestRefineErrors(t *testing.T) {
	broke := func(p *player) { p.xu = 0 }
	refine := func(r items.Ref, locked ...int) func(*Zone) bool {
		return func(z *Zone) bool { return z.Refine(1, 9, r, locked) }
	}
	runForgeErrors(t, []forgeCase{
		{"dead far from the blacksmith, bad slot", true, true, broke, refine(bagRef(60)), codeDead},
		{"far from the blacksmith, bad slot", false, true, broke, refine(bagRef(60)), codeTooFar},
		{"bag slot 60", false, false, broke, refine(bagRef(60)), codeBad},
		{"empty bag slot", false, false, broke, refine(bagRef(7)), codeNotFound},
		{"stones", false, false, broke, refine(bagRef(3)), codeBad},
		{"white helmet", false, false, broke, refine(bagRef(9), 5), pb.ErrorCode_ERROR_CODE_NOT_REFINABLE},
		{"lock 3 of 3 lines", false, false, broke, refine(bagRef(0), 3), codeBad},
		{"lock repeated", false, false, broke, refine(bagRef(0), 1, 1), codeBad},
		{"every line locked", false, false, broke, refine(bagRef(0), 0, 1, 2), codeBad},
		{"3 stones for 2 locks", false, false, broke, refine(bagRef(0), 0, 1), codeMaterials},
		{"1 Xu short for 1 lock", false, false, func(p *player) { p.xu = 399 }, refine(bagRef(0), 2), codeNoXu},
	})
}

// refineNow refines the boots in bag slot 0 of socketInventory with seq and
// locked and returns the reply; the zone's rng is seeded so the new lines are
// the same every run.
func refineNow(t *testing.T, z *Zone, seq uint32, locked ...int) *pb.RefineResp {
	t.Helper()
	out := z.players[1].out.(*fakeOut)
	itemNow(t, z, z.Refine(1, seq, bagRef(0), locked))
	envs := out.take()
	r := respFor(envs, seq, (*pb.Envelope).GetRefineResp)
	if r == nil {
		t.Fatalf("no RefineResp to %d in %v", seq, envs)
	}
	return r
}

// chooseNow sends RefineChooseReq with seq and returns the reply and the
// error, one of them nil.
func chooseNow(t *testing.T, z *Zone, seq uint32, accept bool) (*pb.RefineChooseResp, *pb.ErrorResp) {
	t.Helper()
	out := z.players[1].out.(*fakeOut)
	itemNow(t, z, z.RefineChoose(1, seq, accept))
	envs := out.take()
	return respFor(envs, seq, (*pb.Envelope).GetRefineChooseResp), errorFor(envs, seq)
}

func pbToLines(ls []*pb.ItemLine) []items.Line {
	out := make([]items.Line, len(ls))
	for i, l := range ls {
		out[i] = items.Line{Stat: l.Stat, Value: int(l.Value)}
	}
	return out
}

// The roll pays and saves at once but leaves the item; taking the result
// gives the boots the new lines, keeping the old lines keeps them, and both
// clear the result, so a second choice gets NO_PENDING_REFINE.
func TestRefineChoose(t *testing.T) {
	for _, accept := range []bool{true, false} {
		t.Run(fmt.Sprintf("accept %v", accept), func(t *testing.T) {
			z, p, sv := forgeZone(t, socketInventory(), false)
			z.rng = rand.New(rand.NewPCG(1, 2))
			old := slices.Clone(p.inv.Bag[0].Lines)
			resp := refineNow(t, z, 9, 1)
			lines := pbToLines(resp.Lines)
			if len(lines) != 3 || lines[1] != old[1] || slices.Equal(lines, old) || !slices.Equal(p.inv.Bag[0].Lines, old) {
				t.Fatalf("lines %v from %v, want line 1 kept, others new and the boots untouched", lines, old)
			}
			// One lock on tier 10: 2 stones and (20 + 20) x 10 Xu, paid at the roll.
			want := &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 6}}}
			if p.xu != 100000-400 || p.inv.Bag[6] != nil || !proto.Equal(resp.Update, want) || resp.Stats.Xu != p.xu {
				t.Fatalf("xu %d update %v, want 400 Xu and both stones gone", p.xu, resp.Update)
			}
			if all := sv.all(); len(all) != 1 || all[0].Xu != p.xu || !reflect.DeepEqual(all[0].Items, itemRows(p.inv)) {
				t.Fatalf("saves %+v, want one at the roll", all)
			}

			r, er := chooseNow(t, z, 10, accept)
			kept := old
			if accept {
				kept = lines
			}
			if er != nil || r == nil || !slices.Equal(p.inv.Bag[0].Lines, kept) || p.refine != nil {
				t.Fatalf("choose %v %v lines %v, want %v and no pending result", r, er, p.inv.Bag[0].Lines, kept)
			}
			if !proto.Equal(r.Update, &pb.InventoryUpdate{Bag: []*pb.ItemSlot{itemSlot(0, p.inv.Bag[0])}}) ||
				p.xu != 100000-400 || !proto.Equal(r.Stats, z.selfStats(p)) {
				t.Fatalf("update %v xu %d, want the boots and nothing refunded", r.Update, p.xu)
			}
			if all := sv.all(); len(all) != 2 || !reflect.DeepEqual(all[1].Items, itemRows(p.inv)) {
				t.Fatalf("saves %+v, want a second one at the choice", all)
			}
			if _, er := chooseNow(t, z, 11, true); er == nil || er.Code != codeNoRefine {
				t.Fatalf("second choice: %v, want NO_PENDING_REFINE", er)
			}
		})
	}
}

// Without a refine there is nothing to choose; the error costs nothing.
func TestRefineChooseWithoutRefine(t *testing.T) {
	z, p, sv := forgeZone(t, socketInventory(), false)
	before := p.inv.Clone()
	if r, er := chooseNow(t, z, 9, true); r != nil || er == nil || er.Code != codeNoRefine {
		t.Fatalf("choose %v %v, want NO_PENDING_REFINE", r, er)
	}
	if !reflect.DeepEqual(p.inv, before) || len(sv.all()) != 0 {
		t.Fatal("a rejected choice changed the inventory or saved")
	}
}

// Taking the result of a worn piece recomputes the stats; the choice needs
// neither the blacksmith nor a live character (spec 4.5).
func TestRefineChooseWornWhileDeadOutside(t *testing.T) {
	inv := socketInventory()
	boots := testItemData.Equipment["eq10_boots"].SlotIndex
	inv.Worn[boots], inv.Bag[0] = inv.Bag[0], nil
	z, p, _ := forgeZone(t, inv, false)
	z.rng = rand.New(rand.NewPCG(1, 2))
	out := p.out.(*fakeOut)
	itemNow(t, z, z.Refine(1, 9, wornRef(boots), nil))
	if r := respFor(out.take(), 9, (*pb.Envelope).GetRefineResp); r == nil {
		t.Fatal("no RefineResp")
	}
	stats, maxHP := p.stats, p.maxHP
	z.area.NPCs = nil
	z.damagePlayer(p, killer{}, 1<<30, false)
	z.advance(time.Now())
	out.take()
	r, er := chooseNow(t, z, 10, true)
	if er != nil || r == nil {
		t.Fatalf("choose %v %v, want a reply while dead far from the blacksmith", r, er)
	}
	if p.stats == stats && p.maxHP == maxHP {
		t.Fatalf("stats %+v unchanged by the new lines %v", p.stats, p.inv.Worn[boots].Lines)
	}
	got, gotMax := p.stats, p.maxHP
	z.recompute(p)
	if p.stats != got || p.maxHP != gotMax || !proto.Equal(r.Update, &pb.InventoryUpdate{
		Equipped: []*pb.ItemSlot{itemSlot(boots, p.inv.Worn[boots])}}) {
		t.Fatalf("stats %+v after the choice, %+v recomputed; update %v", got, p.stats, r.Update)
	}
}

// Any other item command, even a rejected one, drops the waiting result
// before it runs; a pickup does not, and a second refine replaces it.
func TestRefinePendingCleared(t *testing.T) {
	for _, c := range []struct {
		name  string
		send  func(z *Zone) bool
		keeps bool
	}{
		{"equip", func(z *Zone) bool { return z.Equip(1, 20, 9) }, false},
		{"rejected equip", func(z *Zone) bool { return z.Equip(1, 20, 7) }, false},
		{"unequip", func(z *Zone) bool { return z.Unequip(1, 20, 0) }, false},
		{"discard", func(z *Zone) bool { return z.Discard(1, 20, 9, 0) }, false},
		{"sort", func(z *Zone) bool { return z.SortBag(1, 20) }, false},
		{"enhance", func(z *Zone) bool { return z.Enhance(1, 20, bagRef(9), false, 1) }, false},
		{"punch", func(z *Zone) bool { return z.Punch(1, 20, bagRef(9)) }, false},
		{"socket", func(z *Zone) bool { return z.Socket(1, 20, bagRef(8), 0, 4) }, false},
		{"unsocket", func(z *Zone) bool { return z.Unsocket(1, 20, bagRef(8), 0) }, false},
		{"rejected refine", func(z *Zone) bool { return z.Refine(1, 20, bagRef(9), nil) }, false},
		{"pickup", func(z *Zone) bool {
			g := putGround(z, z.players[1].charID, z.players[1].pos, items.Item{Code: gamedata.StoneRefine, Count: 1})
			return z.Pickup(1, 20, g.id)
		}, true},
		{"rejected pickup", func(z *Zone) bool { return z.Pickup(1, 20, 999999) }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, _ := forgeZone(t, socketInventory(), false)
			old := slices.Clone(p.inv.Bag[0].Lines)
			lines := pbToLines(refineNow(t, z, 9).Lines)
			itemNow(t, z, c.send(z))
			p.out.(*fakeOut).take()
			r, er := chooseNow(t, z, 21, true)
			switch {
			case c.keeps && (er != nil || !slices.Equal(p.inv.Bag[0].Lines, lines)):
				t.Fatalf("choose %v %v lines %v, want the result %v taken", r, er, p.inv.Bag[0].Lines, lines)
			case !c.keeps && (er == nil || er.Code != codeNoRefine || !slices.Equal(p.inv.Bag[0].Lines, old)):
				t.Fatalf("choose %v %v lines %v, want NO_PENDING_REFINE and %v kept", r, er, p.inv.Bag[0].Lines, old)
			}
		})
	}
}

// A second refine replaces the first result: the choice applies the second.
func TestRefineReplacesPending(t *testing.T) {
	inv := socketInventory()
	inv.Bag[6].Count = 5
	z, p, _ := forgeZone(t, inv, false)
	z.rng = rand.New(rand.NewPCG(3, 4))
	first := pbToLines(refineNow(t, z, 9).Lines)
	second := pbToLines(refineNow(t, z, 10).Lines)
	if slices.Equal(first, second) {
		t.Fatalf("both rolls gave %v; pick another seed", first)
	}
	if _, er := chooseNow(t, z, 11, true); er != nil || !slices.Equal(p.inv.Bag[0].Lines, second) {
		t.Fatalf("choose %v lines %v, want the second result %v", er, p.inv.Bag[0].Lines, second)
	}
}

// Leaving drops the result with the player: back in the zone there is
// nothing to choose.
func TestRefinePendingDroppedOnLeave(t *testing.T) {
	z, p, _ := forgeZone(t, socketInventory(), false)
	refineNow(t, z, 9)
	if p.refine == nil {
		t.Fatal("no pending result after the roll")
	}
	<-z.leave(1)
	in := info(1, 5.5, 5.5)
	in.Inventory = socketInventory()
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	if _, er := chooseNow(t, z, 10, true); er == nil || er.Code != codeNoRefine {
		t.Fatalf("choose after rejoining: %v, want NO_PENDING_REFINE", er)
	}
}

// ______________________TEST DEPART_____________________

// departNow queues a departure of id and runs one tick, so the zone handles
// it in drain as it would from Depart.
func departNow(t *testing.T, z *Zone, id uint64, d Departure) departResult {
	t.Helper()
	reply := make(chan departResult, 1)
	z.inbox <- departCmd{id: id, d: d, reply: reply}
	z.advance(time.Now())
	select {
	case r := <-reply:
		return r
	default:
		t.Fatal("the zone did not answer the departure")
		return departResult{}
	}
}

// joinByInbox queues a join of in and runs one tick, so the join happens in
// drain as it would from Join. It returns the EnterWorldResp.
func joinByInbox(t *testing.T, z *Zone, in PlayerInfo, out *fakeOut) *pb.EnterWorldResp {
	t.Helper()
	reply := make(chan error, 1)
	z.inbox <- joinCmd{info: in, out: out, reply: reply}
	z.advance(time.Now())
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
	r := out.take()[0].GetEnterWorldResp()
	if r == nil {
		t.Fatal("the first message is not EnterWorldResp")
	}
	return r
}

func notDead(d *Departing) pb.ErrorCode {
	if d.Dead() {
		return pb.ErrorCode_ERROR_CODE_DEAD
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// zoneCheck is the zone-change check of spec 3.4 steps 4 and 5.
func zoneCheck(d *Departing) pb.ErrorCode {
	if c := notDead(d); c != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return c
	}
	if !d.ZoneChangeReady() {
		return pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// transferZone is a zone laid out as castZone's, numbered no, with nobody in
// it and thuy_4 on a 3 s cooldown.
func transferZone(t *testing.T, no int) (*Zone, *monster) {
	t.Helper()
	z, m := newFightZone(t, 12, Vec2{30.5, 30.5})
	z.cfg.ZoneNo = no
	z.area.Skills = threeSecSkills()
	m.region.Kind.DetectRange = 0.5
	return z, m
}

func threeSecSkills() map[string]gamedata.Skill {
	s := maps.Clone(repoSkills)
	k := s["thuy_4"]
	k.CooldownMS = slices.Clone(k.CooldownMS)
	k.CooldownMS[0] = 3000
	s["thuy_4"] = k
	return s
}

// A refused departure changes nothing: the player stays in the zone and its
// cell, the count holds, nothing is saved and a waiting refine stays.
func TestDepartRefusedChangesNothing(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 12.5, 7.5)
	p := z.players[1]
	p.refine = &pendingRefine{}
	z.advance(time.Now())
	var seen Vec2
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{3.5, 3.5}}, Check: func(d *Departing) pb.ErrorCode {
		seen = d.Pos()
		return pb.ErrorCode_ERROR_CODE_DEAD
	}})
	if r.code != pb.ErrorCode_ERROR_CODE_DEAD || r.err != nil || r.saved != nil {
		t.Fatalf("result %+v, want a DEAD refusal without a save", r)
	}
	if seen != (Vec2{12.5, 7.5}) {
		t.Fatalf("the check saw %v, want the live position", seen)
	}
	if z.players[1] != p || z.cells[p.cell][1] == nil || z.Count() != 1 || p.refine == nil {
		t.Fatalf("player %v in cell %v count %d refine %v: a refusal must change nothing", z.players[1], z.cells[p.cell][1] != nil, z.Count(), p.refine)
	}
	if s := sv.all(); len(s) != 0 {
		t.Fatalf("saves %v, want none", s)
	}
}

// A departure takes the player out of the zone, saves it at the destination
// and hands over its live state as a deep copy.
func TestDepartHandsOverLiveState(t *testing.T) {
	inv := socketInventory()
	z, p, _, sv := gearZone(t, inv)
	z.area.Skills = repoSkills
	p.skills, p.skillPoints, p.freePoints, p.exp = map[string]int{"viem_1": 3}, 4, 2, 321
	p.hp, p.mp = p.maxHP-17, p.stats.MaxMP-9
	p.refine = &pendingRefine{}
	v := joinNow(t, z, 2, 6.5, 5.5)
	z.advance(time.Now())
	v.take()
	hp, mp, cell := p.hp, p.mp, p.cell
	wantBag := p.inv.Clone()

	r := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{20.5, 21.5}}, Check: notDead})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || r.err != nil || r.saved == nil {
		t.Fatalf("result %+v, want a departure", r)
	}
	if err := <-r.saved; err != nil {
		t.Fatal(err)
	}
	if z.players[1] != nil || z.cells[cell][1] != nil || z.Count() != 1 {
		t.Fatalf("player %v in cell %v count %d, want gone", z.players[1], z.cells[cell][1] != nil, z.Count())
	}
	saves := sv.all()
	st := saves[len(saves)-1]
	if st.ID != p.charID || st.MapID != "m2" || st.X != 20.5 || st.Y != 21.5 || st.HP != hp || st.MP != mp || st.Xu != 77 {
		t.Fatalf("saved %+v, want map m2 at (20.5, 21.5) with HP %d MP %d", st, hp, mp)
	}
	in := r.info
	if in.EntityID != 1 || in.CharID != p.charID || in.Name != p.name || in.Class != p.class || in.Level != p.level ||
		in.Exp != 321 || in.Points != p.points || in.FreePoints != 2 || in.SkillPoints != 4 ||
		in.HP != hp || in.MP != mp || in.Xu != 77 || in.Pos != (Vec2{20.5, 21.5}) || in.RespSeq != 0 {
		t.Fatalf("handoff %+v, want the live state at the destination", in)
	}
	if !maps.Equal(in.Skills, map[string]int{"viem_1": 3}) || !reflect.DeepEqual(in.Inventory, wantBag) {
		t.Fatalf("handoff skills %v inventory %v", in.Skills, in.Inventory)
	}
	// The handoff shares nothing with the zone's player.
	in.Skills["viem_1"] = 9
	for _, it := range in.Inventory.Bag {
		if it != nil {
			it.Count = 99
			if len(it.Lines) > 0 {
				it.Lines[0].Value = -1
			}
			if len(it.Gems) > 0 {
				it.Gems[0] = "mutated"
			}
		}
	}
	if p.skills["viem_1"] != 3 || !reflect.DeepEqual(p.inv, wantBag) {
		t.Fatal("mutating the handoff changed the departed player")
	}
	// The viewer is told the player left.
	z.advance(time.Now())
	if l := leaves(v.take()); !slices.Contains(l, 1) {
		t.Fatalf("viewer leaves %v, want 1", l)
	}
}

// A zone change keeps the position, in the save and in the handoff.
func TestDepartKeepPos(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 12.5, 7.5)
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m1", Pos: Vec2{1, 1}}, KeepPos: true, ZoneChange: true, Check: zoneCheck})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || r.info.Pos != (Vec2{12.5, 7.5}) {
		t.Fatalf("result %+v, want a departure at (12.5, 7.5)", r)
	}
	<-r.saved
	if st := sv.all()[0]; st.MapID != "m1" || st.X != 12.5 || st.Y != 7.5 {
		t.Fatalf("saved %+v, want m1 at (12.5, 7.5)", st)
	}
}

// A monster chasing the departed player lets it go and forgets its hits, and
// a shot already flying at it does not land, even if it comes back at once;
// nor does another player's hit in flight at it.
func TestDepartMonsterLosesTarget(t *testing.T) {
	z, m, p, a := castZone(t, Vec2{30.5, 30.5}, Vec2{35.5, 30.5})
	z.startChase(m, 1)
	m.attackers[1] = z.tickNo
	z.schedule(impact{due: z.tickNo + 3, attacker: m.id, byMonster: true, target: 1, damage: 50})
	z.schedule(impact{due: z.tickNo + 3, attacker: 99, targetIsPlayer: true, attackerName: "x", target: 1, damage: 40})
	a.take()
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m1"}, KeepPos: true, Check: notDead})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("code %v", r.code)
	}
	z.advance(time.Now())
	if m.target != 0 || chasing(m) {
		t.Fatalf("monster target %d state %v, want no target", m.target, m.state)
	}
	if _, ok := m.attackers[1]; ok {
		t.Fatal("the monster still remembers the departed player's hit")
	}
	hp := r.info.HP
	joinByInbox(t, z, r.info, &fakeOut{})
	for range 5 {
		z.advance(time.Now())
	}
	if q := z.players[1]; q == nil || q.hp != hp || p == q {
		t.Fatalf("back in the zone: %v, want a new player with HP %d", q, hp)
	}
}

// A slash and a cast queued in the tick of a departure are answered
// ON_COOLDOWN before the player leaves, so no request of the client is left
// without a reply (the client takes an unanswered one for a dead link).
func TestDepartAnswersPendingAttackAndCast(t *testing.T) {
	z, m, _, a := castZone(t, Vec2{30.5, 30.5}, Vec2{31.5, 30.5})
	z.inbox <- attackCmd{id: 1, seq: 77, target: m.id}
	z.inbox <- castCmd{id: 1, seq: 78, skillID: "thuy_4", target: m.id}
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m1"}, KeepPos: true, Check: notDead})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || r.err != nil {
		t.Fatalf("result %+v, want a departure", r)
	}
	got := map[uint32]pb.ErrorCode{}
	for _, e := range a.take() {
		if e.Seq == 77 || e.Seq == 78 {
			got[e.Seq] = e.GetError().GetCode()
		}
	}
	want := map[uint32]pb.ErrorCode{77: pb.ErrorCode_ERROR_CODE_ON_COOLDOWN, 78: pb.ErrorCode_ERROR_CODE_ON_COOLDOWN}
	if !maps.Equal(got, want) {
		t.Fatalf("replies %v, want ON_COOLDOWN for the slash and the cast", got)
	}
	if m.hp != m.maxHP {
		t.Fatalf("monster HP %d of %d: nothing may hit after the departure", m.hp, m.maxHP)
	}
}

// A fresh login has no cooldowns; a skill cast 1 s before leaving is still on
// cooldown in the next zone for the 2 s left, and EnterWorldResp says so.
func TestDepartCarriesSkillCooldown(t *testing.T) {
	za, _ := transferZone(t, 1)
	outA := &fakeOut{}
	in := info(1, 35.5, 30.5)
	in.Class, in.Level, in.Skills = pb.CharClass_CHAR_CLASS_THUY, 12, castSkills
	if r := joinByInbox(t, za, in, outA); len(r.Cooldowns) != 0 {
		t.Fatalf("fresh login cooldowns %v, want none", r.Cooldowns)
	}
	castNow(t, za, 1, 2, "thuy_4", 0, Vec2{})
	if envs := outA.take(); castRespAt(envs, 2) < 0 {
		t.Fatalf("cast refused: %v", errorFor(envs, 2))
	}
	t0 := za.tickNo
	for za.tickNo < t0+9 {
		za.advance(time.Now())
	}
	r := departNow(t, za, 1, Departure{To: Place{MapID: "m1"}, KeepPos: true, Check: notDead}) // tick t0+10: 1 s later
	if got := r.info.Cooldowns; !maps.Equal(got, map[string]time.Duration{"thuy_4": 2 * time.Second}) {
		t.Fatalf("handoff cooldowns %v, want thuy_4 2s", got)
	}

	zb, _ := transferZone(t, 2)
	outB := &fakeOut{}
	ew := joinByInbox(t, zb, r.info, outB)
	j := zb.tickNo
	if cd := ew.Cooldowns; len(cd) != 1 || cd[0].Key != "thuy_4" || cd[0].RemainingMs != 2000 {
		t.Fatalf("EnterWorldResp cooldowns %v, want thuy_4 2000 ms", cd)
	}
	for zb.tickNo < j+18 {
		zb.advance(time.Now())
	}
	castNow(t, zb, 1, 3, "thuy_4", 0, Vec2{}) // tick j+19
	if er := errorFor(outB.take(), 3); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("cast 1.9 s after arriving: %v, want ON_COOLDOWN", er)
	}
	castNow(t, zb, 1, 4, "thuy_4", 0, Vec2{}) // tick j+20: 3 s after the first cast
	if envs := outB.take(); castRespAt(envs, 4) < 0 {
		t.Fatalf("cast 2 s after arriving refused: %v", errorFor(envs, 4))
	}
}

// The slash interval carries over too.
func TestDepartCarriesSlashCooldown(t *testing.T) {
	za, ma := transferZone(t, 1)
	outA := &fakeOut{}
	in := info(1, 31.5, 30.5)
	in.Class, in.Level = pb.CharClass_CHAR_CLASS_THUY, 12
	joinByInbox(t, za, in, outA)
	if !za.Attack(1, 2, ma.id) {
		t.Fatal("inbox full")
	}
	za.advance(time.Now())
	if envs := outA.take(); attackRespAt(envs, 2) < 0 {
		t.Fatalf("slash refused: %v", errorFor(envs, 2))
	}
	r := departNow(t, za, 1, Departure{To: Place{MapID: "m1"}, KeepPos: true, Check: notDead}) // 1 tick later
	slash := za.ticks(combat.SlashCooldown.Seconds()) - 1
	want := time.Duration(slash) * time.Second / 10
	if got := r.info.Cooldowns; !maps.Equal(got, map[string]time.Duration{"slash": want}) {
		t.Fatalf("handoff cooldowns %v, want slash %v", got, want)
	}

	zb, mb := transferZone(t, 2)
	outB := &fakeOut{}
	ew := joinByInbox(t, zb, r.info, outB)
	j := zb.tickNo
	if cd := ew.Cooldowns; len(cd) != 1 || cd[0].Key != "slash" || cd[0].RemainingMs != int32(want.Milliseconds()) {
		t.Fatalf("EnterWorldResp cooldowns %v, want slash %v", cd, want)
	}
	for zb.tickNo < j+slash-2 {
		zb.advance(time.Now())
	}
	if !zb.Attack(1, 3, mb.id) {
		t.Fatal("inbox full")
	}
	zb.advance(time.Now()) // j+slash-1
	if er := errorFor(outB.take(), 3); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("slash one tick early: %v, want ON_COOLDOWN", er)
	}
	if !zb.Attack(1, 4, mb.id) {
		t.Fatal("inbox full")
	}
	zb.advance(time.Now()) // j+slash
	if envs := outB.take(); attackRespAt(envs, 4) < 0 {
		t.Fatalf("slash on time refused: %v", errorFor(envs, 4))
	}
}

// A zone change starts a 10 s zone-change cooldown: a second zone change
// within it is refused, a portal is not and carries what is left, and once it
// has run out the zone change is accepted.
func TestZoneChangeCooldown(t *testing.T) {
	za, _ := newTestZone(10, openGrid(64, 64))
	joinNow(t, za, 1, 12.5, 7.5)
	change := Departure{To: Place{MapID: "m1"}, KeepPos: true, ZoneChange: true, Check: zoneCheck}
	r := departNow(t, za, 1, change)
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || r.info.Cooldowns["zone_change"] != ZoneChangeCooldown {
		t.Fatalf("first zone change %+v, want accepted with a %v cooldown", r, ZoneChangeCooldown)
	}

	zb, _ := newTestZone(10, openGrid(64, 64))
	ew := joinByInbox(t, zb, r.info, &fakeOut{})
	if cd := ew.Cooldowns; len(cd) != 1 || cd[0].Key != "zone_change" || cd[0].RemainingMs != 10000 {
		t.Fatalf("EnterWorldResp cooldowns %v, want zone_change 10000 ms", cd)
	}
	if r := departNow(t, zb, 1, change); r.code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN || zb.players[1] == nil {
		t.Fatalf("second zone change %+v, want ON_COOLDOWN", r)
	}
	portal := Departure{To: Place{MapID: "m2", Pos: Vec2{3.5, 3.5}}, Check: notDead}
	r = departNow(t, zb, 1, portal) // two ticks after arriving
	if left := ZoneChangeCooldown - 2*100*time.Millisecond; r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED ||
		!maps.Equal(r.info.Cooldowns, map[string]time.Duration{"zone_change": left}) {
		t.Fatalf("portal %+v, want accepted carrying zone_change %v", r, left)
	}

	zc, _ := newTestZone(10, openGrid(64, 64))
	joinByInbox(t, zc, r.info, &fakeOut{})
	k := zc.tickNo // ready at k+98
	for zc.tickNo < k+96 {
		zc.advance(time.Now())
	}
	if r := departNow(t, zc, 1, change); r.code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("zone change one tick early: %+v, want ON_COOLDOWN", r)
	}
	// On time: the zone change would pass, and a portal hands over nothing.
	var ready bool
	r = departNow(t, zc, 1, Departure{To: portal.To, Check: func(d *Departing) pb.ErrorCode {
		ready = zoneCheck(d) == pb.ErrorCode_ERROR_CODE_UNSPECIFIED
		return notDead(d)
	}})
	if !ready || r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || len(r.info.Cooldowns) != 0 {
		t.Fatalf("on time: zone change ready %v, portal %+v, want ready and no cooldowns", ready, r)
	}
}

// Depart through a running zone: it answers, the save completes, a second
// departure finds nobody (ErrNotInZone), and a stopped zone answers
// ErrZoneClosed. A player evicted before its departure keeps its save for Leave.
func TestDepartRunningZone(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	ctx, cancel := context.WithCancel(context.Background())
	go z.Run(ctx)
	out := &fakeOut{}
	if err := z.Join(ctx, info(1, 5.5, 5.5), out); err != nil {
		t.Fatal(err)
	}
	if err := z.Join(ctx, info(2, 6.5, 5.5), &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	in, saved, code, err := z.Depart(ctx, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{9.5, 9.5}}, Check: notDead})
	if err != nil || code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || in.EntityID != 1 || in.Pos != (Vec2{9.5, 9.5}) {
		t.Fatalf("depart: %+v %v %v", in, code, err)
	}
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := z.Depart(ctx, 1, Departure{Check: notDead}); !errors.Is(err, ErrNotInZone) {
		t.Fatalf("second departure: %v, want ErrNotInZone", err)
	}
	cancel()
	<-z.Done()
	if _, _, _, err := z.Depart(context.Background(), 2, Departure{Check: notDead}); !errors.Is(err, ErrZoneClosed) {
		t.Fatalf("departure from a stopped zone: %v, want ErrZoneClosed", err)
	}

	ze, _ := newTestZone(10, openGrid(64, 64))
	joinNow(t, ze, 3, 5.5, 5.5)
	ze.departed[3] = ze.remove(ze.players[3])
	if r := departNow(t, ze, 3, Departure{Check: notDead}); !errors.Is(r.err, ErrNotInZone) {
		t.Fatalf("evicted: %+v, want ErrNotInZone", r)
	}
	if _, ok := ze.departed[3]; !ok {
		t.Fatal("the evicted player's save is gone before Leave")
	}
}

// ______________________TEST SHOP_____________________

// shopZone is gearZone in an area with the Tạp hóa shop NPC 3 tiles east of
// the player at (5.5, 5.5) and a blacksmith 4 tiles south, over data.
func shopZone(t *testing.T, data *gamedata.Items, inv *items.Inventory) (*Zone, *player, *fakeSaver) {
	t.Helper()
	sv := &fakeSaver{}
	npcs := map[string]mapdata.NPC{
		"shop":  {ID: "shop", Role: mapdata.RoleShop, Shop: "general", Pos: mapdata.Point{X: 8.5, Y: 5.5}},
		"smith": {ID: "smith", Role: mapdata.RoleBlacksmith, Pos: mapdata.Point{X: 5.5, Y: 9.5}},
	}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Items: data, NPCs: npcs}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.Points = pb.CharClass_CHAR_CLASS_VIEM, 10, combat.Points{Str: 60, Vit: 30}
	in.Xu, in.Inventory = 77, inv
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	out.take()
	return z, z.players[1], sv
}

func shopInventory() *items.Inventory {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Bag[0] = &items.Item{Code: "potion_hp_1", Count: 5}
	inv.Bag[1] = &items.Item{Code: "eq50_armor", Count: 1, Quality: 2, Enhance: 7}
	inv.Bag[2] = &items.Item{Code: "stone_enhance", Count: 7}
	inv.Bag[3] = &items.Item{Code: "stone_refine", Count: 2}
	inv.Bag[4] = &items.Item{Code: "charm_protect", Count: 3}
	inv.Worn[0] = &items.Item{Code: "eq10_weapon_viem", Count: 1}
	return inv
}

func shopErr(t *testing.T, z *Zone, p *player, queued bool) pb.ErrorCode {
	t.Helper()
	out := p.out.(*fakeOut)
	out.take()
	itemNow(t, z, queued)
	for _, e := range out.take() {
		if e.Seq == 9 && e.GetError() != nil {
			return e.GetError().Code
		}
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

func TestShopErrors(t *testing.T) {
	const (
		dead   = pb.ErrorCode_ERROR_CODE_DEAD
		bad    = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		far    = pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
		xuErr  = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU
		full   = pb.ErrorCode_ERROR_CODE_BAG_FULL
		notFnd = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
		nosell = pb.ErrorCode_ERROR_CODE_NOT_SELLABLE
		ok     = pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	)
	buy := func(npc, code string, n int) func(*Zone) bool {
		return func(z *Zone) bool { return z.Buy(1, 9, npc, "", code, n) }
	}
	sell := func(npc string, slot, n int) func(*Zone) bool {
		return func(z *Zone) bool { return z.Sell(1, 9, npc, slot, n) }
	}
	for _, c := range []struct {
		name  string
		dead  bool
		x     float32 // player x, 0 for 5.5
		fullB bool
		xu    int64
		cmd   func(*Zone) bool
		want  pb.ErrorCode
	}{
		{"buy dead, no npc", true, 0, false, 77, buy("none", "potion_hp_1", 1), dead},
		{"buy no npc", false, 0, false, 77, buy("none", "potion_hp_1", 1), bad},
		{"buy blacksmith", false, 0, false, 77, buy("smith", "potion_hp_1", 1), bad},
		{"buy too far", false, 4.9, false, 77, buy("shop", "potion_hp_1", 1), far},
		{"buy 3.4 away", false, 5.1, false, 77, buy("shop", "potion_hp_1", 1), ok},
		{"buy 3.6 away", false, 4.9, false, 77, buy("shop", "potion_hp_1", 1), far},
		{"buy unknown code", false, 0, false, 77, buy("shop", "stone_enhance", 1), bad},
		{"buy count 0", false, 0, false, 77, buy("shop", "potion_hp_1", 0), bad},
		{"buy count 1000", false, 0, false, 77, buy("shop", "potion_hp_1", 1000), bad},
		{"buy count huge", false, 0, false, 77, buy("shop", "potion_hp_1", 1<<30), bad},
		{"buy short of xu", false, 0, false, 4, buy("shop", "potion_hp_1", 1), xuErr},
		{"buy short of xu and full bag", false, 0, true, 4, buy("shop", "potion_hp_30", 1), xuErr},
		{"buy full bag", false, 0, true, 1000, buy("shop", "potion_hp_30", 1), full},
		{"sell dead, no npc", true, 0, false, 77, sell("none", 60, 0), dead},
		{"sell blacksmith", false, 0, false, 77, sell("smith", 0, 0), bad},
		{"sell too far, bad slot", false, 4.9, false, 77, sell("shop", 60, 0), far},
		{"sell slot -1", false, 0, false, 77, sell("shop", -1, 0), bad},
		{"sell slot 60", false, 0, false, 77, sell("shop", 60, 0), bad},
		{"sell empty slot, bad count", false, 0, false, 77, sell("shop", 9, -1), notFnd},
		{"sell count -1", false, 0, false, 77, sell("shop", 0, -1), bad},
		{"sell more than there is", false, 0, false, 77, sell("shop", 0, 6), bad},
		{"sell ok", false, 0, false, 77, sell("shop", 0, 5), ok},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := shopZone(t, testItemData, shopInventory())
			if c.fullB {
				fillBag(p.inv)
			}
			if c.x != 0 {
				p.pos.X = c.x
			}
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			p.xu = c.xu
			before, saves := p.inv.Clone(), len(sv.all())
			if got := shopErr(t, z, p, c.cmd(z)); got != c.want {
				t.Fatalf("code %v, want %v", got, c.want)
			}
			if c.want != ok {
				if p.xu != c.xu || !reflect.DeepEqual(p.inv, before) || len(sv.all()) != saves {
					t.Fatalf("a rejected request changed xu %d, inventory or saves", p.xu)
				}
			}
		})
	}
}

func TestBuyMergesStackAndSaves(t *testing.T) {
	z, p, sv := shopZone(t, testItemData, shopInventory())
	z.advance(time.Now())
	p.xu = 100
	out := p.out.(*fakeOut)
	out.take()
	saves := len(sv.all())
	itemNow(t, z, z.Buy(1, 9, "shop", "", "potion_hp_1", 10))
	if p.xu != 50 || p.inv.Bag[0].Count != 15 {
		t.Fatalf("xu %d, stack %d, want 50 and 15", p.xu, p.inv.Bag[0].Count)
	}
	if len(sv.all()) != saves+1 {
		t.Fatalf("saves %d, want %d", len(sv.all()), saves+1)
	}
	r := respFor(out.take(), 9, (*pb.Envelope).GetBuyResp)
	if r == nil || r.Stats.Xu != 50 || len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != 0 {
		t.Fatalf("reply %v", r)
	}
}

func TestSellExamples(t *testing.T) {
	for _, c := range []struct {
		name        string
		slot, count int
		xu          int64
		left        int // count left in the slot, 0 for empty
	}{
		{"Vàng armor tier 50", 1, 0, 1077, 0},
		{"7 enhance stones", 2, 0, 77 + 35, 0},
		{"refine stone", 3, 1, 77 + 50, 1},
		{"part of a stack", 0, 2, 77 + 2, 3},
		{"count 0 empties the slot", 0, 0, 77 + 5, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := shopZone(t, testItemData, shopInventory())
			z.advance(time.Now())
			out := p.out.(*fakeOut)
			out.take()
			saves := len(sv.all())
			itemNow(t, z, z.Sell(1, 9, "shop", c.slot, c.count))
			left := 0
			if it := p.inv.Bag[c.slot]; it != nil {
				left = it.Count
			}
			if p.xu != c.xu || left != c.left || len(sv.all()) != saves+1 {
				t.Fatalf("xu %d left %d saves %d, want %d %d %d", p.xu, left, len(sv.all())-saves, c.xu, c.left, 1)
			}
			r := respFor(out.take(), 9, (*pb.Envelope).GetSellResp)
			if r == nil || r.Stats.Xu != c.xu || len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != int32(c.slot) {
				t.Fatalf("reply %v", r)
			}
		})
	}
}

func TestSellNotSellable(t *testing.T) {
	data := *testItemData
	data.Stackables = maps.Clone(testItemData.Stackables)
	s := data.Stackables["charm_protect"]
	s.Sell = 0
	data.Stackables["charm_protect"] = s
	z, p, sv := shopZone(t, &data, shopInventory())
	z.advance(time.Now())
	before, saves := p.inv.Clone(), len(sv.all())
	if got := shopErr(t, z, p, z.Sell(1, 9, "shop", 4, 0)); got != pb.ErrorCode_ERROR_CODE_NOT_SELLABLE {
		t.Fatalf("code %v, want NOT_SELLABLE", got)
	}
	if p.xu != 77 || !reflect.DeepEqual(p.inv, before) || len(sv.all()) != saves {
		t.Fatal("a refused sale changed something")
	}
}

func TestPendingRefineSurvivesBuyNotSell(t *testing.T) {
	z, p, _ := shopZone(t, testItemData, shopInventory())
	z.advance(time.Now())
	p.refine = &pendingRefine{}
	itemNow(t, z, z.Buy(1, 9, "shop", "", "potion_hp_1", 1))
	if p.refine == nil {
		t.Fatal("a buy dropped the refine result")
	}
	p.pos.X = 4.9 // 3.6 tiles from the shop: refused, the result stays
	if got := shopErr(t, z, p, z.Sell(1, 9, "shop", 0, 1)); got != pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR || p.refine == nil {
		t.Fatalf("refused sale: code %v, refine kept %v", got, p.refine != nil)
	}
	p.pos.X = 5.5
	itemNow(t, z, z.Sell(1, 10, "shop", 0, 1))
	if p.refine != nil {
		t.Fatal("a sale kept the refine result")
	}
}

func TestNearRole(t *testing.T) {
	z, p, _ := shopZone(t, testItemData, shopInventory())
	if !z.nearRole(p, mapdata.RoleShop) || z.nearRole(p, mapdata.RoleBlacksmith) || z.nearRole(p, mapdata.RoleCarriage) {
		t.Fatal("want near the shop (3 tiles) only, not the smith (4) or a carriage (none)")
	}
}

// ______________________TEST POTIONS____________________

func potionInventory() *items.Inventory {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Bag[0] = &items.Item{Code: "potion_hp_1", Count: 5}
	inv.Bag[1] = &items.Item{Code: "potion_hp_10", Count: 2}
	inv.Bag[2] = &items.Item{Code: "potion_mp_10", Count: 3}
	inv.Bag[3] = &items.Item{Code: "scroll_return", Count: 1}
	inv.Bag[4] = &items.Item{Code: "stone_enhance", Count: 1}
	return inv
}

// potionZone is shopZone with a player at 500/1640 HP and 0 MP.
func potionZone(t *testing.T) (*Zone, *player, *fakeSaver) {
	t.Helper()
	z, p, sv := shopZone(t, testItemData, potionInventory())
	z.advance(time.Now())
	p.maxHP, p.hp, p.mp = 1640, 500, 0
	p.out.(*fakeOut).take()
	return z, p, sv
}

func TestUseItemErrorsInOrder(t *testing.T) {
	const (
		dead = pb.ErrorCode_ERROR_CODE_DEAD
		bad  = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		nf   = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
		lvl  = pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW
		cd   = pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
		full = pb.ErrorCode_ERROR_CODE_ALREADY_FULL
		ok   = pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	)
	for _, c := range []struct {
		name  string
		slot  int
		setup func(z *Zone, p *player)
		want  pb.ErrorCode
	}{
		{"dead before a bad slot", 60, func(z *Zone, p *player) { z.damagePlayer(p, killer{}, 1<<30, false) }, dead},
		{"slot -1", -1, nil, bad},
		{"slot 60", 60, nil, bad},
		{"empty slot", 9, nil, nf},
		{"not a potion", 4, nil, bad},
		{"return scroll", 3, nil, bad},
		{"level 9 for potion_hp_10", 1, func(z *Zone, p *player) { p.level = 9 }, lvl},
		{"cooldown before full", 0, func(z *Zone, p *player) { p.hp = p.maxHP; p.useReady = map[string]uint32{"hp": z.tickNo + 5} }, cd},
		{"level before cooldown", 1, func(z *Zone, p *player) { p.level = 9; p.useReady = map[string]uint32{"hp": z.tickNo + 5} }, lvl},
		{"hp full", 0, func(z *Zone, p *player) { p.hp = p.maxHP }, full},
		{"mp full", 2, func(z *Zone, p *player) { p.mp = p.stats.MaxMP }, full},
		{"ok", 0, nil, ok},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := potionZone(t)
			if c.setup != nil {
				c.setup(z, p)
			}
			before, saves := p.inv.Clone(), len(sv.all())
			if got := shopErr(t, z, p, z.UseItem(1, 9, c.slot)); got != c.want {
				t.Fatalf("code %v, want %v", got, c.want)
			}
			if c.want != ok && !reflect.DeepEqual(p.inv, before) {
				t.Fatal("a refused use cost an item")
			}
			if len(sv.all()) != saves {
				t.Fatal("using a potion saved")
			}
		})
	}
}

func TestUseItemHeals(t *testing.T) {
	for _, c := range []struct {
		name         string
		hp           int
		slot         int
		wantHP, want int
	}{
		{"500 of 1640", 500, 0, 664, 664},
		{"capped", 1600, 0, 1640, 1640},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, _ := potionZone(t)
			p.hp = c.hp
			out := p.out.(*fakeOut)
			itemNow(t, z, z.UseItem(1, 9, c.slot))
			if p.hp != c.wantHP || p.inv.Bag[0].Count != 4 {
				t.Fatalf("hp %d, count %d", p.hp, p.inv.Bag[0].Count)
			}
			r := respFor(out.take(), 9, (*pb.Envelope).GetUseItemResp)
			if r == nil || r.CooldownMs != 10000 || r.Stats.Hp != int32(c.wantHP) || len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != 0 {
				t.Fatalf("reply %v", r)
			}
		})
	}
}

func TestUseItemMPCapped(t *testing.T) {
	z, p, _ := potionZone(t)
	max := p.stats.MaxMP
	p.mp = max - 30
	itemNow(t, z, z.UseItem(1, 9, 2))
	if p.mp != max || p.inv.Bag[2].Count != 2 {
		t.Fatalf("mp %d of %d, count %d", p.mp, max, p.inv.Bag[2].Count)
	}
	p.mp, p.useReady = 0, map[string]uint32{}
	itemNow(t, z, z.UseItem(1, 10, 2))
	if want := min(max, testItemData.Stackables["potion_mp_10"].Use.Value); p.mp != want {
		t.Fatalf("mp %d, want %d", p.mp, want)
	}
}

func TestUseItemSharedHPCooldown(t *testing.T) {
	z, p, _ := potionZone(t)
	itemNow(t, z, z.UseItem(1, 9, 0))
	if got := shopErr(t, z, p, z.UseItem(1, 9, 1)); got != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("hp_10 right after hp_1: %v", got)
	}
	if got := shopErr(t, z, p, z.UseItem(1, 9, 2)); got != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("mp potion: %v", got)
	}
	ready := p.useReady["hp"]
	for z.tickNo+1 < ready-1 { // the probe's own tick runs the command at tickNo+1
		z.advance(time.Now())
	}
	p.hp = 500
	if got := shopErr(t, z, p, z.UseItem(1, 9, 1)); got != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("one tick early: %v", got)
	}
	if got := shopErr(t, z, p, z.UseItem(1, 9, 1)); got != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("after the cooldown: %v", got)
	}
}

func TestUseItemViewerSeesHPAndNoSave(t *testing.T) {
	z, p, sv := potionZone(t)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	v.take()
	p.out.(*fakeOut).take()
	saves := len(sv.all())
	itemNow(t, z, z.UseItem(1, 9, 0))
	if cs := hpChangesIn(v.take()); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != 664 {
		t.Fatalf("viewer HpChanges %v", cs)
	}
	if len(sv.all()) != saves {
		t.Fatal("a potion queued a save")
	}
	z.saver.Save(z.state(p), nil)
	st := sv.all()[len(sv.all())-1]
	got := 0
	for _, r := range st.Items {
		if !r.Equipped && r.Slot == 0 {
			got = r.Count
		}
	}
	if got != 4 {
		t.Fatalf("a later save holds %d, want 4", got)
	}
}

func TestPotionCooldownCrossesZones(t *testing.T) {
	za, p, _ := potionZone(t)
	itemNow(t, za, za.UseItem(1, 9, 0))
	for range za.ticks(4) {
		za.advance(time.Now())
	}
	r := departNow(t, za, 1, Departure{To: Place{MapID: "m1"}, KeepPos: true, Check: notDead})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("depart %+v", r)
	}
	left := r.info.Cooldowns["potion_hp"]
	if left < 5*time.Second || left > 6*time.Second+200*time.Millisecond {
		t.Fatalf("handed over %v, want about 6s", left)
	}
	if _, ok := r.info.Cooldowns["potion_mp"]; ok {
		t.Fatal("an unused group was handed over")
	}
	_ = p
	zb, _, _ := shopZone(t, testItemData, potionInventory())
	zb.players = map[uint64]*player{}
	out := &fakeOut{}
	ew := joinByInbox(t, zb, r.info, out)
	found := false
	for _, c := range ew.Cooldowns {
		if c.Key == "potion_hp" && c.RemainingMs > 5000 && c.RemainingMs <= 6200 {
			found = true
		}
	}
	if !found {
		t.Fatalf("EnterWorldResp cooldowns %v", ew.Cooldowns)
	}
	zb.players[1].hp = 100
	if got := shopErr(t, zb, zb.players[1], zb.UseItem(1, 9, 0)); got != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("new zone: %v, want ON_COOLDOWN", got)
	}
}

func TestUseItemKeepsPendingRefine(t *testing.T) {
	z, p, _ := potionZone(t)
	p.refine = &pendingRefine{}
	itemNow(t, z, z.UseItem(1, 9, 0))
	if p.refine == nil {
		t.Fatal("drinking a potion dropped the refine result")
	}
}

// ______________________TEST FORGE AT THE BLACKSMITH_____________________

// The five forge commands need a blacksmith NPC in reach: 3.4 tiles away is
// in, 3.6 is out, and a map without a blacksmith (or with only a shop) is out
// too, with NPC_TOO_FAR.
func TestForgeBlacksmithRange(t *testing.T) {
	sends := map[string]func(z *Zone) bool{
		"enhance":  func(z *Zone) bool { return z.Enhance(1, 9, bagRef(60), false, 1) },
		"punch":    func(z *Zone) bool { return z.Punch(1, 9, bagRef(60)) },
		"socket":   func(z *Zone) bool { return z.Socket(1, 9, bagRef(60), 9, 60) },
		"unsocket": func(z *Zone) bool { return z.Unsocket(1, 9, bagRef(60), 9) },
		"refine":   func(z *Zone) bool { return z.Refine(1, 9, bagRef(60), nil) },
	}
	for name, send := range sends {
		for _, c := range []struct {
			name string
			npcs map[string]mapdata.NPC
			want pb.ErrorCode
		}{
			{"3.4 tiles", map[string]mapdata.NPC{"s": {ID: "s", Role: mapdata.RoleBlacksmith, Pos: mapdata.Point{X: 8.9, Y: 5.5}}}, codeBad},
			{"3.6 tiles", map[string]mapdata.NPC{"s": {ID: "s", Role: mapdata.RoleBlacksmith, Pos: mapdata.Point{X: 9.1, Y: 5.5}}}, codeTooFar},
			{"no blacksmith", map[string]mapdata.NPC{"s": {ID: "s", Role: mapdata.RoleShop, Shop: "general", Pos: mapdata.Point{X: 6.5, Y: 5.5}}}, codeTooFar},
			{"no NPCs", nil, codeTooFar},
		} {
			t.Run(name+" "+c.name, func(t *testing.T) {
				z, p, _ := forgeZone(t, socketInventory(), false)
				z.area.NPCs = c.npcs
				out := p.out.(*fakeOut)
				out.take()
				itemNow(t, z, send(z))
				if er := errorFor(out.take(), 9); er == nil || er.Code != c.want {
					t.Fatalf("error %v, want %v", er, c.want)
				}
			})
		}
	}
}

// ______________________TEST SCROLL AND CARRIAGE_____________________

// The departure costs apply only once the check has passed: the handoff and
// the departure save both show them. A check that asks for costs and then
// refuses changes nothing.
func TestDepartCosts(t *testing.T) {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Bag[3] = &items.Item{Code: "scroll_return", Count: 3}
	z, p, _, sv := gearZone(t, inv)
	var seen *items.Item
	refused := departNow(t, z, 1, Departure{To: Place{MapID: "m2"}, Check: func(d *Departing) pb.ErrorCode {
		if it, ok := d.ItemAt(3); ok {
			seen = it
		}
		if _, ok := d.ItemAt(len(p.inv.Bag)); ok {
			t.Error("ItemAt past the bag reported ok")
		}
		if it, ok := d.ItemAt(0); !ok || it != nil {
			t.Errorf("ItemAt of an empty slot = %v %v, want nil true", it, ok)
		}
		if d.Level() != 10 || d.Xu() != 77 {
			t.Errorf("level %d xu %d, want 10 77", d.Level(), d.Xu())
		}
		d.TakeXu(70)
		d.TakeItem(3, 1)
		return pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU
	}})
	if refused.code != pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU || refused.saved != nil {
		t.Fatalf("result %+v, want a NOT_ENOUGH_XU refusal", refused)
	}
	if seen == nil || seen.Code != "scroll_return" || seen.Count != 3 {
		t.Fatalf("ItemAt(3) = %v, want the scroll stack", seen)
	}
	if p.xu != 77 || p.inv.Bag[3].Count != 3 || z.players[1] != p || len(sv.all()) != 0 {
		t.Fatalf("xu %d scrolls %d saves %d after a refusal, want 77 3 0", p.xu, p.inv.Bag[3].Count, len(sv.all()))
	}

	r := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{2.5, 2.5}}, Check: func(d *Departing) pb.ErrorCode {
		d.TakeXu(70)
		d.TakeItem(3, 1)
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || r.err != nil || r.saved == nil {
		t.Fatalf("result %+v, want a departure", r)
	}
	if r.info.Xu != 7 || r.info.Inventory.Bag[3].Count != 2 {
		t.Fatalf("handoff xu %d scrolls %d, want 7 2", r.info.Xu, r.info.Inventory.Bag[3].Count)
	}
	saves := sv.all()
	st := saves[len(saves)-1]
	var scrolls int
	for _, it := range st.Items {
		if it.Code == "scroll_return" {
			scrolls = it.Count
		}
	}
	if st.MapID != "m2" || st.Xu != 7 || scrolls != 2 {
		t.Fatalf("departure save %+v, want m2 with 7 Xu and 2 scrolls", st)
	}
}

// The last scroll of a stack leaves the slot empty in the handoff.
func TestDepartTakesLastItem(t *testing.T) {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Bag[0] = &items.Item{Code: "scroll_return", Count: 1}
	z, _, _, _ := gearZone(t, inv)
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m2"}, Check: func(d *Departing) pb.ErrorCode {
		d.TakeItem(0, 1)
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}})
	if r.code != 0 || r.err != nil || r.info.Inventory.Bag[0] != nil {
		t.Fatalf("result %+v, want a departure with slot 0 empty", r)
	}
}

// NearNPC is the zone's NPC reach check on the live player.
func TestDepartNearNPC(t *testing.T) {
	z, _, _, _ := gearZone(t, items.NewInventory(testItemData.BagSize))
	z.area.NPCs = map[string]mapdata.NPC{
		"xe":  {ID: "xe", Role: mapdata.RoleCarriage, Pos: mapdata.Point{X: 8.5, Y: 5.5}},
		"xa":  {ID: "xa", Role: mapdata.RoleCarriage, Pos: mapdata.Point{X: 20.5, Y: 5.5}},
		"ren": {ID: "ren", Role: mapdata.RoleBlacksmith, Pos: mapdata.Point{X: 5.5, Y: 7.5}},
	}
	var got []pb.ErrorCode
	departNow(t, z, 1, Departure{Check: func(d *Departing) pb.ErrorCode {
		got = []pb.ErrorCode{d.NearNPC("xe", mapdata.RoleCarriage), d.NearNPC("xa", mapdata.RoleCarriage),
			d.NearNPC("ren", mapdata.RoleCarriage), d.NearNPC("nope", mapdata.RoleCarriage)}
		return pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	}})
	want := []pb.ErrorCode{0, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, pb.ErrorCode_ERROR_CODE_BAD_REQUEST}
	if !slices.Equal(got, want) {
		t.Fatalf("NearNPC %v, want %v", got, want)
	}
}

// ______________________TEST ACCOUNT ZONE_____________________

// accountStash is a 30-slot stash with a Xanh boots piece with two holes in
// slot 2 and 500 stones in slot 7.
func accountStash() []*items.Item {
	s := make([]*items.Item, 30)
	s[2] = &items.Item{Code: "eq10_boots", Count: 1, Quality: 1, Lines: []items.Line{{Stat: "hp", Value: 40}},
		Gems: []string{"", "gem_hp_1"}}
	s[7] = &items.Item{Code: "stone_enhance", Count: 500}
	return s
}

// wantAccount is the account part accountZone's player saves.
func wantAccount() *store.AccountState {
	return &store.AccountState{ID: 9, Luong: 3, StashSize: 30, Stash: []store.ItemRow{
		{Slot: 2, Code: "eq10_boots", Count: 1, Quality: 1, Lines: []store.ItemLine{{Stat: "hp", Value: 40}},
			Hidden: []store.ItemLine{}, Gems: []string{"", "gem_hp_1"}},
		{Slot: 7, Code: "stone_enhance", Count: 500, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
	}}
}

// accountInfo is gearZone's level-10 Viêm as player 1 of account 9 with
// 3 Lượng and accountStash.
func accountInfo() PlayerInfo {
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.Points = pb.CharClass_CHAR_CLASS_VIEM, 10, combat.Points{Str: 60, Vit: 30}
	in.Xu, in.Inventory = 77, gearInventory()
	in.AccountID, in.Luong, in.Stash = 9, 3, accountStash()
	return in
}

func itemZone() (*Zone, *fakeSaver) {
	sv := &fakeSaver{}
	return New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Items: testItemData}, sv, quietLog()), sv
}

// accountZone joins accountInfo into a zone with the real item data.
func accountZone(t *testing.T) (*Zone, *player, *pb.EnterWorldResp, *fakeSaver) {
	t.Helper()
	z, sv := itemZone()
	out := &fakeOut{}
	if err := z.join(accountInfo(), out); err != nil {
		t.Fatal(err)
	}
	return z, z.players[1], out.take()[0].GetEnterWorldResp(), sv
}

// Join copies the account part: Lượng shows in the stats, and the player's
// stash shares nothing with PlayerInfo.Stash.
func TestJoinCarriesAccount(t *testing.T) {
	z, _ := itemZone()
	in := accountInfo()
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	if r := out.take()[0].GetEnterWorldResp(); r.Stats.GetLuong() != 3 {
		t.Fatalf("luong %d, want 3", r.Stats.GetLuong())
	}
	p := z.players[1]
	if p.accountID != 9 || p.luong != 3 || !reflect.DeepEqual(p.stash, accountStash()) {
		t.Fatalf("account %d luong %d stash %v", p.accountID, p.luong, p.stash)
	}
	in.Stash[2].Lines[0].Value, in.Stash[2].Gems[1], in.Stash[7] = 1, "", nil
	if !reflect.DeepEqual(p.stash, accountStash()) {
		t.Fatal("the player shares its stash with PlayerInfo.Stash")
	}
}

// Every save carries the whole account part, not only the ones that changed
// it: the saver keeps only the newest state of a character, so a save
// without it could hide an earlier stash change.
func TestEverySaveCarriesAccount(t *testing.T) {
	z, _, _, sv := accountZone(t)
	t0 := time.Now()
	z.advance(t0)
	z.advance(t0.Add(61 * time.Second))  // periodic
	itemNow(t, z, z.SortBag(1, 9))       // an item command
	if err := <-z.leave(1); err != nil { // leave
		t.Fatal(err)
	}
	saves := sv.all()
	if len(saves) != 3 {
		t.Fatalf("%d saves, want periodic, sort and leave", len(saves))
	}
	for i, st := range saves {
		if !reflect.DeepEqual(st.Account, wantAccount()) {
			t.Fatalf("save %d account %+v, want %+v", i, st.Account, wantAccount())
		}
	}
	// Changing Lượng shows in the next save.
	z2, sv2 := itemZone()
	if err := z2.join(accountInfo(), &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	z2.players[1].luong = 0
	<-z2.leave(1)
	if a := sv2.all()[0].Account; a == nil || a.Luong != 0 || a.StashSize != 30 {
		t.Fatalf("account %+v, want Lượng 0", a)
	}
}

// The saved account part shares nothing with the player, both ways.
func TestStateAccountIsDeepCopy(t *testing.T) {
	z, p, _, _ := accountZone(t)
	st := z.state(p)
	a := st.Account
	a.Stash[0].Lines[0].Value, a.Stash[0].Gems[1], a.Stash[1].Count = 1, "", 1
	if !reflect.DeepEqual(p.stash, accountStash()) {
		t.Fatal("mutating the saved stash changed the player's")
	}
	st = z.state(p)
	p.stash[2].Lines[0].Value, p.stash[2].Gems[0], p.stash[7].Count, p.stash[0] = 9, "gem_hp_1", 1, &items.Item{Code: "stone_enhance", Count: 1}
	if !reflect.DeepEqual(st.Account, wantAccount()) {
		t.Fatal("mutating the player's stash changed the saved one")
	}
}

// A departure saves the account part and hands it over as a deep copy; the
// next zone keeps Lượng and the stash, shows Lượng and saves them.
func TestDepartCarriesAccount(t *testing.T) {
	z, p, _, sv := accountZone(t)
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{20.5, 21.5}}, Check: notDead})
	if r.code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || r.err != nil || r.saved == nil {
		t.Fatalf("result %+v, want a departure", r)
	}
	<-r.saved
	if saves := sv.all(); len(saves) != 1 || !reflect.DeepEqual(saves[0].Account, wantAccount()) {
		t.Fatalf("saves %+v, want one with the account part", saves)
	}
	in := r.info
	if in.AccountID != 9 || in.Luong != 3 || !reflect.DeepEqual(in.Stash, accountStash()) {
		t.Fatalf("handoff account %d luong %d stash %v", in.AccountID, in.Luong, in.Stash)
	}
	in.Stash[2].Lines[0].Value, in.Stash[2].Gems[1] = 1, ""
	if !reflect.DeepEqual(p.stash, accountStash()) {
		t.Fatal("the handoff shares its stash with the departed player")
	}
	in.Stash = accountStash()

	z2, sv2 := itemZone()
	resp := joinByInbox(t, z2, in, &fakeOut{})
	if resp.Stats.GetLuong() != 3 {
		t.Fatalf("luong %d after the transfer, want 3", resp.Stats.GetLuong())
	}
	<-z2.leave(1)
	if saves := sv2.all(); len(saves) != 1 || !reflect.DeepEqual(saves[0].Account, wantAccount()) {
		t.Fatalf("saves in the next zone %+v, want the account part", saves)
	}
}

// selfStats sends Lượng on every update, not only at the join.
func TestSelfStatsLuong(t *testing.T) {
	z, p, _, _ := accountZone(t)
	p.luong = 42
	if s := z.selfStats(p); s.GetLuong() != 42 {
		t.Fatalf("luong %d, want 42", s.GetLuong())
	}
}

// ______________________TEST STASH____________________

// stashZone is shopZone with a storage keeper 3 tiles east at "kho", a stash
// of size slots holding stash, and Xu xu.
func stashZone(t *testing.T, size int, stash map[int]*items.Item, bag *items.Inventory, xu int64) (*Zone, *player, *fakeSaver) {
	t.Helper()
	sv := &fakeSaver{}
	npcs := map[string]mapdata.NPC{
		"kho":  {ID: "kho", Role: mapdata.RoleStorage, Pos: mapdata.Point{X: 8.5, Y: 5.5}},
		"shop": {ID: "shop", Role: mapdata.RoleShop, Shop: "general", Pos: mapdata.Point{X: 5.5, Y: 6.5}},
	}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Items: testItemData, NPCs: npcs}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level = pb.CharClass_CHAR_CLASS_VIEM, 10
	in.Xu, in.Inventory, in.AccountID = xu, bag, 7
	in.Stash = make([]*items.Item, size)
	for i, it := range stash {
		in.Stash[i] = it
	}
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	out.take()
	return z, z.players[1], sv
}

func stashBag() *items.Inventory {
	inv := items.NewInventory(testItemData.BagSize)
	inv.Bag[0] = &items.Item{Code: "stone_enhance", Count: 600}
	inv.Bag[1] = &items.Item{Code: "eq50_armor", Count: 1, Quality: 2, Enhance: 7}
	inv.Worn[0] = &items.Item{Code: "eq10_weapon_viem", Count: 1}
	return inv
}

func TestStashErrorsInOrder(t *testing.T) {
	const (
		dead   = pb.ErrorCode_ERROR_CODE_DEAD
		bad    = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		far    = pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
		xuErr  = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU
		bagF   = pb.ErrorCode_ERROR_CODE_BAG_FULL
		stashF = pb.ErrorCode_ERROR_CODE_STASH_FULL
		maxed  = pb.ErrorCode_ERROR_CODE_STASH_MAXED
		notFnd = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
		ok     = pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	)
	open := func(npc string) func(*Zone) bool { return func(z *Zone) bool { return z.StashOpen(1, 9, npc) } }
	dep := func(npc string, slot int) func(*Zone) bool {
		return func(z *Zone) bool { return z.StashDeposit(1, 9, npc, slot) }
	}
	wd := func(npc string, slot int) func(*Zone) bool {
		return func(z *Zone) bool { return z.StashWithdraw(1, 9, npc, slot) }
	}
	exp := func(npc string) func(*Zone) bool { return func(z *Zone) bool { return z.StashExpand(1, 9, npc) } }
	srt := func(npc string) func(*Zone) bool { return func(z *Zone) bool { return z.StashSort(1, 9, npc) } }
	full := func(n int) map[int]*items.Item { // n equipment pieces in slots 0..n-1
		m := map[int]*items.Item{}
		for i := 0; i < n; i++ {
			m[i] = &items.Item{Code: "eq10_helmet", Count: 1}
		}
		return m
	}
	for _, c := range []struct {
		name  string
		dead  bool
		x     float32 // player x, 0 for 5.5
		size  int
		stash map[int]*items.Item
		fullB bool
		xu    int64
		cmd   func(*Zone) bool
		want  pb.ErrorCode
	}{
		{"open dead, no npc", true, 0, 30, nil, false, 0, open("none"), dead},
		{"open no npc", false, 0, 30, nil, false, 0, open("none"), bad},
		{"open shop npc", false, 0, 30, nil, false, 0, open("shop"), bad},
		{"open too far", false, 4.9, 30, nil, false, 0, open("kho"), far},
		{"open ok", false, 0, 30, nil, false, 0, open("kho"), ok},
		{"deposit dead", true, 0, 30, nil, false, 0, dep("none", 99), dead},
		{"deposit shop npc, bad slot", false, 0, 30, nil, false, 0, dep("shop", 99), bad},
		{"deposit too far, bad slot", false, 4.9, 30, nil, false, 0, dep("kho", 99), far},
		{"deposit slot -1", false, 0, 30, nil, false, 0, dep("kho", -1), bad},
		{"deposit slot 60", false, 0, 30, nil, false, 0, dep("kho", 60), bad},
		{"deposit empty slot, full stash", false, 0, 3, full(3), false, 0, dep("kho", 9), notFnd},
		{"deposit full stash", false, 0, 3, full(3), false, 0, dep("kho", 1), stashF},
		{"deposit ok", false, 0, 30, nil, false, 0, dep("kho", 1), ok},
		{"withdraw dead", true, 0, 30, nil, false, 0, wd("none", 99), dead},
		{"withdraw no npc", false, 0, 30, nil, false, 0, wd("none", 0), bad},
		{"withdraw too far", false, 4.9, 30, nil, false, 0, wd("kho", 99), far},
		{"withdraw slot -1", false, 0, 30, nil, false, 0, wd("kho", -1), bad},
		{"withdraw slot 30", false, 0, 30, nil, false, 0, wd("kho", 30), bad},
		{"withdraw empty slot, full bag", false, 0, 30, nil, true, 0, wd("kho", 5), notFnd},
		{"withdraw full bag", false, 0, 30, full(1), true, 0, wd("kho", 0), bagF},
		{"withdraw ok", false, 0, 30, full(1), false, 0, wd("kho", 0), ok},
		{"expand dead", true, 0, 120, nil, false, 0, exp("none"), dead},
		{"expand shop npc", false, 0, 120, nil, false, 0, exp("shop"), bad},
		{"expand too far, maxed", false, 4.9, 120, nil, false, 0, exp("kho"), far},
		{"expand maxed, no xu", false, 0, 120, nil, false, 0, exp("kho"), maxed},
		{"expand short of xu", false, 0, 30, nil, false, 19999, exp("kho"), xuErr},
		{"expand ok", false, 0, 30, nil, false, 20000, exp("kho"), ok},
		{"sort dead", true, 0, 30, nil, false, 0, srt("none"), dead},
		{"sort shop npc", false, 0, 30, nil, false, 0, srt("shop"), bad},
		{"sort too far", false, 4.9, 30, nil, false, 0, srt("kho"), far},
		{"sort ok", false, 0, 30, nil, false, 0, srt("kho"), ok},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := stashZone(t, c.size, c.stash, stashBag(), c.xu)
			if c.fullB {
				fillBag(p.inv)
			}
			if c.x != 0 {
				p.pos.X = c.x
			}
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			beforeInv, beforeStash, saves := p.inv.Clone(), cloneSlots(p.stash), len(sv.all())
			if got := shopErr(t, z, p, c.cmd(z)); got != c.want {
				t.Fatalf("code %v, want %v", got, c.want)
			}
			if c.want != ok && (p.xu != c.xu || !reflect.DeepEqual(p.inv, beforeInv) ||
				!reflect.DeepEqual(p.stash, beforeStash) || len(sv.all()) != saves) {
				t.Fatal("a rejected request changed xu, the bag, the stash or the saves")
			}
		})
	}
}

func TestStashDepositMergesStackAndSaves(t *testing.T) {
	z, p, sv := stashZone(t, 30, map[int]*items.Item{0: {Code: "stone_enhance", Count: 500}}, stashBag(), 0)
	z.advance(time.Now())
	out := p.out.(*fakeOut)
	out.take()
	saves := len(sv.all())
	itemNow(t, z, z.StashDeposit(1, 9, "kho", 0))
	if p.stash[0].Count != 999 || p.stash[1] == nil || p.stash[1].Count != 101 || p.inv.Bag[0] != nil {
		t.Fatalf("stash %v %v, bag %v, want 999 + 101 and an empty bag slot", p.stash[0], p.stash[1], p.inv.Bag[0])
	}
	all := sv.all()
	if len(all) != saves+1 {
		t.Fatalf("saves %d, want %d", len(all), saves+1)
	}
	acc := all[len(all)-1].Account
	if acc == nil || acc.StashSize != 30 || acc.Stash[0].Count != 999 {
		t.Fatalf("saved account part %+v", acc)
	}
	r := respFor(out.take(), 9, (*pb.Envelope).GetStashDepositResp)
	if r == nil || len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != 0 || r.Update.Bag[0].Item != nil ||
		len(r.Stash) != 2 || r.Stash[0].Slot != 0 || r.Stash[1].Slot != 1 || r.Stash[1].Item.Count != 101 {
		t.Fatalf("reply %v", r)
	}
}

func TestStashDepositKeepsEquipmentDetail(t *testing.T) {
	z, p, _ := stashZone(t, 30, nil, stashBag(), 0)
	z.advance(time.Now())
	want := *p.inv.Bag[1]
	itemNow(t, z, z.StashDeposit(1, 9, "kho", 1))
	if p.inv.Bag[1] != nil || p.stash[0] == nil || !reflect.DeepEqual(*p.stash[0], want) {
		t.Fatalf("stash slot 0 %v, want %v", p.stash[0], want)
	}
}

func TestStashWithdrawAndSave(t *testing.T) {
	z, p, sv := stashZone(t, 30, map[int]*items.Item{4: {Code: "stone_enhance", Count: 100}}, stashBag(), 0)
	z.advance(time.Now())
	out := p.out.(*fakeOut)
	out.take()
	saves := len(sv.all())
	itemNow(t, z, z.StashWithdraw(1, 9, "kho", 4))
	if p.stash[4] != nil || p.inv.Bag[0].Count != 700 {
		t.Fatalf("stash %v, bag stack %v, want an empty slot and 700", p.stash[4], p.inv.Bag[0])
	}
	if len(sv.all()) != saves+1 {
		t.Fatalf("saves %d, want %d", len(sv.all()), saves+1)
	}
	r := respFor(out.take(), 9, (*pb.Envelope).GetStashWithdrawResp)
	if r == nil || len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != 0 || len(r.Stash) != 1 ||
		r.Stash[0].Slot != 4 || r.Stash[0].Item != nil {
		t.Fatalf("reply %v", r)
	}
}

func TestStashFullAndBagFullChangeNothing(t *testing.T) {
	// Two full 999 stacks in a 2-slot stash: 600 more stones have no room.
	z, p, _ := stashZone(t, 2, map[int]*items.Item{0: {Code: "stone_enhance", Count: 999}, 1: {Code: "stone_enhance", Count: 999}}, stashBag(), 0)
	z.advance(time.Now())
	before := cloneSlots(p.stash)
	got := shopErr(t, z, p, z.StashDeposit(1, 9, "kho", 0))
	if got != pb.ErrorCode_ERROR_CODE_STASH_FULL || !reflect.DeepEqual(p.stash, before) || p.inv.Bag[0].Count != 600 {
		t.Fatalf("code %v, stash %v, bag %v", got, p.stash, p.inv.Bag[0])
	}
}

func TestStashExpandCostsAndMax(t *testing.T) {
	z, p, sv := stashZone(t, 30, nil, stashBag(), 70000)
	z.advance(time.Now())
	out := p.out.(*fakeOut)
	out.take()
	saves := len(sv.all())
	itemNow(t, z, z.StashExpand(1, 9, "kho"))
	if len(p.stash) != 40 || p.xu != 50000 {
		t.Fatalf("size %d xu %d, want 40 and 50000", len(p.stash), p.xu)
	}
	r := respFor(out.take(), 9, (*pb.Envelope).GetStashExpandResp)
	if r == nil || r.Size != 40 || r.Stats.Xu != 50000 {
		t.Fatalf("reply %v", r)
	}
	itemNow(t, z, z.StashExpand(1, 10, "kho"))
	if len(p.stash) != 50 || p.xu != 10000 {
		t.Fatalf("size %d xu %d, want 50 and 10000 (40,000 for the second step)", len(p.stash), p.xu)
	}
	if got := len(sv.all()); got != saves+2 {
		t.Fatalf("saves %d, want %d", got, saves+2)
	}
	if st := sv.all()[len(sv.all())-1].Account; st == nil || st.StashSize != 50 {
		t.Fatalf("saved account part %+v", st)
	}
	p.xu = 1 << 40
	for len(p.stash) < 120 {
		itemNow(t, z, z.StashExpand(1, 11, "kho"))
	}
	if got := shopErr(t, z, p, z.StashExpand(1, 9, "kho")); got != pb.ErrorCode_ERROR_CODE_STASH_MAXED {
		t.Fatalf("at 120: %v, want STASH_MAXED", got)
	}
}

// A size kept on load that is off the step grid grows to the next grid size
// for that step's price; the last step is capped at max.
func TestStashExpandOffGridSize(t *testing.T) {
	z, p, _ := stashZone(t, 35, nil, stashBag(), 20000)
	z.advance(time.Now())
	itemNow(t, z, z.StashExpand(1, 9, "kho"))
	if len(p.stash) != 40 || p.xu != 0 {
		t.Fatalf("size %d xu %d, want 40 and 0", len(p.stash), p.xu)
	}
	z, p, _ = stashZone(t, 25, nil, stashBag(), 20000) // below base: first step
	z.advance(time.Now())
	itemNow(t, z, z.StashExpand(1, 9, "kho"))
	if len(p.stash) != 40 || p.xu != 0 {
		t.Fatalf("size %d xu %d, want 40 and 0", len(p.stash), p.xu)
	}
	z, p, _ = stashZone(t, 115, nil, stashBag(), 180000)
	z.advance(time.Now())
	itemNow(t, z, z.StashExpand(1, 9, "kho"))
	if len(p.stash) != 120 || p.xu != 0 {
		t.Fatalf("size %d xu %d, want 120 and 0 (step 9 = 180,000)", len(p.stash), p.xu)
	}
}

func TestStashSortAndOpen(t *testing.T) {
	z, p, sv := stashZone(t, 30, map[int]*items.Item{
		5: {Code: "stone_enhance", Count: 100}, 9: {Code: "stone_enhance", Count: 200}}, stashBag(), 0)
	z.advance(time.Now())
	out := p.out.(*fakeOut)
	out.take()
	itemNow(t, z, z.StashOpen(1, 8, "kho"))
	o := respFor(out.take(), 8, (*pb.Envelope).GetStashOpenResp)
	if o == nil || o.Stash.Size != 30 || len(o.Stash.Items) != 2 || o.Stash.Items[0].Slot != 5 || o.Stash.Items[1].Slot != 9 {
		t.Fatalf("open reply %v", o)
	}
	saves := len(sv.all())
	itemNow(t, z, z.StashSort(1, 9, "kho"))
	if p.stash[0] == nil || p.stash[0].Count != 300 || p.stash[1] != nil || p.stash[5] != nil {
		t.Fatalf("sorted stash %v %v", p.stash[0], p.stash[1])
	}
	if len(sv.all()) != saves+1 {
		t.Fatalf("saves %d, want %d", len(sv.all()), saves+1)
	}
	r := respFor(out.take(), 9, (*pb.Envelope).GetStashSortResp)
	if r == nil || len(r.Stash) != 30 || r.Stash[0].Item.Count != 300 || r.Stash[9].Item != nil {
		t.Fatalf("sort reply %v", r)
	}
}

func TestPendingRefineAndStash(t *testing.T) {
	stash := map[int]*items.Item{3: {Code: "stone_refine", Count: 1}}
	for name, c := range map[string]struct {
		cmd  func(*Zone) bool
		drop bool
	}{
		"open":     {func(z *Zone) bool { return z.StashOpen(1, 9, "kho") }, false},
		"expand":   {func(z *Zone) bool { return z.StashExpand(1, 9, "kho") }, false},
		"deposit":  {func(z *Zone) bool { return z.StashDeposit(1, 9, "kho", 0) }, true},
		"withdraw": {func(z *Zone) bool { return z.StashWithdraw(1, 9, "kho", 3) }, true},
		"sort":     {func(z *Zone) bool { return z.StashSort(1, 9, "kho") }, true},
	} {
		t.Run(name, func(t *testing.T) {
			z, p, _ := stashZone(t, 30, stash, stashBag(), 99999)
			z.advance(time.Now())
			p.refine = &pendingRefine{}
			itemNow(t, z, c.cmd(z))
			if (p.refine == nil) != c.drop {
				t.Fatalf("refine dropped %v, want %v", p.refine == nil, c.drop)
			}
		})
	}
}

// A refused stash command keeps the waiting refine result; only a successful
// deposit, withdraw or sort drops it (phase 4c spec 4.2, like a sale).
func TestPendingRefineSurvivesRefusedStash(t *testing.T) {
	two := map[int]*items.Item{0: {Code: "stone_enhance", Count: 999}, 1: {Code: "stone_enhance", Count: 999}}
	for name, c := range map[string]struct {
		size  int
		stash map[int]*items.Item
		fullB bool
		dead  bool
		x     float32
		cmd   func(*Zone) bool
		want  pb.ErrorCode
	}{
		"deposit stash full":  {2, two, false, false, 0, func(z *Zone) bool { return z.StashDeposit(1, 9, "kho", 0) }, pb.ErrorCode_ERROR_CODE_STASH_FULL},
		"deposit empty slot":  {30, nil, false, false, 0, func(z *Zone) bool { return z.StashDeposit(1, 9, "kho", 9) }, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		"deposit too far":     {30, nil, false, false, 4.9, func(z *Zone) bool { return z.StashDeposit(1, 9, "kho", 0) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		"deposit dead":        {30, nil, false, true, 0, func(z *Zone) bool { return z.StashDeposit(1, 9, "kho", 0) }, pb.ErrorCode_ERROR_CODE_DEAD},
		"withdraw bag full":   {30, map[int]*items.Item{3: {Code: "eq10_helmet", Count: 1}}, true, false, 0, func(z *Zone) bool { return z.StashWithdraw(1, 9, "kho", 3) }, pb.ErrorCode_ERROR_CODE_BAG_FULL},
		"withdraw empty slot": {30, nil, false, false, 0, func(z *Zone) bool { return z.StashWithdraw(1, 9, "kho", 3) }, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND},
		"withdraw too far":    {30, nil, false, false, 4.9, func(z *Zone) bool { return z.StashWithdraw(1, 9, "kho", 3) }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		"withdraw dead":       {30, nil, false, true, 0, func(z *Zone) bool { return z.StashWithdraw(1, 9, "kho", 3) }, pb.ErrorCode_ERROR_CODE_DEAD},
		"sort too far":        {30, nil, false, false, 4.9, func(z *Zone) bool { return z.StashSort(1, 9, "kho") }, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR},
		"sort dead":           {30, nil, false, true, 0, func(z *Zone) bool { return z.StashSort(1, 9, "kho") }, pb.ErrorCode_ERROR_CODE_DEAD},
	} {
		t.Run(name, func(t *testing.T) {
			z, p, _ := stashZone(t, c.size, c.stash, stashBag(), 0)
			if c.fullB {
				fillBag(p.inv)
			}
			if c.x != 0 {
				p.pos.X = c.x
			}
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			p.refine = &pendingRefine{}
			if got := shopErr(t, z, p, c.cmd(z)); got != c.want {
				t.Fatalf("code %v, want %v", got, c.want)
			}
			if p.refine == nil {
				t.Fatal("a refused stash command dropped the refine result")
			}
		})
	}
}

// ____TEST LUONG____

// luongCmd is a Lượng-shop style buy: no NPC, the shop named by id.
func luongBuy(shop, code string, n int) func(*Zone) bool {
	return func(z *Zone) bool { return z.Buy(1, 9, "", shop, code, n) }
}

func TestLuongShopBuy(t *testing.T) {
	z, p, sv := shopZone(t, testItemData, shopInventory())
	z.advance(time.Now())
	p.pos = Vec2{X: 40.5, Y: 40.5} // far from every NPC: the shop opens anywhere
	p.accountID, p.luong, p.xu = 9, 5, 77
	out := p.out.(*fakeOut)
	out.take()
	saves := len(sv.all())
	itemNow(t, z, luongBuy("luong", "charm_protect", 2)(z))
	if p.luong != 1 || p.xu != 77 || p.inv.Bag[4].Count != 5 {
		t.Fatalf("luong %d xu %d charms %d, want 1, 77 and 5", p.luong, p.xu, p.inv.Bag[4].Count)
	}
	if len(sv.all()) != saves+1 || sv.all()[saves].Account == nil || sv.all()[saves].Account.Luong != 1 {
		t.Fatalf("saves %d, want one more carrying Luong 1", len(sv.all())-saves)
	}
	r := respFor(out.take(), 9, (*pb.Envelope).GetBuyResp)
	if r == nil || r.Stats.GetLuong() != 1 || len(r.Update.Bag) != 1 {
		t.Fatalf("reply %v", r)
	}
}

func TestLuongShopRefusals(t *testing.T) {
	const (
		bad  = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		lErr = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_LUONG
		full = pb.ErrorCode_ERROR_CODE_BAG_FULL
		dead = pb.ErrorCode_ERROR_CODE_DEAD
	)
	for _, c := range []struct {
		name  string
		dead  bool
		fullB bool
		luong int64
		cmd   func(*Zone) bool
		want  pb.ErrorCode
	}{
		{"general without an npc", false, false, 9, luongBuy("general", "potion_hp_1", 1), bad},
		{"unknown shop", false, false, 9, luongBuy("nope", "charm_protect", 1), bad},
		{"empty shop id", false, false, 9, luongBuy("", "charm_protect", 1), bad},
		{"code not sold", false, false, 9, luongBuy("luong", "potion_hp_1", 1), bad},
		{"count 0", false, false, 9, luongBuy("luong", "charm_protect", 0), bad},
		{"count too big", false, false, 9, luongBuy("luong", "charm_protect", 1<<30), bad},
		{"short of luong", false, false, 1, luongBuy("luong", "charm_protect", 1), lErr},
		{"short of luong and full bag", false, true, 1, luongBuy("luong", "charm_protect", 1), lErr},
		{"full bag", false, true, 9, luongBuy("luong", "charm_protect", 1), full},
		{"dead", true, false, 9, luongBuy("luong", "charm_protect", 1), dead},
		{"npc shop ignores shop_id", false, false, 9, func(z *Zone) bool { return z.Buy(1, 9, "shop", "luong", "charm_protect", 1) }, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, sv := shopZone(t, testItemData, shopInventory())
			if c.fullB {
				fillBag(p.inv)
				p.inv.Bag[4] = &items.Item{Code: "charm_protect", Count: testItemData.StackMax}
			}
			if c.dead {
				z.damagePlayer(p, killer{}, 1<<30, false)
			}
			z.advance(time.Now())
			p.luong, p.xu = c.luong, 77
			before, saves := p.inv.Clone(), len(sv.all())
			if got := shopErr(t, z, p, c.cmd(z)); got != c.want {
				t.Fatalf("code %v, want %v", got, c.want)
			}
			if p.luong != c.luong || p.xu != 77 || !reflect.DeepEqual(p.inv, before) || len(sv.all()) != saves {
				t.Fatal("a refused buy changed Luong, Xu, the bag or the saves")
			}
		})
	}
}

func TestRespawnInPlace(t *testing.T) {
	z, p, sv := shopZone(t, testItemData, shopInventory())
	z.advance(time.Now())
	p.pos = Vec2{X: 20.5, Y: 21.5}
	p.accountID, p.luong = 9, 1
	z.damagePlayer(p, killer{}, 1<<30, false)
	if !p.dead {
		t.Fatal("not dead")
	}
	out := p.out.(*fakeOut)
	out.take()
	saves := len(sv.all())
	z.respawn(respawnCmd{id: 1, seq: 4, inPlace: true})
	if p.dead || p.pos != (Vec2{X: 20.5, Y: 21.5}) || p.hp != p.maxHP || p.mp != p.stats.MaxMP || p.luong != 0 {
		t.Fatalf("dead %v at %v hp %d mp %d luong %d, want alive in place, full, 0 Luong", p.dead, p.pos, p.hp, p.mp, p.luong)
	}
	if !slices.Contains(z.events.hpChanges, p.id) {
		t.Fatal("viewers are not told of the new HP")
	}
	if len(sv.all()) != saves+1 || sv.all()[saves].Account == nil || sv.all()[saves].Account.Luong != 0 {
		t.Fatalf("saves %d, want one more carrying Luong 0", len(sv.all())-saves)
	}
	envs := out.take()
	r, _ := respawnRespFor(envs, 4)
	if r == nil || r.X != 20.5 || r.Y != 21.5 {
		t.Fatalf("reply %v", r)
	}
	if ss := selfStatsIn(envs); len(ss) == 0 || ss[len(ss)-1].GetLuong() != 0 {
		t.Fatalf("stats %v, want a SelfStats with Luong 0", ss)
	}
}

func TestRespawnInPlaceRefused(t *testing.T) {
	z, p, sv := shopZone(t, testItemData, shopInventory())
	z.advance(time.Now())
	p.pos = Vec2{X: 20.5, Y: 21.5}
	out := p.out.(*fakeOut)
	// alive: NOT_DEAD before the Luong check, Luong kept
	p.luong = 3
	out.take()
	z.respawn(respawnCmd{id: 1, seq: 4, inPlace: true})
	if e := errorFor(out.take(), 4).GetCode(); e != pb.ErrorCode_ERROR_CODE_NOT_DEAD || p.luong != 3 {
		t.Fatalf("alive: code %v luong %d, want NOT_DEAD and 3", e, p.luong)
	}
	// dead with no Luong
	p.accountID, p.luong = 9, 0
	z.damagePlayer(p, killer{}, 1<<30, false)
	out.take()
	saves := len(sv.all())
	z.respawn(respawnCmd{id: 1, seq: 5, inPlace: true})
	if e := errorFor(out.take(), 5).GetCode(); e != pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_LUONG || !p.dead || p.luong != 0 || len(sv.all()) != saves {
		t.Fatalf("code %v dead %v luong %d saves %d, want NOT_ENOUGH_LUONG, still dead, nothing saved", e, p.dead, p.luong, len(sv.all())-saves)
	}
}

func TestRespawnToVillageIsFree(t *testing.T) {
	z, p, _ := shopZone(t, testItemData, shopInventory())
	z.area.Respawns = []Vec2{{X: 2.5, Y: 2.5}}
	z.advance(time.Now())
	p.pos = Vec2{X: 20.5, Y: 21.5}
	p.luong = 2
	z.damagePlayer(p, killer{}, 1<<30, false)
	z.respawn(respawnCmd{id: 1, seq: 4})
	if p.dead || p.pos != (Vec2{X: 2.5, Y: 2.5}) || p.luong != 2 {
		t.Fatalf("dead %v at %v luong %d, want the respawn point and 2 Luong", p.dead, p.pos, p.luong)
	}
}

func TestAutoRespawnIsToVillageAndFree(t *testing.T) {
	z, p, _ := shopZone(t, testItemData, shopInventory())
	z.area.Respawns = []Vec2{{X: 2.5, Y: 2.5}}
	z.advance(time.Now())
	p.pos = Vec2{X: 20.5, Y: 21.5}
	p.luong = 2
	z.damagePlayer(p, killer{}, 1<<30, false)
	for i := uint32(0); p.dead && i < z.ticks(autoRespawnSec)+5; i++ {
		z.advance(time.Now())
	}
	if p.dead || p.pos != (Vec2{X: 2.5, Y: 2.5}) || p.luong != 2 {
		t.Fatalf("dead %v at %v luong %d, want the respawn point and 2 Luong", p.dead, p.pos, p.luong)
	}
}

// ____TEST PK MODE____

// setPkNow queues a PK switch of id and runs one tick.
func setPkNow(t *testing.T, z *Zone, id uint64, seq uint32, on bool) {
	t.Helper()
	if !z.SetPk(id, seq, on) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func TestSetPkWhileDead(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	out := joinNow(t, z, 1, 12.5, 7.5)
	p := z.players[1]
	z.damagePlayer(p, killer{}, 1<<30, false)
	z.advance(time.Now())
	out.take()
	setPkNow(t, z, 1, 9, true)
	envs := out.take()
	if er := errorFor(envs, 9); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_DEAD {
		t.Fatalf("error %v, want DEAD", er)
	}
	if p.pkOn || len(looksIn(envs)) != 0 || len(sv.all()) != 0 {
		t.Fatalf("pk %v looks %v saves %d: a refusal must change nothing", p.pkOn, looksIn(envs), len(sv.all()))
	}
}

// Asking for the state the player is already in answers SetPkResp and
// nothing else: no LookChange, no SelfStats.
func TestSetPkSameState(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	out := joinNow(t, z, 1, 12.5, 7.5)
	v := joinNow(t, z, 2, 13.5, 7.5)
	z.advance(time.Now())
	out.take()
	v.take()
	setPkNow(t, z, 1, 9, false)
	envs := out.take()
	if r := respFor(envs, 9, (*pb.Envelope).GetSetPkResp); r == nil || r.On {
		t.Fatalf("reply %v, want SetPkResp off", r)
	}
	if ls := append(looksIn(envs), looksIn(v.take())...); len(ls) != 0 || len(selfStatsIn(envs)) != 0 {
		t.Fatalf("looks %v self stats %v, want none", ls, selfStatsIn(envs))
	}
}

// Turning PK on answers SetPkResp, sends SelfStats with pk_on and one
// LookChange with pk_on and the current weapon glow to the player and a
// viewer. It does not save.
func TestSetPkOn(t *testing.T) {
	z, p, out, _ := regenZone(t, pb.CharClass_CHAR_CLASS_THUY, 19, 0, map[string]int{"eq10_weapon_thuy": 12})
	sv := z.saver.(*fakeSaver)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	setPkNow(t, z, 1, 9, true)
	envs := out.take()
	if r := respFor(envs, 9, (*pb.Envelope).GetSetPkResp); r == nil || !r.On {
		t.Fatalf("reply %v, want SetPkResp on", r)
	}
	if ss := selfStatsIn(envs); len(ss) != 1 || !ss[0].PkOn {
		t.Fatalf("self stats %v, want one with pk_on", ss)
	}
	want := &pb.LookChange{Id: 1, WeaponEnhance: 12, PkOn: true}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if ls := looksIn(es); len(ls) != 1 || !proto.Equal(ls[0], want) {
			t.Fatalf("%s LookChanges %v, want %v", name, ls, want)
		}
	}
	if !p.pkOn || len(sv.all()) != 0 {
		t.Fatalf("pk %v saves %d, want on and no save", p.pkOn, len(sv.all()))
	}
	if st := z.state(p); !st.PkOn {
		t.Fatal("the saved state has PK off")
	}
}

// Turning PK off is refused IN_PVP_COMBAT until pvpUntil, then accepted.
func TestSetPkOffInPvPCombat(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	out := joinNow(t, z, 1, 12.5, 7.5)
	p := z.players[1]
	p.pkOn = true
	z.advance(time.Now())
	p.pvpUntil = z.tickNo + z.ticks(pvpLockSec) // a PvP hit landed this tick
	out.take()
	setPkNow(t, z, 1, 9, false)
	if er := errorFor(out.take(), 9); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT || !p.pkOn {
		t.Fatalf("error %v pk %v, want IN_PVP_COMBAT and still on", er, p.pkOn)
	}
	for z.tickNo < p.pvpUntil-2 {
		z.advance(time.Now())
	}
	out.take()
	setPkNow(t, z, 1, 10, false) // the last locked tick
	if er := errorFor(out.take(), 10); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT {
		t.Fatalf("error %v one tick before the lock ends, want IN_PVP_COMBAT", er)
	}
	setPkNow(t, z, 1, 11, false) // 10 s after the hit
	if r := respFor(out.take(), 11, (*pb.Envelope).GetSetPkResp); r == nil || r.On || p.pkOn {
		t.Fatalf("reply %v pk %v, want PK off 10 s later", r, p.pkOn)
	}
}

// A glow change and a PK change in the same tick make one LookChange with
// both values.
func TestLookChangeIsWholeLook(t *testing.T) {
	inv := gearInventory()
	inv.Bag[5] = &items.Item{Code: "eq10_weapon_viem", Count: 1, Enhance: 10}
	z, _, _, _ := gearZone(t, inv)
	out := z.players[1].out.(*fakeOut)
	v := joinNow(t, z, 2, 7.5, 5.5)
	z.advance(time.Now())
	out.take()
	v.take()
	if !z.Equip(1, 9, 5) {
		t.Fatal("inbox full")
	}
	setPkNow(t, z, 1, 10, true)
	want := &pb.LookChange{Id: 1, WeaponEnhance: 10, PkOn: true}
	for name, es := range map[string][]*pb.Envelope{"player": out.take(), "viewer": v.take()} {
		if ls := looksIn(es); len(ls) != 1 || !proto.Equal(ls[0], want) {
			t.Fatalf("%s LookChanges %v, want %v", name, ls, want)
		}
	}
}

// A PK-on player comes into view with pk_on, sees it in its own
// EnterWorldResp and saves it on leaving.
func TestPkOnEnterAndSave(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	in := info(1, 12.5, 7.5)
	in.PkOn = true
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	if r := out.take()[0].GetEnterWorldResp(); !r.GetStats().GetPkOn() {
		t.Fatalf("EnterWorldResp stats %v, want pk_on", r.GetStats())
	}
	v := joinNow(t, z, 2, 13.5, 7.5)
	z.advance(time.Now())
	if en := enterOf(v.take(), 1); en == nil || !en.PkOn {
		t.Fatalf("viewer's EntityEnter %v, want pk_on", en)
	}
	if en := enterOf(out.take(), 2); en == nil || en.PkOn {
		t.Fatalf("player's EntityEnter for the viewer %v, want PK off", en)
	}
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	if s := sv.all(); len(s) != 1 || !s[0].PkOn {
		t.Fatalf("saves %+v, want one with PkOn", s)
	}
}

// Quitting the world (LeaveWorldReq) is refused IN_PVP_COMBAT while the PvP
// lock runs: the player stays, nothing is saved. Once the lock is over it
// leaves and saves as leave does.
func TestQuitInPvPCombat(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 12.5, 7.5)
	p := z.players[1]
	p.pvpUntil = z.tickNo + z.ticks(pvpLockSec)
	if saved, code := z.quit(1); saved != nil || code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT {
		t.Fatalf("quit = %v, %v, want IN_PVP_COMBAT", saved, code)
	}
	if z.players[1] != p || len(sv.all()) != 0 {
		t.Fatalf("a refused quit removed or saved the player (saves %d)", len(sv.all()))
	}
	for z.pvpLocked(p) {
		z.advance(time.Now())
	}
	saved, code := z.quit(1)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || saved == nil {
		t.Fatalf("quit after the lock = %v, %v", saved, code)
	}
	if err := <-saved; err != nil || z.players[1] != nil || len(sv.all()) != 1 {
		t.Fatalf("err %v, still in zone %v, saves %d", err, z.players[1] != nil, len(sv.all()))
	}
	if saved, code := z.quit(1); code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED || saved == nil {
		t.Fatalf("quit of a player gone = %v, %v, want a closed channel", saved, code)
	}
}

// A player removed while its PvP lock runs (a disconnect, a kick) is
// removed at once, and Config.KeepPvPLock gets the time left on the lock so
// the world can hand it to the next login. No lock, no call.
func TestLeaveKeepsPvPLock(t *testing.T) {
	type kept struct {
		char int64
		left time.Duration
	}
	var got []kept
	cfg := testConfig(10)
	cfg.KeepPvPLock = func(char int64, left time.Duration) { got = append(got, kept{char, left}) }
	z := New(cfg, Area{Grid: openGrid(64, 64), Exp: testExp}, &fakeSaver{}, quietLog())
	joinNow(t, z, 1, 12.5, 7.5)
	joinNow(t, z, 2, 13.5, 7.5)
	joinNow(t, z, 3, 14.5, 7.5)
	z.players[1].pvpUntil = z.tickNo + z.ticks(4)
	z.players[3].pvpUntil = z.tickNo + z.ticks(2)
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	if err := <-z.leave(2); err != nil {
		t.Fatal(err)
	}
	z.evictAll(pb.KickReason_KICK_REASON_SERVER_SHUTDOWN)
	if want := []kept{{100, 4 * time.Second}, {300, 2 * time.Second}}; !slices.Equal(got, want) {
		t.Fatalf("kept %v, want %v", got, want)
	}
}

// The handoff carries PK and the 4 s left on the PvP lock: the new zone lists
// pvp_combat and refuses turning PK off until the lock ends.
func TestDepartCarriesPk(t *testing.T) {
	za, _ := transferZone(t, 1)
	outA := &fakeOut{}
	in := info(1, 35.5, 30.5)
	in.PkOn = true
	joinByInbox(t, za, in, outA)
	p := za.players[1]
	p.pvpUntil = za.tickNo + 1 + za.ticks(4) // 4 s left at the departing tick
	r := departNow(t, za, 1, Departure{To: Place{MapID: "m1"}, KeepPos: true, Check: notDead})
	if !r.info.PkOn || !maps.Equal(r.info.Cooldowns, map[string]time.Duration{"pvp_combat": 4 * time.Second}) {
		t.Fatalf("handoff PK %v cooldowns %v, want on and pvp_combat 4s", r.info.PkOn, r.info.Cooldowns)
	}
	if st := za.saver.(*fakeSaver).all(); !st[len(st)-1].PkOn {
		t.Fatal("the departure save has PK off")
	}

	zb, _ := transferZone(t, 2)
	outB := &fakeOut{}
	ew := joinByInbox(t, zb, r.info, outB)
	j := zb.tickNo
	if cd := ew.Cooldowns; len(cd) != 1 || cd[0].Key != "pvp_combat" || cd[0].RemainingMs != 4000 || !ew.Stats.GetPkOn() {
		t.Fatalf("EnterWorldResp cooldowns %v pk %v, want pvp_combat 4000 ms and PK on", cd, ew.Stats.GetPkOn())
	}
	for zb.tickNo < j+38 {
		zb.advance(time.Now())
	}
	outB.take()
	setPkNow(t, zb, 1, 3, false) // tick j+39
	if er := errorFor(outB.take(), 3); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT {
		t.Fatalf("PK off 3.9 s after arriving: %v, want IN_PVP_COMBAT", er)
	}
	setPkNow(t, zb, 1, 4, false) // tick j+40
	if r := respFor(outB.take(), 4, (*pb.Envelope).GetSetPkResp); r == nil || r.On {
		t.Fatalf("PK off 4 s after arriving: %v, want SetPkResp off", r)
	}
}

// ____TEST PARTY CELL____

// The zone keeps the party-id cell it is given and only reads it: no cell
// reads 0, a set cell reads what the party service wrote last, and the
// handoff carries the same cell, not its value.
func TestPartyCellJoinAndHandoff(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 2, 5.5, 5.5)
	if id := z.players[2].partyID(); id != 0 {
		t.Fatalf("party id without a cell %d, want 0", id)
	}
	cell := new(atomic.Uint64)
	in := info(1, 5.5, 5.5)
	in.Party = cell
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	if p.party != cell || p.partyID() != 0 {
		t.Fatalf("cell %p id %d, want the given cell at 0", p.party, p.partyID())
	}
	cell.Store(7)
	if p.partyID() != 7 {
		t.Fatalf("party id %d, want 7", p.partyID())
	}
	r := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{5.5, 5.5}}, Check: notDead})
	if r.err != nil || r.info.Party != cell {
		t.Fatalf("handoff cell %p err %v, want the same cell %p", r.info.Party, r.err, cell)
	}
}

// gainExp tells Leveled the new level once per level change, never for EXP
// within a level nor at the level cap.
func TestLeveledHook(t *testing.T) {
	type call struct {
		id    uint64
		level int
	}
	var calls []call
	cfg := testConfig(10)
	cfg.Leveled = func(id uint64, level int) { calls = append(calls, call{id, level}) }
	z := New(cfg, Area{Grid: openGrid(64, 64), Exp: testExp}, &fakeSaver{}, quietLog())
	joinNow(t, z, 1, 5.5, 5.5)
	p := z.players[1]
	z.gainExp(p, 1)
	if len(calls) != 0 {
		t.Fatalf("calls %v after EXP within the level, want none", calls)
	}
	z.gainExp(p, testExp.ToNext(1)+testExp.ToNext(2)) // two levels at once
	if !slices.Equal(calls, []call{{1, 3}}) {
		t.Fatalf("calls %v, want one to level 3", calls)
	}
	calls = nil
	p.level, p.exp = combat.MaxLevel, 0
	z.gainExp(p, 1000)
	if len(calls) != 0 {
		t.Fatalf("calls %v at the level cap, want none", calls)
	}
}

// ____TEST QUESTS____

// testQuestData is the real game data: quests.json with the skills its
// books teach.
var testQuestData = func() *gamedata.Data {
	d, err := gamedata.Load("../../data")
	if err != nil {
		panic(err)
	}
	return d
}()

// questZone joins a Viêm of the given level as player 1 at (5.5, 5.5) of
// mapID, with the real quest and item data, the quest NPCs tho_san,
// chi_huy and qua_truyen_tin 3 tiles east, south and north and a shop NPC
// 3 tiles west. Its outbox is emptied.
func questZone(t *testing.T, mapID string, level int, log quest.Log) (*Zone, *player, *fakeOut, *fakeSaver) {
	t.Helper()
	return questZoneAt(t, mapID, level, log, nil)
}

// questZoneAt is questZone with now as the zone clock (nil: time.Now).
func questZoneAt(t *testing.T, mapID string, level int, log quest.Log, now func() time.Time) (*Zone, *player, *fakeOut, *fakeSaver) {
	t.Helper()
	sv := &fakeSaver{}
	npcs := map[string]mapdata.NPC{
		"tho_san":        {ID: "tho_san", Role: mapdata.RoleQuest, Pos: mapdata.Point{X: 8.5, Y: 5.5}},
		"chi_huy":        {ID: "chi_huy", Role: mapdata.RoleQuest, Pos: mapdata.Point{X: 5.5, Y: 8.5}},
		"qua_truyen_tin": {ID: "qua_truyen_tin", Role: mapdata.RoleQuest, Pos: mapdata.Point{X: 5.5, Y: 2.5}},
		"shop":           {ID: "shop", Role: mapdata.RoleShop, Shop: "general", Pos: mapdata.Point{X: 2.5, Y: 5.5}},
	}
	cfg := testConfig(10)
	cfg.MapID, cfg.Now = mapID, now
	z := New(cfg, Area{Grid: openGrid(64, 64), Exp: testExp, Items: testItemData, NPCs: npcs, Skills: testQuestData.Skills,
		Quests: testQuestData}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Class, in.Level, in.Points = pb.CharClass_CHAR_CLASS_VIEM, level, combat.Points{Str: 60, Vit: 30}
	in.Xu, in.Inventory, in.Quests = 77, items.NewInventory(testItemData.BagSize), log
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	out.take()
	return z, z.players[1], out, sv
}

// questErr runs cmd for player 1 with seq 9 and returns its error code,
// UNSPECIFIED when it was not refused.
func questErr(t *testing.T, z *Zone, out *fakeOut, cmd func(z *Zone) bool) pb.ErrorCode {
	t.Helper()
	out.take()
	itemNow(t, z, cmd(z))
	if er := errorFor(out.take(), 9); er != nil {
		return er.Code
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

func accept(npc, q string) func(*Zone) bool {
	return func(z *Zone) bool { return z.QuestAccept(1, 9, npc, q) }
}

func turnIn(npc, q string) func(*Zone) bool {
	return func(z *Zone) bool { return z.QuestTurnIn(1, 9, npc, q) }
}

type questCase struct {
	name  string
	mapID string
	level int
	log   quest.Log
	setup func(z *Zone, p *player)
	cmd   func(*Zone) bool
	want  pb.ErrorCode
}

// runQuestErrors checks each refusal and that it changes nothing: no save,
// the same log, bag, Xu and EXP.
func runQuestErrors(t *testing.T, cases []questCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mapID := c.mapID
			if mapID == "" {
				mapID = "kumotori_1"
			}
			level := c.level
			if level == 0 {
				level = 1
			}
			z, p, out, sv := questZone(t, mapID, level, c.log.Clone())
			if c.setup != nil {
				c.setup(z, p)
			}
			bag, log, xu, exp := p.inv.Clone(), p.quests.Clone(), p.xu, p.exp
			if got := questErr(t, z, out, c.cmd); got != c.want {
				t.Fatalf("code %v, want %v", got, c.want)
			}
			if n := len(sv.all()); n != 0 {
				t.Fatalf("%d saves after a refusal", n)
			}
			if !reflect.DeepEqual(p.inv, bag) || !reflect.DeepEqual(p.quests, log) || p.xu != xu || p.exp != exp {
				t.Fatalf("refusal changed the player: log %v xu %d exp %d", p.quests, p.xu, p.exp)
			}
		})
	}
}

func TestQuestAcceptErrorsInOrder(t *testing.T) {
	const (
		dead  = pb.ErrorCode_ERROR_CODE_DEAD
		bad   = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		far   = pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
		notAv = pb.ErrorCode_ERROR_CODE_QUEST_NOT_AVAILABLE
		low   = pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW
	)
	kill := func(_ *Zone, p *player) { p.dead = true }
	away := func(_ *Zone, p *player) { p.pos = Vec2{30.5, 30.5} }
	runQuestErrors(t, []questCase{
		{name: "dead first", setup: func(z *Zone, p *player) { kill(z, p); away(z, p) }, cmd: accept("nope", "zzz"), want: dead},
		{name: "no such npc", cmd: accept("nope", "m01"), want: bad},
		{name: "not a quest npc", cmd: accept("shop", "m01"), want: bad},
		{name: "far before the quest", setup: away, cmd: accept("tho_san", "zzz"), want: far},
		{name: "no such quest", cmd: accept("tho_san", "zzz"), want: bad},
		{name: "another npc gives it", cmd: accept("chi_huy", "m01"), want: bad},
		{name: "given on another map", mapID: "ubuyashiki", cmd: accept("tho_san", "m01"), want: bad},
		{name: "requires before level", cmd: accept("tho_san", "m02"), want: notAv},
		{name: "already active", log: quest.Log{"m01": {State: quest.Active}}, cmd: accept("tho_san", "m01"), want: notAv},
		{name: "already done", level: 5, log: quest.Log{"m01": {State: quest.Done, Progress: 5}}, cmd: accept("tho_san", "m01"), want: notAv},
		{name: "level", log: quest.Log{"m01": {State: quest.Done, Progress: 5}}, cmd: accept("tho_san", "m02"), want: low},
	})
}

func TestQuestTurnInErrorsInOrder(t *testing.T) {
	const (
		dead      = pb.ErrorCode_ERROR_CODE_DEAD
		bad       = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		far       = pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
		notActive = pb.ErrorCode_ERROR_CODE_QUEST_NOT_ACTIVE
		notDone   = pb.ErrorCode_ERROR_CODE_QUEST_NOT_COMPLETE
		full      = pb.ErrorCode_ERROR_CODE_BAG_FULL
	)
	ready := quest.Log{"m01": {State: quest.Active, Progress: 5}}
	away := func(_ *Zone, p *player) { p.pos = Vec2{30.5, 30.5} }
	runQuestErrors(t, []questCase{
		{name: "dead first", log: ready, setup: func(z *Zone, p *player) { p.dead = true; away(z, p) }, cmd: turnIn("nope", "zzz"), want: dead},
		{name: "no such npc", log: ready, cmd: turnIn("nope", "m01"), want: bad},
		{name: "not a quest npc", log: ready, cmd: turnIn("shop", "m01"), want: bad},
		{name: "far before the quest", log: ready, setup: away, cmd: turnIn("tho_san", "zzz"), want: far},
		{name: "no such quest", log: ready, cmd: turnIn("tho_san", "zzz"), want: bad},
		{name: "not taken", cmd: turnIn("tho_san", "m01"), want: notActive},
		{name: "done", log: quest.Log{"m01": {State: quest.Done, Progress: 5}}, cmd: turnIn("tho_san", "m01"), want: notActive},
		// m06 is turned in to truong_lang: not active comes before the NPC.
		{name: "not active before npc", cmd: turnIn("tho_san", "m06"), want: notActive},
		{name: "another npc takes it", log: ready, cmd: turnIn("chi_huy", "m01"), want: bad},
		{name: "taken on another map", mapID: "ubuyashiki", log: ready, cmd: turnIn("tho_san", "m01"), want: bad},
		{name: "not complete", log: quest.Log{"m01": {State: quest.Active, Progress: 4}}, cmd: turnIn("tho_san", "m01"), want: notDone},
		{name: "bag full", log: ready, setup: func(_ *Zone, p *player) { fillBag(p.inv) }, cmd: turnIn("tho_san", "m01"), want: full},
	})
}

// Accepting a talk quest starts it at progress 1 (a kill quest at 0),
// saves once and answers the entry.
func TestQuestAccept(t *testing.T) {
	z, p, out, sv := questZone(t, "kumotori_1", 1, nil)
	out.take()
	itemNow(t, z, z.QuestAccept(1, 9, "tho_san", "m01"))
	r := respFor(out.take(), 9, (*pb.Envelope).GetQuestAcceptResp)
	want := &pb.QuestEntry{QuestId: "m01", State: pb.QuestState_QUEST_STATE_ACTIVE}
	if r == nil || !proto.Equal(r.Quest, want) || p.quests["m01"] != (quest.Entry{State: quest.Active}) {
		t.Fatalf("resp %v log %v, want m01 active at 0", r, p.quests)
	}
	saves := sv.all()
	if len(saves) != 1 || !reflect.DeepEqual(saves[0].Quests, []store.QuestRow{{QuestID: "m01", State: 1}}) {
		t.Fatalf("saves %+v, want one with m01 active", saves)
	}

	log := quest.Log{"m04": {State: quest.Done, Progress: 6}}
	z, p, out, _ = questZone(t, "kumotori_1", 5, log)
	itemNow(t, z, z.QuestAccept(1, 9, "tho_san", "m05"))
	r = respFor(out.take(), 9, (*pb.Envelope).GetQuestAcceptResp)
	if r == nil || r.Quest.Progress != 1 || p.quests["m05"] != (quest.Entry{State: quest.Active, Progress: 1}) {
		t.Fatalf("talk resp %v log %v, want m05 active at 1", r, p.quests)
	}
}

// Turning m01 in pays 20 EXP, 40 Xu and 5 HP potions, marks it done,
// saves and answers what changed; the waiting refine result stays, and the
// next quest of the chain can be taken.
func TestQuestTurnInM01(t *testing.T) {
	z, p, out, sv := questZone(t, "kumotori_1", 1, quest.Log{"m01": {State: quest.Active, Progress: 5}})
	p.inv.Bag[0] = &items.Item{Code: "potion_hp_1", Count: 3}
	pending := &pendingRefine{}
	p.refine = pending
	p.level = 2 // m02 needs level 2
	itemNow(t, z, z.QuestTurnIn(1, 9, "tho_san", "m01"))
	r := respFor(out.take(), 9, (*pb.Envelope).GetQuestTurnInResp)
	if r == nil || r.QuestId != "m01" || r.Exp != 20 || r.Xu != 40 {
		t.Fatalf("resp %v, want m01 paying 20 EXP and 40 Xu", r)
	}
	if p.exp != 20 || p.xu != 117 || p.inv.Bag[0].Count != 8 || p.quests["m01"].State != quest.Done || p.refine != pending {
		t.Fatalf("exp %d xu %d potions %d log %v refine %v", p.exp, p.xu, p.inv.Bag[0].Count, p.quests, p.refine)
	}
	if len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != 0 || r.Update.Bag[0].Item.Count != 8 || r.Stats.Xu != 117 || r.Stats.Exp != 20 {
		t.Fatalf("update %v stats %v", r.Update, r.Stats)
	}
	saves := sv.all()
	if len(saves) != 1 || saves[0].Xu != 117 || !reflect.DeepEqual(saves[0].Quests, []store.QuestRow{{QuestID: "m01", State: 2, Progress: 5}}) {
		t.Fatalf("saves %+v, want one with m01 done", saves)
	}
	if got := questErr(t, z, out, accept("tho_san", "m01")); got != pb.ErrorCode_ERROR_CODE_QUEST_NOT_AVAILABLE {
		t.Fatalf("accept m01 again: %v", got)
	}
	if got := questErr(t, z, out, accept("tho_san", "m02")); got != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("accept m02: %v", got)
	}
}

// m11 turned in to chi_huy by a Viêm pays the first book and a white +0
// tier-10 Viêm weapon; with a waiver of 10 the book is left out.
func TestQuestTurnInM11(t *testing.T) {
	for _, c := range []struct {
		waiver int
		book   bool
	}{{0, true}, {10, false}} {
		z, p, out, _ := questZone(t, "ubuyashiki", 10, quest.Log{"m11": {State: quest.Active, Progress: 1}})
		p.bookWaiver = c.waiver
		p.inv.Bag[1] = &items.Item{Code: "stone_enhance", Count: 2}
		itemNow(t, z, z.QuestTurnIn(1, 9, "chi_huy", "m11"))
		r := respFor(out.take(), 9, (*pb.Envelope).GetQuestTurnInResp)
		if r == nil {
			t.Fatalf("waiver %d: no QuestTurnInResp", c.waiver)
		}
		got := map[string]*items.Item{}
		for _, it := range p.inv.Bag {
			if it != nil {
				got[it.Code] = it
			}
		}
		w := got["eq10_weapon_viem"]
		if w == nil || !reflect.DeepEqual(*w, items.Item{Code: "eq10_weapon_viem", Count: 1}) {
			t.Fatalf("waiver %d: weapon %+v, want white +0 with no lines", c.waiver, w)
		}
		if b := got["book_viem_1"]; (b != nil) != c.book || (b != nil && b.Count != 1) {
			t.Fatalf("waiver %d: book %+v, want %v", c.waiver, b, c.book)
		}
		if got["potion_hp_10"].Count != 10 || got["potion_mp_10"].Count != 10 {
			t.Fatalf("waiver %d: potions %v", c.waiver, got)
		}
		n := 4
		if !c.book {
			n = 3
		}
		if len(r.Update.Bag) != n || len(r.Update.Equipped) != 0 {
			t.Fatalf("waiver %d: update %v, want %d bag slots", c.waiver, r.Update, n)
		}
		for i := 1; i < len(r.Update.Bag); i++ {
			if r.Update.Bag[i-1].Slot >= r.Update.Bag[i].Slot {
				t.Fatalf("update slots not ascending: %v", r.Update.Bag)
			}
		}
	}
}

// A bag with room for all but the book refuses m11 with BAG_FULL.
func TestQuestTurnInAllOrNothing(t *testing.T) {
	z, p, out, sv := questZone(t, "ubuyashiki", 10, quest.Log{"m11": {State: quest.Active, Progress: 1}})
	fillBag(p.inv)
	for i := range 3 {
		p.inv.Bag[i] = nil
	}
	bag := p.inv.Clone()
	if got := questErr(t, z, out, turnIn("chi_huy", "m11")); got != pb.ErrorCode_ERROR_CODE_BAG_FULL {
		t.Fatalf("code %v, want BAG_FULL", got)
	}
	if !reflect.DeepEqual(p.inv, bag) || p.quests["m11"].State != quest.Active || p.xu != 77 || len(sv.all()) != 0 {
		t.Fatal("a refused turn-in changed the player")
	}
}

// A turn-in that crosses a level sends LevelUp in the next WorldTick and
// tells the Leveled hook.
func TestQuestTurnInLevelsUp(t *testing.T) {
	z, p, out, _ := questZone(t, "kumotori_1", 1, quest.Log{"m01": {State: quest.Active, Progress: 5}})
	var leveled []int
	z.cfg.Leveled = func(_ uint64, lv int) { leveled = append(leveled, lv) }
	p.exp = testExp.ToNext(1) - 10
	itemNow(t, z, z.QuestTurnIn(1, 9, "tho_san", "m01"))
	envs := out.take()
	wt := lastTick(envs)
	if p.level != 2 || p.exp != 10 || wt == nil || len(wt.LevelUps) != 1 || wt.LevelUps[0].Level != 2 || !slices.Equal(leveled, []int{2}) {
		t.Fatalf("level %d exp %d tick %v hook %v, want level 2 with a LevelUp", p.level, p.exp, wt, leveled)
	}
}

// questKillZone is lootZone with the real quest data and a Viêm player 1
// next to the monster with log.
func questKillZone(t *testing.T, log quest.Log) (*Zone, *monster, *fakeOut) {
	t.Helper()
	z, m := lootZone(t, testItemData)
	z.area.Quests = testQuestData
	in := info(1, 31.5, 30.5)
	in.Class, in.Quests = pb.CharClass_CHAR_CLASS_VIEM, log
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	return z, m, out
}

// questProgressIn is every QuestProgress in envs.
func questProgressIn(envs []*pb.Envelope) []*pb.QuestProgress {
	var out []*pb.QuestProgress
	for _, e := range envs {
		if q := e.GetQuestProgress(); q != nil {
			out = append(out, q)
		}
	}
	return out
}

// The killer's active quests on the monster's kind go up by one, both of
// them; the others stay. QuestProgress (seq 0) comes after that tick's
// WorldTick and before SelfStats, with only the changed entries, and is
// sent once. Credit is not saved at once.
func TestQuestCreditKiller(t *testing.T) {
	log := quest.Log{
		"m01": {State: quest.Active, Progress: 2},
		"m02": {State: quest.Active},
		"m03": {State: quest.Active},
		"m04": {State: quest.Done, Progress: 6},
	}
	z, m, out := questKillZone(t, log)
	sv := z.saver.(*fakeSaver)
	z.rng = rand.New(winSource{}) // every collect roll hits
	out.take()
	attackLand(t, z, 1, 1, m.id)
	p := z.players[1]
	if !m.dead || p.quests["m01"].Progress != 3 || p.quests["m02"].Progress != 1 || p.quests["m03"].Progress != 0 {
		t.Fatalf("dead %v log %v, want m01 3 and m02 1", m.dead, p.quests)
	}
	envs := out.take()
	qi, wi, si := -1, -1, -1
	for i, e := range envs {
		switch {
		case e.GetQuestProgress() != nil:
			qi = i
		case e.GetWorldTick() != nil && qi < 0:
			wi = i
		case e.GetSelfStats() != nil && qi >= 0 && si < 0:
			si = i
		}
	}
	qp := questProgressIn(envs)
	want := []*pb.QuestEntry{
		{QuestId: "m01", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 3},
		{QuestId: "m02", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 1},
	}
	if len(qp) != 1 || len(qp[0].Quests) != 2 || !proto.Equal(qp[0].Quests[0], want[0]) || !proto.Equal(qp[0].Quests[1], want[1]) {
		t.Fatalf("QuestProgress %v, want one with m01 3 and m02 1", qp)
	}
	if envs[qi].Seq != 0 || wi < 0 || wi > qi || si < qi {
		t.Fatalf("order: WorldTick %d QuestProgress %d SelfStats %d seq %d", wi, qi, si, envs[qi].Seq)
	}
	for _, st := range sv.all() {
		t.Fatalf("credit saved %+v", st)
	}
	z.advance(time.Now())
	if qp := questProgressIn(out.take()); len(qp) != 0 {
		t.Fatalf("QuestProgress sent again: %v", qp)
	}
}

// Only the killer is credited: a player who only did damage and a teammate
// in the same zone get nothing.
func TestQuestCreditOnlyKiller(t *testing.T) {
	active := quest.Log{"m01": {State: quest.Active}}
	z, m, out1 := questKillZone(t, active.Clone())
	m.hp = 1000
	for _, id := range []uint64{2, 3} {
		in := info(id, 29.5, 30.5+float32(id-2))
		in.Class, in.Quests = pb.CharClass_CHAR_CLASS_VIEM, active.Clone()
		if err := z.join(in, &fakeOut{}); err != nil {
			t.Fatal(err)
		}
	}
	partied(z, 7, 1, 3)
	attackLand(t, z, 2, 1, m.id) // damage only
	if m.dead {
		t.Fatal("the first slash killed")
	}
	m.hp = 1
	z.players[2].slashReady = 0
	out1.take()
	attackLand(t, z, 1, 1, m.id)
	if !m.dead || z.players[1].quests["m01"].Progress != 1 {
		t.Fatalf("dead %v killer log %v", m.dead, z.players[1].quests)
	}
	for _, id := range []uint64{2, 3} {
		if pr := z.players[id].quests["m01"].Progress; pr != 0 {
			t.Fatalf("player %d progress %d, want 0", id, pr)
		}
	}
}

// A killer who died before the impact lands is credited; one who left the
// zone is not, and nobody else is.
func TestQuestCreditDeadOrGoneKiller(t *testing.T) {
	z, m, _ := questKillZone(t, quest.Log{"m01": {State: quest.Active}})
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	z.damagePlayer(p, killer{}, 10000, false)
	settle(z)
	if !p.dead || !m.dead || p.quests["m01"].Progress != 1 {
		t.Fatalf("dead %v/%v log %v, want the dead killer credited", p.dead, m.dead, p.quests)
	}

	z, m, _ = questKillZone(t, quest.Log{"m01": {State: quest.Active}})
	in := info(2, 29.5, 30.5)
	in.Quests = quest.Log{"m01": {State: quest.Active}}
	out2 := &fakeOut{}
	if err := z.join(in, out2); err != nil {
		t.Fatal(err)
	}
	attackNow(t, z, 1, 1, m.id)
	if err := <-z.leave(1); err != nil {
		t.Fatal(err)
	}
	out2.take()
	settle(z)
	if !m.dead || z.players[2].quests["m01"].Progress != 0 || len(questProgressIn(out2.take())) != 0 {
		t.Fatalf("dead %v other log %v, want nobody credited", m.dead, z.players[2].quests)
	}
}

// EnterWorldResp lists every quest entry and the waiver; the handoff and
// every save carry deep copies of both.
func TestQuestsJoinSaveAndHandoff(t *testing.T) {
	log := quest.Log{"m02": {State: quest.Active, Progress: 3}, "m01": {State: quest.Done, Progress: 5}}
	sv := &fakeSaver{}
	z := New(testConfig(10), Area{Grid: openGrid(64, 64), Exp: testExp, Quests: testQuestData}, sv, quietLog())
	in := info(1, 5.5, 5.5)
	in.Quests, in.SkillBookWaiver = log, 12
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	r := out.take()[0].GetEnterWorldResp()
	want := []*pb.QuestEntry{
		{QuestId: "m01", State: pb.QuestState_QUEST_STATE_DONE, Progress: 5},
		{QuestId: "m02", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 3},
	}
	if r.SkillBookWaiver != 12 || len(r.Quests.GetQuests()) != 2 || !proto.Equal(r.Quests.Quests[0], want[0]) || !proto.Equal(r.Quests.Quests[1], want[1]) {
		t.Fatalf("EnterWorldResp quests %v waiver %d", r.Quests, r.SkillBookWaiver)
	}
	p := z.players[1]
	log["m03"] = quest.Entry{State: quest.Active}
	if _, ok := p.quests["m03"]; ok {
		t.Fatal("join shares the log")
	}
	st := z.state(p)
	rows := []store.QuestRow{{QuestID: "m01", State: 2, Progress: 5}, {QuestID: "m02", State: 1, Progress: 3}}
	if !reflect.DeepEqual(st.Quests, rows) || st.SkillBookWaiver != 12 {
		t.Fatalf("state quests %+v waiver %d", st.Quests, st.SkillBookWaiver)
	}
	res := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{5.5, 5.5}}, Check: notDead})
	if res.err != nil || res.info.SkillBookWaiver != 12 || !reflect.DeepEqual(res.info.Quests, quest.Log{"m02": {State: quest.Active, Progress: 3}, "m01": {State: quest.Done, Progress: 5}}) {
		t.Fatalf("handoff quests %v waiver %d err %v", res.info.Quests, res.info.SkillBookWaiver, res.err)
	}
	res.info.Quests["m01"] = quest.Entry{}
	if p.quests["m01"].State != quest.Done {
		t.Fatal("handoff shares the log")
	}
	if saves := sv.all(); len(saves) == 0 || !reflect.DeepEqual(saves[len(saves)-1].Quests, rows) || saves[len(saves)-1].SkillBookWaiver != 12 {
		t.Fatalf("departure save %+v", saves)
	}
}

// ____TEST SKILL BOOKS____

// The first learn of a skill above the waiver takes a book from the lowest
// matching slot; a refused learn costs nothing.
func TestLearnNeedsBook(t *testing.T) {
	setup := func(level, points, waiver int, skills map[string]int, book int) (*Zone, *player, *fakeOut, *fakeSaver) {
		z, p, out, sv := learnZone(t, level, points, skills)
		p.bookWaiver = waiver
		p.inv = items.NewInventory(20)
		p.inv.Bag[2] = &items.Item{Code: "stone_enhance", Count: 3}
		if book > 0 {
			p.inv.Bag[4] = &items.Item{Code: "book_nham_1", Count: book}
			p.inv.Bag[6] = &items.Item{Code: "book_nham_1", Count: 1}
		}
		return z, p, out, sv
	}
	t.Run("no book", func(t *testing.T) {
		z, p, out, sv := setup(10, 2, 0, nil, 0)
		learnNow(t, z, 1, 3, "nham_1")
		if er := errorFor(out.take(), 3); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_NO_SKILL_BOOK {
			t.Fatalf("error %v, want NO_SKILL_BOOK", er)
		}
		if p.skillPoints != 2 || p.skills["nham_1"] != 0 || len(sv.all()) != 0 {
			t.Fatalf("refused learn changed points %d skills %v", p.skillPoints, p.skills)
		}
	})
	t.Run("no points and no book", func(t *testing.T) {
		z, _, out, _ := setup(10, 0, 0, nil, 0)
		learnNow(t, z, 1, 3, "nham_1")
		if er := errorFor(out.take(), 3); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS {
			t.Fatalf("error %v, want NOT_ENOUGH_POINTS", er)
		}
	})
	t.Run("with a book", func(t *testing.T) {
		z, p, out, sv := setup(10, 2, 0, nil, 2)
		learnNow(t, z, 1, 3, "nham_1")
		r := learnRespFor(out.take(), 3)
		if r == nil || p.skills["nham_1"] != 1 || p.skillPoints != 1 {
			t.Fatalf("resp %v skills %v points %d", r, p.skills, p.skillPoints)
		}
		if p.inv.Bag[4].Count != 1 || p.inv.Bag[6].Count != 1 {
			t.Fatalf("books %v %v, want one fewer in the lowest slot", p.inv.Bag[4], p.inv.Bag[6])
		}
		if r.Update == nil || len(r.Update.Bag) != 1 || r.Update.Bag[0].Slot != 4 {
			t.Fatalf("update %v, want slot 4", r.Update)
		}
		if len(sv.all()) != 1 {
			t.Fatalf("%d saves, want 1", len(sv.all()))
		}
		// Raising the level needs no book.
		p.level = 11
		learnNow(t, z, 1, 4, "nham_1")
		r = learnRespFor(out.take(), 4)
		if r == nil || p.skills["nham_1"] != 2 || p.inv.Bag[4].Count != 1 || (r.Update != nil && len(r.Update.Bag) != 0) {
			t.Fatalf("level 2: resp %v skills %v", r, p.skills)
		}
	})
	t.Run("last book empties the slot", func(t *testing.T) {
		z, p, out, _ := setup(10, 1, 0, nil, 0)
		p.inv.Bag[4] = &items.Item{Code: "book_nham_1", Count: 1}
		learnNow(t, z, 1, 3, "nham_1")
		r := learnRespFor(out.take(), 3)
		if r == nil || p.inv.Bag[4] != nil || r.Update == nil || len(r.Update.Bag) != 1 || r.Update.Bag[0].Item != nil {
			t.Fatalf("resp %v slot %v", r, p.inv.Bag[4])
		}
	})
	t.Run("waiver", func(t *testing.T) {
		z, p, out, _ := setup(10, 1, 10, nil, 0)
		learnNow(t, z, 1, 3, "nham_1")
		r := learnRespFor(out.take(), 3)
		if r == nil || p.skills["nham_1"] != 1 || (r.Update != nil && len(r.Update.Bag) != 0) {
			t.Fatalf("waiver 10: resp %v skills %v", r, p.skills)
		}
	})
}

// ____TEST DAILY____

// fakeClock is a settable Config.Now. The tests drive the zone on their own
// goroutine, so it needs no lock.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// beforeMidnight is half a second before 00:00 UTC+7 (17:00 UTC), years
// away from the wall clock.
var beforeMidnight = time.Date(2030, 3, 14, 16, 59, 59, 500e6, time.UTC)

// dailyZone is questZoneAt on ubuyashiki, where qua_truyen_tin gives and
// takes the daily quests, with clock as the zone clock.
func dailyZone(t *testing.T, level int, log quest.Log, clock *fakeClock) (*Zone, *player, *fakeOut, *fakeSaver) {
	t.Helper()
	return questZoneAt(t, "ubuyashiki", level, log, clock.now)
}

// questLogsIn is every pushed QuestLog in envs.
func questLogsIn(envs []*pb.Envelope) []*pb.QuestLog {
	var out []*pb.QuestLog
	for _, e := range envs {
		if q := e.GetQuestLog(); q != nil {
			out = append(out, q)
		}
	}
	return out
}

// Accepting d_kill stamps the zone day of the zone clock (not the wall
// clock) and saves it; accepting it again the same day is refused.
func TestDailyAcceptUsesZoneDay(t *testing.T) {
	clock := &fakeClock{beforeMidnight.Add(-36 * time.Hour)}
	day := quest.Day(clock.t)
	z, p, out, sv := dailyZone(t, 10, nil, clock)
	itemNow(t, z, z.QuestAccept(1, 9, "qua_truyen_tin", "d_kill"))
	r := respFor(out.take(), 9, (*pb.Envelope).GetQuestAcceptResp)
	if r == nil || r.Quest.QuestId != "d_kill" || p.quests["d_kill"] != (quest.Entry{State: quest.Active, Day: day}) {
		t.Fatalf("resp %v log %v, want d_kill active on day %d", r, p.quests, day)
	}
	if saves := sv.all(); len(saves) != 1 || !reflect.DeepEqual(saves[0].Quests, []store.QuestRow{{QuestID: "d_kill", State: 1, Day: day}}) {
		t.Fatalf("saves %+v, want d_kill on day %d", saves, day)
	}
	if got := questErr(t, z, out, accept("qua_truyen_tin", "d_kill")); got != pb.ErrorCode_ERROR_CODE_QUEST_NOT_AVAILABLE {
		t.Fatalf("accept again: %v, want QUEST_NOT_AVAILABLE", got)
	}
	if got := questErr(t, z, out, accept("qua_truyen_tin", "d_enhance")); got != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("accept d_enhance: %v", got)
	}
}

// When the clock crosses 00:00 UTC+7 the zone notices within a second:
// yesterday's daily entries go, active or done, the main ones stay, and
// every player gets one QuestLog (seq 0) with about 24 h to the next
// reset, before the QuestProgress of the same tick; nothing is saved. The
// daily quests can be taken again, on the new day.
func TestDailyResetAtMidnight(t *testing.T) {
	clock := &fakeClock{beforeMidnight}
	day := quest.Day(clock.t)
	log := quest.Log{
		"d_kill":    {State: quest.Active, Progress: 12, Day: day},
		"d_collect": {State: quest.Done, Progress: 20, Day: day},
		"m01":       {State: quest.Done, Progress: 5},
		"m02":       {State: quest.Active, Progress: 1},
	}
	z, p, out, sv := dailyZone(t, 10, log, clock)
	out2 := &fakeOut{}
	if err := z.join(info(2, 6.5, 5.5), out2); err != nil {
		t.Fatal(err)
	}
	tick := func() []*pb.Envelope {
		p.questChanged = []string{"m02"}
		z.advance(time.Now())
		return out.take()
	}
	for range 2 * z.cfg.TickRate {
		if ql := questLogsIn(tick()); len(ql) != 0 {
			t.Fatalf("QuestLog %v before midnight", ql)
		}
	}
	out2.take()
	if !reflect.DeepEqual(p.quests, log) {
		t.Fatalf("log %v changed before midnight", p.quests)
	}

	clock.t = beforeMidnight.Add(time.Second) // 17:00:00.5 UTC
	var envs []*pb.Envelope
	n := 0
	for n < z.cfg.TickRate {
		n++
		if envs = tick(); len(questLogsIn(envs)) > 0 {
			break
		}
	}
	ql := questLogsIn(envs)
	want := []*pb.QuestEntry{
		{QuestId: "m01", State: pb.QuestState_QUEST_STATE_DONE, Progress: 5},
		{QuestId: "m02", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 1},
	}
	if len(ql) != 1 || len(ql[0].Quests) != 2 || !proto.Equal(ql[0].Quests[0], want[0]) || !proto.Equal(ql[0].Quests[1], want[1]) {
		t.Fatalf("after %d ticks QuestLog %v, want one with m01 and m02", n, ql)
	}
	if ms := ql[0].DailyResetMs; ms <= 86_400_000-1000 || ms > 86_400_000 {
		t.Fatalf("daily_reset_ms %d, want about 86,400,000", ms)
	}
	li := slices.IndexFunc(envs, func(e *pb.Envelope) bool { return e.GetQuestLog() != nil })
	qi := slices.IndexFunc(envs, func(e *pb.Envelope) bool { return e.GetQuestProgress() != nil })
	if envs[li].Seq != 0 || qi < li {
		t.Fatalf("QuestLog at %d (seq %d), QuestProgress at %d", li, envs[li].Seq, qi)
	}
	if want := (quest.Log{"m01": log["m01"], "m02": log["m02"]}); !reflect.DeepEqual(p.quests, want) {
		t.Fatalf("log %v, want %v", p.quests, want)
	}
	ql2 := questLogsIn(out2.take())
	if len(ql2) != 1 || len(ql2[0].Quests) != 0 || ql2[0].DailyResetMs != ql[0].DailyResetMs {
		t.Fatalf("player 2 QuestLog %v, want one with no quests", ql2)
	}
	if saves := sv.all(); len(saves) != 0 {
		t.Fatalf("the reset saved %+v", saves)
	}
	for range 2 * z.cfg.TickRate {
		if ql := questLogsIn(tick()); len(ql) != 0 {
			t.Fatalf("QuestLog %v sent again", ql)
		}
	}
	for _, id := range []string{"d_kill", "d_collect"} {
		if got := questErr(t, z, out, accept("qua_truyen_tin", id)); got != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
			t.Fatalf("accept %s on the new day: %v", id, got)
		}
		if e := p.quests[id]; e != (quest.Entry{State: quest.Active, Day: day + 1}) {
			t.Fatalf("%s %+v, want active on day %d", id, e, day+1)
		}
	}
}

// A player joining with yesterday's daily entries sees the log without
// them, and the time to the next reset by the zone clock.
func TestDailyJoinPrunes(t *testing.T) {
	clock := &fakeClock{beforeMidnight.Add(-time.Hour)}
	day := quest.Day(clock.t)
	z, _, _, _ := dailyZone(t, 10, nil, clock)
	in := info(2, 5.5, 5.5)
	in.Quests = quest.Log{
		"d_kill":    {State: quest.Active, Progress: 30, Day: day - 1},
		"d_collect": {State: quest.Done, Progress: 20, Day: day - 1},
		"d_enhance": {State: quest.Active, Progress: 2, Day: day},
		"m01":       {State: quest.Done, Progress: 5},
	}
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	r := out.take()[0].GetEnterWorldResp()
	want := []*pb.QuestEntry{
		{QuestId: "d_enhance", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: 2},
		{QuestId: "m01", State: pb.QuestState_QUEST_STATE_DONE, Progress: 5},
	}
	q := r.GetQuests()
	if len(q.GetQuests()) != 2 || !proto.Equal(q.Quests[0], want[0]) || !proto.Equal(q.Quests[1], want[1]) {
		t.Fatalf("EnterWorldResp quests %v, want d_enhance and m01", q)
	}
	if q.DailyResetMs != (time.Hour + 500*time.Millisecond).Milliseconds() {
		t.Fatalf("daily_reset_ms %d, want 3,600,500", q.DailyResetMs)
	}
	if got := z.players[2].quests; len(got) != 2 {
		t.Fatalf("log %v, want d_enhance and m01", got)
	}
}

// A daily entry rides the handoff and stays in a zone on the same day; a
// zone already on the next day drops it at the join.
func TestDailyHandoff(t *testing.T) {
	clock := &fakeClock{beforeMidnight}
	day := quest.Day(clock.t)
	z, _, _, _ := dailyZone(t, 10, quest.Log{"d_kill": {State: quest.Active, Progress: 7, Day: day}, "m01": {State: quest.Done, Progress: 5}}, clock)
	res := departNow(t, z, 1, Departure{To: Place{MapID: "m2", Pos: Vec2{5.5, 5.5}}, Check: notDead})
	if res.err != nil || res.info.Quests["d_kill"] != (quest.Entry{State: quest.Active, Progress: 7, Day: day}) {
		t.Fatalf("handoff quests %v err %v", res.info.Quests, res.err)
	}
	for _, c := range []struct {
		name   string
		now    time.Time
		kept   bool
		listed int
	}{
		{"same day", beforeMidnight, true, 2},
		{"next day", beforeMidnight.Add(time.Second), false, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := testConfig(10)
			cfg.Now = (&fakeClock{c.now}).now
			z2 := New(cfg, Area{Grid: openGrid(64, 64), Exp: testExp, Quests: testQuestData}, &fakeSaver{}, quietLog())
			out := &fakeOut{}
			if err := z2.join(res.info, out); err != nil {
				t.Fatal(err)
			}
			_, kept := z2.players[1].quests["d_kill"]
			r := out.take()[0].GetEnterWorldResp()
			if kept != c.kept || len(r.GetQuests().GetQuests()) != c.listed {
				t.Fatalf("d_kill kept %v, EnterWorldResp quests %v", kept, r.GetQuests())
			}
		})
	}
}

// A daily credit queued in a tick whose drain then prunes it (a join that is
// the first to see the new day) is not sent as a QuestProgress: the player
// gets the QuestLog without the entry and nothing for the dropped id.
func TestDailyJoinPruneDropsPendingProgress(t *testing.T) {
	clock := &fakeClock{beforeMidnight}
	yesterday := quest.Day(clock.t)
	z, p, out, _ := dailyZone(t, 10, quest.Log{"d_enhance": {State: quest.Active, Progress: 1, Day: yesterday}}, clock)
	out.take()
	p.questChanged = []string{"d_enhance"} // an enhance credit earlier in the drain
	clock.t = beforeMidnight.Add(time.Second)
	if err := z.join(info(2, 6.5, 5.5), &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	z.advance(time.Now())
	envs := out.take()
	if ql := questLogsIn(envs); len(ql) != 1 || len(ql[0].Quests) != 0 {
		t.Fatalf("QuestLog %v, want one with no quests", ql)
	}
	if qp := questProgressIn(envs); len(qp) != 0 {
		t.Fatalf("QuestProgress %v for a pruned quest", qp)
	}
}

// A zone whose once-a-second day check has not yet seen midnight still
// prunes a joining player by the clock: an entry of the new day, accepted in
// a zone that already moved on, is kept, and one of the old day is dropped,
// with the countdown of the new day. The zone's own players get the midnight
// QuestLog in the same tick, and the joiner gets no second log.
func TestDailyJoinUsesClockDay(t *testing.T) {
	clock := &fakeClock{beforeMidnight}
	yesterday := quest.Day(clock.t)
	z, p, out, _ := dailyZone(t, 10, quest.Log{"d_collect": {State: quest.Active, Day: yesterday}}, clock)
	clock.t = beforeMidnight.Add(time.Second) // 17:00:00.5 UTC, z still on yesterday
	today := quest.Day(clock.t)

	in := info(2, 6.5, 5.5)
	in.Level, in.Quests = 10, quest.Log{
		"d_kill":    {State: quest.Done, Progress: 30, Day: today},
		"d_enhance": {State: quest.Active, Progress: 1, Day: yesterday},
		"m01":       {State: quest.Done, Progress: 5},
	}
	out2 := &fakeOut{}
	if err := z.join(in, out2); err != nil {
		t.Fatal(err)
	}
	want := quest.Log{"d_kill": in.Quests["d_kill"], "m01": in.Quests["m01"]}
	if !reflect.DeepEqual(z.players[2].quests, want) {
		t.Fatalf("joiner log %v, want %v", z.players[2].quests, want)
	}
	r := out2.take()[0].GetEnterWorldResp()
	if len(r.GetQuests().GetQuests()) != 2 || r.GetQuests().DailyResetMs != 86_399_500 {
		t.Fatalf("EnterWorldResp quests %v", r.GetQuests())
	}
	if z.day != today {
		t.Fatalf("zone day %d, want %d", z.day, today)
	}

	out.take()
	z.advance(time.Now())
	if ql := questLogsIn(out.take()); len(ql) != 1 || len(ql[0].Quests) != 0 {
		t.Fatalf("player 1 QuestLog %v, want one with no quests", ql)
	}
	if len(p.quests) != 0 {
		t.Fatalf("player 1 log %v, want empty", p.quests)
	}
	if ql := questLogsIn(out2.take()); len(ql) != 0 {
		t.Fatalf("joiner got QuestLog %v after EnterWorldResp", ql)
	}
	if e := questErr(t, z, out2, func(z *Zone) bool { return z.QuestAccept(2, 9, "qua_truyen_tin", "d_kill") }); e != pb.ErrorCode_ERROR_CODE_QUEST_NOT_AVAILABLE {
		t.Fatalf("second d_kill today: %v, want QUEST_NOT_AVAILABLE", e)
	}
}

// d_enhance counts the attempts an EnhanceReq ran, after the loop and only
// when it succeeds, capped at 3: times 5 stopping at the first success is
// 1, two failed attempts with charms are 2, a refused request is 0. The
// change goes out as QuestProgress and in the save of the command.
func TestDailyEnhanceCredit(t *testing.T) {
	for _, c := range []struct {
		name        string
		fail        bool
		charms      int
		ref         items.Ref
		protect     bool
		times       int
		start, want int
	}{
		{"first success of 5", false, 0, items.Ref{Slot: 0}, false, 5, 0, 1},
		{"two failed with charms", true, 2, items.Ref{Slot: 9}, true, 100, 0, 2},
		{"capped at 3", true, 2, items.Ref{Slot: 9}, true, 100, 2, 3},
		{"refused", false, 0, items.Ref{Slot: 60}, false, 5, 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			inv := forgeInventory()
			if c.charms > 0 {
				inv.Bag[10] = &items.Item{Code: gamedata.CharmProtect, Count: c.charms}
			}
			z, p, sv := forgeZone(t, inv, false)
			if c.fail {
				z.rng = rand.New(failSource{})
			}
			z.area.Quests = testQuestData
			p.quests = quest.Log{
				"d_enhance": {State: quest.Active, Progress: c.start, Day: z.day},
				"d_kill":    {State: quest.Active, Day: z.day},
			}
			out := p.out.(*fakeOut)
			out.take()
			itemNow(t, z, z.Enhance(1, 9, c.ref, c.protect, c.times))
			envs := out.take()
			if got := p.quests["d_enhance"].Progress; got != c.want || p.quests["d_kill"].Progress != 0 {
				t.Fatalf("log %v, want d_enhance at %d", p.quests, c.want)
			}
			qp := questProgressIn(envs)
			if c.want == c.start {
				if len(qp) != 0 {
					t.Fatalf("QuestProgress %v, want none", qp)
				}
				return
			}
			wantQ := &pb.QuestEntry{QuestId: "d_enhance", State: pb.QuestState_QUEST_STATE_ACTIVE, Progress: int32(c.want)}
			if len(qp) != 1 || len(qp[0].Quests) != 1 || !proto.Equal(qp[0].Quests[0], wantQ) {
				t.Fatalf("QuestProgress %v, want d_enhance at %d", qp, c.want)
			}
			saves := sv.all()
			i := slices.IndexFunc(saves[len(saves)-1].Quests, func(r store.QuestRow) bool { return r.QuestID == "d_enhance" })
			if i < 0 || saves[len(saves)-1].Quests[i].Progress != c.want {
				t.Fatalf("save %+v, want d_enhance at %d", saves[len(saves)-1].Quests, c.want)
			}
		})
	}
}

// kill_any counts a monster within 10 levels of the killer only: a level-1
// monster counts for a level-11 killer, not for a level-20 one.
func TestDailyKillAnyLevelWindow(t *testing.T) {
	for _, c := range []struct {
		level, want int
	}{{11, 1}, {20, 0}} {
		z, m, _ := questKillZone(t, nil)
		p := z.players[1]
		p.level = c.level
		p.quests = quest.Log{"d_kill": {State: quest.Active, Day: z.day}}
		attackLand(t, z, 1, 1, m.id)
		if !m.dead || p.quests["d_kill"].Progress != c.want {
			t.Fatalf("level %d: dead %v log %v, want d_kill at %d", c.level, m.dead, p.quests, c.want)
		}
	}
}

// A daily turn-in pays by the level at the turn-in: d_enhance at level 12
// gives 5 stones and 480 Xu and no EXP, d_kill at level 30 gives 545,080
// EXP and 2,400 Xu. QuestTurnInResp tells the amounts paid. The done entry
// keeps its day, so it cannot be taken again that day.
func TestDailyTurnInRewards(t *testing.T) {
	clock := &fakeClock{beforeMidnight.Add(-time.Hour)}
	day := quest.Day(clock.t)
	z, p, out, sv := dailyZone(t, 12, quest.Log{"d_enhance": {State: quest.Active, Progress: 3, Day: day}}, clock)
	itemNow(t, z, z.QuestTurnIn(1, 9, "qua_truyen_tin", "d_enhance"))
	r := respFor(out.take(), 9, (*pb.Envelope).GetQuestTurnInResp)
	if r == nil || r.Exp != 0 || r.Xu != 480 || p.xu != 77+480 || p.exp != 0 {
		t.Fatalf("resp %v xu %d exp %d, want 480 Xu and no EXP", r, p.xu, p.exp)
	}
	if !reflect.DeepEqual(p.inv.Bag[0], &items.Item{Code: "stone_enhance", Count: 5}) {
		t.Fatalf("bag slot 0 %+v, want 5 stones", p.inv.Bag[0])
	}
	if e := p.quests["d_enhance"]; e != (quest.Entry{State: quest.Done, Progress: 3, Day: day}) {
		t.Fatalf("entry %+v, want done on day %d", e, day)
	}
	if saves := sv.all(); len(saves) != 1 || saves[0].Xu != 557 || saves[0].Quests[0].Day != day {
		t.Fatalf("saves %+v", saves)
	}
	if got := questErr(t, z, out, accept("qua_truyen_tin", "d_enhance")); got != pb.ErrorCode_ERROR_CODE_QUEST_NOT_AVAILABLE {
		t.Fatalf("accept again: %v", got)
	}

	z, p, out, _ = dailyZone(t, 30, quest.Log{"d_kill": {State: quest.Active, Progress: 40, Day: day}}, clock)
	itemNow(t, z, z.QuestTurnIn(1, 9, "qua_truyen_tin", "d_kill"))
	r = respFor(out.take(), 9, (*pb.Envelope).GetQuestTurnInResp)
	if r == nil || r.Exp != 545_080 || r.Xu != 2_400 || p.exp != 545_080 || p.xu != 77+2_400 || p.level != 30 {
		t.Fatalf("resp %v exp %d xu %d level %d, want 545,080 EXP and 2,400 Xu", r, p.exp, p.xu, p.level)
	}
}

// A bag that cannot take the stones refuses d_enhance with BAG_FULL and
// pays nothing.
func TestDailyTurnInBagFull(t *testing.T) {
	runQuestErrors(t, []questCase{{name: "bag full", mapID: "ubuyashiki", level: 12,
		setup: func(z *Zone, p *player) {
			p.quests["d_enhance"] = quest.Entry{State: quest.Active, Progress: 3, Day: z.day}
			fillBag(p.inv)
		},
		cmd: turnIn("qua_truyen_tin", "d_enhance"), want: pb.ErrorCode_ERROR_CODE_BAG_FULL}})
}
