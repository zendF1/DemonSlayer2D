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
	"math/rand/v2"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/items"
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
	// NewID hands out entity ids for monsters. It must draw from the same
	// counter as player ids. Required when the area has regions.
	NewID func() uint64
	// Rand drives monster behaviour and spawns; a nil Rand is randomly seeded.
	// Only the zone goroutine uses it.
	Rand *rand.Rand
}

type PlayerInfo struct {
	EntityID uint64
	CharID   int64
	Name     string
	Class    pb.CharClass
	Level    int
	Exp      int64
	Pos      Vec2
	// Combat state loaded from the store. HP and MP are -1 for "full"; HP 0
	// means the character logged out dead. Join copies Level, Exp and these
	// into the player, which owns them from then on.
	Points     combat.Points
	FreePoints int
	HP, MP     int
	// SkillPoints are unspent; Skills maps learned skill ids, all of the
	// player's class and in Area.Skills, to their level. Join copies Skills.
	SkillPoints int
	Skills      map[string]int
	// Xu is the wallet. Inventory is the bag and worn items, already checked
	// against Area.Items; Join clones it, and nil means an empty bag.
	Xu        int64
	Inventory *items.Inventory
	RespSeq   uint32 // seq of the EnterWorldReq; the zone answers it with EnterWorldResp
}

var (
	ErrZoneFull   = errors.New("zone: full")
	ErrZoneClosed = errors.New("zone: closed")
)

const slowTick = 50 * time.Millisecond

type player struct {
	entity
	charID   int64
	out      Outbox
	inputSeq uint32
	visible  map[uint64]struct{}

	// Progression; level and hp live in entity.
	stats      combat.Stats
	points     combat.Points
	freePoints int
	exp        int64
	mp         int
	// skillPoints are unspent; skills maps a learned skill id to its level.
	skillPoints int
	skills      map[string]int
	// xu is the wallet; inv is never nil.
	xu  int64
	inv *items.Inventory

	deadUntil     uint32     // tick at which a dead player respawns by itself
	restTicks     uint32     // consecutive ticks standing still, alive, on a rest point
	lastAttack    uint32     // tick of the last accepted attack; 0 means never
	pendingAttack *attackCmd // this tick's attack, resolved after drain
	pendingCast   *castCmd   // this tick's cast, resolved after the attack
	// skillReady maps a skill id to the first tick it may be cast again.
	skillReady map[string]uint32
	statsDirty bool // send SelfStats after this tick's WorldTick
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

type attackCmd struct {
	id     uint64
	seq    uint32
	target uint64
}

type allocateCmd struct {
	id     uint64
	seq    uint32
	points combat.Points
}

type respawnCmd struct {
	id  uint64
	seq uint32
}

type learnCmd struct {
	id      uint64
	seq     uint32
	skillID string
}

type Zone struct {
	cfg   Config
	area  Area
	mgrid Grid // area.monsterGrid()
	rng   *rand.Rand
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
	monsters map[uint64]*monster
	regions  []*regionState
	cells    map[cellKey]map[uint64]*entity
	departed map[uint64]chan error // evicted players whose Leave has not arrived yet
	lastSave time.Time
	events   tickEvents // combat events of the current tick
	// impacts are the attacks in flight, ordered by due tick and then seq;
	// impactSeq is the seq of the last one scheduled.
	impacts   []impact
	impactSeq uint32
}

// New creates a zone on area and fills its monster regions.
func New(cfg Config, area Area, saver Saver, log *slog.Logger) *Zone {
	if cfg.NewID == nil && len(area.Regions) > 0 {
		panic("zone: Config.NewID is required when the area has regions")
	}
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	z := &Zone{
		cfg:      cfg,
		area:     area,
		rng:      rng,
		saver:    saver,
		log:      log.With("map", cfg.MapID, "zone", cfg.ZoneNo),
		inbox:    make(chan any, 1024),
		done:     make(chan struct{}),
		players:  make(map[uint64]*player),
		monsters: make(map[uint64]*monster),
		cells:    make(map[cellKey]map[uint64]*entity),
		departed: make(map[uint64]chan error),
	}
	z.mgrid = z.area.monsterGrid()
	z.spawnAll()
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

// Attack queues an attack by player id on target. The zone answers seq with
// AttackResp or ErrorResp on the player's outbox. It never blocks and reports
// false if the inbox is full.
func (z *Zone) Attack(id uint64, seq uint32, target uint64) bool {
	select {
	case z.inbox <- attackCmd{id: id, seq: seq, target: target}:
		return true
	default:
		return false
	}
}

// Allocate queues spending free potential points. The zone answers seq with
// AllocatePointsResp or ErrorResp on the player's outbox. It never blocks and
// reports false if the inbox is full.
func (z *Zone) Allocate(id uint64, seq uint32, p combat.Points) bool {
	select {
	case z.inbox <- allocateCmd{id: id, seq: seq, points: p}:
		return true
	default:
		return false
	}
}

// Respawn queues a dead player's respawn request. The zone answers seq with
// RespawnResp or ErrorResp on the player's outbox. It never blocks and
// reports false if the inbox is full.
func (z *Zone) Respawn(id uint64, seq uint32) bool {
	select {
	case z.inbox <- respawnCmd{id: id, seq: seq}:
		return true
	default:
		return false
	}
}

// Learn queues learning skillID, or raising it a level. The zone answers seq
// with LearnSkillResp or ErrorResp on the player's outbox. It never blocks
// and reports false if the inbox is full.
func (z *Zone) Learn(id uint64, seq uint32, skillID string) bool {
	select {
	case z.inbox <- learnCmd{id: id, seq: seq, skillID: skillID}:
		return true
	default:
		return false
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

// advance runs one tick in the order of spec 6.2 and attack phases spec 3.2:
// apply commands, land the impacts now due, resolve attacks, move players,
// run monsters, run timers, then send each player its view.
func (z *Zone) advance(now time.Time) {
	z.tickNo++
	z.events = tickEvents{}
	z.drain()
	z.runImpacts()
	z.resolveAttacks()
	z.movePlayers()
	z.thinkMonsters()
	z.runSpawnTimers()
	z.runPlayerTimers()

	var slow []*player
	for _, p := range z.players {
		if !z.syncView(p) {
			slow = append(slow, p)
		}
	}
	for _, p := range z.players {
		p.changed, p.snap = false, false
	}
	for _, m := range z.monsters {
		m.changed, m.snap = false, false
	}
	for _, p := range slow {
		p.out.Kick(pb.KickReason_KICK_REASON_SLOW_CONSUMER)
		z.departed[p.id] = z.remove(p)
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

// movePlayers moves the living players and counts how long each has stood
// still on a rest point.
func (z *Zone) movePlayers() {
	step := z.cfg.MoveSpeed / float32(z.cfg.TickRate)
	for _, p := range z.players {
		if !p.dead && p.dir != (Vec2{}) {
			if np := Step(z.area.Grid, p.pos, p.dir, step); np != p.pos {
				p.pos = np
				p.changed = true
				z.updateCell(&p.entity)
			}
		}
		z.countRest(p)
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
	case attackCmd:
		z.queueAttack(c)
	case allocateCmd:
		z.allocate(c)
	case respawnCmd:
		z.respawn(c)
	case learnCmd:
		z.learn(c)
	case castCmd:
		z.queueCast(c)
	}
}

func (z *Zone) join(info PlayerInfo, out Outbox) error {
	if len(z.players) >= z.cfg.Capacity {
		return ErrZoneFull
	}
	if _, dup := z.players[info.EntityID]; dup {
		return fmt.Errorf("zone: duplicate entity %d", info.EntityID)
	}
	p := &player{
		entity: entity{
			id: info.EntityID, kind: pb.EntityKind_ENTITY_KIND_PLAYER, name: info.Name, class: info.Class, level: info.Level,
			pos: info.Pos,
		},
		charID: info.CharID, out: out, visible: make(map[uint64]struct{}),
		points: info.Points, freePoints: info.FreePoints, exp: info.Exp,
		skillPoints: info.SkillPoints, skills: maps.Clone(info.Skills), skillReady: map[string]uint32{},
		xu: info.Xu,
	}
	if p.skills == nil {
		p.skills = map[string]int{}
	}
	if info.Inventory != nil {
		p.inv = info.Inventory.Clone()
	} else {
		p.inv = items.NewInventory(z.bagSize())
	}
	z.recompute(p)
	p.hp, p.mp = info.HP, info.MP // -1 = full
	// HP 0: logged out dead, so back at the respawn point with full HP/MP (spec 3.6).
	if p.hp == 0 {
		p.pos, p.hp, p.mp = z.area.Respawn, -1, -1
	}
	if p.hp < 0 || p.hp > p.maxHP {
		p.hp = p.maxHP
	}
	if p.mp < 0 || p.mp > p.stats.MaxMP {
		p.mp = p.stats.MaxMP
	}
	p.cell = z.cellOf(p.pos)
	z.players[info.EntityID] = p
	z.addToCell(&p.entity)
	z.setCount()
	out.Send(&pb.Envelope{Seq: info.RespSeq, Payload: &pb.Envelope_EnterWorldResp{EnterWorldResp: &pb.EnterWorldResp{
		SelfEntityId: info.EntityID,
		MapId:        z.cfg.MapID,
		Zone:         int32(z.cfg.ZoneNo),
		X:            p.pos.X,
		Y:            p.pos.Y,
		ServerTick:   z.tickNo,
		MoveSpeed:    z.cfg.MoveSpeed,
		TickRate:     uint32(z.cfg.TickRate),
		Stats:        z.selfStats(p),
		Skills:       skillLevels(p),
		Inventory:    z.inventory(p),
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
	p.inputSeq = c.seq // acked even while dead, so the client stops resending
	if p.dead {
		return
	}
	if p.dir != c.dir {
		p.dir = c.dir
		p.changed = true
	}
}

// remove takes p out of the zone and starts saving it.
func (z *Zone) remove(p *player) chan error {
	delete(z.players, p.id)
	z.removeFromCell(&p.entity)
	z.setCount()
	done := make(chan error, 1)
	z.saver.Save(z.state(p), done)
	return done
}

func (z *Zone) evictAll(reason pb.KickReason) {
	for _, p := range z.players {
		p.out.Kick(reason)
		z.departed[p.id] = z.remove(p)
	}
}

// state is what gets saved, all from the player's live state; a full player
// saves its real HP and MP rather than -1.
func (z *Zone) state(p *player) store.CharState {
	// The saver runs on another goroutine, so it gets its own skill map and
	// item rows that share nothing with p (phase 3a spec 3).
	return store.CharState{ID: p.charID, MapID: z.cfg.MapID, X: p.pos.X, Y: p.pos.Y, Level: p.level, Exp: p.exp,
		Points: p.points, FreePoints: p.freePoints, HP: p.hp, MP: p.mp,
		SkillPoints: p.skillPoints, Skills: maps.Clone(p.skills), Xu: p.xu, Items: itemRows(p.inv)}
}

// itemRows is inv as store rows, bag first, then worn, each by slot. The
// rows share no slices with inv.
func itemRows(inv *items.Inventory) []store.ItemRow {
	var rows []store.ItemRow
	add := func(equipped bool, slot int, it *items.Item) {
		if it == nil {
			return
		}
		rows = append(rows, store.ItemRow{Equipped: equipped, Slot: slot, Code: it.Code, Count: it.Count,
			Quality: it.Quality, Element: it.Element, Enhance: it.Enhance,
			Lines: storeLines(it.Lines), Hidden: storeLines(it.Hidden)})
	}
	for i, it := range inv.Bag {
		add(false, i, it)
	}
	for i, it := range inv.Worn {
		add(true, i, it)
	}
	return rows
}

func storeLines(ls []items.Line) []store.ItemLine {
	out := make([]store.ItemLine, len(ls))
	for i, l := range ls {
		out[i] = store.ItemLine{Stat: l.Stat, Value: l.Value}
	}
	return out
}

// bagSize is the bag size of the area's item data; an area without item
// data (some tests) has no bag.
func (z *Zone) bagSize() int {
	if z.area.Items == nil {
		return 0
	}
	return z.area.Items.BagSize
}

// itemInfo is it on the wire (phase 3a spec 5).
func itemInfo(it *items.Item) *pb.ItemInfo {
	return &pb.ItemInfo{Code: it.Code, Count: int32(it.Count), Quality: int32(it.Quality),
		Element: classes.ToPB(it.Element), Enhance: int32(it.Enhance), Lines: pbLines(it.Lines), Hidden: pbLines(it.Hidden)}
}

func pbLines(ls []items.Line) []*pb.ItemLine {
	if len(ls) == 0 {
		return nil
	}
	out := make([]*pb.ItemLine, len(ls))
	for i, l := range ls {
		out[i] = &pb.ItemLine{Stat: l.Stat, Value: int32(l.Value)}
	}
	return out
}

// itemSlot is slot on the wire; an empty slot has no item.
func itemSlot(slot int, it *items.Item) *pb.ItemSlot {
	s := &pb.ItemSlot{Slot: int32(slot)}
	if it != nil {
		s.Item = itemInfo(it)
	}
	return s
}

// inventory is p's whole bag and worn items for EnterWorldResp: the bag
// size and the occupied slots only.
func (z *Zone) inventory(p *player) *pb.Inventory {
	out := &pb.Inventory{BagSize: int32(len(p.inv.Bag))}
	for i, it := range p.inv.Bag {
		if it != nil {
			out.Bag = append(out.Bag, itemSlot(i, it))
		}
	}
	for i, it := range p.inv.Worn {
		if it != nil {
			out.Equipped = append(out.Equipped, itemSlot(i, it))
		}
	}
	return out
}

// inventoryUpdate lists the given bag and worn slots of inv as they are now;
// an empty slot goes without an item.
func inventoryUpdate(inv *items.Inventory, bag, equip []int) *pb.InventoryUpdate {
	out := &pb.InventoryUpdate{}
	for _, i := range bag {
		out.Bag = append(out.Bag, itemSlot(i, inv.Bag[i]))
	}
	for _, i := range equip {
		out.Equipped = append(out.Equipped, itemSlot(i, inv.Worn[i]))
	}
	return out
}

// syncView sends p its EntityLeave/EntityEnter messages and WorldTick, with
// this tick's events about the entities p sees, then SelfStats if p's stats
// changed. It returns false if p's outbox is full.
func (z *Zone) syncView(p *player) bool {
	ok := true
	send := func(env *pb.Envelope) {
		if ok && !p.out.Send(env) {
			ok = false
		}
	}
	near := z.nearby(p.cell)
	for _, id := range slices.Sorted(maps.Keys(p.visible)) {
		if _, still := near[id]; !still {
			delete(p.visible, id)
			send(&pb.Envelope{Payload: &pb.Envelope_EntityLeave{EntityLeave: &pb.EntityLeave{Id: id}}})
		}
	}
	var moves []*pb.EntityMove
	for _, id := range slices.Sorted(maps.Keys(near)) {
		e := near[id]
		if _, seen := p.visible[id]; !seen {
			if e.dead && e.kind == pb.EntityKind_ENTITY_KIND_MONSTER {
				continue // corpses are only shown to those who saw the monster alive
			}
			p.visible[id] = struct{}{}
			send(enterEnv(e))
			continue
		}
		if e.changed {
			moves = append(moves, &pb.EntityMove{Id: id, X: e.pos.X, Y: e.pos.Y, DirX: e.dir.X, DirY: e.dir.Y, Snap: e.snap})
		}
	}
	wt := &pb.WorldTick{Tick: z.tickNo, AckInputSeq: p.inputSeq, Moves: moves}
	z.addEvents(p, wt)
	send(&pb.Envelope{Payload: &pb.Envelope_WorldTick{WorldTick: wt}})
	if p.statsDirty {
		p.statsDirty = false
		send(&pb.Envelope{Payload: &pb.Envelope_SelfStats{SelfStats: z.selfStats(p)}})
	}
	return ok
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

// ticks converts seconds to a whole number of ticks, rounding up, at least 1.
func (z *Zone) ticks(sec float64) uint32 {
	// The epsilon keeps float error (0.7*10 = 7.000000000000001) from adding a tick.
	n := math.Ceil(sec*float64(z.cfg.TickRate) - 1e-9)
	return uint32(max(n, 1))
}

func closedNil() <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	return ch
}
