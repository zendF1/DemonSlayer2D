package zone

import (
	"strings"

	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/pb"
)

// cooldownPotionPrefix starts the handoff key of a potion group
// ("potion_hp", "potion_mp"; phase 4b spec 4.4).
const cooldownPotionPrefix = "potion_"

type useItemCmd struct {
	id      uint64
	seq     uint32
	bagSlot int
}

// UseItem queues drinking the potion in bagSlot. The zone answers seq with
// UseItemResp or ErrorResp on the player's outbox. It never blocks and
// reports false if the inbox is full.
func (z *Zone) UseItem(id uint64, seq uint32, bagSlot int) bool {
	return z.offer(useItemCmd{id: id, seq: seq, bagSlot: bagSlot})
}

// useItem runs a potion (phase 4b spec 4.4). Checks run in the spec's order
// and the first failure is answered at no cost. Nothing is saved: potions are
// drunk every few seconds, so the next periodic save holds the result.
func (z *Zone) useItem(c useItemCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	d := z.area.Items
	var use *gamedata.Use
	switch {
	case c.bagSlot < 0 || c.bagSlot >= len(p.inv.Bag):
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	case p.inv.Bag[c.bagSlot] == nil:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND)
		return
	}
	use = d.Stackables[p.inv.Bag[c.bagSlot].Code].Use
	switch {
	case use == nil || use.Effect == gamedata.EffectReturnTown:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	case p.level < use.Level:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW)
		return
	case z.tickNo < p.useReady[use.Group]:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN)
		return
	}
	healHP, healMP := items.HealAmount(*use, p.maxHP, p.stats.MaxMP)
	if (healHP > 0 && p.hp >= p.maxHP) || (healMP > 0 && p.mp >= p.stats.MaxMP) {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ALREADY_FULL)
		return
	}
	if _, err := p.inv.TakeSlot(c.bagSlot, 1); err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	if healHP > 0 {
		p.hp = min(p.hp+healHP, p.maxHP)
		z.hpChanged(&p.entity)
	}
	if healMP > 0 {
		p.mp = min(p.mp+healMP, p.stats.MaxMP)
	}
	sec := d.UseCooldownSec[use.Group]
	p.useReady[use.Group] = z.tickNo + z.ticks(sec)
	p.statsDirty = true
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_UseItemResp{UseItemResp: &pb.UseItemResp{
		Update: inventoryUpdate(p.inv, []int{c.bagSlot}, nil), Stats: z.selfStats(p), CooldownMs: int32(sec * 1000)}}})
}

// potionGroup is the cooldown group of a handoff key, if it is a potion's.
func potionGroup(key string) (string, bool) {
	return strings.CutPrefix(key, cooldownPotionPrefix)
}
