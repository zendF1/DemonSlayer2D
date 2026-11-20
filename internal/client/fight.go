package client

import (
	"context"
	"fmt"
	"math"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/pb"
)

// Fighter stop distance and retry delays.
const (
	fightReach      = 1.3                    // tiles; stop walking this close to the target
	fightRetrySoon  = 150 * time.Millisecond // after ON_COOLDOWN or RATE_LIMITED
	fightRetryLater = 300 * time.Millisecond // after OUT_OF_RANGE or IN_SAFE_ZONE
)

// Kill is a monster the fighter landed the killing blow on, with the
// fighter's level and EXP once that hit was paid.
type Kill struct {
	ID    uint64
	Level int32
	Exp   int64
}

type seenMonster struct {
	x, y float32
	dead bool
}

// Fighter is the chase-and-slash loop shared by botclient and the e2e tests.
// It tracks the monsters it sees from pushes, walks to the nearest live one,
// stops within reach and slashes it every combat.SlashCooldown. With no
// monster in view it walks towards Seek.
type Fighter struct {
	c        *Client
	self     uint64
	x, y     float32
	stats    *pb.SelfStats
	monsters map[uint64]*seenMonster
	target   uint64
	dirX     float32 // last direction sent
	dirY     float32
	resend   bool // send the next direction even if unchanged

	// Seek is where to walk when no live monster is in view, e.g. a region's nest.
	Seek [2]float32
	// Observe, if set, sees every push before the fighter acts on it.
	Observe func(*pb.Envelope)
}

// NewFighter starts a fighter for the character that ew entered the world with.
func NewFighter(c *Client, ew *pb.EnterWorldResp) *Fighter {
	return &Fighter{c: c, self: ew.SelfEntityId, x: ew.X, y: ew.Y, stats: ew.Stats,
		monsters: make(map[uint64]*seenMonster), Seek: [2]float32{ew.X, ew.Y}}
}

type attackResult struct {
	target uint64
	err    error
}

// Fight runs until n kills, ctx ends or the connection fails. It calls onKill,
// if set, for each kill, and respawns the character when it dies.
func (f *Fighter) Fight(ctx context.Context, n int, onKill func(Kill)) error {
	results := make(chan attackResult, 1)
	inFlight := false
	// Return only once the attack goroutine is done with the client, so the
	// caller owns the connection again (Attack returns as soon as ctx ends).
	defer func() {
		if inFlight {
			<-results
		}
	}()
	nextAttack := time.Now()
	retry := time.NewTimer(0)
	defer retry.Stop()
	var pending []uint64 // kills waiting for the SelfStats that pays them
	kills := 0
	finish := func() bool {
		for _, id := range pending {
			kills++
			if onKill != nil {
				onKill(Kill{ID: id, Level: f.stats.GetLevel(), Exp: f.stats.GetExp()})
			}
		}
		pending = pending[:0]
		return kills >= n
	}
	for {
		if !inFlight && f.target != 0 && f.inReach() && !time.Now().Before(nextAttack) {
			inFlight = true
			target := f.target
			go func() { results <- attackResult{target, f.c.Attack(ctx, target)} }()
		}
		select {
		case env := <-f.c.Pushes():
			if f.Observe != nil {
				f.Observe(env)
			}
			if k := env.GetKicked(); k != nil {
				return fmt.Errorf("fighter: kicked: %v", k.Reason)
			}
			if s := env.GetSelfStats(); s != nil {
				f.stats = s
				if finish() {
					return nil
				}
				continue
			}
			wt := env.GetWorldTick()
			if wt != nil && len(pending) > 0 && finish() {
				return nil // the killing hit paid no EXP, so no SelfStats came
			}
			killed, died := f.observe(env)
			pending = append(pending, killed...)
			if died {
				if err := f.respawn(ctx); err != nil {
					return err
				}
			}
			if wt != nil {
				if err := f.steer(ctx); err != nil {
					return err
				}
			}
		case r := <-results:
			inFlight = false
			next, err := f.handleAttack(ctx, r)
			if err != nil {
				return err
			}
			nextAttack = time.Now().Add(next)
			retry.Reset(next)
		case <-retry.C:
		case <-f.c.Closed():
			return fmt.Errorf("fighter: connection closed: %v", f.c.Err())
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// handleAttack applies an attack reply and returns how long to wait before
// the next attack.
func (f *Fighter) handleAttack(ctx context.Context, r attackResult) (time.Duration, error) {
	switch code := Code(r.err); {
	case r.err == nil:
		return combat.SlashCooldown, nil
	case code == pb.ErrorCode_ERROR_CODE_INVALID_TARGET:
		delete(f.monsters, r.target)
		if f.target == r.target {
			f.target = 0
		}
		return 0, nil
	case code == pb.ErrorCode_ERROR_CODE_ON_COOLDOWN || code == pb.ErrorCode_ERROR_CODE_RATE_LIMITED:
		return fightRetrySoon, nil
	case code == pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE || code == pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE:
		return fightRetryLater, nil // keep walking
	case code == pb.ErrorCode_ERROR_CODE_DEAD:
		return 0, f.respawn(ctx)
	case ctx.Err() != nil:
		return 0, ctx.Err()
	default:
		return 0, fmt.Errorf("fighter: attack: %w", r.err)
	}
}

func (f *Fighter) respawn(ctx context.Context) error {
	r, err := f.c.Respawn(ctx)
	if Code(err) == pb.ErrorCode_ERROR_CODE_NOT_DEAD {
		return nil // already respawned
	}
	if err != nil {
		return fmt.Errorf("fighter: respawn: %w", err)
	}
	f.x, f.y = r.X, r.Y
	f.target, f.resend = 0, true
	return nil
}

// observe applies a push to the fighter's view. It returns the monsters the
// fighter killed in it and whether the fighter itself died.
func (f *Fighter) observe(env *pb.Envelope) (killed []uint64, died bool) {
	if e := env.GetEntityEnter(); e != nil {
		if e.Kind == pb.EntityKind_ENTITY_KIND_MONSTER {
			f.monsters[e.Id] = &seenMonster{x: e.X, y: e.Y, dead: e.Dead}
		}
		return nil, false
	}
	if l := env.GetEntityLeave(); l != nil {
		delete(f.monsters, l.Id)
		if f.target == l.Id {
			f.target = 0
		}
		return nil, false
	}
	wt := env.GetWorldTick()
	if wt == nil {
		return nil, false
	}
	for _, m := range wt.Moves {
		if m.Id == f.self {
			f.x, f.y = m.X, m.Y
		} else if s := f.monsters[m.Id]; s != nil {
			s.x, s.y = m.X, m.Y
		}
	}
	finished := make(map[uint64]bool)
	for _, h := range wt.Hits {
		if h.AttackerId == f.self && h.HpAfter == 0 {
			finished[h.TargetId] = true
		}
	}
	for _, d := range wt.Deaths {
		if d.Id == f.self {
			died = true
			continue
		}
		if s := f.monsters[d.Id]; s != nil {
			s.dead = true
		}
		if f.target == d.Id {
			f.target = 0
		}
		if finished[d.Id] {
			killed = append(killed, d.Id)
		}
	}
	return killed, died
}

// steer keeps the current target if it is alive, else picks the nearest live
// monster, and sends a MoveInput when the direction changes.
func (f *Fighter) steer(ctx context.Context) error {
	if s := f.monsters[f.target]; s == nil || s.dead {
		f.target = 0
		best := float32(math.MaxFloat32)
		for id, s := range f.monsters {
			if d := dist(f.x, f.y, s.x, s.y); !s.dead && d < best {
				f.target, best = id, d
			}
		}
	}
	gx, gy, reach := f.Seek[0], f.Seek[1], float32(0.5)
	if s := f.monsters[f.target]; s != nil {
		gx, gy, reach = s.x, s.y, fightReach
	}
	var dx, dy float32
	if d := dist(f.x, f.y, gx, gy); d > reach {
		dx, dy = (gx-f.x)/d, (gy-f.y)/d
	}
	// Resend only on a real change, to stay well under the move rate limit.
	stopped, wasStopped := dx == 0 && dy == 0, f.dirX == 0 && f.dirY == 0
	if !f.resend && stopped == wasStopped && dist(dx, dy, f.dirX, f.dirY) < 0.1 {
		return nil
	}
	f.dirX, f.dirY, f.resend = dx, dy, false
	_, err := f.c.Move(ctx, dx, dy)
	return err
}

func (f *Fighter) inReach() bool {
	s := f.monsters[f.target]
	return s != nil && !s.dead && dist(f.x, f.y, s.x, s.y) <= combat.SlashRange
}

func dist(ax, ay, bx, by float32) float32 {
	return float32(math.Hypot(float64(bx-ax), float64(by-ay)))
}
