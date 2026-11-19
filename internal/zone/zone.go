// Package zone runs one map instance ("khu"). A single goroutine owns every
// entity in it, simulates movement each tick and tells each player what it sees.
package zone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
)

// Outbox delivers messages to one connected player. Implementations must not block.
type Outbox interface {
	// Send queues env and reports whether it was accepted.
	Send(env *pb.Envelope) bool
	// Kick tells the connection to send Kicked and close.
	Kick(reason pb.KickReason)
}

// Saver persists character state without blocking.
type Saver interface {
	Save(st store.CharState, done chan<- error)
}

type Config struct {
	MapID        string
	ZoneNo       int
	TickRate     int
	MoveSpeed    float32 // tiles per second at speed factor 1
	AOICellSize  int
	Capacity     int
	SaveInterval time.Duration
}

type PlayerInfo struct {
	EntityID uint64
	CharID   int64
	Name     string
	Class    pb.CharClass
	Level    int
	Exp      int64
	Pos      Vec2
	RespSeq  uint32 // seq of the EnterWorldReq; the zone answers it with EnterWorldResp
}

var (
	ErrZoneFull   = errors.New("zone: full")
	ErrZoneClosed = errors.New("zone: closed")
)

const slowTick = 50 * time.Millisecond

type cellKey struct{ X, Y int }

type player struct {
	info     PlayerInfo
	out      Outbox
	pos, dir Vec2
	inputSeq uint32
	cell     cellKey
	visible  map[uint64]struct{}
	changed  bool
}

type joinCmd struct {
	info  PlayerInfo
	out   Outbox
	reply chan error
}

type leaveCmd struct {
	id    uint64
	reply chan (<-chan error)
}

type moveCmd struct {
	id  uint64
	seq uint32
	dir Vec2
}

type Zone struct {
	cfg   Config
	grid  Grid
	saver Saver
	log   *slog.Logger

	inbox chan any
	done  chan struct{}

	count      atomic.Int32
	emptySince atomic.Int64 // unix nanos, 0 while occupied
	stats      tickStats

	// Owned by the zone goroutine.
	tickNo   uint32
	players  map[uint64]*player
	cells    map[cellKey]map[uint64]*player
	departed map[uint64]chan error // evicted players whose Leave has not arrived yet
	lastSave time.Time
}

func New(cfg Config, grid Grid, saver Saver, log *slog.Logger) *Zone {
	z := &Zone{
		cfg:      cfg,
		grid:     grid,
		saver:    saver,
		log:      log.With("map", cfg.MapID, "zone", cfg.ZoneNo),
		inbox:    make(chan any, 1024),
		done:     make(chan struct{}),
		players:  make(map[uint64]*player),
		cells:    make(map[cellKey]map[uint64]*player),
		departed: make(map[uint64]chan error),
	}
	z.emptySince.Store(time.Now().UnixNano())
	return z
}

func (z *Zone) Config() Config        { return z.cfg }
func (z *Zone) Count() int            { return int(z.count.Load()) }
func (z *Zone) Done() <-chan struct{} { return z.done }
func (z *Zone) TakeStats() TickStats  { return z.stats.take() }

// EmptySince is when the zone last became empty, or zero while it has players.
func (z *Zone) EmptySince() time.Time {
	v := z.emptySince.Load()
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(0, v)
}

// Join adds a player. On success the zone has already queued EnterWorldResp on out.
// ctx only bounds waiting for room in the inbox; once queued, Join waits for the zone's answer.
func (z *Zone) Join(ctx context.Context, info PlayerInfo, out Outbox) error {
	reply := make(chan error, 1)
	select {
	case z.inbox <- joinCmd{info: info, out: out, reply: reply}:
	case <-z.done:
		return ErrZoneClosed
	case <-ctx.Done():
		return ctx.Err()
	}
	// Once queued, the zone will apply the join (and send EnterWorldResp) within a
	// tick, so wait for its answer instead of giving up on ctx: returning an error
	// here would contradict the EnterWorldResp the client is about to receive.
	select {
	case err := <-reply:
		return err
	case <-z.done:
		select {
		case err := <-reply:
			return err
		default:
			return ErrZoneClosed
		}
	}
}

// Leave removes a player. The returned channel yields the save result.
func (z *Zone) Leave(id uint64) <-chan error {
	reply := make(chan (<-chan error), 1)
	select {
	case z.inbox <- leaveCmd{id: id, reply: reply}:
	case <-z.done:
		return closedNil()
	}
	select {
	case ch := <-reply:
		return ch
	case <-z.done:
		return closedNil()
	}
}

// Move sets a player's direction. It never blocks; input is dropped if the inbox is full.
func (z *Zone) Move(id uint64, seq uint32, x, y float32) {
	select {
	case z.inbox <- moveCmd{id: id, seq: seq, dir: ClampDir(x, y)}:
	default:
		z.log.Debug("inbox full, dropping move", "entity", id)
	}
}

// Run ticks until ctx is cancelled, then saves and kicks every remaining player.
func (z *Zone) Run(ctx context.Context) {
	defer close(z.done)
	t := time.NewTicker(time.Second / time.Duration(z.cfg.TickRate))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			z.drain()
			z.evictAll(pb.KickReason_KICK_REASON_SERVER_SHUTDOWN)
			return
		case now := <-t.C:
			start := time.Now()
			z.safeAdvance(now)
			d := time.Since(start)
			z.stats.record(d)
			if d > slowTick {
				z.log.Warn("slow tick", "duration", d, "players", len(z.players))
			}
		}
	}
}

func (z *Zone) safeAdvance(now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			z.log.Error("zone panic, evicting players", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			z.evictAll(pb.KickReason_KICK_REASON_ZONE_RESTART)
		}
	}()
	z.advance(now)
}

// advance runs one tick: apply commands, move, then send each player its view.
func (z *Zone) advance(now time.Time) {
	z.tickNo++
	z.drain()

	step := z.cfg.MoveSpeed / float32(z.cfg.TickRate)
	for _, p := range z.players {
		if p.dir == (Vec2{}) {
			continue
		}
		if np := Step(z.grid, p.pos, p.dir, step); np != p.pos {
			p.pos = np
			p.changed = true
			z.updateCell(p)
		}
	}

	var slow []*player
	for _, p := range z.players {
		if !z.syncView(p) {
			slow = append(slow, p)
		}
	}
	for _, p := range z.players {
		p.changed = false
	}
	for _, p := range slow {
		p.out.Kick(pb.KickReason_KICK_REASON_SLOW_CONSUMER)
		z.departed[p.info.EntityID] = z.remove(p)
	}

	if z.lastSave.IsZero() {
		z.lastSave = now
	} else if now.Sub(z.lastSave) >= z.cfg.SaveInterval {
		for _, p := range z.players {
			z.saver.Save(z.state(p), nil)
		}
		z.lastSave = now
	}
}

// drain handles the commands queued so far (not ones that arrive meanwhile).
func (z *Zone) drain() {
	for n := len(z.inbox); n > 0; n-- {
		z.handle(<-z.inbox)
	}
}

func (z *Zone) handle(c any) {
	switch c := c.(type) {
	case joinCmd:
		c.reply <- z.join(c.info, c.out)
	case leaveCmd:
		c.reply <- z.leave(c.id)
	case moveCmd:
		z.move(c)
	}
}

func (z *Zone) join(info PlayerInfo, out Outbox) error {
	if len(z.players) >= z.cfg.Capacity {
		return ErrZoneFull
	}
	if _, dup := z.players[info.EntityID]; dup {
		return fmt.Errorf("zone: duplicate entity %d", info.EntityID)
	}
	p := &player{info: info, out: out, pos: info.Pos, visible: make(map[uint64]struct{})}
	p.cell = z.cellOf(p.pos)
	z.players[info.EntityID] = p
	z.addToCell(p)
	z.setCount()
	out.Send(&pb.Envelope{Seq: info.RespSeq, Payload: &pb.Envelope_EnterWorldResp{EnterWorldResp: &pb.EnterWorldResp{
		SelfEntityId: info.EntityID,
		MapId:        z.cfg.MapID,
		Zone:         int32(z.cfg.ZoneNo),
		X:            p.pos.X,
		Y:            p.pos.Y,
		ServerTick:   z.tickNo,
		MoveSpeed:    z.cfg.MoveSpeed,
	}}})
	return nil
}

func (z *Zone) leave(id uint64) <-chan error {
	if p, ok := z.players[id]; ok {
		return z.remove(p)
	}
	if d, ok := z.departed[id]; ok {
		delete(z.departed, id)
		return d
	}
	return closedNil()
}

func (z *Zone) move(c moveCmd) {
	p, ok := z.players[c.id]
	if !ok || c.seq < p.inputSeq {
		return
	}
	p.inputSeq = c.seq
	if p.dir != c.dir {
		p.dir = c.dir
		p.changed = true
	}
}

// remove takes p out of the zone and starts saving it.
func (z *Zone) remove(p *player) chan error {
	delete(z.players, p.info.EntityID)
	z.removeFromCell(p)
	z.setCount()
	done := make(chan error, 1)
	z.saver.Save(z.state(p), done)
	return done
}

func (z *Zone) evictAll(reason pb.KickReason) {
	for _, p := range z.players {
		p.out.Kick(reason)
		z.departed[p.info.EntityID] = z.remove(p)
	}
}

func (z *Zone) state(p *player) store.CharState {
	return store.CharState{ID: p.info.CharID, MapID: z.cfg.MapID, X: p.pos.X, Y: p.pos.Y, Level: p.info.Level, Exp: p.info.Exp}
}

// syncView sends p its EntityLeave/EntityEnter messages and WorldTick. It returns false if p's outbox is full.
func (z *Zone) syncView(p *player) bool {
	ok := true
	send := func(env *pb.Envelope) {
		if ok && !p.out.Send(env) {
			ok = false
		}
	}
	near := make(map[uint64]*player)
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for id, o := range z.cells[cellKey{p.cell.X + dx, p.cell.Y + dy}] {
				near[id] = o
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.visible)) {
		if _, still := near[id]; !still {
			delete(p.visible, id)
			send(&pb.Envelope{Payload: &pb.Envelope_EntityLeave{EntityLeave: &pb.EntityLeave{Id: id}}})
		}
	}
	var moves []*pb.EntityMove
	for _, id := range slices.Sorted(maps.Keys(near)) {
		o := near[id]
		if _, seen := p.visible[id]; !seen {
			p.visible[id] = struct{}{}
			send(enterEnv(o))
			continue
		}
		if o.changed {
			moves = append(moves, &pb.EntityMove{Id: id, X: o.pos.X, Y: o.pos.Y, DirX: o.dir.X, DirY: o.dir.Y})
		}
	}
	send(&pb.Envelope{Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
		Tick: z.tickNo, AckInputSeq: p.inputSeq, Moves: moves,
	}}})
	return ok
}

func enterEnv(p *player) *pb.Envelope {
	return &pb.Envelope{Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
		Id:    p.info.EntityID,
		Kind:  pb.EntityKind_ENTITY_KIND_PLAYER,
		Name:  p.info.Name,
		Class: p.info.Class,
		Level: int32(p.info.Level),
		X:     p.pos.X,
		Y:     p.pos.Y,
		DirX:  p.dir.X,
		DirY:  p.dir.Y,
	}}}
}

func (z *Zone) cellOf(pos Vec2) cellKey {
	size := float64(z.cfg.AOICellSize)
	return cellKey{int(math.Floor(float64(pos.X) / size)), int(math.Floor(float64(pos.Y) / size))}
}

func (z *Zone) addToCell(p *player) {
	c := z.cells[p.cell]
	if c == nil {
		c = make(map[uint64]*player)
		z.cells[p.cell] = c
	}
	c[p.info.EntityID] = p
}

func (z *Zone) removeFromCell(p *player) {
	if c := z.cells[p.cell]; c != nil {
		delete(c, p.info.EntityID)
		if len(c) == 0 {
			delete(z.cells, p.cell)
		}
	}
}

func (z *Zone) updateCell(p *player) {
	if nc := z.cellOf(p.pos); nc != p.cell {
		z.removeFromCell(p)
		p.cell = nc
		z.addToCell(p)
	}
}

func (z *Zone) setCount() {
	n := len(z.players)
	z.count.Store(int32(n))
	if n > 0 {
		z.emptySince.Store(0)
	} else if z.emptySince.Load() == 0 {
		z.emptySince.Store(time.Now().UnixNano())
	}
}

func closedNil() <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	return ch
}
