package gateway

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"demonslayer2d/internal/pb"
)

// A connection that registers after Shutdown has taken its snapshot must kick itself,
// otherwise Shutdown would wait for it until its deadline.
func TestConnTrackedDuringShutdownIsKicked(t *testing.T) {
	s := NewServer(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := s.newConn(nil, "127.0.0.1")
	defer c.idle.Stop()
	defer c.authTimer.Stop()

	s.track(c, true)
	if !c.kicked.Load() {
		t.Fatal("connection tracked after shutdown started was not kicked")
	}
	env := <-c.out.ch
	if env.GetKicked().GetReason() != pb.KickReason_KICK_REASON_SERVER_SHUTDOWN {
		t.Fatalf("final message = %v, want Kicked{SERVER_SHUTDOWN}", env)
	}
}

// Behind a trusted proxy the connection must be keyed on the player's address from
// X-Forwarded-For, not on the proxy's.
func TestConnUsesForwardedIPFromTrustedProxy(t *testing.T) {
	s := NewServer(Deps{
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("::1/128")},
	})
	srv := httptest.NewServer(s)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Forwarded-For": {"198.51.100.9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	var ip string
	for ip == "" && ctx.Err() == nil {
		s.mu.Lock()
		for c := range s.conns {
			ip = c.ip
		}
		s.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	if ip != "198.51.100.9" {
		t.Fatalf("connection ip = %q, want the forwarded player address", ip)
	}
}
