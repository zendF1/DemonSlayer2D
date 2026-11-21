package zone

import (
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
)

type stashOpenCmd struct {
	id    uint64
	seq   uint32
	npcID string
}

type stashDepositCmd struct {
	id      uint64
	seq     uint32
	npcID   string
	bagSlot int
}

type stashWithdrawCmd struct {
	id        uint64
	seq       uint32
	npcID     string
	stashSlot int
}

type stashExpandCmd struct {
	id    uint64
	seq   uint32
	npcID string
}

type stashSortCmd struct {
	id    uint64
	seq   uint32
	npcID string
}

// StashOpen queues opening the stash at the storage keeper npcID. The zone
// answers seq with StashOpenResp or ErrorResp on the player's outbox. It
// never blocks and reports false if the inbox is full.
func (z *Zone) StashOpen(id uint64, seq uint32, npcID string) bool {
	return z.offer(stashOpenCmd{id: id, seq: seq, npcID: npcID})
}

// StashDeposit queues moving the item in bagSlot into the stash. The zone
// answers seq with StashDepositResp or ErrorResp. It never blocks and
// reports false if the inbox is full.
func (z *Zone) StashDeposit(id uint64, seq uint32, npcID string, bagSlot int) bool {
	return z.offer(stashDepositCmd{id: id, seq: seq, npcID: npcID, bagSlot: bagSlot})
}

// StashWithdraw queues moving the item in stashSlot into the bag. The zone
// answers seq with StashWithdrawResp or ErrorResp. It never blocks and
// reports false if the inbox is full.
func (z *Zone) StashWithdraw(id uint64, seq uint32, npcID string, stashSlot int) bool {
	return z.offer(stashWithdrawCmd{id: id, seq: seq, npcID: npcID, stashSlot: stashSlot})
}

// StashExpand queues growing the stash by one step for Xu. The zone answers
// seq with StashExpandResp or ErrorResp. It never blocks and reports false
// if the inbox is full.
func (z *Zone) StashExpand(id uint64, seq uint32, npcID string) bool {
	return z.offer(stashExpandCmd{id: id, seq: seq, npcID: npcID})
}

// StashSort queues merging and sorting the stash. The zone answers seq with
// StashSortResp or ErrorResp. It never blocks and reports false if the
// inbox is full.
func (z *Zone) StashSort(id uint64, seq uint32, npcID string) bool {
	return z.offer(stashSortCmd{id: id, seq: seq, npcID: npcID})
}

// stashSlots lists the stash slots idx, an empty ItemSlot for a slot that
// holds nothing now.
func (z *Zone) stashSlots(p *player, idx []int) []*pb.ItemSlot {
	out := make([]*pb.ItemSlot, 0, len(idx))
	for _, i := range idx {
		out = append(out, itemSlot(i, p.stash[i]))
	}
	return out
}

// stashPlayer is the shared start of the five stash commands (phase 4c spec
// 4.2): DEAD, then the storage keeper NPC checks. nil when the request has
// been answered.
func (z *Zone) stashPlayer(id uint64, seq uint32, npcID string) *player {
	p := z.itemPlayer(id, seq)
	if p == nil {
		return nil
	}
	if _, code := z.nearNPC(p, npcID, mapdata.RoleStorage); code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, seq, code)
		return nil
	}
	return p
}

func (z *Zone) stashOpen(c stashOpenCmd) {
	p := z.stashPlayer(c.id, c.seq, c.npcID)
	if p == nil {
		return
	}
	st := &pb.Stash{Size: int32(len(p.stash))}
	for i, it := range p.stash {
		if it != nil {
			st.Items = append(st.Items, itemSlot(i, it))
		}
	}
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_StashOpenResp{StashOpenResp: &pb.StashOpenResp{Stash: st}}})
}

// stashDeposit moves a whole bag slot into the stash (phase 4c spec 4.2). The
// stash takes it all or nothing changes; like the other item commands it
// drops a waiting refine result (dropsRefine); success saves at once.
func (z *Zone) stashDeposit(c stashDepositCmd) {
	p := z.stashPlayer(c.id, c.seq, c.npcID)
	if p == nil {
		return
	}
	switch {
	case c.bagSlot < 0 || c.bagSlot >= len(p.inv.Bag):
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	case p.inv.Bag[c.bagSlot] == nil:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND)
		return
	}
	changed, err := items.AddTo(z.area.Items, p.stash, *p.inv.Bag[c.bagSlot])
	if err != nil {
		code := itemErrorCode(err)
		if code == pb.ErrorCode_ERROR_CODE_BAG_FULL {
			code = pb.ErrorCode_ERROR_CODE_STASH_FULL
		}
		z.replyError(p, c.seq, code)
		return
	}
	p.inv.Bag[c.bagSlot] = nil
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_StashDepositResp{StashDepositResp: &pb.StashDepositResp{
		Update: inventoryUpdate(p.inv, []int{c.bagSlot}, nil), Stash: z.stashSlots(p, changed)}}})
}

// stashWithdraw moves a whole stash slot into the bag (phase 4c spec 4.2).
func (z *Zone) stashWithdraw(c stashWithdrawCmd) {
	p := z.stashPlayer(c.id, c.seq, c.npcID)
	if p == nil {
		return
	}
	switch {
	case c.stashSlot < 0 || c.stashSlot >= len(p.stash):
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
		return
	case p.stash[c.stashSlot] == nil:
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_ITEM_NOT_FOUND)
		return
	}
	changed, err := p.inv.Add(z.area.Items, *p.stash[c.stashSlot])
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	p.stash[c.stashSlot] = nil
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_StashWithdrawResp{StashWithdrawResp: &pb.StashWithdrawResp{
		Update: inventoryUpdate(p.inv, changed, nil), Stash: z.stashSlots(p, []int{c.stashSlot})}}})
}

// stashExpand grows the stash by one step for Xu (phase 4c spec 4.2). Step k
// costs expand_xu_per_step * k with k = (size - base) / step + 1, so a size
// off the step grid (kept on load when inside [base, max]) grows to the next
// grid size, capped at max, for that step's price.
func (z *Zone) stashExpand(c stashExpandCmd) {
	p := z.stashPlayer(c.id, c.seq, c.npcID)
	if p == nil {
		return
	}
	r := z.area.Items.Stash
	size := len(p.stash)
	if size >= r.Max {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_STASH_MAXED)
		return
	}
	k := (max(size, r.Base)-r.Base)/r.Step + 1
	cost := r.ExpandXuPerStep * int64(k)
	if p.xu < cost {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_XU)
		return
	}
	newSize := min(r.Max, r.Base+k*r.Step)
	p.xu -= cost
	p.stash = append(p.stash, make([]*items.Item, newSize-size)...)
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_StashExpandResp{StashExpandResp: &pb.StashExpandResp{
		Size: int32(newSize), Stats: z.selfStats(p)}}})
}

// stashSort merges and sorts the stash like the bag (phase 4c spec 4.2) and
// replies with every stash slot.
func (z *Zone) stashSort(c stashSortCmd) {
	p := z.stashPlayer(c.id, c.seq, c.npcID)
	if p == nil {
		return
	}
	items.SortSlots(z.area.Items, p.stash)
	z.saver.Save(z.state(p), nil)
	all := make([]int, len(p.stash))
	for i := range all {
		all[i] = i
	}
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_StashSortResp{StashSortResp: &pb.StashSortResp{
		Stash: z.stashSlots(p, all)}}})
}
