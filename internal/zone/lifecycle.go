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
	// regenScale is the unit of the HP regen and its carry: 1/10000 of max
	// HP (phase 3b spec 4.2).
	regenScale = 10000
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

// respawnPlayer brings dead p back at the respawn point nearest to where it
// died, with full HP and MP. Viewers jump it there instead of interpolating.
func (z *Zone) respawnPlayer(p *player) {
	p.dead, p.deadUntil, p.restTicks = false, 0, 0
	p.pos = z.area.nearestRespawn(p.pos)
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
// gives the living their regen once a second, and fully heals those who have
// rested for restSec (spec 6.2 step 5). A player already full keeps resting
// but gets nothing sent.
func (z *Zone) runPlayerTimers() {
	rest := z.ticks(restSec)
	second := z.tickNo%z.ticks(1) == 0
	for _, p := range z.players {
		if p.dead {
			// A player killed this tick may have completed its rest count
			// before dying; the dead never rest nor regenerate.
			if z.tickNo >= p.deadUntil {
				z.respawnPlayer(p)
			}
			continue
		}
		if second {
			z.regen(p)
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

// regen gives living p one second of its enhancement regen (phase 3b spec
// 4.2), in combat too. HP grows by whole points of the carry, which keeps the
// fraction for the next second, so a small regen of a small max HP still
// heals; full HP drops the carry. Only a change is sent.
func (z *Zone) regen(p *player) {
	if p.hp >= p.maxHP || p.regenHPBP <= 0 {
		p.hpCarry = 0
	} else {
		p.hpCarry += p.maxHP * p.regenHPBP
		gain := p.hpCarry / regenScale
		p.hpCarry %= regenScale
		if gain > 0 {
			p.hp = min(p.hp+gain, p.maxHP)
			if p.hp == p.maxHP {
				p.hpCarry = 0
			}
			z.hpChanged(&p.entity)
			p.statsDirty = true
		}
	}
	if p.regenMP > 0 && p.mp < p.stats.MaxMP {
		p.mp = min(p.mp+p.regenMP, p.stats.MaxMP)
		p.statsDirty = true
	}
}
