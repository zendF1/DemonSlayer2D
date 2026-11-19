package zone

import "demonslayer2d/internal/pb"

// autoRespawnSec is how long a dead player lies before the server respawns it
// (spec 3.6).
const autoRespawnSec = 60

// tickEvents are the combat events of one tick. syncView sends each viewer the
// ones about entities it can see.
type tickEvents struct {
	hits      []*pb.Hit
	deaths    []*pb.Death
	hpChanges []*pb.HpChange
	levelUps  []*pb.LevelUp
}

// damagePlayer applies a hit of dmg from attacker to p and kills p at 0 HP.
func (z *Zone) damagePlayer(p *player, attacker uint64, dmg int) {
	if p.dead {
		return
	}
	p.hp = max(p.hp-dmg, 0)
	z.events.hits = append(z.events.hits, &pb.Hit{AttackerId: attacker, TargetId: p.id, Damage: int32(dmg), HpAfter: int32(p.hp)})
	if p.hp == 0 {
		z.killPlayer(p)
	}
}

// killPlayer makes p a corpse that ignores movement until it respawns, by
// request or automatically at deadUntil.
func (z *Zone) killPlayer(p *player) {
	p.dead = true
	if p.dir != (Vec2{}) {
		p.dir = Vec2{}
		p.changed = true
	}
	p.deadUntil = z.tickNo + z.ticks(autoRespawnSec)
	z.events.deaths = append(z.events.deaths, &pb.Death{Id: p.id})
}

// hpChanged records an HP change that did not come from a hit.
func (z *Zone) hpChanged(e *entity) {
	z.events.hpChanges = append(z.events.hpChanges, &pb.HpChange{Id: e.id, Hp: int32(e.hp), MaxHp: int32(e.maxHP)})
}
