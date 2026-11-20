// Package world owns every map's zones, places characters into zones and tracks
// which account is online where.
package world

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
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
	// skills is skills.json by id, shared and read-only.
	skills map[string]gamedata.Skill
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
		cfg: cfg, maps: maps, areas: areas, skills: monsters.Skills, saver: saver, log: log,
		start:    m.Spawns["default"],
		zones:    make(map[string][]*slot),
		online:   make(map[int64]Session),
		handoffs: make(map[int64]*accountLock),
	}, nil
}

// areaOf converts a map's collision and triggers, the monster data its
// regions use, the EXP table and the skills into the zone's view of the map.
// Region tiles are computed here once.
func areaOf(m *mapdata.Map, d *gamedata.Data) zone.Area {
	a := zone.Area{Grid: m, Levels: d.Levels, Exp: d.Exp, Skills: d.Skills}
	sp := m.Spawns["default"]
	a.Respawn = zone.Vec2{X: sp.X, Y: sp.Y}
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

// Start opens zone 1 of every map and the idle-zone janitor. Everything stops when ctx is cancelled.
// It must be called before Enter.
func (w *World) Start(ctx context.Context) {
	w.ctx = ctx
	w.mu.Lock()
	for id := range w.maps {
		w.openZoneLocked(id)
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
		RespSeq:     req.RespSeq,
	}
	for attempt := 0; attempt < 10; attempt++ {
		s := w.reserve(mapID)
		if s == nil {
			return nil, ErrStopped
		}
		err := s.z.Join(ctx, info, req.Out)
		s.reserved.Add(-1)
		switch {
		case err == nil:
			return &Membership{z: s.z, EntityID: info.EntityID, MapID: mapID, ZoneNo: s.no}, nil
		case errors.Is(err, zone.ErrZoneFull), errors.Is(err, zone.ErrZoneClosed):
			continue
		default:
			// zone.Join only fails this way before the join is queued, so nothing to undo.
			return nil, err
		}
	}
	return nil, zone.ErrZoneFull
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
	for _, s := range w.zones[mapID] {
		if s == nil {
			continue
		}
		select {
		case <-s.z.Done():
			continue
		default:
		}
		if s.z.Count()+int(s.reserved.Load()) < w.cfg.ZoneCapacity {
			s.reserved.Add(1)
			return s
		}
	}
	s := w.openZoneLocked(mapID)
	s.reserved.Add(1)
	return s
}

// closeIdle stops zones other than zone 1 that have been empty for IdleZoneTTL.
func (w *World) closeIdle(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for mapID, slots := range w.zones {
		for i, s := range slots {
			if i == 0 || s == nil || s.reserved.Load() > 0 || s.z.Count() > 0 {
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
