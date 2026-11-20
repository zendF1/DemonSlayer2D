package zone

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"demonslayer2d/internal/pb"
)

// ZoneChangeCooldown is the time between two zone changes (phase 4a spec 3.4).
const ZoneChangeCooldown = 10 * time.Second

// Cooldown keys besides skill ids (phase 4a spec 4).
const (
	CooldownSlash      = "slash"
	CooldownZoneChange = "zone_change"
)

// ErrNotInZone answers a departure of a player the zone no longer has: it
// was evicted meanwhile, and its connection is being kicked.
var ErrNotInZone = errors.New("zone: player not in zone")

// Place is where a departing player is saved and arrives.
type Place struct {
	MapID string
	Pos   Vec2
}

// Departure is a request to hand a player over to another zone (phase 4a
// spec 3.2 step 3). Check runs on the zone goroutine with the live player; a
// non-zero code refuses the departure and changes nothing. KeepPos saves and
// hands over the current position instead of To.Pos (a zone change).
// ZoneChange starts the zone-change cooldown in the handoff.
type Departure struct {
	Check      func(d *Departing) pb.ErrorCode
	To         Place
	KeepPos    bool
	ZoneChange bool
}

// Departing is the zone's view of a departing player for Departure.Check.
// It is valid only during the check.
type Departing struct {
	z *Zone
	p *player
}

func (d *Departing) Dead() bool { return d.p.dead }
func (d *Departing) Pos() Vec2  { return d.p.pos }

// ZoneChangeReady reports whether the zone-change cooldown has run out.
func (d *Departing) ZoneChangeReady() bool { return d.z.tickNo >= d.p.zoneReady }

type departCmd struct {
	id    uint64
	d     Departure
	reply chan departResult
}

type departResult struct {
	info  PlayerInfo
	saved <-chan error
	code  pb.ErrorCode
	err   error
}

// Depart asks the zone to hand player id over (phase 4a spec 3.2). On
// success the player has left the zone, info is its handoff for the next
// zone and saved yields the result of its save at the destination. A
// non-zero code means the check refused and nothing changed. err is
// ErrNotInZone when the player is gone and ErrZoneClosed when the zone has
// stopped. ctx only bounds waiting for room in the inbox; once queued, Depart
// waits for the zone's answer, as Join does.
func (z *Zone) Depart(ctx context.Context, id uint64, d Departure) (info PlayerInfo, saved <-chan error, code pb.ErrorCode, err error) {
	reply := make(chan departResult, 1)
	select {
	case z.inbox <- departCmd{id: id, d: d, reply: reply}:
	case <-z.done:
		return PlayerInfo{}, nil, 0, ErrZoneClosed
	case <-ctx.Done():
		return PlayerInfo{}, nil, 0, ctx.Err()
	}
	var r departResult
	select {
	case r = <-reply:
	case <-z.done:
		select {
		case r = <-reply:
		default:
			return PlayerInfo{}, nil, 0, ErrZoneClosed
		}
	}
	return r.info, r.saved, r.code, r.err
}

// depart runs a departure on the zone goroutine. On success it removes the
// player as leaving does, makes monsters let it go as killPlayer does, drops
// the monster shots flying at it, and saves it at the destination. Its own
// attacks in flight land without it, as after a logout; its pending slash,
// cast and refine result are not handed over.
func (z *Zone) depart(c departCmd) departResult {
	p, ok := z.players[c.id]
	if !ok {
		return departResult{err: ErrNotInZone}
	}
	if c.d.Check != nil {
		if code := c.d.Check(&Departing{z: z, p: p}); code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
			return departResult{code: code}
		}
	}
	pos := c.d.To.Pos
	if c.d.KeepPos {
		pos = p.pos
	}
	info := z.handoff(p)
	info.Pos = pos
	if c.d.ZoneChange {
		info.Cooldowns[CooldownZoneChange] = ZoneChangeCooldown
	}

	delete(z.players, p.id)
	z.removeFromCell(&p.entity)
	z.setCount()
	for _, m := range z.monsters {
		if m.target == p.id {
			m.target = 0
		}
		delete(m.attackers, p.id)
	}
	z.impacts = slices.DeleteFunc(z.impacts, func(im impact) bool { return im.byMonster && im.target == p.id })
	p.pendingAttack, p.pendingCast, p.refine = nil, nil, nil

	st := z.state(p)
	st.MapID, st.X, st.Y = c.d.To.MapID, pos.X, pos.Y
	done := make(chan error, 1)
	z.saver.Save(st, done)
	return departResult{info: info, saved: done}
}

// handoff is p's live state for the next zone (phase 4a spec 3.2.1): HP and
// MP as they are, deep copies of the skills and items, and the cooldowns
// still running. Pos and RespSeq are left for the caller.
func (z *Zone) handoff(p *player) PlayerInfo {
	return PlayerInfo{
		EntityID: p.id, CharID: p.charID, Name: p.name, Class: p.class, Level: p.level, Exp: p.exp,
		Pos: p.pos, Points: p.points, FreePoints: p.freePoints, HP: p.hp, MP: p.mp,
		SkillPoints: p.skillPoints, Skills: maps.Clone(p.skills), Xu: p.xu, Inventory: p.inv.Clone(),
		Cooldowns: z.cooldownsOf(p),
	}
}

// cooldownsOf maps each of p's running cooldowns to the time left; an
// expired one is left out. The map is never nil.
func (z *Zone) cooldownsOf(p *player) map[string]time.Duration {
	cd := map[string]time.Duration{}
	add := func(key string, ready uint32) {
		if ready > z.tickNo {
			cd[key] = time.Duration(ready-z.tickNo) * time.Second / time.Duration(z.cfg.TickRate)
		}
	}
	add(CooldownSlash, p.slashReady)
	add(CooldownZoneChange, p.zoneReady)
	for id, ready := range p.skillReady {
		add(id, ready)
	}
	return cd
}

// setCooldowns turns handed-over cooldowns into ready ticks of this zone,
// rounding up; entries with no time left are dropped.
func (z *Zone) setCooldowns(p *player, cd map[string]time.Duration) {
	for key, left := range cd {
		if left <= 0 {
			continue
		}
		ready := z.tickNo + z.ticks(left.Seconds())
		switch key {
		case CooldownSlash:
			p.slashReady = ready
		case CooldownZoneChange:
			p.zoneReady = ready
		default:
			p.skillReady[key] = ready
		}
	}
}

// cooldownList is p's running cooldowns for EnterWorldResp, sorted by key.
func (z *Zone) cooldownList(p *player) []*pb.Cooldown {
	cd := z.cooldownsOf(p)
	var out []*pb.Cooldown
	for _, key := range slices.Sorted(maps.Keys(cd)) {
		out = append(out, &pb.Cooldown{Key: key, RemainingMs: int32(cd[key].Milliseconds())})
	}
	return out
}
