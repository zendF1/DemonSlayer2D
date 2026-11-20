package zone

import (
	"errors"

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

// pickup puts a ground item into its owner's bag (phase 3a spec 4.6). Checks
// run in the spec's order and the first failure is answered. The whole item
// or stack goes in or nothing does; on success it leaves the ground (the
// owner gets EntityLeave in syncView) and the bag is saved at once.
func (z *Zone) pickup(c pickupCmd) {
	p, ok := z.players[c.id]
	if !ok {
		return // left or evicted; see queueAttack
	}
	g := z.ground[c.itemID]
	switch {
	case p.dead:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_DEAD)
		return
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
