package zone

import (
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
)

// npcRange is how far from an NPC a player may use it (phase 4b spec 4.1), in
// tiles; npcSlack is added on top as for a slash.
const (
	npcRange = 3.0
	npcSlack = 0.5
)

type buyCmd struct {
	id          uint64
	seq         uint32
	npcID, code string
	count       int
}

type sellCmd struct {
	id             uint64
	seq            uint32
	npcID          string
	bagSlot, count int
}

// Buy queues buying count of code from the shop NPC npcID. The zone answers
// seq with BuyResp or ErrorResp on the player's outbox. It never blocks and
// reports false if the inbox is full.
func (z *Zone) Buy(id uint64, seq uint32, npcID, code string, count int) bool {
	return z.offer(buyCmd{id: id, seq: seq, npcID: npcID, code: code, count: count})
}

// Sell queues selling count (0: all) of the item in bagSlot to the shop NPC
// npcID. The zone answers seq with SellResp or ErrorResp on the player's
// outbox. It never blocks and reports false if the inbox is full.
func (z *Zone) Sell(id uint64, seq uint32, npcID string, bagSlot, count int) bool {
	return z.offer(sellCmd{id: id, seq: seq, npcID: npcID, bagSlot: bagSlot, count: count})
}

func (z *Zone) inNPCRange(p *player, n mapdata.NPC) bool {
	return distance(p.pos, Vec2{X: n.Pos.X, Y: n.Pos.Y}) <= npcRange+npcSlack
}

// nearNPC is the NPC id of the given role when p is within reach of it.
// BAD_REQUEST when the map has no such NPC or its role differs, NPC_TOO_FAR
// when it is out of range; UNSPECIFIED means ok.
func (z *Zone) nearNPC(p *player, id, role string) (mapdata.NPC, pb.ErrorCode) {
	n, ok := z.area.NPCs[id]
	if !ok || n.Role != role {
		return mapdata.NPC{}, pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	}
	if !z.inNPCRange(p, n) {
		return mapdata.NPC{}, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR
	}
	return n, pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// nearRole reports whether p is within reach of any NPC of the role.
func (z *Zone) nearRole(p *player, role string) bool {
	for _, n := range z.area.NPCs {
		if n.Role == role && z.inNPCRange(p, n) {
			return true
		}
	}
	return false
}

// buy sells count of a shop good to the player (phase 4b spec 4.2). Checks
// run in the spec's order, the first failure is answered and costs nothing;
// count is checked against stack_max before the cost is multiplied, so the
// cost cannot overflow.
func (z *Zone) buy(c buyCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	npc, code := z.nearNPC(p, c.npcID, mapdata.RoleShop)
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	d := z.area.Items
	var price int64
	ok := false
	for _, s := range d.Shops {
		if s.ID == npc.Shop {
			price, ok = s.Price(c.code)
			break
		}
	}
	switch {
	case !ok, c.count < 1, c.count > d.StackMax:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	case p.xu < price*int64(c.count):
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU)
		return
	}
	changed, err := p.inv.Add(d, items.Item{Code: c.code, Count: c.count})
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	p.xu -= price * int64(c.count)
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_BuyResp{BuyResp: &pb.BuyResp{
		Update: inventoryUpdate(p.inv, changed, nil), Stats: z.selfStats(p)}}})
}

// sell buys count of the item in a bag slot from the player (phase 4b spec
// 4.3); worn pieces cannot be sold. A waiting refine result is dropped by
// dropsRefine before this runs.
func (z *Zone) sell(c sellCmd) {
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	if _, code := z.nearNPC(p, c.npcID, mapdata.RoleShop); code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	// Peek first: the sellable check comes after the slot and count checks
	// and must not cost the item.
	switch {
	case c.bagSlot < 0 || c.bagSlot >= len(p.inv.Bag):
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	case p.inv.Bag[c.bagSlot] == nil:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND)
		return
	case c.count < 0, c.count > p.inv.Bag[c.bagSlot].Count:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	}
	unit := items.SellValue(z.area.Items, p.inv.Bag[c.bagSlot])
	if unit <= 0 {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NOT_SELLABLE)
		return
	}
	taken, err := p.inv.TakeSlot(c.bagSlot, c.count)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	p.xu += unit * int64(taken.Count)
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_SellResp{SellResp: &pb.SellResp{
		Update: inventoryUpdate(p.inv, []int{c.bagSlot}, nil), Stats: z.selfStats(p)}}})
}
