package gateway

import (
	"context"
	"io"
	"log/slog"
	"testing"

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
