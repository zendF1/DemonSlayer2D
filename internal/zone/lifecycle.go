package zone

import (
	"demonslayer2d/internal/pb"
)

const (
	// autoRespawnSec is how long a dead player lies before the server
	// respawns it (spec 3.6).
	autoRespawnSec = 60
	// restSec is how long a player must stand still on a rest point to be
	// fully healed (spec 3.7).
	restSec = 5
)

// respawn answers a respawn request: only the dead may ask (spec 3.6).
func (z *Zone) respawn(c respawnCmd) {
	p, ok := z.players[c.id]
	if !ok {
		return // left or evicted; see queueAttack
	}
	if !p.dead {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NOT_DEAD)
		return
	}
	z.respawnPlayer(p)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_RespawnResp{RespawnResp: &pb.RespawnResp{X: p.pos.X, Y: p.pos.Y}}})
}

// respawnPlayer brings dead p back at the map's respawn point with full HP and
// MP. Viewers jump it there instead of interpolating.
func (z *Zone) respawnPlayer(p *player) {
	p.dead, p.deadUntil, p.restTicks = false, 0, 0
	p.pos = z.area.Respawn
	p.hp, p.mp = p.maxHP, p.stats.MaxMP
	p.changed, p.snap = true, true
	z.updateCell(&p.entity)
	z.hpChanged(&p.entity)
	p.statsDirty = true
}

// countRest counts the ticks p has stood still, alive, within a rest point's
// radius; any other tick starts the count again (spec 3.7).
func (z *Zone) countRest(p *player) {
	if p.dead || p.dir != (Vec2{}) || !z.onRestPoint(p.pos) {
		p.restTicks = 0
		return
	}
	p.restTicks++
}

func (z *Zone) onRestPoint(pos Vec2) bool {
	for _, r := range z.area.Rests {
		if distance(pos, r.C) <= r.R {
			return true
		}
	}
	return false
}

// runPlayerTimers respawns the players who have lain dead for autoRespawnSec,
// and fully heals those who have rested for restSec (spec 6.2 step 5). A
// player already full keeps resting but gets nothing sent.
func (z *Zone) runPlayerTimers() {
	rest := z.ticks(restSec)
	for _, p := range z.players {
		if p.dead {
			// A player killed this tick may have completed its rest count
			// before dying; the dead never rest.
			if z.tickNo >= p.deadUntil {
				z.respawnPlayer(p)
			}
			continue
		}
		if p.restTicks < rest {
			continue
		}
		p.restTicks = 0
		if p.hp != p.maxHP {
			p.hp = p.maxHP
			z.hpChanged(&p.entity)
			p.statsDirty = true
		}
		if p.mp != p.stats.MaxMP {
			p.mp = p.stats.MaxMP
			p.statsDirty = true
		}
	}
}
