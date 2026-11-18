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

	"demonslayer2d/internal/app"
	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/client"
	"demonslayer2d/internal/config"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store/storetest"
)

type server struct{ ws, http string }

func startServer(t *testing.T) server {
	t.Helper()
	st := storetest.New(t)
	cfg := config.Config{
		MapsDir: "../maps", StartMap: "kumotori_1", TickRate: 10, MoveSpeedTiles: 4,
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
	return server{ws: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws", http: srv.URL}
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
	c      *client.Client
	charID int64
	enter  *pb.EnterWorldResp
}

func newPlayer(t *testing.T, s server, username, charName string, class pb.CharClass) player {
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
	ew, err := c.EnterWorld(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	return player{c: c, charID: ch.Id, enter: ew}
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
	_, err = c.LoginVersion(ctx, "erin01", "password123", 999)
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
