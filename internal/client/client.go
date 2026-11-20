// Package client is a Go implementation of the game protocol, used by botclient and e2e tests.
package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gateway"
	"demonslayer2d/internal/pb"
)

// ServerError is an Error envelope returned for a request.
type ServerError struct {
	Code    pb.ErrorCode
	Message string
}

func (e *ServerError) Error() string { return fmt.Sprintf("server error %s: %s", e.Code, e.Message) }

// Code returns the server error code inside err, or UNSPECIFIED.
func Code(err error) pb.ErrorCode {
	var se *ServerError
	if errors.As(err, &se) {
		return se.Code
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

type Client struct {
	ws       *websocket.Conn
	seq      atomic.Uint32
	inputSeq atomic.Uint32

	mu      sync.Mutex
	pending map[uint32]chan *pb.Envelope
	readErr error

	pushes chan *pb.Envelope
	closed chan struct{}
}

func Dial(ctx context.Context, url string) (*Client, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(1 << 20)
	c := &Client{
		ws:      ws,
		pending: make(map[uint32]chan *pb.Envelope),
		pushes:  make(chan *pb.Envelope, 4096),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Pushes yields server-initiated messages (seq 0). Messages are dropped if it is not drained.
func (c *Client) Pushes() <-chan *pb.Envelope { return c.pushes }

// Closed is closed when the connection ends.
func (c *Client) Closed() <-chan struct{} { return c.closed }

// Err is the read error that ended the connection.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}

func (c *Client) Close() { _ = c.ws.Close(websocket.StatusNormalClosure, "") }

func (c *Client) readLoop() {
	defer close(c.closed)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			c.mu.Unlock()
			return
		}
		env := &pb.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			continue
		}
		if env.Seq != 0 {
			c.mu.Lock()
			ch, ok := c.pending[env.Seq]
			delete(c.pending, env.Seq)
			c.mu.Unlock()
			if ok {
				ch <- env
				continue
			}
		}
		select {
		case c.pushes <- env:
		default:
		}
	}
}

func (c *Client) write(ctx context.Context, env *pb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return err
	}
	return c.ws.Write(ctx, websocket.MessageBinary, data)
}

// Request sends env with a fresh seq and waits for its response. Error envelopes become *ServerError.
func (c *Client) Request(ctx context.Context, env *pb.Envelope) (*pb.Envelope, error) {
	env.Seq = c.seq.Add(1)
	ch := make(chan *pb.Envelope, 1)
	c.mu.Lock()
	c.pending[env.Seq] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, env.Seq)
		c.mu.Unlock()
	}
	if err := c.write(ctx, env); err != nil {
		forget()
		return nil, err
	}
	select {
	case resp := <-ch:
		if e := resp.GetError(); e != nil {
			return nil, &ServerError{Code: e.Code, Message: e.Detail}
		}
		return resp, nil
	case <-c.closed:
		forget()
		return nil, fmt.Errorf("client: connection closed: %w", c.Err())
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	}
}

func (c *Client) Register(ctx context.Context, username, password string) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_RegisterReq{RegisterReq: &pb.RegisterReq{Username: username, Password: password}}})
	return err
}

func (c *Client) Login(ctx context.Context, username, password string) (int64, error) {
	return c.LoginVersion(ctx, username, password, gateway.ProtocolVersion)
}

func (c *Client) LoginVersion(ctx context.Context, username, password string, version uint32) (int64, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_LoginReq{LoginReq: &pb.LoginReq{
		Username: username, Password: password, ProtocolVersion: version,
	}}})
	if err != nil {
		return 0, err
	}
	return resp.GetLoginResp().GetAccountId(), nil
}

func (c *Client) ListChars(ctx context.Context) ([]*pb.CharSummary, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_CharListReq{CharListReq: &pb.CharListReq{}}})
	if err != nil {
		return nil, err
	}
	return resp.GetCharListResp().GetChars(), nil
}

func (c *Client) CreateChar(ctx context.Context, slot int32, name string, class pb.CharClass) (*pb.CharSummary, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_CreateCharReq{CreateCharReq: &pb.CreateCharReq{
		Slot: slot, Name: name, CharClass: class,
	}}})
	if err != nil {
		return nil, err
	}
	return resp.GetCreateCharResp().GetChar(), nil
}

func (c *Client) DeleteChar(ctx context.Context, charID int64, confirmName string) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_DeleteCharReq{DeleteCharReq: &pb.DeleteCharReq{
		CharId: charID, ConfirmName: confirmName,
	}}})
	return err
}

func (c *Client) EnterWorld(ctx context.Context, charID int64) (*pb.EnterWorldResp, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_EnterWorldReq{EnterWorldReq: &pb.EnterWorldReq{CharId: charID}}})
	if err != nil {
		return nil, err
	}
	r := resp.GetEnterWorldResp()
	if r == nil {
		return nil, fmt.Errorf("client: unexpected response to EnterWorldReq: %v", resp)
	}
	return r, nil
}

func (c *Client) LeaveWorld(ctx context.Context) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_LeaveWorldReq{LeaveWorldReq: &pb.LeaveWorldReq{}}})
	return err
}

// Attack attacks target and waits for the zone's reply.
func (c *Client) Attack(ctx context.Context, target uint64) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_AttackReq{AttackReq: &pb.AttackReq{TargetId: target}}})
	return err
}

// Cast casts skillID at target or along (dirX, dirY) and waits for the
// zone's reply.
func (c *Client) Cast(ctx context.Context, skillID string, target uint64, dirX, dirY float32) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_CastReq{CastReq: &pb.CastReq{
		SkillId: skillID, TargetId: target, DirX: dirX, DirY: dirY}}})
	return err
}

// AllocatePoints spends free potential points and returns the resulting stats.
func (c *Client) AllocatePoints(ctx context.Context, p combat.Points) (*pb.SelfStats, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_AllocatePointsReq{AllocatePointsReq: &pb.AllocatePointsReq{
		Strength: uint32(p.Str), Agility: uint32(p.Agi), Vitality: uint32(p.Vit), Intellect: uint32(p.Int),
	}}})
	if err != nil {
		return nil, err
	}
	r := resp.GetAllocatePointsResp()
	if r == nil {
		return nil, fmt.Errorf("client: unexpected response to AllocatePointsReq: %v", resp)
	}
	return r.Stats, nil
}

// Respawn asks to respawn and returns where the character stands.
func (c *Client) Respawn(ctx context.Context) (*pb.RespawnResp, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_RespawnReq{RespawnReq: &pb.RespawnReq{}}})
	if err != nil {
		return nil, err
	}
	r := resp.GetRespawnResp()
	if r == nil {
		return nil, fmt.Errorf("client: unexpected response to RespawnReq: %v", resp)
	}
	return r, nil
}

// Learn learns skillID, or raises it a level, and returns the new stats and
// learned skills.
func (c *Client) Learn(ctx context.Context, skillID string) (*pb.LearnSkillResp, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_LearnSkillReq{LearnSkillReq: &pb.LearnSkillReq{SkillId: skillID}}})
	if err != nil {
		return nil, err
	}
	r := resp.GetLearnSkillResp()
	if r == nil {
		return nil, fmt.Errorf("client: unexpected response to LearnSkillReq: %v", resp)
	}
	return r, nil
}

// Move sends a direction (no response). It returns the input seq used.
func (c *Client) Move(ctx context.Context, dx, dy float32) (uint32, error) {
	seq := c.inputSeq.Add(1)
	return seq, c.write(ctx, &pb.Envelope{Payload: &pb.Envelope_MoveInput{MoveInput: &pb.MoveInput{InputSeq: seq, DirX: dx, DirY: dy}}})
}

// Ping measures the round trip time.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_Ping{Ping: &pb.Ping{ClientTime: start.UnixMilli()}}})
	if err != nil {
		return 0, err
	}
	if resp.GetPong() == nil {
		return 0, fmt.Errorf("client: unexpected response to Ping: %v", resp)
	}
	return time.Since(start), nil
}

// SendRaw writes arbitrary bytes as one binary message (for protocol tests).
func (c *Client) SendRaw(ctx context.Context, data []byte) error {
	return c.ws.Write(ctx, websocket.MessageBinary, data)
}
