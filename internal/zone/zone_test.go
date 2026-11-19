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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"demonslayer2d/internal/gamedata"
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
	return New(testConfig(capacity), Area{Grid: g}, sv, quietLog()), sv
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
	z := New(cfg, Area{Grid: openGrid(64, 64)}, sv, quietLog())
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

var testKind = gamedata.Kind{ID: "wanderer", Name: "Quỷ lang thang", AttackRange: 1.5, AttackSec: 1.5, Speed: 0.8,
	AggroRange: 4, LeashRange: 10, RespawnSec: 8}

var testLevels = map[int]gamedata.Level{
	1: {Level: 1, HP: 56, Attack: 30, Defense: 8},
	2: {Level: 2, HP: 104, Attack: 39, Defense: 16},
}

// newMonsterZone builds a zone on a 60x60 open grid with the safe rect (0,0,10,10)
// and one camp of 3 monsters on the given tiles.
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
		Camps:   []Camp{{ID: "c1", Tiles: tiles, Count: 3, MinLevel: 1, MaxLevel: 2, Kind: testKind}},
		Levels:  testLevels,
	}
	return New(cfg, area, &fakeSaver{}, quietLog()), ids
}

var campTiles = []Vec2{{30.5, 30.5}, {31.5, 30.5}, {30.5, 31.5}}

func dist(a, b Vec2) float64 { return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) }

func TestSpawnFillsCamps(t *testing.T) {
	z, ids := newMonsterZone(t, campTiles...)
	if len(z.monsters) != 3 {
		t.Fatalf("monsters = %d, want 3", len(z.monsters))
	}
	got := slices.Sorted(maps.Keys(z.monsters))
	if !slices.Equal(got, ids.issued) {
		t.Fatalf("monster ids %v, want the ids NewID issued %v", got, ids.issued)
	}
	for id, m := range z.monsters {
		if m.id != id || m.kind != pb.EntityKind_ENTITY_KIND_MONSTER || m.name != testKind.Name {
			t.Fatalf("monster %d: bad identity %+v", id, m.entity)
		}
		if !slices.Contains(campTiles, m.pos) || m.home != m.pos {
			t.Fatalf("monster %d at %v (home %v), want on a camp tile", id, m.pos, m.home)
		}
		if m.level < 1 || m.level > 2 {
			t.Fatalf("monster %d level %d out of range", id, m.level)
		}
		if m.maxHP != testLevels[m.level].HP || m.hp != m.maxHP || m.dead {
			t.Fatalf("monster %d hp %d/%d dead=%v, want full %d", id, m.hp, m.maxHP, m.dead, testLevels[m.level].HP)
		}
		if _, in := z.cells[m.cell][id]; !in {
			t.Fatalf("monster %d is not in its AOI cell", id)
		}
	}
}

func TestPlayerSeesMonsters(t *testing.T) {
	z, _ := newMonsterZone(t, campTiles...)
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

func TestMonstersWander(t *testing.T) {
	z, _ := newMonsterZone(t, campTiles...)
	a := joinNow(t, z, 1, 30.5, 39.5) // in view, beyond aggro range plus the wander radius
	home := map[uint64]Vec2{}
	for id, m := range z.monsters {
		home[id] = m.home
	}
	step := 0.5 * testKind.Speed * 4 / 10
	moved := map[uint64]bool{}
	broadcast := map[uint64]bool{}
	prev := maps.Clone(home)
	now := time.Now()
	for i := 0; i < 300; i++ { // 30 s: several wander cycles of 4-8 s
		z.advance(now)
		for _, mv := range lastTick(a.take()).GetMoves() {
			broadcast[mv.Id] = true
		}
		for id, m := range z.monsters {
			if m.pos != home[id] {
				moved[id] = true
			}
			if d := dist(m.pos, prev[id]); d > step+1e-4 {
				t.Fatalf("tick %d: monster %d moved %.4f tiles, faster than the wander step %.4f", i, id, d, step)
			}
			prev[id] = m.pos
			if d := dist(m.pos, home[id]); d > 2+step/2 {
				t.Fatalf("tick %d: monster %d is %.3f tiles from home", i, id, d)
			}
		}
	}
	for id := range home {
		if !moved[id] || !broadcast[id] {
			t.Fatalf("monster %d moved=%v broadcast=%v, want both", id, moved[id], broadcast[id])
		}
	}
}

func TestMonstersNeverEnterSafeZone(t *testing.T) {
	// Camp tiles border the safe rect (0,0,10,10) on its right edge.
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

// A wall right next to home: walks towards tiles behind it slide along the wall.
// Each walk must end soon, and a walking monster must really move every tick.
func TestWanderStopsAtWalls(t *testing.T) {
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		if y >= 25 && y <= 36 {
			r[32] = '#'
		}
		rows[y] = string(r)
	}
	z, _ := newMonsterZone(t, Vec2{31.5, 30.5}, Vec2{31.5, 29.5}, Vec2{31.5, 31.5})
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	step := 0.5 * testKind.Speed * 4 / 10
	prev := map[uint64]Vec2{}
	walk := map[uint64]int{}
	walks := 0
	now := time.Now()
	for id, m := range z.monsters {
		prev[id] = m.pos
	}
	for i := 0; i < 2000; i++ {
		z.advance(now)
		for id, m := range z.monsters {
			if m.dir == (Vec2{}) {
				if walk[id] > 0 {
					walks++
				}
				walk[id] = 0
			} else {
				if d := dist(m.pos, prev[id]); d < 0.1*step-1e-4 {
					t.Fatalf("tick %d: monster %d walks (dir %v) but moved only %.5f tiles", i, id, m.dir, d)
				}
				if walk[id]++; walk[id] > 30 {
					t.Fatalf("tick %d: monster %d has been walking for %d ticks", i, id, walk[id])
				}
			}
			prev[id] = m.pos
		}
	}
	if walks < 20 {
		t.Fatalf("only %d wander walks finished", walks)
	}
}

func TestWalkToResults(t *testing.T) {
	z, _ := newMonsterZone(t, campTiles...)
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
	m := &monster{entity: entity{pos: Vec2{10.5, 5.5}}, home: Vec2{10.5, 5.5}}
	for i := 0; i < 1000; i++ {
		to, ok := z.wanderTarget(m)
		if !ok || z.area.InSafe(to) || dist(to, m.home) > wanderRadius {
			t.Fatalf("wander target %v ok=%v, want a non-safe tile within 2 of home", to, ok)
		}
	}
	if p := Step(z.mgrid, Vec2{11.5, 5.5}, Vec2{-1, 0}, 5); p.X != 10+halfSize {
		t.Fatalf("a monster walking into the safe rect stops at %v, want x=%v", p, 10+halfSize)
	}
}

func TestDeadMonsterIsNotNewlyEntered(t *testing.T) {
	z, _ := newMonsterZone(t, campTiles...)
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
