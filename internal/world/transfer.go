package world

import (
	"context"
	"errors"
	"fmt"

	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/zone"
)

// TransferKind is how a player moves between zones.
type TransferKind int

const (
	TransferPortal TransferKind = iota + 1 // walk into a portal (spec 3.3)
	TransferZone                           // switch to another zone of the same map (spec 3.4)
)

// TransferReq asks to move a player: through the portal PortalID of its map,
// or to zone number Zone of its map. The new zone answers RespSeq with
// EnterWorldResp.
type TransferReq struct {
	Kind     TransferKind
	PortalID string
	Zone     int
	RespSeq  uint32
}

// TransferError is a refused transfer; the player stays where it is and the
// request is answered with Code.
type TransferError struct{ Code pb.ErrorCode }

func (e *TransferError) Error() string { return "world: transfer refused: " + e.Code.String() }

func refuse(code pb.ErrorCode) error { return &TransferError{Code: code} }

// portalReach is how far, in tiles, a player may stand from a portal's
// rectangle and still use it (spec 1.3: about network lag at 4 tiles/s).
const portalReach = 1

// departState is what the transfer checks read of the departing player;
// *zone.Departing implements it.
type departState interface {
	Dead() bool
	Pos() zone.Vec2
	ZoneChangeReady() bool
}

// portalCheck is the zone side of a portal (spec 3.3 steps 2-3).
func portalCheck(r mapdata.Rect) func(departState) pb.ErrorCode {
	return func(d departState) pb.ErrorCode {
		p := d.Pos()
		switch {
		case d.Dead():
			return pb.ErrorCode_ERROR_CODE_DEAD
		case mapdata.RectDistance(mapdata.Point{X: p.X, Y: p.Y}, r) > portalReach:
			return pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE
		}
		return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
}

// zoneChangeCheck is the zone side of a zone change (spec 3.4 steps 4-5).
func zoneChangeCheck(d departState) pb.ErrorCode {
	switch {
	case d.Dead():
		return pb.ErrorCode_ERROR_CODE_DEAD
	case !d.ZoneChangeReady():
		return pb.ErrorCode_ERROR_CODE_ON_COOLDOWN
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// route is where a transfer goes and how (spec 3.2 step 1).
type route struct {
	to     zone.Place
	prefer int  // zone number wanted in to.MapID
	exact  bool // only that zone will do
	check  func(departState) pb.ErrorCode
	// keepPos and zoneChange are as in zone.Departure.
	keepPos, zoneChange bool
}

// route resolves req for a player at m; a refusal is a *TransferError.
func (w *World) route(m *Membership, req TransferReq) (route, error) {
	here := w.maps[m.MapID]
	if here == nil {
		return route{}, fmt.Errorf("world: membership on unknown map %q", m.MapID)
	}
	switch req.Kind {
	case TransferPortal:
		p, ok := here.Portal(req.PortalID)
		if !ok {
			return route{}, refuse(pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		}
		target := w.maps[p.ToMap]
		if target == nil {
			return route{}, fmt.Errorf("world: portal %q of %s leads to unknown map %q", p.ID, here.ID, p.ToMap)
		}
		sp, ok := target.Spawns[p.ToSpawn]
		if !ok {
			return route{}, fmt.Errorf("world: portal %q of %s leads to unknown spawn %q", p.ID, here.ID, p.ToSpawn)
		}
		return route{to: zone.Place{MapID: p.ToMap, Pos: zone.Vec2{X: sp.X, Y: sp.Y}}, prefer: m.ZoneNo,
			check: portalCheck(p.Rect)}, nil
	case TransferZone:
		if req.Zone < 1 || req.Zone == m.ZoneNo {
			return route{}, refuse(pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		}
		return route{to: zone.Place{MapID: m.MapID}, prefer: req.Zone, exact: true,
			check: zoneChangeCheck, keepPos: true, zoneChange: true}, nil
	}
	return route{}, refuse(pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
}

// Transfer moves the player of m to another zone (spec 3.2): it resolves the
// destination, holds a seat there, has m's zone run the check on the live
// player and hand it over (saving it at the destination), then joins the
// held zone, which answers req.RespSeq with EnterWorldResp on m's outbox.
//
// It must run on the goroutine that owns m (the connection's), so no other
// request reaches the old zone after the departure. A refusal is a
// *TransferError and leaves the player in m. After the departure the join
// does not give up on ctx, only when the world stops: then the player is in
// no zone, the error is ErrStopped (or the join error) and the returned
// channel yields the departure save, which the caller waits for before
// closing the connection. The channel is nil in every other case.
func (w *World) Transfer(ctx context.Context, m *Membership, req TransferReq) (*Membership, <-chan error, error) {
	r, err := w.route(m, req)
	if err != nil {
		return nil, nil, err
	}
	s, code := w.reserveIn(r.to.MapID, r.prefer, r.exact)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return nil, nil, refuse(code)
	}
	if s == nil {
		return nil, nil, ErrStopped
	}
	info, saved, code, err := m.z.Depart(ctx, m.EntityID, zone.Departure{
		Check:   func(d *zone.Departing) pb.ErrorCode { return r.check(d) },
		To:      r.to,
		KeepPos: r.keepPos, ZoneChange: r.zoneChange,
	})
	if err != nil || code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		s.reserved.Add(-1)
		if err != nil {
			// ErrNotInZone: the player was evicted and its connection is being kicked.
			return nil, nil, err
		}
		return nil, nil, refuse(code)
	}
	info.RespSeq = req.RespSeq
	nm, err := w.joinZone(w.ctx, s, r.to.MapID, info, m.out)
	if err != nil {
		if w.ctx.Err() != nil {
			err = ErrStopped
		}
		return nil, saved, err
	}
	return nm, nil, nil
}

// joinZone joins info to s, which the caller has reserved, and on a full or
// closed zone retries the normal reservation of mapID, up to 10 joins in all.
func (w *World) joinZone(ctx context.Context, s *slot, mapID string, info zone.PlayerInfo, out zone.Outbox) (*Membership, error) {
	for attempt := 1; ; attempt++ {
		err := s.z.Join(ctx, info, out)
		s.reserved.Add(-1)
		switch {
		case err == nil:
			return &Membership{z: s.z, out: out, EntityID: info.EntityID, MapID: mapID, ZoneNo: s.no}, nil
		case !errors.Is(err, zone.ErrZoneFull) && !errors.Is(err, zone.ErrZoneClosed):
			// zone.Join only fails this way before the join is queued, so nothing to undo.
			return nil, err
		case attempt == 10:
			return nil, zone.ErrZoneFull
		}
		if s = w.reserve(mapID); s == nil {
			return nil, ErrStopped
		}
	}
}

// reserveIn holds a seat in zone prefer of mapID. With exact, only that zone
// will do: ZONE_NOT_FOUND when it is not running, ZONE_FULL when its players
// and held seats reach the capacity. Otherwise it falls back to reserve. It
// returns a nil slot and no code once the world is stopped.
func (w *World) reserveIn(mapID string, prefer int, exact bool) (*slot, pb.ErrorCode) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx.Err() != nil {
		return nil, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	var want *slot
	for _, s := range w.zones[mapID] {
		if s != nil && s.no == prefer && running(s) {
			want = s
		}
	}
	switch {
	case want != nil && w.hasRoom(want):
		want.reserved.Add(1)
		return want, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	case exact && want == nil:
		return nil, pb.ErrorCode_ERROR_CODE_ZONE_NOT_FOUND
	case exact:
		return nil, pb.ErrorCode_ERROR_CODE_ZONE_FULL
	}
	return w.reserveLocked(mapID), pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// ZoneInfo is a running zone and its player count.
type ZoneInfo struct {
	No      int
	Players int
}

// Zones lists the running zones of mapID by number (spec 3.5).
func (w *World) Zones(mapID string) []ZoneInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []ZoneInfo
	for _, s := range w.zones[mapID] { // index i holds zone i+1
		if s != nil && running(s) {
			out = append(out, ZoneInfo{No: s.no, Players: s.z.Count()})
		}
	}
	return out
}

// Capacity is the number of players a zone holds.
func (w *World) Capacity() int { return w.cfg.ZoneCapacity }
