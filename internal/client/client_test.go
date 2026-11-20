package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/pb"
)

// echoServer answers every request (seq above 0) with a Pong carrying its seq.
func echoServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		for {
			_, data, err := ws.Read(r.Context())
			if err != nil {
				return
			}
			env := &pb.Envelope{}
			if err := proto.Unmarshal(data, env); err != nil {
				return
			}
			if env.Seq == 0 {
				continue
			}
			out, _ := proto.Marshal(&pb.Envelope{Seq: env.Seq, Payload: &pb.Envelope_Pong{Pong: &pb.Pong{}}})
			if err := ws.Write(r.Context(), websocket.MessageBinary, out); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// ______________________TEST REQUEST_____________________

// Cancelling one request's context must not close the connection. The
// websocket library closes the connection when the context of a write ends
// before the write does, or is already done when it starts; the Fighter's
// attack goroutine and steer write with a context the caller may cancel at
// any time (TestSkillsOverWebSocket was flaky with "use of closed network
// connection").
func TestCancelledRequestKeepsConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Dial(ctx, echoServer(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	done, stop := context.WithCancel(context.Background())
	stop()
	for i := range 50 {
		if _, err := c.Ping(done); err == nil {
			t.Fatalf("ping %d with a cancelled context: no error", i)
		}
		if _, err := c.Move(done, 1, 0); err == nil {
			t.Fatalf("move %d with a cancelled context: no error", i)
		}
		if _, err := c.Ping(ctx); err != nil {
			t.Fatalf("ping %d after cancelled requests: %v", i, err)
		}
	}
}
