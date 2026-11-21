package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/app"
	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/client"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/config"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

type server struct {
	ws, http string
	app      *app.App
	store    *store.Store
}

func startServer(t *testing.T) server {
	t.Helper()
	return startServerData(t, "../data")
}

// startServerData starts a server that reads its game data from dataDir.
func startServerData(t *testing.T, dataDir string) server {
	t.Helper()
	st := storetest.New(t)
	cfg := config.Config{
		MapsDir: "../maps", DataDir: dataDir, StartMap: "kumotori_1", TickRate: 10, MoveSpeedTiles: 4,
		AOICellSize: 10, ZoneCapacity: 50, SaveInterval: time.Minute, LogLevel: slog.LevelInfo,
		RegisterPerHour: 100,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.New(context.Background(), cfg, log, app.Options{
		Store:      st,
		AuthParams: &auth.Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.Start()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		srv.Close()
	})
	return server{ws: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws", http: srv.URL, app: a, store: st}
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func dial(t *testing.T, s server) *client.Client {
	t.Helper()
	c, err := client.Dial(ctxT(t), s.ws)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

type player struct {
	c         *client.Client
	accountID int64
	charID    int64
	enter     *pb.EnterWorldResp
}

func newPlayer(t *testing.T, s server, username, charName string, class pb.CharClass) player {
	t.Helper()
	ctx := ctxT(t)
	c := dial(t, s)
	if err := c.Register(ctx, username, "password123"); err != nil {
		t.Fatal(err)
	}
	accountID, err := c.Login(ctx, username, "password123")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := c.CreateChar(ctx, 1, charName, class)
	if err != nil {
		t.Fatal(err)
	}
	ew, err := c.EnterWorld(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if ew.TickRate != 10 {
		t.Fatalf("tick_rate = %d, want 10", ew.TickRate)
	}
	return player{c: c, accountID: accountID, charID: ch.Id, enter: ew}
}

func waitFor(t *testing.T, c *client.Client, d time.Duration, match func(*pb.Envelope) bool) *pb.Envelope {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		select {
		case env := <-c.Pushes():
			if match(env) {
				return env
			}
		case <-c.Closed():
			for {
				select {
				case env := <-c.Pushes():
					if match(env) {
						return env
					}
				default:
					t.Fatalf("connection closed before a matching message: %v", c.Err())
					return nil
				}
			}
		case <-deadline.C:
			t.Fatalf("no matching message within %v", d)
			return nil
		}
	}
}

func enterOf(id uint64) func(*pb.Envelope) bool {
	return func(e *pb.Envelope) bool { return e.GetEntityEnter().GetId() == id }
}

func isKicked(e *pb.Envelope) bool { return e.GetKicked() != nil }

func TestTwoPlayersSeeEachOtherMove(t *testing.T) {
	s := startServer(t)
	alice := newPlayer(t, s, "alice01", "Alice", pb.CharClass_CHAR_CLASS_VIEM)
	bob := newPlayer(t, s, "bob01", "Bob", pb.CharClass_CHAR_CLASS_THUY)

	waitFor(t, bob.c, 2*time.Second, enterOf(alice.enter.SelfEntityId))
	waitFor(t, alice.c, 2*time.Second, enterOf(bob.enter.SelfEntityId))

	start := time.Now()
	if _, err := alice.c.Move(ctxT(t), 1, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, bob.c, 2*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == alice.enter.SelfEntityId && m.X > alice.enter.X {
				return true
			}
		}
		return false
	})
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("alice's move reached bob after %v, want <= 200ms", elapsed)
	}
}

func TestDuplicateLoginKicksOldSession(t *testing.T) {
	s := startServer(t)
	first := newPlayer(t, s, "carol01", "Carol", pb.CharClass_CHAR_CLASS_MINH)

	second := dial(t, s)
	if _, err := second.Login(ctxT(t), "carol01", "password123"); err != nil {
		t.Fatal(err)
	}
	k := waitFor(t, first.c, 2*time.Second, isKicked)
	if k.GetKicked().Reason != pb.KickReason_KICK_REASON_DUPLICATE_LOGIN {
		t.Fatalf("reason = %v", k.GetKicked().Reason)
	}
	select {
	case <-first.c.Closed():
	case <-time.After(5 * time.Second):
		t.Fatal("old connection was not closed")
	}
	chars, err := second.ListChars(ctxT(t))
	if err != nil || len(chars) != 1 {
		t.Fatalf("chars = %v, %v", chars, err)
	}
}

func TestLeaveAndReenterRestoresPosition(t *testing.T) {
	s := startServer(t)
	p := newPlayer(t, s, "dave01", "Dave", pb.CharClass_CHAR_CLASS_NHAM)
	ctx := ctxT(t)
	last := walkRightAndStop(t, p)

	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(again.X-last)) > 1e-3 || again.Y != p.enter.Y {
		t.Fatalf("re-entered at (%v, %v), want (%v, %v)", again.X, again.Y, last, p.enter.Y)
	}
}

// walkRightAndStop moves p at least one tile to the right, stops, and returns the
// x position the server reported once it stopped.
func walkRightAndStop(t *testing.T, p player) float32 {
	t.Helper()
	self := p.enter.SelfEntityId
	ctx := ctxT(t)
	if _, err := p.c.Move(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, p.c, 3*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == self && m.X >= p.enter.X+1 {
				return true
			}
		}
		return false
	})
	if _, err := p.c.Move(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	var last float32
	waitFor(t, p.c, 2*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == self && m.DirX == 0 && m.DirY == 0 {
				last = m.X
				return true
			}
		}
		return false
	})
	return last
}

// The saver must still flush when the gateway has used up the shutdown deadline,
// otherwise every player still connected at shutdown loses their position.
func TestShutdownFlushesSavesAfterGatewayTimeout(t *testing.T) {
	s := startServer(t)
	p := newPlayer(t, s, "gina01", "Gina", pb.CharClass_CHAR_CLASS_VIEM)
	last := walkRightAndStop(t, p)

	expired, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.app.Shutdown(expired)
	if err == nil || !strings.Contains(err.Error(), "gateway") {
		t.Fatalf("Shutdown = %v, want only the gateway wait to time out", err)
	}
	if strings.Contains(err.Error(), "saver") {
		t.Fatalf("Shutdown = %v, the saver must not time out", err)
	}
	ch, err := s.store.GetCharacter(context.Background(), p.accountID, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(ch.X-last)) > 1e-3 {
		t.Fatalf("saved x = %v, want %v: the position was lost during shutdown", ch.X, last)
	}
}

func TestMalformedPacketKicks(t *testing.T) {
	s := startServer(t)
	c := dial(t, s)
	if err := c.SendRaw(ctxT(t), []byte{0xff, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	k := waitFor(t, c, 2*time.Second, isKicked)
	if k.GetKicked().Reason != pb.KickReason_KICK_REASON_PROTOCOL_ERROR {
		t.Fatalf("reason = %v", k.GetKicked().Reason)
	}
}

func TestRequestErrors(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	expect := func(err error, want pb.ErrorCode) {
		t.Helper()
		if got := client.Code(err); got != want {
			t.Fatalf("error code = %v (%v), want %v", got, err, want)
		}
	}

	c := dial(t, s)
	_, err := c.ListChars(ctx)
	expect(err, pb.ErrorCode_ERROR_CODE_BAD_REQUEST) // not logged in
	if err := c.Register(ctx, "erin01", "password123"); err != nil {
		t.Fatal(err)
	}
	expect(c.Register(ctx, "ERIN01", "password123"), pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN)
	expect(c.Register(ctx, "x", "password123"), pb.ErrorCode_ERROR_CODE_USERNAME_INVALID)
	_, err = c.LoginVersion(ctx, "erin01", "password123", 1)
	expect(err, pb.ErrorCode_ERROR_CODE_VERSION_MISMATCH)
	_, err = c.Login(ctx, "erin01", "wrongpass")
	expect(err, pb.ErrorCode_ERROR_CODE_AUTH_FAILED)
	if _, err := c.Login(ctx, "erin01", "password123"); err != nil {
		t.Fatal(err)
	}
	ch, err := c.CreateChar(ctx, 1, "Erin", pb.CharClass_CHAR_CLASS_PHONG)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateChar(ctx, 1, "Other", pb.CharClass_CHAR_CLASS_VIEM)
	expect(err, pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED)
	_, err = c.CreateChar(ctx, 2, "có space", pb.CharClass_CHAR_CLASS_VIEM)
	expect(err, pb.ErrorCode_ERROR_CODE_NAME_INVALID)
	_, err = c.CreateChar(ctx, 4, "Fine", pb.CharClass_CHAR_CLASS_VIEM)
	expect(err, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
	_, err = c.EnterWorld(ctx, 999999)
	expect(err, pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND)
	expect(c.LeaveWorld(ctx), pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD)
	expect(c.Attack(ctx, 1), pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD)
	_, err = c.AllocatePoints(ctx, combat.Points{Str: 1})
	expect(err, pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD)
	_, err = c.Respawn(ctx, false)
	expect(err, pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD)
	expect(c.DeleteChar(ctx, ch.Id, "Wrong"), pb.ErrorCode_ERROR_CODE_BAD_REQUEST)

	other := dial(t, s)
	if err := other.Register(ctx, "frank01", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Login(ctx, "frank01", "password123"); err != nil {
		t.Fatal(err)
	}
	_, err = other.CreateChar(ctx, 1, "ERIN", pb.CharClass_CHAR_CLASS_THUY)
	expect(err, pb.ErrorCode_ERROR_CODE_NAME_TAKEN)

	if _, err := c.EnterWorld(ctx, ch.Id); err != nil {
		t.Fatal(err)
	}
	_, err = c.EnterWorld(ctx, ch.Id)
	expect(err, pb.ErrorCode_ERROR_CODE_ALREADY_IN_WORLD)
	if _, err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteChar(ctx, ch.Id, "Erin"); err != nil {
		t.Fatal(err)
	}
}

func TestHealthz(t *testing.T) {
	s := startServer(t)
	resp, err := http.Get(s.http + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

// ______________________TEST COMBAT_____________________

// nestR1 is the nest of region r1 on kumotori_1: wanderers of level 1-2 east
// of the safe zone.
var nestR1 = [2]float32{22.5, 10.5}

func TestKillMonsterOverWebSocket(t *testing.T) {
	s := startServer(t)
	p := newPlayer(t, s, "hana01", "Hana", pb.CharClass_CHAR_CLASS_VIEM)
	if p.enter.Stats.GetExp() != 0 {
		t.Fatalf("a new character starts with exp %d", p.enter.Stats.GetExp())
	}

	deaths := make(map[uint64]bool)
	var stats *pb.SelfStats
	f := client.NewFighter(p.c, p.enter)
	f.Seek = nestR1
	f.Observe = func(env *pb.Envelope) {
		for _, d := range env.GetWorldTick().GetDeaths() {
			deaths[d.Id] = true
		}
		if st := env.GetSelfStats(); st != nil {
			stats = st
		}
	}
	var kills []client.Kill
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := f.Fight(ctx, 1, func(k client.Kill) { kills = append(kills, k) }); err != nil {
		t.Fatalf("fight: %v (kills %v)", err, kills)
	}
	if len(kills) != 1 {
		t.Fatalf("kills = %v, want 1", kills)
	}
	k := kills[0]
	if !deaths[k.ID] {
		t.Fatalf("no Death for monster %d; deaths %v", k.ID, deaths)
	}
	if stats.GetExp() <= 0 || k.Exp != stats.GetExp() {
		t.Fatalf("SelfStats exp = %d, kill exp = %d: want the same value > 0", stats.GetExp(), k.Exp)
	}
}

// Combat progress must survive a logout. The character is seeded at one EXP
// short of level 4 with free points, spends some over the websocket, levels
// up on the first kill and leaves; re-entering must bring back its level,
// EXP, points and HP exactly as the stream last showed them.
func TestCombatProgressSurvivesLogout(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	c := dial(t, s)
	if err := c.Register(ctx, "kira01", "password123"); err != nil {
		t.Fatal(err)
	}
	accountID, err := c.Login(ctx, "kira01", "password123")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := c.CreateChar(ctx, 1, "Kira", pb.CharClass_CHAR_CLASS_VIEM)
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.store.GetCharacter(ctx, accountID, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := gamedata.Load("../data")
	if err != nil {
		t.Fatal(err)
	}
	const seedLevel, seedFree = 3, 20
	seedExp := data.Exp.ToNext(seedLevel) - 1
	if err := s.store.SaveCharacterState(ctx, store.CharState{ID: row.ID, MapID: row.MapID, X: row.X, Y: row.Y,
		Level: seedLevel, Exp: seedExp, FreePoints: seedFree, HP: -1, MP: -1}); err != nil {
		t.Fatal(err)
	}
	ew, err := c.EnterWorld(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if st := ew.Stats; st.GetLevel() != seedLevel || st.GetExp() != seedExp || st.GetFreePoints() != seedFree {
		t.Fatalf("entered with %v, want the seeded level %d exp %d free points %d", st, seedLevel, seedExp, seedFree)
	}

	spend := combat.Points{Str: 5, Agi: 2, Vit: 4, Int: 1}
	last, err := c.AllocatePoints(ctx, spend)
	if err != nil {
		t.Fatal(err)
	}
	if last.GetFreePoints() != seedFree-int32(spend.Total()) || last.GetStrength() != 5 || last.GetVitality() != 4 {
		t.Fatalf("after allocating %v: %v", spend, last)
	}

	// Track our own stats and HP in stream order. A hit we take moves HP
	// without a SelfStats push, so Hit and HpChange count too.
	self := ew.SelfEntityId
	hp := last.GetHp()
	observe := func(env *pb.Envelope) {
		if wt := env.GetWorldTick(); wt != nil {
			for _, h := range wt.Hits {
				if h.TargetId == self {
					hp = h.HpAfter
				}
			}
			for _, hc := range wt.HpChanges {
				if hc.Id == self {
					hp = hc.Hp
				}
			}
		}
		if st := env.GetSelfStats(); st != nil {
			last, hp = st, st.GetHp()
		}
	}
	f := client.NewFighter(c, ew)
	f.Seek = nestR1
	f.Observe = observe
	fctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := f.Fight(fctx, 1, nil); err != nil {
		t.Fatalf("fight: %v", err)
	}
	if last.GetLevel() != seedLevel+1 || last.GetFreePoints() != seedFree-int32(spend.Total())+combat.PointsPerLevel {
		t.Fatalf("after the kill: %v, want level %d with 10 more free points", last, seedLevel+1)
	}

	if err := c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	// Every push from the zone came before the LeaveWorldResp on the wire.
	for drained := false; !drained; {
		select {
		case env := <-c.Pushes():
			observe(env)
		default:
			drained = true
		}
	}
	again, err := c.EnterWorld(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	got := again.Stats
	if got.GetLevel() != last.GetLevel() || got.GetExp() != last.GetExp() || got.GetFreePoints() != last.GetFreePoints() ||
		got.GetStrength() != last.GetStrength() || got.GetAgility() != last.GetAgility() ||
		got.GetVitality() != last.GetVitality() || got.GetIntellect() != last.GetIntellect() ||
		got.GetMaxHp() != last.GetMaxHp() || got.GetHp() != hp {
		t.Fatalf("re-entered with %v\nwant level %d exp %d free %d points %d/%d/%d/%d hp %d/%d", got,
			last.GetLevel(), last.GetExp(), last.GetFreePoints(), last.GetStrength(), last.GetAgility(),
			last.GetVitality(), last.GetIntellect(), hp, last.GetMaxHp())
	}
	if got.GetExp() <= 0 {
		t.Fatalf("re-entered with exp %d, want the EXP earned past level %d", got.GetExp(), seedLevel+1)
	}
}

// ______________________TEST SKILLS_____________________

// A character raised to level 20 with DevSetLevel learns its slot 1 and 2
// skills, walks up to a monster and casts slot 1 on it: the cast is answered,
// costs MP, lands a Hit carrying its skill id, and a second cast right after
// is ON_COOLDOWN.
func TestSkillsOverWebSocket(t *testing.T) {
	s := startServer(t)
	p := newPlayer(t, s, "suki01", "Suki", pb.CharClass_CHAR_CLASS_THUY)
	ctx := ctxT(t)
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	// Drop the first stay's pushes, which all came before the LeaveWorldResp.
	for drained := false; !drained; {
		select {
		case <-p.c.Pushes():
		default:
			drained = true
		}
	}
	res, err := s.store.DevSetLevel(ctx, "suki01", 20)
	if err != nil || res.Level != 20 || res.SkillPoints != 19 {
		t.Fatalf("DevSetLevel: %+v, %v", res, err)
	}
	ew, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if st := ew.Stats; st.GetLevel() != 20 || st.GetSkillPoints() != 19 || st.GetFreePoints() != 190 {
		t.Fatalf("entered with %v, want level 20, 19 skill points and 190 free points", st)
	}

	const skill = "thuy_1" // slot 1: single target, range 5
	var stats *pb.SelfStats
	for _, id := range []string{skill, "thuy_2"} { // slots 1 and 2
		r, err := p.c.Learn(ctx, id)
		if err != nil {
			t.Fatalf("learn %s: %v", id, err)
		}
		stats = r.Stats
	}
	if stats.GetSkillPoints() != 17 {
		t.Fatalf("after learning two skills: %v, want 17 skill points", stats)
	}

	// The fighter walks towards the nest and then the nearest monster; stop
	// it once the character is out of the safe zone and a live monster is in
	// cast range but not yet in slash range.
	loaded, err := mapdata.LoadDir("../maps")
	if err != nil {
		t.Fatal(err)
	}
	world := loaded[ew.MapId]
	self := ew.SelfEntityId
	x, y := ew.X, ew.Y
	type seen struct {
		x, y float32
		dead bool
	}
	monsters := make(map[uint64]*seen)
	var target uint64
	near := func() uint64 {
		if world.InSafeZone(mapdata.Point{X: x, Y: y}) {
			return 0
		}
		var best uint64
		bestD := float32(3)
		for id, m := range monsters {
			if d := float32(math.Hypot(float64(m.x-x), float64(m.y-y))); !m.dead && d <= bestD && d > combat.SlashRange+0.5 {
				best, bestD = id, d
			}
		}
		return best
	}
	fctx, stop := context.WithTimeout(context.Background(), 60*time.Second)
	defer stop()
	f := client.NewFighter(p.c, ew)
	f.Seek = nestR1
	f.Observe = func(env *pb.Envelope) {
		if e := env.GetEntityEnter(); e != nil && e.Kind == pb.EntityKind_ENTITY_KIND_MONSTER {
			monsters[e.Id] = &seen{x: e.X, y: e.Y, dead: e.Dead}
		}
		if l := env.GetEntityLeave(); l != nil {
			delete(monsters, l.Id)
		}
		if st := env.GetSelfStats(); st != nil {
			stats = st
		}
		wt := env.GetWorldTick()
		for _, m := range wt.GetMoves() {
			if m.Id == self {
				x, y = m.X, m.Y
			} else if s := monsters[m.Id]; s != nil {
				s.x, s.y = m.X, m.Y
			}
		}
		for _, d := range wt.GetDeaths() {
			if s := monsters[d.Id]; s != nil {
				s.dead = true
			}
		}
		if target == 0 {
			if target = near(); target != 0 {
				stop()
			}
		}
	}
	if err := f.Fight(fctx, 1000, nil); target == 0 {
		t.Fatalf("no monster came within cast range: %v", err)
	}
	if _, err := p.c.Move(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}

	mpBefore := stats.GetMp()
	if mpBefore != stats.GetMaxMp() || mpBefore <= 0 {
		t.Fatalf("stats before the cast %v, want full MP", stats)
	}
	if err := p.c.Cast(ctx, skill, target, 0, 0); err != nil {
		t.Fatalf("cast %s on %d: %v", skill, target, err)
	}
	var spent *pb.SelfStats
	var hit *pb.Hit
	waitFor(t, p.c, 3*time.Second, func(e *pb.Envelope) bool {
		if st := e.GetSelfStats(); st != nil && st.GetMp() < mpBefore {
			spent = st
		}
		for _, h := range e.GetWorldTick().GetHits() {
			if h.AttackerId == self && h.SkillId == skill {
				hit = h
			}
		}
		return spent != nil && hit != nil
	})
	if hit.TargetId != target || hit.Damage <= 0 {
		t.Fatalf("hit %v, want damage on monster %d", hit, target)
	}

	if err := p.c.Cast(ctx, skill, target, 0, 0); client.Code(err) != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("second cast: %v, want ON_COOLDOWN", err)
	}
}

// AttackReq and CastReq are each limited to 5 per second with a burst of 5,
// so the 6th of six written back to back is refused by the gateway. The first
// five reach the zone, which may refuse them for its own reasons; only the
// 6th must be RATE_LIMITED.
func TestAttackRateLimit(t *testing.T) {
	sixthRateLimited(t, "ivan01", "Ivan", func(seq uint32) *pb.Envelope {
		return &pb.Envelope{Seq: seq, Payload: &pb.Envelope_AttackReq{AttackReq: &pb.AttackReq{TargetId: 1}}}
	})
}

func TestCastRateLimit(t *testing.T) {
	sixthRateLimited(t, "ivan02", "Ivanka", func(seq uint32) *pb.Envelope {
		return &pb.Envelope{Seq: seq, Payload: &pb.Envelope_CastReq{CastReq: &pb.CastReq{SkillId: "viem_1", TargetId: 1}}}
	})
}

// sixthRateLimited writes six requests from req back to back and checks that
// only the 6th is RATE_LIMITED. Raw seqs are used so the replies can be
// matched by seq.
func sixthRateLimited(t *testing.T, username, charName string, req func(seq uint32) *pb.Envelope) {
	t.Helper()
	s := startServer(t)
	p := newPlayer(t, s, username, charName, pb.CharClass_CHAR_CLASS_VIEM)
	ctx := ctxT(t)
	const first, n = 1001, 6
	for seq := uint32(first); seq < first+n; seq++ {
		data, err := proto.Marshal(req(seq))
		if err != nil {
			t.Fatal(err)
		}
		if err := p.c.SendRaw(ctx, data); err != nil {
			t.Fatal(err)
		}
	}
	replies := make(map[uint32]*pb.Envelope)
	waitFor(t, p.c, 3*time.Second, func(e *pb.Envelope) bool {
		if e.Seq >= first && e.Seq < first+n {
			replies[e.Seq] = e
		}
		return len(replies) == n
	})
	for seq := uint32(first); seq < first+n-1; seq++ {
		if code := replies[seq].GetError().GetCode(); code == pb.ErrorCode_ERROR_CODE_RATE_LIMITED {
			t.Errorf("request %d was rate limited, want only the 6th", seq-first+1)
		}
	}
	if code := replies[first+n-1].GetError().GetCode(); code != pb.ErrorCode_ERROR_CODE_RATE_LIMITED {
		t.Fatalf("6th request: %v, want RATE_LIMITED", replies[first+n-1])
	}
}

// ______________________TEST ITEMS_____________________

// sureDrops copies ../data into a temporary directory with every normal
// monster's equipment and stack drop chances at 100%, and returns it.
func sureDrops(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir("../data")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join("../data", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if e.Name() == "items.json" {
			var doc map[string]any
			if err := json.Unmarshal(b, &doc); err != nil {
				t.Fatal(err)
			}
			normal := doc["drops"].(map[string]any)["normal"].(map[string]any)
			normal["equipment"].(map[string]any)["chance_bp"] = 10000
			for _, st := range normal["stacks"].([]any) {
				st.(map[string]any)["chance_bp"] = 10000
			}
			if b, err = json.Marshal(doc); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// watcher reads a client's pushes in the background and records the deaths
// and the ITEM entities it was shown.
type watcher struct {
	mu     sync.Mutex
	deaths map[uint64]bool
	items  []uint64
	stop   chan struct{}
	done   chan struct{}
}

func watch(c *client.Client) *watcher {
	w := &watcher{deaths: map[uint64]bool{}, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		for {
			select {
			case env := <-c.Pushes():
				w.mu.Lock()
				if e := env.GetEntityEnter(); e != nil && e.Kind == pb.EntityKind_ENTITY_KIND_ITEM {
					w.items = append(w.items, e.Id)
				}
				for _, d := range env.GetWorldTick().GetDeaths() {
					w.deaths[d.Id] = true
				}
				w.mu.Unlock()
			case <-w.stop:
				return
			case <-c.Closed():
				return
			}
		}
	}()
	return w
}

func (w *watcher) close() {
	close(w.stop)
	<-w.done
}

// gearStats is the part of SelfStats that worn gear changes.
func gearStats(s *pb.SelfStats) [5]int32 {
	return [5]int32{s.GetMaxHp(), s.GetPhysAttack(), s.GetMagicAttack(), s.GetPhysDefense(), s.GetMagicDefense()}
}

// A level 19 Viêm kills a monster with sure drops: it gets Xu at once and an
// equipment piece and a stone beside the corpse, which an onlooker in view
// is never shown. It picks both up, wears the piece (stats rise), takes it
// off (stats fall back), wears it again and sorts the bag; after leaving and
// re-entering, the Xu, the bag and the worn piece are all still there.
func TestItemsOverWebSocket(t *testing.T) {
	s := startServerData(t, sureDrops(t))
	ctx := ctxT(t)

	// The onlooker stands in the camp at (10.5, 10.5): its view covers x and y
	// up to 30, which takes in the region r1 monsters near the camp.
	o := newPlayer(t, s, "miko01", "Miko", pb.CharClass_CHAR_CLASS_THUY)
	if err := o.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.store.SaveCharacterState(ctx, store.CharState{ID: o.charID, MapID: "kumotori_1", X: 10.5, Y: 10.5,
		Level: 1, HP: -1, MP: -1}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.c.EnterWorld(ctx, o.charID); err != nil {
		t.Fatal(err)
	}
	onlooker := watch(o.c)

	p := newPlayer(t, s, "rin01", "Rin", pb.CharClass_CHAR_CLASS_VIEM)
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	for drained := false; !drained; {
		select {
		case <-p.c.Pushes():
		default:
			drained = true
		}
	}
	if _, err := s.store.DevSetLevel(ctx, "rin01", 19); err != nil {
		t.Fatal(err)
	}
	ew, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if inv := ew.Inventory; inv.GetBagSize() != 60 || len(inv.GetBag()) != 0 || len(inv.GetEquipped()) != 0 || ew.Stats.GetXu() != 0 {
		t.Fatalf("entered with inventory %v and %d Xu, want an empty 60-slot bag and no Xu", inv, ew.Stats.GetXu())
	}

	self := ew.SelfEntityId
	x, y := ew.X, ew.Y
	stats := ew.Stats
	ground := map[uint64]*pb.EntityEnter{}
	observe := func(env *pb.Envelope) {
		if e := env.GetEntityEnter(); e != nil && e.Kind == pb.EntityKind_ENTITY_KIND_ITEM {
			ground[e.Id] = e
		}
		if l := env.GetEntityLeave(); l != nil {
			delete(ground, l.Id)
		}
		if st := env.GetSelfStats(); st != nil {
			stats = st
		}
		for _, m := range env.GetWorldTick().GetMoves() {
			if m.Id == self {
				x, y = m.X, m.Y
			}
		}
	}
	f := client.NewFighter(p.c, ew)
	f.Seek = nestR1
	f.Observe = observe
	var kill client.Kill
	fctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := f.Fight(fctx, 1, func(k client.Kill) { kill = k }); err != nil {
		t.Fatalf("fight: %v", err)
	}
	if _, err := p.c.Move(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}

	// Xu and the two drops (one piece, one stone) may come a push or two after the kill.
	var piece, stone *pb.EntityEnter
	waitFor(t, p.c, 3*time.Second, func(env *pb.Envelope) bool {
		observe(env)
		return stats.GetXu() > 0 && len(ground) == 2
	})
	for _, e := range ground {
		if strings.HasPrefix(e.Item.GetCode(), "eq10_") {
			piece = e
		} else {
			stone = e
		}
	}
	if piece == nil || stone == nil || stone.Item.GetCode() != "stone_enhance" || stone.Item.GetCount() != 1 {
		t.Fatalf("ground items %v, want a tier 10 piece and one stone_enhance", ground)
	}
	if w := piece.Item.GetCode(); strings.HasPrefix(w, "eq10_weapon_") && w != "eq10_weapon_viem" {
		t.Fatalf("dropped weapon %s, want the killer's class", w)
	}
	if xu := stats.GetXu(); xu < 3 || xu > 5*2 {
		t.Fatalf("Xu %d after killing a level 1-2 monster, want 3-10", xu)
	}

	// walkTo walks straight to (gx, gy) and stops within reach of it.
	walkTo := func(gx, gy float32) {
		t.Helper()
		d := float32(math.Hypot(float64(gx-x), float64(gy-y)))
		if d <= 0.8 {
			return
		}
		if _, err := p.c.Move(ctx, (gx-x)/d, (gy-y)/d); err != nil {
			t.Fatal(err)
		}
		waitFor(t, p.c, 3*time.Second, func(env *pb.Envelope) bool {
			observe(env)
			return math.Hypot(float64(gx-x), float64(gy-y)) <= 0.8
		})
		if _, err := p.c.Move(ctx, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	slotOf := func(u *pb.InventoryUpdate, code string) int {
		t.Helper()
		for _, sl := range u.GetBag() {
			if sl.Item.GetCode() == code {
				return int(sl.Slot)
			}
		}
		t.Fatalf("update %v has no %s", u, code)
		return -1
	}
	walkTo(piece.X, piece.Y)
	r, err := p.c.Pickup(ctx, piece.Id)
	if err != nil {
		t.Fatalf("pick up %v: %v", piece, err)
	}
	pieceSlot := slotOf(r.Update, piece.Item.Code)
	walkTo(stone.X, stone.Y)
	if r, err = p.c.Pickup(ctx, stone.Id); err != nil {
		t.Fatalf("pick up %v: %v", stone, err)
	}
	slotOf(r.Update, "stone_enhance")
	if _, err := p.c.Pickup(ctx, stone.Id); client.Code(err) != pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND {
		t.Fatalf("second pickup of the stone: %v, want ITEM_NOT_FOUND", err)
	}

	// Wearing the piece raises a gear stat and lowers none; taking it off undoes that.
	bare := gearStats(stats)
	er, err := p.c.Equip(ctx, pieceSlot)
	if err != nil {
		t.Fatal(err)
	}
	worn, raised := gearStats(er.Stats), false
	for i := range worn {
		if worn[i] < bare[i] {
			t.Fatalf("stats %v after wearing %s, below %v", worn, piece.Item.Code, bare)
		}
		raised = raised || worn[i] > bare[i]
	}
	if !raised || len(er.Update.GetEquipped()) != 1 {
		t.Fatalf("equip %s: stats %v from %v, update %v", piece.Item.Code, worn, bare, er.Update)
	}
	equipSlot := int(er.Update.Equipped[0].Slot)
	ur, err := p.c.Unequip(ctx, equipSlot)
	if err != nil || gearStats(ur.Stats) != bare {
		t.Fatalf("unequip: %v, %v, want stats back at %v", ur, err, bare)
	}
	if er, err = p.c.Equip(ctx, slotOf(ur.Update, piece.Item.Code)); err != nil || gearStats(er.Stats) != worn {
		t.Fatalf("equip again: %v, %v", er, err)
	}
	sr, err := p.c.SortBag(ctx)
	if err != nil || len(sr.Update.GetBag()) != 60 || sr.Update.Bag[0].Item.GetCode() != "stone_enhance" {
		t.Fatalf("sort: %v, %v, want 60 slots with the stone first", sr, err)
	}

	// The onlooker saw the monster die but was never shown either drop.
	onlooker.close()
	if !onlooker.deaths[kill.ID] {
		t.Logf("the onlooker did not see monster %d die; the owner-only check proves nothing this run", kill.ID)
	}
	if len(onlooker.items) != 0 {
		t.Fatalf("the onlooker was shown ground items %v", onlooker.items)
	}

	// Everything survives leaving and re-entering.
	xu := stats.GetXu()
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	inv := again.Inventory
	if again.Stats.GetXu() != xu || gearStats(again.Stats) != worn {
		t.Fatalf("re-entered with %v, want %d Xu and gear stats %v", again.Stats, xu, worn)
	}
	if len(inv.GetBag()) != 1 || inv.Bag[0].Slot != 0 || !proto.Equal(inv.Bag[0].Item, stone.Item) {
		t.Fatalf("re-entered with bag %v, want the stone in slot 0", inv.GetBag())
	}
	if len(inv.GetEquipped()) != 1 || int(inv.Equipped[0].Slot) != equipSlot || !proto.Equal(inv.Equipped[0].Item, piece.Item) {
		t.Fatalf("re-entered wearing %v, want %v in slot %d", inv.GetEquipped(), piece.Item, equipSlot)
	}
}

// ______________________TEST ENHANCE_____________________

// Phase 3b spec 8: a level 10 Viêm given a Lữ Hành sword, 20 stones and
// 10,000 Xu wears the sword beside the Ubuyashiki blacksmith and enhances it to +4
// with four requests (+1 to +4 always succeed): its attack rises each time,
// 700 Xu are spent and its own glow level follows. Away from the blacksmith the
// request is refused with NPC_TOO_FAR; after leaving and re-entering
// the sword is still +4.
func TestEnhanceOverWebSocket(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	p := newPlayer(t, s, "kai01", "Kai", pb.CharClass_CHAR_CLASS_VIEM)
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DevSetLevel(ctx, "kai01", 10); err != nil {
		t.Fatal(err)
	}
	// Beside the Ubuyashiki blacksmith (tho_ren at 17.5, 16.5).
	if _, err := s.store.DevSetPlace(ctx, "kai01", "ubuyashiki", 16.5, 16.5); err != nil {
		t.Fatal(err)
	}
	give := func([]store.ItemRow) ([]store.ItemRow, error) {
		return []store.ItemRow{
			{Slot: 0, Code: "eq10_weapon_viem", Count: 1, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
			{Slot: 1, Code: "stone_enhance", Count: 20, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
		}, nil
	}
	if _, err := s.store.DevGiveItem(ctx, "kai01", give, 10000); err != nil {
		t.Fatal(err)
	}
	for drained := false; !drained; {
		select {
		case <-p.c.Pushes():
		default:
			drained = true
		}
	}
	ew, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	self := ew.SelfEntityId
	er, err := p.c.Equip(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	weapon := &pb.ItemRef{Equipped: true, Slot: er.Update.GetEquipped()[0].GetSlot()}
	attack := er.Stats.GetPhysAttack()
	for want := int32(1); want <= 4; want++ {
		r, err := p.c.Enhance(ctx, weapon, false, 1)
		if err != nil {
			t.Fatalf("enhance to +%d: %v", want, err)
		}
		if r.Attempts != 1 || !r.Success || r.Level != want || r.Stats.GetPhysAttack() <= attack {
			t.Fatalf("enhance to +%d: %v, want one success and attack above %d", want, r, attack)
		}
		attack = r.Stats.GetPhysAttack()
		if want == 4 && (r.Stats.GetXu() != 10000-700 || r.Update.GetEquipped()[0].GetItem().GetEnhance() != 4 ||
			r.Update.GetBag()[0].GetItem().GetCount() != 16) {
			t.Fatalf("after +4: %v, want 9,300 Xu, the sword at +4 and 16 stones", r)
		}
	}
	waitFor(t, p.c, 3*time.Second, func(e *pb.Envelope) bool {
		for _, l := range e.GetWorldTick().GetLooks() {
			if l.Id == self && l.WeaponEnhance == 4 {
				return true
			}
		}
		return false
	})

	// Walk away from the blacksmith (more than 3.5 tiles) and try again.
	if _, err := p.c.Move(ctx, -1, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, p.c, 5*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == self && m.X <= 12 {
				return true
			}
		}
		return false
	})
	if _, err := p.c.Move(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := p.c.Enhance(ctx, weapon, false, 1); client.Code(err) != pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR {
		t.Fatalf("enhance away from the blacksmith: %v, want NPC_TOO_FAR", err)
	}

	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if eq := again.Inventory.GetEquipped(); len(eq) != 1 || eq[0].Item.GetEnhance() != 4 || again.Stats.GetXu() != 9300 {
		t.Fatalf("re-entered wearing %v with %d Xu, want the +4 sword and 9,300 Xu", eq, again.Stats.GetXu())
	}
}

// ______________________TEST FORGE_____________________

// Phase 3c spec 8: a level 25 Viêm given a Thanh Thiết Vàng armour (Thủy), a
// Lữ Hành Vàng necklace (Minh), a Ngọc Sinh Lực cấp 2, 3 refine stones and
// 10,000 Xu forges at the camp. Wearing the necklace lights the armour's
// hidden line 1 (Minh generates Thủy; the necklace is the armour's first
// activating slot) but not line 2 or the necklace's own lines. It punches
// two holes in the worn armour and sockets the gem into hole 1 from bag slot
// 3, so both arguments cross the gateway: max HP rises by exactly 156, and
// unsocketing drops it again. A refine with line 0 locked keeps that line
// and is taken; a second, unlocked refine is declined and changes nothing.
// After leaving and re-entering, the holes, the gem and the taken lines are
// all still there.
func TestForgeOverWebSocket(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	b, err := os.ReadFile("../data/items.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := gamedata.ParseItems(b)
	if err != nil {
		t.Fatal(err)
	}
	p := newPlayer(t, s, "rin01", "Rin", pb.CharClass_CHAR_CLASS_VIEM)
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DevSetLevel(ctx, "rin01", 25); err != nil {
		t.Fatal(err)
	}
	// Beside the Ubuyashiki blacksmith (tho_ren at 17.5, 16.5).
	if _, err := s.store.DevSetPlace(ctx, "rin01", "ubuyashiki", 16.5, 16.5); err != nil {
		t.Fatal(err)
	}
	lines := func(ls ...store.ItemLine) []store.ItemLine { return ls }
	give := func([]store.ItemRow) ([]store.ItemRow, error) {
		return []store.ItemRow{
			{Slot: 0, Code: "eq20_armor", Count: 1, Quality: 2, Element: "thuy",
				Lines:  lines(store.ItemLine{Stat: "hp", Value: 100}, store.ItemLine{Stat: "phys_attack", Value: 5}, store.ItemLine{Stat: "magic_attack", Value: 5}),
				Hidden: lines(store.ItemLine{Stat: "magic_defense", Value: 1}, store.ItemLine{Stat: "phys_defense", Value: 1})},
			{Slot: 1, Code: "eq10_necklace", Count: 1, Quality: 2, Element: "minh",
				Lines:  lines(store.ItemLine{Stat: "hp", Value: 50}, store.ItemLine{Stat: "phys_attack", Value: 3}, store.ItemLine{Stat: "magic_attack", Value: 3}),
				Hidden: lines(store.ItemLine{Stat: "magic_defense", Value: 5}, store.ItemLine{Stat: "magic_defense", Value: 5})},
			{Slot: 2, Code: "stone_refine", Count: 3, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
			{Slot: 3, Code: "gem_hp_2", Count: 1, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
		}, nil
	}
	if _, err := s.store.DevGiveItem(ctx, "rin01", give, 10000); err != nil {
		t.Fatal(err)
	}
	for drained := false; !drained; {
		select {
		case <-p.c.Pushes():
		default:
			drained = true
		}
	}
	if _, err := p.c.EnterWorld(ctx, p.charID); err != nil {
		t.Fatal(err)
	}

	// Resonance: the armour alone has no lit lines; the Minh necklace lights
	// its line 1 (+1 magic defense) and nothing else.
	er, err := p.c.Equip(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	armour := &pb.ItemRef{Equipped: true, Slot: er.Update.GetEquipped()[0].GetSlot()}
	before := er.Stats
	nr, err := p.c.Equip(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	neck := data.Equipment["eq10_necklace"]
	if md, pd := nr.Stats.GetMagicDefense()-before.GetMagicDefense(), nr.Stats.GetPhysDefense()-before.GetPhysDefense(); md != int32(neck.MagicDefense[0])+1 || pd != int32(neck.PhysDefense[0]) {
		t.Fatalf("the necklace added magic defense %d and phys defense %d, want %d and %d", md, pd,
			neck.MagicDefense[0]+1, neck.PhysDefense[0])
	}
	maxHP := nr.Stats.GetMaxHp()

	// Two holes: 100 × 20 + 200 × 20 Xu.
	for want := int32(1); want <= 2; want++ {
		r, err := p.c.Punch(ctx, armour)
		if err != nil {
			t.Fatalf("punch %d: %v", want, err)
		}
		if got := r.Update.GetEquipped()[0].GetItem(); got.GetHoles() != want || len(got.GetGems()) != int(want) {
			t.Fatalf("punch %d: %v", want, got)
		}
	}
	sr, err := p.c.Socket(ctx, armour, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := sr.Update.GetEquipped()[0].GetItem().GetGems(); len(got) != 2 || got[0] != "" || got[1] != "gem_hp_2" {
		t.Fatalf("socketed gems %q, want the gem in hole 1", got)
	}
	if sr.Stats.GetMaxHp() != maxHP+156 || sr.Stats.GetXu() != 10000-6000 {
		t.Fatalf("after socketing: max HP %d, %d Xu, want %d and 4,000", sr.Stats.GetMaxHp(), sr.Stats.GetXu(), maxHP+156)
	}

	// Unsocketing (500 × 2 Xu) puts the gem back in the bag; socket it again.
	ur, err := p.c.Unsocket(ctx, armour, 1)
	if err != nil {
		t.Fatal(err)
	}
	gemSlot := -1
	for _, e := range ur.Update.GetBag() {
		if e.GetItem().GetCode() == "gem_hp_2" {
			gemSlot = int(e.GetSlot())
		}
	}
	if gemSlot < 0 || ur.Stats.GetMaxHp() != maxHP || ur.Stats.GetXu() != 3000 ||
		ur.Update.GetEquipped()[0].GetItem().GetGems()[1] != "" {
		t.Fatalf("after unsocketing: %v, want the gem in the bag, max HP %d and 3,000 Xu", ur, maxHP)
	}
	if sr, err = p.c.Socket(ctx, armour, 1, gemSlot); err != nil || sr.Stats.GetMaxHp() != maxHP+156 {
		t.Fatalf("socket again: %v, %v", sr, err)
	}

	// Refine with line 0 locked: 2 stones and (20 + 20) × 20 Xu; take it.
	rr, err := p.c.Refine(ctx, armour, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := rr.GetLines()
	stats := map[string]bool{}
	for _, l := range got {
		stats[l.GetStat()] = true
	}
	if len(got) != 3 || len(stats) != 3 || got[0].GetStat() != "hp" || got[0].GetValue() != 100 || rr.Stats.GetXu() != 2200 {
		t.Fatalf("refine: %v, want 3 distinct lines with hp+100 first and 2,200 Xu", rr)
	}
	cr, err := p.c.RefineChoose(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	taken := cr.Update.GetEquipped()[0].GetItem().GetLines()
	if !proto.Equal(&pb.ItemInfo{Lines: taken}, &pb.ItemInfo{Lines: got}) {
		t.Fatalf("taken lines %v, want %v", taken, got)
	}

	// An unlocked refine (1 stone, 20 × 20 Xu), declined, keeps the lines.
	if rr, err = p.c.Refine(ctx, armour); err != nil || rr.Stats.GetXu() != 1800 {
		t.Fatalf("second refine: %v, %v", rr, err)
	}
	if cr, err = p.c.RefineChoose(ctx, false); err != nil {
		t.Fatal(err)
	}
	if kept := cr.Update.GetEquipped()[0].GetItem().GetLines(); !proto.Equal(&pb.ItemInfo{Lines: kept}, &pb.ItemInfo{Lines: taken}) {
		t.Fatalf("declined, the lines are %v, want %v", kept, taken)
	}
	if _, err := p.c.RefineChoose(ctx, true); client.Code(err) != pb.ErrorCode_ERROR_CODE_NO_PENDING_REFINE {
		t.Fatalf("a second choice: %v, want NO_PENDING_REFINE", err)
	}
	wantHP := cr.Stats.GetMaxHp()

	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	var worn *pb.ItemInfo
	for _, e := range again.Inventory.GetEquipped() {
		if e.GetSlot() == armour.Slot {
			worn = e.GetItem()
		}
	}
	if worn.GetHoles() != 2 || len(worn.GetGems()) != 2 || worn.GetGems()[1] != "gem_hp_2" ||
		!proto.Equal(&pb.ItemInfo{Lines: worn.GetLines()}, &pb.ItemInfo{Lines: taken}) ||
		again.Stats.GetMaxHp() != wantHP || again.Stats.GetXu() != 1800 {
		t.Fatalf("re-entered wearing %v with max HP %d and %d Xu, want 2 holes, the gem in hole 1, lines %v, max HP %d and 1,800 Xu",
			worn, again.Stats.GetMaxHp(), again.Stats.GetXu(), taken, wantHP)
	}
}

// ______________________TEST PORTALS_____________________

// offlinePlayer registers username and creates its character without
// entering the world, so the test can move it with DevSetPlace first.
func offlinePlayer(t *testing.T, s server, username, charName string, class pb.CharClass) (*client.Client, int64) {
	t.Helper()
	ctx := ctxT(t)
	c := dial(t, s)
	if err := c.Register(ctx, username, "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Login(ctx, username, "password123"); err != nil {
		t.Fatal(err)
	}
	ch, err := c.CreateChar(ctx, 1, charName, class)
	if err != nil {
		t.Fatal(err)
	}
	return c, ch.Id
}

// zoneCounts is a zone list as zone number → players, checking the capacity.
func zoneCounts(t *testing.T, c *client.Client) map[int32]int32 {
	t.Helper()
	zones, err := c.ZoneList(ctxT(t))
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[int32]int32)
	for i, z := range zones {
		if z.Capacity != 50 || (i > 0 && z.Zone <= zones[i-1].Zone) {
			t.Fatalf("zone list %v: want capacity 50 in increasing zone order", zones)
		}
		out[z.Zone] = z.Players
	}
	return out
}

// Phase 4a spec 7: A stands next to the east portal of Kumotori and B in
// Ubuyashiki next to where that portal leads. A crosses into Ubuyashiki
// (B sees A arrive and move there), is refused a zone change in the
// one-zone town and a far portal, walks into the west portal back to
// Kumotori (B sees A leave), changes to zone 2 (and is refused changing back
// during the cooldown), leaves and enters again at the saved map and
// position.
func TestPortalsOverWebSocket(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	loaded, err := mapdata.LoadDir("../maps")
	if err != nil {
		t.Fatal(err)
	}
	town, kumotori := loaded["ubuyashiki"], loaded["kumotori_1"]
	arrival, back := town.Spawns["from_kumotori_1"], kumotori.Spawns["from_ubuyashiki"]

	a, aChar := offlinePlayer(t, s, "akira01", "Akira", pb.CharClass_CHAR_CLASS_VIEM)
	b, bChar := offlinePlayer(t, s, "botan01", "Botan", pb.CharClass_CHAR_CLASS_THUY)
	// A is inside gate_east, 0.5 tiles from the to_ubuyashiki portal; B is in
	// the AOI cell where A arrives.
	if _, err := s.store.DevSetPlace(ctx, "akira01", "kumotori_1", 61.5, 24.0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DevSetPlace(ctx, "botan01", "ubuyashiki", 8.5, 16.5); err != nil {
		t.Fatal(err)
	}
	bew, err := b.EnterWorld(ctx, bChar)
	if err != nil || bew.MapId != "ubuyashiki" || bew.Zone != 1 {
		t.Fatalf("B entered %v, %v; want ubuyashiki zone 1", bew, err)
	}
	aew, err := a.EnterWorld(ctx, aChar)
	if err != nil || aew.MapId != "kumotori_1" || aew.X != 61.5 || aew.Y != 24 {
		t.Fatalf("A entered %v, %v; want kumotori_1 at (61.5, 24)", aew, err)
	}
	self := aew.SelfEntityId

	// Kumotori → Ubuyashiki: same entity, HP and MP, at the arrival point, and B sees A come in.
	in, err := a.UsePortal(ctx, "to_ubuyashiki")
	if err != nil {
		t.Fatalf("portal to ubuyashiki: %v", err)
	}
	if in.MapId != "ubuyashiki" || in.Zone != 1 || in.X != arrival.X || in.Y != arrival.Y || in.SelfEntityId != self ||
		in.Stats.GetHp() != aew.Stats.GetHp() || in.Stats.GetMp() != aew.Stats.GetMp() || len(in.Cooldowns) != 0 {
		t.Fatalf("arrived %v, want ubuyashiki zone 1 at (%v, %v) as entity %d with HP and MP kept and no cooldowns",
			in, arrival.X, arrival.Y, self)
	}
	waitFor(t, b, 2*time.Second, enterOf(self))

	// The town has one zone holding both players.
	if got := zoneCounts(t, a); !maps.Equal(got, map[int32]int32{1: 2}) {
		t.Fatalf("town zones %v, want zone 1 with 2 players", got)
	}
	if _, err := a.ChangeZone(ctx, 2); client.Code(err) != pb.ErrorCode_ERROR_CODE_ZONE_NOT_FOUND {
		t.Fatalf("change to zone 2 of the town: %v, want ZONE_NOT_FOUND", err)
	}
	if _, err := a.UsePortal(ctx, "to_sagiri_1"); client.Code(err) != pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE {
		t.Fatalf("the far east portal: %v, want OUT_OF_RANGE", err)
	}
	if _, err := a.UsePortal(ctx, "to_ubuyashiki"); client.Code(err) != pb.ErrorCode_ERROR_CODE_BAD_REQUEST {
		t.Fatalf("a portal of another map: %v, want BAD_REQUEST", err)
	}

	// A walks west into to_kumotori_1; its moves now reach the town zone, so B sees them.
	if _, err := a.Move(ctx, -1, 0); err != nil {
		t.Fatal(err)
	}
	westOf := func(x float32) func(*pb.Envelope) bool {
		return func(e *pb.Envelope) bool {
			for _, m := range e.GetWorldTick().GetMoves() {
				if m.Id == self && m.X <= x {
					return true
				}
			}
			return false
		}
	}
	waitFor(t, b, 3*time.Second, westOf(arrival.X-0.5))
	waitFor(t, a, 3*time.Second, westOf(2.8)) // within 1 tile of the portal (x 1-2)
	if _, err := a.Move(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	out, err := a.UsePortal(ctx, "to_kumotori_1")
	if err != nil {
		t.Fatalf("portal back to kumotori_1: %v", err)
	}
	if out.MapId != "kumotori_1" || out.Zone != 1 || out.X != back.X || out.Y != back.Y || out.SelfEntityId != self {
		t.Fatalf("arrived %v, want kumotori_1 zone 1 (same number as the town's) at (%v, %v)", out, back.X, back.Y)
	}
	waitFor(t, b, 2*time.Second, func(e *pb.Envelope) bool { return e.GetEntityLeave().GetId() == self })

	// Kumotori keeps two zones open; A changes to zone 2 in place.
	if got := zoneCounts(t, a); !maps.Equal(got, map[int32]int32{1: 1, 2: 0}) {
		t.Fatalf("kumotori zones %v, want zone 1 with A and an empty zone 2", got)
	}
	for _, c := range []struct {
		zone int
		want pb.ErrorCode
	}{{0, pb.ErrorCode_ERROR_CODE_BAD_REQUEST}, {1, pb.ErrorCode_ERROR_CODE_BAD_REQUEST}, {3, pb.ErrorCode_ERROR_CODE_ZONE_NOT_FOUND}} {
		if _, err := a.ChangeZone(ctx, c.zone); client.Code(err) != c.want {
			t.Fatalf("change to zone %d: %v, want %v", c.zone, err, c.want)
		}
	}
	moved, err := a.ChangeZone(ctx, 2)
	if err != nil {
		t.Fatalf("change to zone 2: %v", err)
	}
	if moved.MapId != "kumotori_1" || moved.Zone != 2 || moved.X != back.X || moved.Y != back.Y || moved.SelfEntityId != self ||
		len(moved.Cooldowns) != 1 || moved.Cooldowns[0].Key != "zone_change" ||
		moved.Cooldowns[0].RemainingMs <= 9000 || moved.Cooldowns[0].RemainingMs > 10000 {
		t.Fatalf("changed to %v, want kumotori_1 zone 2 at (%v, %v) with a 10 s zone_change cooldown", moved, back.X, back.Y)
	}
	if got := zoneCounts(t, a); !maps.Equal(got, map[int32]int32{1: 0, 2: 1}) {
		t.Fatalf("kumotori zones %v after the change, want A in zone 2", got)
	}
	if _, err := a.ChangeZone(ctx, 1); client.Code(err) != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("change straight back: %v, want ON_COOLDOWN", err)
	}

	// Leaving and entering again lands at the saved map and position, in
	// the zone the usual rule picks (the first with room).
	if err := a.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := a.EnterWorld(ctx, aChar)
	if err != nil {
		t.Fatal(err)
	}
	if again.MapId != "kumotori_1" || again.Zone != 1 || again.X != back.X || again.Y != back.Y || len(again.Cooldowns) != 0 {
		t.Fatalf("re-entered %v, want kumotori_1 zone 1 at (%v, %v) with no cooldowns", again, back.X, back.Y)
	}
}

// ______________________TEST SHOPS_____________________

// slotOf is the bag slot an inventory update shows holding code.
func slotOf(t *testing.T, u *pb.InventoryUpdate, code string) int {
	t.Helper()
	for _, s := range u.GetBag() {
		if s.GetItem().GetCode() == code {
			return int(s.Slot)
		}
	}
	t.Fatalf("no %s in the update %v", code, u)
	return -1
}

// Phase 4b spec 8: a character with 10,000 Xu beside the Kumotori
// apothecary buys 3 level 1 HP potions (-15 Xu), is refused drinking one at full
// HP, sells one back (+1 Xu), buys a return scroll and reads it to arrive at the
// default point of Ubuyashiki. Beside the Ubuyashiki carriage driver it rides to
// sagiri_1 for 120 Xu and arrives at that map's default point. NPC positions
// come from the loaded maps.
func TestShopsOverWebSocket(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	loaded, err := mapdata.LoadDir("../maps")
	if err != nil {
		t.Fatal(err)
	}
	apothecary, ok := loaded["kumotori_1"].NPC("duoc_su")
	if !ok {
		t.Fatal("kumotori_1 has no duoc_su")
	}
	carriage, ok := loaded["ubuyashiki"].NPC("xa_phu")
	if !ok {
		t.Fatal("ubuyashiki has no xa_phu")
	}
	routeToSagiri := -1
	for i, r := range carriage.Routes {
		if r.Map == "sagiri_1" {
			routeToSagiri = i
		}
	}
	if routeToSagiri < 0 {
		t.Fatalf("the Ubuyashiki carriage has no route to sagiri_1: %v", carriage.Routes)
	}
	fare := carriage.Routes[routeToSagiri].Xu
	townDefault := loaded["ubuyashiki"].Spawns["default"]
	sagiriDefault := loaded["sagiri_1"].Spawns["default"]

	p := newPlayer(t, s, "yuki01", "Yuki", pb.CharClass_CHAR_CLASS_VIEM)
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DevGiveItem(ctx, "yuki01", nil, 10000); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DevSetPlace(ctx, "yuki01", "kumotori_1", apothecary.Pos.X+1, apothecary.Pos.Y); err != nil {
		t.Fatal(err)
	}
	ew, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if ew.MapId != "kumotori_1" || ew.Stats.GetXu() != 10000 {
		t.Fatalf("entered %v, want kumotori_1 with 10,000 Xu", ew)
	}

	// Buy 3 potions at 5 Xu.
	buy, err := p.c.Buy(ctx, "duoc_su", "", "potion_hp_1", 3)
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if buy.Stats.GetXu() != 10000-15 {
		t.Fatalf("Xu %d after buying 3 potions, want 9,985", buy.Stats.GetXu())
	}
	slot := slotOf(t, buy.Update, "potion_hp_1")
	if got := buy.Update.Bag[0].GetItem().GetCount(); got != 3 {
		t.Fatalf("bought stack %v, want 3 potions", buy.Update.Bag[0])
	}
	// At full HP a potion is refused and kept.
	if _, err := p.c.UseItem(ctx, slot); client.Code(err) != pb.ErrorCode_ERROR_CODE_ALREADY_FULL {
		t.Fatalf("drink at full HP: %v, want ALREADY_FULL", err)
	}
	sold, err := p.c.Sell(ctx, "duoc_su", slot, 1)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if sold.Stats.GetXu() != 10000-15+1 {
		t.Fatalf("Xu %d after selling one potion, want 9,986", sold.Stats.GetXu())
	}

	// A return scroll at 50 Xu takes the character to the town.
	scroll, err := p.c.Buy(ctx, "duoc_su", "", "scroll_return", 1)
	if err != nil {
		t.Fatalf("buy scroll: %v", err)
	}
	if scroll.Stats.GetXu() != 9986-50 {
		t.Fatalf("Xu %d after the scroll, want 9,936", scroll.Stats.GetXu())
	}
	town, err := p.c.ReturnTown(ctx, slotOf(t, scroll.Update, "scroll_return"))
	if err != nil {
		t.Fatalf("return scroll: %v", err)
	}
	if town.MapId != "ubuyashiki" || town.X != townDefault.X || town.Y != townDefault.Y {
		t.Fatalf("arrived %v, want ubuyashiki at (%v, %v)", town, townDefault.X, townDefault.Y)
	}

	// Beside the town carriage driver, ride to Sagiri.
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DevSetPlace(ctx, "yuki01", "ubuyashiki", carriage.Pos.X-1, carriage.Pos.Y); err != nil {
		t.Fatal(err)
	}
	if _, err := p.c.EnterWorld(ctx, p.charID); err != nil {
		t.Fatal(err)
	}
	sag, err := p.c.Travel(ctx, "xa_phu", routeToSagiri)
	if err != nil {
		t.Fatalf("carriage: %v", err)
	}
	if sag.MapId != "sagiri_1" || sag.X != sagiriDefault.X || sag.Y != sagiriDefault.Y ||
		sag.Stats.GetXu() != 9936-fare {
		t.Fatalf("arrived %v, want sagiri_1 at (%v, %v) with %d Xu", sag, sagiriDefault.X, sagiriDefault.Y, 9936-fare)
	}
}

// ______________________TEST STASH_____________________

// Phase 4c spec 8: one account with two characters. Character 1 (given a
// sword by devitem and enough Xu) enters at the default point of Kumotori,
// within reach of the storage keeper, deposits the sword and expands the
// stash, then leaves. Character 2 opens the stash and sees the sword and 40
// slots, withdraws it; with 2 Lượng from devitem it buys one protection charm
// in the Lượng shop (Lượng 0) and is refused a second with NOT_ENOUGH_LUONG.
// After leaving and entering again its Lượng is still 0 and the stash 40
// slots. The keeper's id and position come from the loaded maps.
func TestStashOverWebSocket(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	loaded, err := mapdata.LoadDir("../maps")
	if err != nil {
		t.Fatal(err)
	}
	var keeper mapdata.NPC
	for _, n := range loaded["kumotori_1"].NPCs {
		if n.Role == mapdata.RoleStorage {
			keeper = n
		}
	}
	if keeper.ID == "" {
		t.Fatal("kumotori_1 has no storage keeper")
	}

	p := newPlayer(t, s, "stash01", "Khomot", pb.CharClass_CHAR_CLASS_VIEM)
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := p.c.CreateChar(ctx, 2, "Khohai", pb.CharClass_CHAR_CLASS_VIEM)
	if err != nil {
		t.Fatal(err)
	}
	sword := store.ItemRow{Slot: 0, Code: "eq10_weapon_viem", Count: 1, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{},
		Gems: []string{}}
	if _, err := s.store.DevGiveItem(ctx, "stash01", func([]store.ItemRow) ([]store.ItemRow, error) {
		return []store.ItemRow{sword}, nil
	}, 20000); err != nil {
		t.Fatal(err)
	}

	// Character 1 enters at the default point, in reach of the keeper.
	ew, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if want := loaded["kumotori_1"].Spawns["default"]; ew.MapId != "kumotori_1" || ew.X != want.X || ew.Y != want.Y {
		t.Fatalf("entered %v, want the default point of kumotori_1", ew)
	}
	dep, err := p.c.StashDeposit(ctx, keeper.ID, 0)
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	if len(dep.Stash) != 1 || dep.Stash[0].GetItem().GetCode() != "eq10_weapon_viem" {
		t.Fatalf("stash after the deposit %v, want the sword", dep.Stash)
	}
	exp, err := p.c.StashExpand(ctx, keeper.ID)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if exp.Size != 40 || exp.Stats.GetXu() != 0 {
		t.Fatalf("expand %v, want 40 slots and 0 Xu", exp)
	}
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}

	// Lượng for the account, then character 2 (a new character enters at the default point).
	if total, err := s.store.DevAddLuong(ctx, "stash01", 2); err != nil || total != 2 {
		t.Fatalf("devitem -luong: %d, %v", total, err)
	}
	ew, err = p.c.EnterWorld(ctx, second.Id)
	if err != nil {
		t.Fatal(err)
	}
	if ew.Stats.GetLuong() != 2 {
		t.Fatalf("entered with %d Lượng, want 2", ew.Stats.GetLuong())
	}
	open, err := p.c.StashOpen(ctx, keeper.ID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if open.Stash.GetSize() != 40 || len(open.Stash.Items) != 1 || open.Stash.Items[0].GetItem().GetCode() != "eq10_weapon_viem" {
		t.Fatalf("opened %v, want 40 slots holding the sword", open.Stash)
	}
	wd, err := p.c.StashWithdraw(ctx, keeper.ID, int(open.Stash.Items[0].Slot))
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if len(wd.Stash) != 1 || wd.Stash[0].Item != nil || len(wd.Update.GetBag()) != 1 || wd.Update.Bag[0].GetItem().GetCode() != "eq10_weapon_viem" {
		t.Fatalf("withdraw %v, want the sword in the bag and its stash slot emptied", wd)
	}

	// One charm for 2 Lượng; the second is refused.
	buy, err := p.c.Buy(ctx, "", "luong", "charm_protect", 1)
	if err != nil {
		t.Fatalf("buy charm: %v", err)
	}
	if buy.Stats.GetLuong() != 0 {
		t.Fatalf("Lượng %d after the charm, want 0", buy.Stats.GetLuong())
	}
	if _, err := p.c.Buy(ctx, "", "luong", "charm_protect", 1); client.Code(err) != pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_LUONG {
		t.Fatalf("second charm: %v, want NOT_ENOUGH_LUONG", err)
	}

	// Leave and enter again: Lượng stays 0 and the stash 40 slots.
	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if ew, err = p.c.EnterWorld(ctx, second.Id); err != nil {
		t.Fatal(err)
	}
	if ew.Stats.GetLuong() != 0 {
		t.Fatalf("re-entered with %d Lượng, want 0", ew.Stats.GetLuong())
	}
	if open, err = p.c.StashOpen(ctx, keeper.ID); err != nil || open.Stash.GetSize() != 40 {
		t.Fatalf("re-opened %v, %v, want 40 slots", open, err)
	}
}

// ______________________TEST PVP_____________________

// pvpSpot finds two walkable tiles side by side east of the Kumotori camp and
// outside every safe zone, for the PvP test.
func pvpSpot(t *testing.T, m *mapdata.Map) (a, b mapdata.Point) {
	t.Helper()
	for x := 11; x < m.Width-1; x++ {
		for y := 1; y < m.Height-1; y++ {
			pa, pb2 := mapdata.Point{X: float32(x) + 0.5, Y: float32(y) + 0.5}, mapdata.Point{X: float32(x) + 1.5, Y: float32(y) + 0.5}
			if m.Walkable(pa) && m.Walkable(pb2) && !m.InSafeZone(pa) && !m.InSafeZone(pb2) {
				return pa, pb2
			}
		}
	}
	t.Fatal("kumotori_1 has no two walkable tiles outside the safe zones")
	return
}

// attackRetry attacks target, waiting out the swing cooldown.
func attackRetry(t *testing.T, c *client.Client, target uint64) error {
	t.Helper()
	ctx := ctxT(t)
	for i := 0; i < 100; i++ {
		err := c.Attack(ctx, target)
		if client.Code(err) != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("attack stayed on cooldown")
	return nil
}

// Phase 5a spec 8: two level 30 players stand next to each other just east of
// the Kumotori camp. With PK off on both, a slash is PK_FORBIDDEN; once A
// turns PK on, B sees the look change, A's slash lands, B (PK off) strikes
// back and A cannot switch PK off during the fight. B then turns PK on and A
// kills B: B is told who killed it and loses 5% of the EXP for level 31, which
// stays after B leaves and enters again with PK still on.
func TestPvPOverWebSocket(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	loaded, err := mapdata.LoadDir("../maps")
	if err != nil {
		t.Fatal(err)
	}
	data, err := gamedata.Load("../data")
	if err != nil {
		t.Fatal(err)
	}
	spotA, spotB := pvpSpot(t, loaded["kumotori_1"])
	ca, _ := offlinePlayer(t, s, "ren01", "Ren", pb.CharClass_CHAR_CLASS_VIEM)
	cb, chB := offlinePlayer(t, s, "sora01", "Sora", pb.CharClass_CHAR_CLASS_THUY)
	for _, u := range []struct {
		name string
		at   mapdata.Point
	}{{"ren01", spotA}, {"sora01", spotB}} {
		if _, err := s.store.DevSetLevel(ctx, u.name, 30); err != nil {
			t.Fatal(err)
		}
		if _, err := s.store.DevSetPlace(ctx, u.name, "kumotori_1", u.at.X, u.at.Y); err != nil {
			t.Fatal(err)
		}
	}
	chars, err := ca.ListChars(ctx)
	if err != nil || len(chars) != 1 {
		t.Fatalf("list chars: %v, %v", chars, err)
	}
	ewA, err := ca.EnterWorld(ctx, chars[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	ewB, err := cb.EnterWorld(ctx, chB)
	if err != nil {
		t.Fatal(err)
	}
	idA, idB := ewA.SelfEntityId, ewB.SelfEntityId
	waitFor(t, ca, 5*time.Second, enterOf(idB))
	waitFor(t, cb, 5*time.Second, enterOf(idA))
	expect := func(err error, want pb.ErrorCode) {
		t.Helper()
		if got := client.Code(err); got != want {
			t.Fatalf("error code = %v (%v), want %v", got, err, want)
		}
	}

	expect(attackRetry(t, ca, idB), pb.ErrorCode_ERROR_CODE_PK_FORBIDDEN)
	if r, err := ca.SetPk(ctxT(t), true); err != nil || !r.On {
		t.Fatalf("SetPk(true) = %v, %v", r, err)
	}
	waitFor(t, cb, 5*time.Second, func(e *pb.Envelope) bool {
		for _, l := range e.GetWorldTick().GetLooks() {
			if l.Id == idA && l.PkOn {
				return true
			}
		}
		return false
	})
	isHit := func(from, to uint64) func(*pb.Envelope) bool {
		return func(e *pb.Envelope) bool {
			for _, h := range e.GetWorldTick().GetHits() {
				if h.AttackerId == from && h.TargetId == to {
					return true
				}
			}
			return false
		}
	}
	if err := attackRetry(t, ca, idB); err != nil {
		t.Fatalf("A slashes B: %v", err)
	}
	waitFor(t, cb, 5*time.Second, isHit(idA, idB))
	if err := attackRetry(t, cb, idA); err != nil {
		t.Fatalf("B strikes back with PK off: %v", err)
	}
	waitFor(t, ca, 5*time.Second, isHit(idB, idA))
	_, err = ca.SetPk(ctxT(t), false)
	expect(err, pb.ErrorCode_ERROR_CODE_IN_PVP_COMBAT)

	if r, err := cb.SetPk(ctxT(t), true); err != nil || !r.On {
		t.Fatalf("SetPk(true) for B = %v, %v", r, err)
	}
	expStart := ewB.Stats.GetExp()
	var notice *pb.DeathNotice
	var expNow int64
	var gotExp bool
	deadline := time.Now().Add(60 * time.Second)
	for notice == nil && time.Now().Before(deadline) {
		if err := attackRetry(t, ca, idB); err != nil {
			// B may already be dead; the notice is then on its way.
			if client.Code(err) != pb.ErrorCode_ERROR_CODE_INVALID_TARGET && client.Code(err) != pb.ErrorCode_ERROR_CODE_DEAD {
				t.Fatalf("A attacks B: %v (A closed: %v)", err, ca.Err())
			}
		}
		for drained := false; !drained; {
			select {
			case env := <-cb.Pushes():
				if n := env.GetDeathNotice(); n != nil {
					notice = n
				}
				if st := env.GetSelfStats(); st != nil {
					expNow, gotExp = st.GetExp(), true
				}
			default:
				drained = true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if notice == nil {
		t.Fatal("B was never told it died")
	}
	lost := data.Exp.ToNext(30) * 5 / 100
	if notice.KillerId != idA || notice.KillerName != "Ren" || !notice.ByPlayer || notice.ExpLost != lost || lost <= 0 {
		t.Fatalf("death notice %v, want killer %d Ren by a player, %d EXP lost", notice, idA, lost)
	}
	// SelfStats follows the notice in the same tick and may still be on its
	// way; earlier ones carry the old EXP.
	if want := expStart - lost; !gotExp || expNow != want {
		waitFor(t, cb, 5*time.Second, func(e *pb.Envelope) bool {
			st := e.GetSelfStats()
			return st != nil && st.GetExp() == want
		})
	}

	if err := cb.LeaveWorld(ctxT(t)); err != nil {
		t.Fatal(err)
	}
	ew2, err := cb.EnterWorld(ctxT(t), chB)
	if err != nil {
		t.Fatal(err)
	}
	if !ew2.Stats.GetPkOn() || ew2.Stats.GetExp() != expStart-lost {
		t.Fatalf("B re-entered with pk_on %v exp %d, want on and %d", ew2.Stats.GetPkOn(), ew2.Stats.GetExp(), expStart-lost)
	}
}
