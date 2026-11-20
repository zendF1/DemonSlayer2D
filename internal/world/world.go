// Package world owns every map's zones, places characters into zones and tracks
// which account is online where.
package world

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

// Session is an online connection that can be kicked.
type Session interface {
	Kick(reason pb.KickReason)
	// Done is closed once the session has left the world and its character is saved.
	Done() <-chan struct{}
}

// ErrStopped is returned by Enter once the world's Start context is cancelled.
var ErrStopped = errors.New("world: stopped")

type Config struct {
	StartMap     string
	TickRate     int
	MoveSpeed    float32
	AOICellSize  int
	ZoneCapacity int
	SaveInterval time.Duration
	IdleZoneTTL  time.Duration
	JanitorEvery time.Duration
}

type World struct {
	cfg   Config
	maps  map[string]*mapdata.Map
	areas map[string]zone.Area // per map, shared by its zones
	// skills is skills.json by id and items is items.json, shared and read-only.
	skills map[string]gamedata.Skill
	items  *gamedata.Items
	saver  zone.Saver
	log    *slog.Logger
	start  mapdata.Point

	ctx context.Context
	wg  sync.WaitGroup

	mu       sync.Mutex
	zones    map[string][]*slot // index i holds zone number i+1; nil = closed
	online   map[int64]Session
	handoffs map[int64]*accountLock // per-account Takeover serialisation

	nextEntity atomic.Uint64
}

type slot struct {
	z        *zone.Zone
	cancel   context.CancelFunc
	no       int
	reserved atomic.Int32 // joins in flight
}

// New creates a world over maps. monsters must already be validated against maps.
func New(cfg Config, maps map[string]*mapdata.Map, monsters *gamedata.Data, saver zone.Saver, log *slog.Logger) (*World, error) {
	m, ok := maps[cfg.StartMap]
	if !ok {
		return nil, fmt.Errorf("world: start map %q not found", cfg.StartMap)
	}
	if monsters == nil {
		return nil, errors.New("world: no monster data")
	}
	if monsters.Items == nil {
		return nil, errors.New("world: no item data")
	}
	// Monsters look for players only in the 3x3 AOI cells around them.
	for _, k := range monsters.Kinds {
		if float64(cfg.AOICellSize) < k.DetectRange {
			return nil, fmt.Errorf("world: AOI cell size %d is below the detect range %v of monster %q", cfg.AOICellSize, k.DetectRange, k.ID)
		}
	}
	areas := make(map[string]zone.Area, len(maps))
	for id, mp := range maps {
		areas[id] = areaOf(mp, monsters)
	}
	return &World{
		cfg: cfg, maps: maps, areas: areas, skills: monsters.Skills, items: monsters.Items, saver: saver, log: log,
		start:    m.Spawns["default"],
		zones:    make(map[string][]*slot),
		online:   make(map[int64]Session),
		handoffs: make(map[int64]*accountLock),
	}, nil
}

// areaOf converts a map's collision and triggers, the monster data its
// regions use, the EXP table, the skills and the item data into the zone's
// view of the map. Region tiles are computed here once.
func areaOf(m *mapdata.Map, d *gamedata.Data) zone.Area {
	a := zone.Area{Grid: m, Levels: d.Levels, Exp: d.Exp, Skills: d.Skills, Items: d.Items}
	for _, sp := range m.Respawns {
		a.Respawns = append(a.Respawns, zone.Vec2{X: sp.X, Y: sp.Y})
	}
	for _, r := range m.SafeZones {
		a.Safe = append(a.Safe, zone.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H})
	}
	for _, r := range m.RestPoints {
		a.Rests = append(a.Rests, zone.Circle{C: zone.Vec2{X: r.X, Y: r.Y}, R: r.Radius})
	}
	for _, r := range m.Regions {
		reg := zone.Region{ID: r.ID, Nest: zone.Vec2{X: r.Nest.X, Y: r.Nest.Y}, Count: r.Count,
			MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, Kind: d.Kinds[r.Monster]}
		for _, rc := range r.Rects {
			reg.Rects = append(reg.Rects, zone.Rect{X: rc.X, Y: rc.Y, W: rc.W, H: rc.H})
		}
		for _, p := range m.RegionTiles(r) {
			reg.Tiles = append(reg.Tiles, zone.Vec2{X: p.X, Y: p.Y})
		}
		a.Regions = append(a.Regions, reg)
	}
	return a
}

// Start opens zones 1 to min_zones of every map (spec 3.1) and the idle-zone
// janitor. Everything stops when ctx is cancelled. It must be called before Enter.
func (w *World) Start(ctx context.Context) {
	w.ctx = ctx
	w.mu.Lock()
	for id := range w.maps {
		for range w.minZones(id) {
			w.openZoneLocked(id)
		}
	}
	w.mu.Unlock()
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		t := time.NewTicker(w.cfg.JanitorEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				w.closeIdle(now)
			}
		}
	}()
}

// Wait blocks until every zone goroutine has exited (after the Start context is cancelled).
func (w *World) Wait() { w.wg.Wait() }

// minZones is how many zones of mapID are always open, at least one.
func (w *World) minZones(mapID string) int {
	if m := w.maps[mapID]; m != nil {
		return max(m.MinZones, 1)
	}
	return 1
}

// StartPosition is where new characters appear.
func (w *World) StartPosition() (string, mapdata.Point) { return w.cfg.StartMap, w.start }

// openZoneLocked starts a zone. Callers hold w.mu and, after Start, have checked
// w.ctx is not cancelled, so wg.Add never happens after Wait has started.
func (w *World) openZoneLocked(mapID string) *slot {
	slots := w.zones[mapID]
	idx := len(slots)
	for i, s := range slots {
		if s == nil {
			idx = i
			break
		}
	}
	z := zone.New(zone.Config{
		MapID: mapID, ZoneNo: idx + 1, TickRate: w.cfg.TickRate, MoveSpeed: w.cfg.MoveSpeed,
		AOICellSize: w.cfg.AOICellSize, Capacity: w.cfg.ZoneCapacity, SaveInterval: w.cfg.SaveInterval,
		NewID: func() uint64 { return w.nextEntity.Add(1) },
	}, w.areas[mapID], w.saver, w.log)
	zctx, cancel := context.WithCancel(w.ctx)
	s := &slot{z: z, cancel: cancel, no: idx + 1}
	if idx == len(slots) {
		w.zones[mapID] = append(slots, s)
	} else {
		slots[idx] = s
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		z.Run(zctx)
	}()
	return s
}

type EnterRequest struct {
	Char    store.Character
	Out     zone.Outbox
	RespSeq uint32
}

// Membership is a character's presence in a zone.
type Membership struct {
	z        *zone.Zone
	out      zone.Outbox // the connection's, kept across transfers
	EntityID uint64
	MapID    string
	ZoneNo   int
}

func (m *Membership) Move(seq uint32, x, y float32) { m.z.Move(m.EntityID, seq, x, y) }

// Attack queues an attack on target; the zone answers seq on the outbox. It
// reports false if the zone's inbox is full.
func (m *Membership) Attack(seq uint32, target uint64) bool {
	return m.z.Attack(m.EntityID, seq, target)
}

// Allocate queues spending free potential points; the zone answers seq on the
// outbox. It reports false if the zone's inbox is full.
func (m *Membership) Allocate(seq uint32, p combat.Points) bool {
	return m.z.Allocate(m.EntityID, seq, p)
}

// Respawn queues a respawn request; the zone answers seq on the outbox. It
// reports false if the zone's inbox is full.
func (m *Membership) Respawn(seq uint32) bool { return m.z.Respawn(m.EntityID, seq) }

// Learn queues learning skillID or raising it a level; the zone answers seq
// on the outbox. It reports false if the zone's inbox is full.
func (m *Membership) Learn(seq uint32, skillID string) bool {
	return m.z.Learn(m.EntityID, seq, skillID)
}

// Cast queues casting skillID at target or along (dirX, dirY); the zone
// sanitises the direction and answers seq on the outbox. It reports false if
// the zone's inbox is full.
func (m *Membership) Cast(seq uint32, skillID string, target uint64, dirX, dirY float32) bool {
	return m.z.Cast(m.EntityID, seq, skillID, target, zone.Vec2{X: dirX, Y: dirY})
}

// Pickup queues picking up the ground item itemID; the zone answers seq on
// the outbox. It reports false if the zone's inbox is full.
func (m *Membership) Pickup(seq uint32, itemID uint64) bool {
	return m.z.Pickup(m.EntityID, seq, itemID)
}

// Equip queues wearing the piece in bagSlot; the zone answers seq on the
// outbox. It reports false if the zone's inbox is full.
func (m *Membership) Equip(seq uint32, bagSlot int) bool { return m.z.Equip(m.EntityID, seq, bagSlot) }

// Unequip queues taking off the piece worn in equipSlot; the zone answers seq
// on the outbox. It reports false if the zone's inbox is full.
func (m *Membership) Unequip(seq uint32, equipSlot int) bool {
	return m.z.Unequip(m.EntityID, seq, equipSlot)
}

// Discard queues destroying count of the item in bagSlot (0: all of it); the
// zone answers seq on the outbox. It reports false if the zone's inbox is
// full.
func (m *Membership) Discard(seq uint32, bagSlot, count int) bool {
	return m.z.Discard(m.EntityID, seq, bagSlot, count)
}

// SortBag queues sorting the bag; the zone answers seq on the outbox. It
// reports false if the zone's inbox is full.
func (m *Membership) SortBag(seq uint32) bool { return m.z.SortBag(m.EntityID, seq) }

// Enhance queues up to times attempts at enhancing the item ref points to,
// with protection charms when protect is set; the zone answers seq on the
// outbox. It reports false if the zone's inbox is full.
func (m *Membership) Enhance(seq uint32, ref items.Ref, protect bool, times int) bool {
	return m.z.Enhance(m.EntityID, seq, ref, protect, times)
}

// Punch queues punching a hole in the item ref points to; the zone answers
// seq on the outbox. It reports false if the zone's inbox is full.
func (m *Membership) Punch(seq uint32, ref items.Ref) bool {
	return m.z.Punch(m.EntityID, seq, ref)
}

// Socket queues socketing the gem in bag slot gemSlot into hole of the item
// ref points to; the zone answers seq on the outbox. It reports false if the
// zone's inbox is full.
func (m *Membership) Socket(seq uint32, ref items.Ref, hole, gemSlot int) bool {
	return m.z.Socket(m.EntityID, seq, ref, hole, gemSlot)
}

// Unsocket queues taking the gem in hole of the item ref points to back
// into the bag; the zone answers seq on the outbox. It reports false if the
// zone's inbox is full.
func (m *Membership) Unsocket(seq uint32, ref items.Ref, hole int) bool {
	return m.z.Unsocket(m.EntityID, seq, ref, hole)
}

// Refine queues refining the sub-lines of the item ref points to, keeping
// the locked ones; the zone answers seq on the outbox. It reports false if
// the zone's inbox is full.
func (m *Membership) Refine(seq uint32, ref items.Ref, locked []int) bool {
	return m.z.Refine(m.EntityID, seq, ref, locked)
}

// RefineChoose queues taking (accept) or declining the waiting refine
// result; the zone answers seq on the outbox. It reports false if the zone's
// inbox is full.
func (m *Membership) RefineChoose(seq uint32, accept bool) bool {
	return m.z.RefineChoose(m.EntityID, seq, accept)
}

// Leave removes the character; the channel yields the save result.
func (m *Membership) Leave() <-chan error { return m.z.Leave(m.EntityID) }

// Enter puts the character into the first zone with room, opening a new zone if all are full.
// The zone sends EnterWorldResp (with req.RespSeq) on req.Out.
func (w *World) Enter(ctx context.Context, req EnterRequest) (*Membership, error) {
	mapID, pos := w.placement(req.Char)
	info := zone.PlayerInfo{
		EntityID:    w.nextEntity.Add(1),
		CharID:      req.Char.ID,
		Name:        req.Char.Name,
		Class:       classes.ToPB(req.Char.Class),
		Level:       req.Char.Level,
		Exp:         req.Char.Exp,
		Pos:         pos,
		Points:      req.Char.Points,
		FreePoints:  req.Char.FreePoints,
		HP:          req.Char.HP,
		MP:          req.Char.MP,
		SkillPoints: req.Char.SkillPoints,
		Skills:      w.knownSkills(req.Char),
		Xu:          req.Char.Xu,
		Inventory:   w.knownItems(req.Char),
		RespSeq:     req.RespSeq,
	}
	s := w.reserve(mapID)
	if s == nil {
		return nil, ErrStopped
	}
	return w.joinZone(ctx, s, mapID, info, req.Out)
}

// knownSkills is c's stored skills without those missing from the skill data,
// of another class or learned above c's level (a devchar level-down), with one
// warning per dropped skill (phase 2 spec 2.2).
func (w *World) knownSkills(c store.Character) map[string]int {
	out := make(map[string]int, len(c.Skills))
	for id, lv := range c.Skills {
		if s, ok := w.skills[id]; !ok || s.Class != c.Class || s.LearnLevel > c.Level {
			w.log.Warn("dropped stored skill", "char", c.ID, "class", c.Class, "char_level", c.Level, "skill", id, "level", lv)
			continue
		}
		out[id] = lv
	}
	return out
}

// knownItems is c's stored items as an inventory, without the rows that
// break the phase 3a spec 3 rules, with one warning per dropped row: an
// unknown code; a bag slot outside the bag; a worn slot outside 0-9 or not
// the piece's slot (stackables are never worn); a slot already taken by an
// earlier row; a count outside 1..stack_max, or not 1 for equipment; a
// stackable with a quality, element, enhance level or lines; equipment with
// a line of an unknown stat. A worn piece c may not wear (level, class) is
// kept and simply adds nothing. A kept row whose pity is not empty or 15
// entries in 0-99, or that is a stackable with a pity, keeps the item with a
// zero pity and one warning (phase 3b spec 3). Its holes are fixed by
// knownGems (phase 3c spec 3).
func (w *World) knownItems(c store.Character) *items.Inventory {
	inv := items.NewInventory(w.items.BagSize)
	for _, r := range c.Items {
		if reason := w.badItem(inv, r); reason != "" {
			w.log.Warn("dropped stored item", "char", c.ID, "equipped", r.Equipped, "slot", r.Slot,
				"code", r.Code, "count", r.Count, "reason", reason)
			continue
		}
		it := &items.Item{Code: r.Code, Count: r.Count, Quality: r.Quality, Element: r.Element, Enhance: r.Enhance,
			Lines: itemLines(r.Lines), Hidden: itemLines(r.Hidden)}
		_, isStack := w.items.Stackables[r.Code]
		if !goodPity(r.Pity) || isStack && len(r.Pity) > 0 {
			w.log.Warn("reset stored pity", "char", c.ID, "equipped", r.Equipped, "slot", r.Slot,
				"code", r.Code, "pity", r.Pity)
		} else {
			copy(it.Pity[:], r.Pity)
		}
		it.Gems = w.knownGems(c, r)
		if r.Equipped {
			inv.Worn[r.Slot] = it
		} else {
			inv.Bag[r.Slot] = it
		}
	}
	return inv
}

// badItem is why r cannot go into inv, or "" if it can.
func (w *World) badItem(inv *items.Inventory, r store.ItemRow) string {
	e, isEquip := w.items.Equipment[r.Code]
	_, isStack := w.items.Stackables[r.Code]
	switch {
	case !isEquip && !isStack:
		return "unknown code"
	case r.Equipped && (!isEquip || r.Slot != e.SlotIndex):
		return "not this piece's worn slot"
	case !r.Equipped && (r.Slot < 0 || r.Slot >= len(inv.Bag)):
		return "bag slot out of range"
	case r.Equipped && inv.Worn[r.Slot] != nil, !r.Equipped && inv.Bag[r.Slot] != nil:
		return "slot taken"
	case isEquip && r.Count != 1:
		return "equipment count not 1"
	case isStack && (r.Count < 1 || r.Count > w.items.StackMax):
		return "stack count out of range"
	case isStack && (r.Quality != 0 || r.Element != "" || r.Enhance != 0 || len(r.Lines) > 0 || len(r.Hidden) > 0):
		return "stackable with equipment fields"
	case isEquip && (!knownStats(w.items, r.Lines) || !knownStats(w.items, r.Hidden)):
		return "unknown line stat"
	}
	return ""
}

// knownGems is the holes of a kept row r with one warning per fix: holes on
// a stackable are dropped, holes past sockets.max are cut, and a hole with an
// unknown code, a stackable that is not a gem or a gem above the piece's
// tier / 10 is left empty. No holes is nil.
func (w *World) knownGems(c store.Character, r store.ItemRow) []string {
	if len(r.Gems) == 0 {
		return nil
	}
	warn := func(msg string, args ...any) {
		w.log.Warn(msg, append([]any{"char", c.ID, "equipped", r.Equipped, "slot", r.Slot, "code", r.Code}, args...)...)
	}
	e, isEquip := w.items.Equipment[r.Code]
	if !isEquip {
		warn("dropped stored gems of a stackable", "gems", r.Gems)
		return nil
	}
	gems := slices.Clone(r.Gems)
	if max := w.items.Sockets.Max; len(gems) > max {
		warn("cut stored gems", "holes", len(gems), "max", max)
		gems = gems[:max]
	}
	for i, g := range gems {
		if g == "" {
			continue
		}
		if gd := w.items.Stackables[g].Gem; gd == nil || gd.Level > e.Tier/gamedata.TierStep {
			warn("emptied a hole of stored gems", "hole", i, "gem", g)
			gems[i] = ""
		}
	}
	return gems
}

// goodPity reports whether p is empty or one entry per target level, each in
// 0-99.
func goodPity(p []int) bool {
	if len(p) != 0 && len(p) != items.MaxEnhance {
		return false
	}
	for _, v := range p {
		if v < 0 || v > 99 {
			return false
		}
	}
	return true
}

func knownStats(d *gamedata.Items, ls []store.ItemLine) bool {
	for _, l := range ls {
		if !slices.Contains(d.LineStats, l.Stat) {
			return false
		}
	}
	return true
}

// itemLines converts stored lines; none is nil, as a fresh roll has.
func itemLines(ls []store.ItemLine) []items.Line {
	if len(ls) == 0 {
		return nil
	}
	out := make([]items.Line, len(ls))
	for i, l := range ls {
		out[i] = items.Line{Stat: l.Stat, Value: l.Value}
	}
	return out
}

func (w *World) placement(c store.Character) (string, zone.Vec2) {
	if m, ok := w.maps[c.MapID]; ok && m.Walkable(mapdata.Point{X: c.X, Y: c.Y}) {
		return c.MapID, zone.Vec2{X: c.X, Y: c.Y}
	}
	return w.cfg.StartMap, zone.Vec2{X: w.start.X, Y: w.start.Y}
}

// reserve returns a running zone with room, opening one if needed, or nil once the world is stopped.
func (w *World) reserve(mapID string) *slot {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx.Err() != nil {
		return nil
	}
	return w.reserveLocked(mapID)
}

// reserveLocked holds a seat in the first running zone of mapID with room,
// opening a zone if all are full. Callers hold w.mu and have checked w.ctx.
func (w *World) reserveLocked(mapID string) *slot {
	for _, s := range w.zones[mapID] {
		if s != nil && running(s) && w.hasRoom(s) {
			s.reserved.Add(1)
			return s
		}
	}
	s := w.openZoneLocked(mapID)
	s.reserved.Add(1)
	return s
}

// hasRoom reports whether s's players and held seats are below the capacity.
func (w *World) hasRoom(s *slot) bool {
	return s.z.Count()+int(s.reserved.Load()) < w.cfg.ZoneCapacity
}

// running reports whether s's zone has not stopped.
func running(s *slot) bool {
	select {
	case <-s.z.Done():
		return false
	default:
		return true
	}
}

// closeIdle stops zones above min_zones of their map that have been empty for
// IdleZoneTTL (spec 3.1).
func (w *World) closeIdle(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for mapID, slots := range w.zones {
		keep := w.minZones(mapID)
		for i, s := range slots {
			if s == nil || s.no <= keep || s.reserved.Load() > 0 || s.z.Count() > 0 {
				continue
			}
			since := s.z.EmptySince()
			if since.IsZero() || now.Sub(since) < w.cfg.IdleZoneTTL {
				continue
			}
			s.cancel()
			slots[i] = nil
			w.log.Info("closed idle zone", "map", mapID, "zone", s.no)
		}
	}
}

// Claim registers s as the online session of accountID and returns the previous one, if any.
func (w *World) Claim(accountID int64, s Session) Session {
	w.mu.Lock()
	defer w.mu.Unlock()
	prev := w.online[accountID]
	w.online[accountID] = s
	return prev
}

// Takeover makes s the online session of accountID. A previous session is kicked
// with DUPLICATE_LOGIN and Takeover waits until it has left the world and saved its
// character, so s never loads stale data. Takeovers of one account run one at a
// time: a login arriving while another is still waiting queues behind it, so it can
// never skip past a session whose save has not landed. If ctx ends first, the online
// session is left unchanged (so the next attempt waits for it again) and ctx.Err()
// is returned.
func (w *World) Takeover(ctx context.Context, accountID int64, s Session) error {
	unlock, err := w.lockAccount(ctx, accountID)
	if err != nil {
		return err
	}
	defer unlock()
	prev := w.Claim(accountID, s)
	if prev == nil {
		return nil
	}
	prev.Kick(pb.KickReason_KICK_REASON_DUPLICATE_LOGIN)
	select {
	case <-prev.Done():
		return nil
	case <-ctx.Done():
		w.mu.Lock()
		if w.online[accountID] == s {
			w.online[accountID] = prev
		}
		w.mu.Unlock()
		return ctx.Err()
	}
}

// accountLock is a mutex that can be waited on with a context.
type accountLock struct {
	token chan struct{} // holds one value while locked
	refs  int           // holders and waiters; the entry is dropped at zero
}

// lockAccount waits until no other Takeover of accountID is running, or ctx ends.
func (w *World) lockAccount(ctx context.Context, accountID int64) (unlock func(), err error) {
	w.mu.Lock()
	l := w.handoffs[accountID]
	if l == nil {
		l = &accountLock{token: make(chan struct{}, 1)}
		w.handoffs[accountID] = l
	}
	l.refs++
	w.mu.Unlock()
	release := func() {
		w.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(w.handoffs, accountID)
		}
		w.mu.Unlock()
	}
	select {
	case l.token <- struct{}{}:
		return func() { <-l.token; release() }, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}

// Release unregisters s if it is still the online session of accountID.
func (w *World) Release(accountID int64, s Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.online[accountID] == s {
		delete(w.online, accountID)
	}
}

// Online is the number of logged-in accounts.
func (w *World) Online() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.online)
}

type ZoneStat struct {
	MapID   string
	ZoneNo  int
	Players int
	zone.TickStats
}

// Stats returns per-zone player counts and tick timings since the last call.
func (w *World) Stats() []ZoneStat {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []ZoneStat
	for mapID, slots := range w.zones {
		for _, s := range slots {
			if s == nil {
				continue
			}
			out = append(out, ZoneStat{MapID: mapID, ZoneNo: s.no, Players: s.z.Count(), TickStats: s.z.TakeStats()})
		}
	}
	return out
}
