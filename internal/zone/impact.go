package zone

import (
	"slices"
	"sort"

	"demonslayer2d/internal/pb"
)

// impact is an attack decided at its press tick that lands at tick due
// (attack phases spec 3.1). seq is its creation order, which orders impacts
// due on the same tick. The damage is fixed at the press; the target is only
// an id, and a monster that respawns gets a new one, so an impact on a dead
// monster never lands on its replacement.
type impact struct {
	due, seq  uint32
	attacker  uint64
	byMonster bool // the attacker is a monster and the target a player
	target    uint64
	damage    int
	skillID   string // empty for a basic attack
	ranged    bool   // a monster's shot
}

// schedule queues im, keeping the queue ordered by due tick and then by
// creation order. It sets im.seq.
func (z *Zone) schedule(im impact) {
	z.impactSeq++
	im.seq = z.impactSeq
	// seq only grows, so im goes after every impact due no later than it.
	i := sort.Search(len(z.impacts), func(i int) bool { return z.impacts[i].due > im.due })
	z.impacts = slices.Insert(z.impacts, i, im)
}

// runImpacts lands every impact due by this tick, in queue order (spec
// 3.2).
func (z *Zone) runImpacts() {
	for len(z.impacts) > 0 && z.impacts[0].due <= z.tickNo {
		im := z.impacts[0]
		z.impacts = z.impacts[1:]
		if im.byMonster {
			z.applyToPlayer(im)
		} else {
			z.applyToMonster(im)
		}
	}
	if len(z.impacts) == 0 {
		z.impacts = nil // let the drained backing array go
	}
}

// impactTicks is how many ticks after the press an attack with windupMS of
// windup and flight seconds of flight lands: at least one (spec 2.1).
func (z *Zone) impactTicks(windupMS int, flight float64) uint32 {
	return z.ticks(float64(windupMS)/1000 + flight)
}

// impactMS is the Cast's impact_ms for an impact due at tick due, counted
// from this tick (spec 4).
func (z *Zone) impactMS(due uint32) int32 {
	return int32(int64(due-z.tickNo) * 1000 / int64(z.cfg.TickRate))
}

// flightSec is how long a projectile at speed tiles per second takes over
// dist tiles; speed 0 means there is no projectile (spec 2.1).
func flightSec(dist float32, speed float64) float64 {
	if speed <= 0 {
		return 0
	}
	return float64(dist) / speed
}

// applyToMonster lands a player's im on its monster (spec 3.3). It is
// skipped if the monster is gone or dead, a later impact on a monster just
// killed included. The Hit shows all the damage, but only what the monster
// had left counts for EXP. The attacker earns EXP only if still in the zone,
// and a monster not already chasing turns on it only if it is also alive.
// The impact that brings the monster to 0 HP is the killing blow.
func (z *Zone) applyToMonster(im impact) {
	m := z.monsters[im.target]
	if m == nil || m.dead {
		return
	}
	counted := min(im.damage, m.hp) // overkill earns no EXP
	m.hp -= counted
	z.events.hits = append(z.events.hits, &pb.Hit{AttackerId: im.attacker, TargetId: m.id, Damage: int32(im.damage), HpAfter: int32(m.hp), SkillId: im.skillID})
	if p := z.players[im.attacker]; p != nil {
		z.payExpShare(p, m, counted)
		// A dead attacker is not recorded: killPlayer has just made monsters
		// forget it, so that it is not chased once respawned.
		if !p.dead {
			m.attackers[p.id] = z.tickNo
			if m.state != chase && m.state != attack {
				z.startChase(m, p.id)
			}
		}
	}
	if m.hp == 0 {
		z.killMonster(m)
	}
}

// applyToPlayer lands a monster's im on its player: skipped if the player
// has left or is dead, else dealt as damagePlayer does.
func (z *Zone) applyToPlayer(im impact) {
	if p := z.players[im.target]; p != nil && !p.dead {
		z.damagePlayer(p, im.attacker, im.damage, im.ranged)
	}
}
