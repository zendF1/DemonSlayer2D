package e2e

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
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
	st := storetest.New(t)
	cfg := config.Config{
		MapsDir: "../maps", DataDir: "../data", StartMap: "kumotori_1", TickRate: 10, MoveSpeedTiles: 4,
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
	_, err = c.Respawn(ctx)
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
