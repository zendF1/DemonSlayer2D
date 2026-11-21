// Package gateway accepts WebSocket connections and turns protocol messages
// into calls on the auth service, the character store and the world.
package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
)

// ProtocolVersion must match LoginReq.protocol_version.
const ProtocolVersion uint32 = 13

const (
	maxMessageSize = 4096
	outboxSize     = 256
	authTimeout    = 30 * time.Second
	idleTimeout    = 30 * time.Second
	writeTimeout   = 10 * time.Second
	requestTimeout = 10 * time.Second
	packetsPerSec  = 30
	movesPerSec    = 20
	attacksPerSec  = 5
	castsPerSec    = 5         // same as attacks (phase 2 spec 4.7)
	maxPointsTotal = 1_000_000 // guards the zone's int arithmetic against overflow
	dropStreakKick = 5         // consecutive seconds with dropped packets before kicking
)

type CharacterStore interface {
	ListCharacters(ctx context.Context, accountID int64) ([]store.Character, error)
	GetCharacter(ctx context.Context, accountID, charID int64) (store.Character, error)
	CreateCharacter(ctx context.Context, nc store.NewCharacter) (store.Character, error)
	DeleteCharacter(ctx context.Context, accountID, charID int64) error
}

type Deps struct {
	Auth  *auth.Service
	Chars CharacterStore
	World *world.World
	Log   *slog.Logger
	// TrustedProxies are the reverse proxies whose X-Forwarded-For is believed.
	TrustedProxies []netip.Prefix
}

type Server struct {
	deps   Deps
	log    *slog.Logger
	nextID atomic.Uint64

	mu      sync.Mutex
	conns   map[*conn]struct{}
	closing bool
	wg      sync.WaitGroup
}

func NewServer(d Deps) *Server {
	return &Server{deps: d, log: d.Log, conns: make(map[*conn]struct{})}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.log.Debug("websocket accept failed", "err", err)
		return
	}
	ws.SetReadLimit(maxMessageSize)
	c := s.newConn(ws, clientIP(r, s.deps.TrustedProxies))
	s.track(c, true)
	defer s.track(c, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.writeLoop()
	c.readLoop(ctx)
	c.cleanup()
}

// track adds or removes c. A connection added after Shutdown took its snapshot
// kicks itself, so Shutdown never waits on a connection it did not kick.
func (s *Server) track(c *conn, add bool) {
	s.mu.Lock()
	if !add {
		delete(s.conns, c)
		s.mu.Unlock()
		return
	}
	s.conns[c] = struct{}{}
	closing := s.closing
	s.mu.Unlock()
	if closing {
		c.Kick(pb.KickReason_KICK_REASON_SERVER_SHUTDOWN)
	}
}

// ConnCount is the number of open connections.
func (s *Server) ConnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Shutdown refuses new connections, kicks every open one with SERVER_SHUTDOWN
// and waits until they have all left the world.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		c.Kick(pb.KickReason_KICK_REASON_SERVER_SHUTDOWN)
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
