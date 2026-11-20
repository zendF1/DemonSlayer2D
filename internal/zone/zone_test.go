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
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/pb"
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
	MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8, MeleeWindupMS: 200, ShotSpeed: 15}

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
		Grid:    openGrid(60, 60),
		Safe:    []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Rests:   []Circle{{C: Vec2{3.5, 3.5}, R: 1.5}},
		Respawn: Vec2{5.5, 5.5},
		Regions: []Region{{ID: "r1", Tiles: tiles, Nest: tiles[0], Count: 3, MinLevel: 1, MaxLevel: 2, Kind: testKind}},
		Levels:  testLevels,
		Exp:     testExp,
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
		Grid:    openGrid(60, 60),
		Safe:    []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Respawn: Vec2{5.5, 5.5},
		Regions: regions,
		Levels:  testLevels,
		Exp:     testExp,
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
				z.damagePlayer(p, 0, 1<<30, false)
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
// an all-zero pity saves as none.
func TestStateSavesItemsAndXu(t *testing.T) {
	z, p, _, _ := gearZone(t, gearInventory())
	p.xu = 1234
	p.inv.Worn[0].Enhance, p.inv.Worn[0].Pity[7], p.inv.Worn[0].Pity[14] = 8, 4, 90
	st := z.state(p)
	want := []store.ItemRow{
		{Slot: 0, Code: "eq10_boots", Count: 1, Quality: 2, Element: "thuy",
			Lines:  []store.ItemLine{{Stat: "hp", Value: 43}, {Stat: "phys_defense", Value: 3}, {Stat: "magic_attack", Value: 4}},
			Hidden: []store.ItemLine{{Stat: "hp", Value: 86}, {Stat: "phys_attack", Value: 5}}},
		{Slot: 3, Code: "stone_enhance", Count: 12, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
		{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1, Enhance: 8, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{},
			Pity: []int{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 90}},
	}
	if st.Xu != 1234 || !reflect.DeepEqual(st.Items, want) {
		t.Fatalf("saved xu %d items %+v\nwant %+v", st.Xu, st.Items, want)
	}
	// Changing the saved copy leaves the player alone, and the other way round.
	st.Items[0].Lines[0].Value, st.Items[0].Hidden[1].Stat, st.Items[2].Pity[7] = 1, "mp", 5
	if p.inv.Bag[0].Lines[0].Value != 43 || p.inv.Bag[0].Hidden[1].Stat != "phys_attack" || p.inv.Worn[0].Pity[7] != 4 {
		t.Fatal("the saved rows share lines with the player")
	}
	st = z.state(p)
	p.inv.Bag[0].Lines[1].Value, p.inv.Bag[3].Count = 9, 13
	if st.Items[0].Lines[1].Value != 3 || st.Items[1].Count != 12 {
		t.Fatal("the player shares lines or items with the saved rows")
	}
}

// inventoryUpdate lists the asked slots as they are; an empty one has no item.
func TestInventoryUpdate(t *testing.T) {
	inv := gearInventory()
	got := inventoryUpdate(inv, []int{3, 1}, []int{0, 9})
	want := &pb.InventoryUpdate{
		Bag:      []*pb.ItemSlot{{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 12}}, {Slot: 1}},
		Equipped: []*pb.ItemSlot{{Slot: 0, Item: &pb.ItemInfo{Code: "eq10_weapon_viem", Count: 1}}, {Slot: 9}},
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
				z.damagePlayer(p, 0, 1<<30, false)
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
				z.damagePlayer(p, 0, 1<<30, false)
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
// standing inside a safe zone unless outside is set.
func forgeZone(t *testing.T, inv *items.Inventory, outside bool) (*Zone, *player, *fakeSaver) {
	t.Helper()
	z, p, _, sv := gearZoneAt(t, 19, inv)
	if !outside {
		z.area.Safe = []Rect{{X: 0, Y: 0, W: 10, H: 10}}
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
		notSafe   = pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE
		bad       = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
		notFound  = pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
		maxed     = pb.ErrorCode_ERROR_CODE_ENHANCE_MAXED
		materials = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_MATERIALS
		noXu      = pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU
	)
	for _, c := range []struct {
		name          string
		dead, outside bool
		setup         func(*player)
		ref           items.Ref
		protect       bool
		times         int
		code          pb.ErrorCode
	}{
		{"dead outside the safe zone, bad slot", true, true, broke, bag(60), false, 101, dead},
		{"outside the safe zone, bad slot", false, true, broke, bag(60), false, 101, notSafe},
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
			z, p, sv := forgeZone(t, forgeInventory(), c.outside)
			out := p.out.(*fakeOut)
			c.setup(p)
			if c.dead {
				z.damagePlayer(p, 0, 1<<30, false)
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
	z.damagePlayer(p, 0, 1<<30, false)
	advanceSec(z, 3)
	if !p.dead || p.hp != 0 || p.mp != 0 {
		t.Fatalf("dead player at HP %d MP %d, want 0 and 0", p.hp, p.mp)
	}

	z, p, _, _ = regenZone(t, pb.CharClass_CHAR_CLASS_THUY, 19, 0, fullForge())
	p.hp, p.mp = 1000, 0
	n := 2 * int(z.ticks(1))
	for range n {
		z.damagePlayer(p, 0, 1, false)
		z.advance(time.Now())
	}
	gain := 2 * p.maxHP * 150 / 10000
	if p.hp != 1000-n+gain || p.mp != 50 {
		t.Fatalf("HP %d MP %d after 2 s of hits, want %d and 50", p.hp, p.mp, 1000-n+gain)
	}
}
