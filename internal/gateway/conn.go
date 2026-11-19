package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/world"
)

type connState int

const (
	stateConnected connState = iota
	stateAuthed
	stateInWorld
)

type conn struct {
	srv *Server
	ws  *websocket.Conn
	id  uint64
	ip  string
	log *slog.Logger // never reassigned: timers log from other goroutines
	out *outbox

	kicked   atomic.Bool
	kickOnce sync.Once
	done     chan struct{} // closed after cleanup (left world, character saved)

	idle      *time.Timer
	authTimer *time.Timer

	// Owned by the read goroutine.
	state       connState
	accountID   int64
	member      *world.Membership
	packets     *rate.Limiter
	moves       *rate.Limiter
	attacks     *rate.Limiter
	lastDropSec int64
	dropStreak  int
}

func (s *Server) newConn(ws *websocket.Conn, ip string) *conn {
	c := &conn{
		srv:     s,
		ws:      ws,
		id:      s.nextID.Add(1),
		ip:      ip,
		done:    make(chan struct{}),
		packets: rate.NewLimiter(packetsPerSec, packetsPerSec),
		moves:   rate.NewLimiter(movesPerSec, movesPerSec),
		attacks: rate.NewLimiter(attacksPerSec, attacksPerSec),
	}
	c.log = s.log.With("conn_id", c.id)
	c.out = newOutbox(outboxSize, c.Kick)
	c.idle = time.AfterFunc(idleTimeout, func() { c.Kick(pb.KickReason_KICK_REASON_IDLE_TIMEOUT) })
	c.authTimer = time.AfterFunc(authTimeout, func() { c.Kick(pb.KickReason_KICK_REASON_IDLE_TIMEOUT) })
	return c
}

// Kick sends Kicked and closes the connection. Safe from any goroutine; only the first call counts.
func (c *conn) Kick(reason pb.KickReason) {
	c.kickOnce.Do(func() {
		c.kicked.Store(true)
		c.log.Info("kick", "reason", reason.String())
		c.out.closeWith(&pb.Envelope{Payload: &pb.Envelope_Kicked{Kicked: &pb.Kicked{Reason: reason}}})
	})
}

func (c *conn) Done() <-chan struct{} { return c.done }

func (c *conn) writeLoop() {
	for env := range c.out.ch {
		data, err := proto.Marshal(env)
		if err != nil {
			c.log.Error("marshal envelope", "err", err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err = c.ws.Write(ctx, websocket.MessageBinary, data)
		cancel()
		if err != nil {
			c.out.closeWith(nil)
			c.ws.CloseNow()
			return
		}
	}
	c.ws.Close(websocket.StatusNormalClosure, "")
}

func (c *conn) readLoop(ctx context.Context) {
	for {
		typ, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		c.idle.Reset(idleTimeout)
		if c.kicked.Load() {
			continue // keep reading until the close handshake finishes
		}
		if typ != websocket.MessageBinary {
			c.Kick(pb.KickReason_KICK_REASON_PROTOCOL_ERROR)
			continue
		}
		if !c.allowPacket(time.Now()) {
			continue
		}
		env := &pb.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			c.Kick(pb.KickReason_KICK_REASON_PROTOCOL_ERROR)
			continue
		}
		c.safeDispatch(ctx, env)
	}
}

// safeDispatch handles one request; a panic is logged and closes only this connection.
func (c *conn) safeDispatch(ctx context.Context, env *pb.Envelope) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("panic handling request", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			c.Kick(pb.KickReason_KICK_REASON_PROTOCOL_ERROR)
		}
	}()
	c.dispatch(ctx, env)
}

// allowPacket applies the per-connection packet limit; dropping in 5 consecutive seconds kicks.
func (c *conn) allowPacket(now time.Time) bool {
	if c.packets.AllowN(now, 1) {
		return true
	}
	sec := now.Unix()
	switch {
	case sec == c.lastDropSec:
	case sec == c.lastDropSec+1:
		c.dropStreak++
	default:
		c.dropStreak = 1
	}
	c.lastDropSec = sec
	if c.dropStreak >= dropStreakKick {
		c.Kick(pb.KickReason_KICK_REASON_RATE_LIMITED)
	}
	return false
}

func (c *conn) cleanup() {
	c.idle.Stop()
	c.authTimer.Stop()
	if c.member != nil {
		// Done must only close after the save, or a duplicate login could read stale data.
		c.waitSaved(c.member.Leave(), "save on disconnect")
		c.member = nil
	}
	if c.accountID != 0 {
		c.srv.deps.World.Release(c.accountID, c)
	}
	c.out.closeWith(nil)
	close(c.done)
}

// waitSaved waits for a character save to finish and logs if it is slow or fails.
// It never gives up early: continuing before the save lands would let the next
// login or EnterWorld read an older position. The saver always answers, either
// when the write succeeds or with an error when it is shut down.
func (c *conn) waitSaved(done <-chan error, what string) {
	select {
	case err := <-done:
		if err != nil {
			c.log.Error(what+" failed", "err", err)
		}
		return
	case <-time.After(requestTimeout):
		c.log.Warn(what+" is slow, still waiting", "account_id", c.accountID)
	}
	if err := <-done; err != nil {
		c.log.Error(what+" failed", "err", err)
	}
}

// send queues env; a full queue means the client is not reading, so it is kicked.
func (c *conn) send(env *pb.Envelope) {
	if !c.out.Send(env) {
		c.Kick(pb.KickReason_KICK_REASON_SLOW_CONSUMER)
	}
}

func (c *conn) fail(seq uint32, code pb.ErrorCode, msg string) {
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Error{Error: &pb.ErrorResp{Code: code, Detail: msg}}})
}

func (c *conn) failErr(seq uint32, err error) {
	code := errorCode(err)
	if code == pb.ErrorCode_ERROR_CODE_INTERNAL {
		c.log.Error("request failed", "account_id", c.accountID, "err", err)
	}
	c.fail(seq, code, code.String())
}
