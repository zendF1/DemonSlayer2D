package zone

import (
	"errors"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/pb"
)

// pickupRange is how far from an item a player may pick it up (phase 3a spec
// 1.3), in tiles; attackRangeSlack is added as for a slash.
const pickupRange = 1.5

type pickupCmd struct {
	id     uint64
	seq    uint32
	itemID uint64
}

type equipCmd struct {
	id      uint64
	seq     uint32
	bagSlot int
}

type unequipCmd struct {
	id        uint64
	seq       uint32
	equipSlot int
}

type discardCmd struct {
	id             uint64
	seq            uint32
	bagSlot, count int
}

type sortBagCmd struct {
	id  uint64
	seq uint32
}

// offer queues an item command. It never blocks and reports false if the
// inbox is full.
func (z *Zone) offer(c any) bool {
	select {
	case z.inbox <- c:
		return true
	default:
		return false
	}
}

// Pickup queues picking up the ground item itemID. The zone answers seq with
// PickupResp or ErrorResp on the player's outbox. It never blocks and reports
// false if the inbox is full.
func (z *Zone) Pickup(id uint64, seq uint32, itemID uint64) bool {
	return z.offer(pickupCmd{id: id, seq: seq, itemID: itemID})
}

// Equip queues wearing the piece in bagSlot. The zone answers seq with
// EquipResp or ErrorResp on the player's outbox. It never blocks and reports
// false if the inbox is full.
func (z *Zone) Equip(id uint64, seq uint32, bagSlot int) bool {
	return z.offer(equipCmd{id: id, seq: seq, bagSlot: bagSlot})
}

// Unequip queues taking off the piece worn in equipSlot. The zone answers seq
// with UnequipResp or ErrorResp on the player's outbox. It never blocks and
// reports false if the inbox is full.
func (z *Zone) Unequip(id uint64, seq uint32, equipSlot int) bool {
	return z.offer(unequipCmd{id: id, seq: seq, equipSlot: equipSlot})
}

// Discard queues destroying count of the item in bagSlot (0: all of it). The
// zone answers seq with DiscardResp or ErrorResp on the player's outbox. It
// never blocks and reports false if the inbox is full.
func (z *Zone) Discard(id uint64, seq uint32, bagSlot, count int) bool {
	return z.offer(discardCmd{id: id, seq: seq, bagSlot: bagSlot, count: count})
}

// SortBag queues sorting the bag. The zone answers seq with SortBagResp or
// ErrorResp on the player's outbox. It never blocks and reports false if the
// inbox is full.
func (z *Zone) SortBag(id uint64, seq uint32) bool {
	return z.offer(sortBagCmd{id: id, seq: seq})
}

// itemPlayer is the player an item command from id is for, or nil when
// there is nothing more to do: the player has left, or is dead (DEAD comes
// first for every item command, phase 3a spec 4.6 and 4.7), or the area has
// no item data (only in tests).
func (z *Zone) itemPlayer(id uint64, seq uint32) *player {
	p, ok := z.players[id]
	if !ok {
		return nil // left or evicted; see queueAttack
	}
	switch {
	case p.dead:
		z.replyError(p, seq, pb.ErrorCode_ERROR_CODE_DEAD)
		return nil
	case z.area.Items == nil:
		z.replyError(p, seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return nil
	}
	return p
}

// pickup puts a ground item into its owner's bag (phase 3a spec 4.6). Checks
// run in the spec's order and the first failure is answered. The whole item
// or stack goes in or nothing does; on success it leaves the ground (the
// owner gets EntityLeave in syncView) and the bag is saved at once.
func (z *Zone) pickup(c pickupCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	g := z.ground[c.itemID]
	switch {
	case g == nil || g.owner != p.charID:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND)
		return
	case distance(p.pos, g.pos) > pickupRange+attackRangeSlack:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE)
		return
	}
	changed, err := p.inv.Add(z.area.Items, g.item)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	z.removeFromCell(&g.entity)
	delete(z.ground, g.id)
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_PickupResp{PickupResp: &pb.PickupResp{
		Update: inventoryUpdate(p.inv, changed, nil)}}})
}

// equip wears the piece in a bag slot, swapping with the one worn in its
// slot (phase 3a spec 4.7). The items package checks the rest in the spec's
// order. The new stats follow the HP rule of gearChanged, and the inventory
// is saved at once.
func (z *Zone) equip(c equipCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	class, _ := classes.FromPB(p.class)
	slot, err := p.inv.Equip(z.area.Items, c.bagSlot, p.level, class)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	z.gearChanged(p)
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_EquipResp{EquipResp: &pb.EquipResp{
		Update: inventoryUpdate(p.inv, []int{c.bagSlot}, []int{slot}), Stats: z.selfStats(p)}}})
}

// unequip moves a worn piece to the first empty bag slot (phase 3a spec
// 4.7), then works as equip.
func (z *Zone) unequip(c unequipCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	slot, err := p.inv.Unequip(c.equipSlot)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	z.gearChanged(p)
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_UnequipResp{UnequipResp: &pb.UnequipResp{
		Update: inventoryUpdate(p.inv, []int{slot}, []int{c.equipSlot}), Stats: z.selfStats(p)}}})
}

// discard destroys some or all of a bag slot (phase 3a spec 4.7) and saves
// at once. The server does not ask for confirmation; the client does.
func (z *Zone) discard(c discardCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	if err := p.inv.Discard(c.bagSlot, c.count); err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_DiscardResp{DiscardResp: &pb.DiscardResp{
		Update: inventoryUpdate(p.inv, []int{c.bagSlot}, nil)}}})
}

// sortBag merges and sorts the bag (phase 3a spec 4.7), saves at once and
// replies with every bag slot.
func (z *Zone) sortBag(c sortBagCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	p.inv.Sort(z.area.Items)
	z.saver.Save(z.state(p), nil)
	all := make([]int, len(p.inv.Bag))
	for i := range all {
		all[i] = i
	}
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_SortBagResp{SortBagResp: &pb.SortBagResp{
		Update: inventoryUpdate(p.inv, all, nil)}}})
}

// itemErrorCode maps an items error to the code the client gets (phase 3a
// spec 4.7).
func itemErrorCode(err error) pb.ErrorCode {
	switch {
	case errors.Is(err, items.ErrBagFull):
		return pb.ErrorCode_ERROR_CODE_BAG_FULL
	case errors.Is(err, items.ErrNotFound):
		return pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND
	case errors.Is(err, items.ErrWrongClass):
		return pb.ErrorCode_ERROR_CODE_WRONG_CLASS
	case errors.Is(err, items.ErrLevelTooLow):
		return pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW
	case errors.Is(err, items.ErrBadSlot), errors.Is(err, items.ErrNotEquipment), errors.Is(err, items.ErrBadCount):
		return pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	}
	return pb.ErrorCode_ERROR_CODE_INTERNAL
}
