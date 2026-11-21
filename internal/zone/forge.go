package zone

import (
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
)

type enhanceCmd struct {
	id      uint64
	seq     uint32
	ref     items.Ref
	protect bool
	times   int
}

// Enhance queues up to times attempts (0 means 1) at enhancing the item ref
// points to, with a protection charm per attempt when protect is set and the
// target level can drop. The zone answers seq with EnhanceResp or ErrorResp
// on the player's outbox. It never blocks and reports false if the inbox is
// full.
func (z *Zone) Enhance(id uint64, seq uint32, ref items.Ref, protect bool, times int) bool {
	return z.offer(enhanceCmd{id: id, seq: seq, ref: ref, protect: protect, times: times})
}

// enhance runs an enhancement request (phase 3b spec 4.1): DEAD first, then
// NPC_TOO_FAR, then the item package's checks in the spec's order; a
// rejected request costs nothing. Otherwise the stats are recomputed with the
// HP rule of gearChanged if the item is worn, the character is saved at once
// and the reply lists the item's slot and the bag slots materials came from.
func (z *Zone) enhance(c enhanceCmd) {
	p := z.forgePlayer(c.id, c.seq)
	if p == nil {
		return
	}
	res, err := p.inv.Enhance(z.rng, z.area.Items, c.ref, c.protect, c.times, &p.xu)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	// res.Changed holds only bag slots: a worn item's own slot is added here.
	var worn []int
	if c.ref.Equipped {
		worn = []int{c.ref.Slot}
		z.gearChanged(p)
	}
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_EnhanceResp{EnhanceResp: &pb.EnhanceResp{
		Attempts: int32(res.Attempts), Success: res.Success, Level: int32(res.Level),
		Update: inventoryUpdate(p.inv, res.Changed, worn), Stats: z.selfStats(p)}}})
}

type punchCmd struct {
	id  uint64
	seq uint32
	ref items.Ref
}

type socketCmd struct {
	id            uint64
	seq           uint32
	ref           items.Ref
	hole, gemSlot int
}

type unsocketCmd struct {
	id   uint64
	seq  uint32
	ref  items.Ref
	hole int
}

// Punch queues punching one more hole in the item ref points to. The zone
// answers seq with PunchResp or ErrorResp on the player's outbox. It never
// blocks and reports false if the inbox is full.
func (z *Zone) Punch(id uint64, seq uint32, ref items.Ref) bool {
	return z.offer(punchCmd{id: id, seq: seq, ref: ref})
}

// Socket queues socketing the gem in bag slot gemSlot into hole (0-based) of
// the item ref points to. The zone answers seq with SocketResp or ErrorResp
// on the player's outbox. It never blocks and reports false if the inbox is
// full.
func (z *Zone) Socket(id uint64, seq uint32, ref items.Ref, hole, gemSlot int) bool {
	return z.offer(socketCmd{id: id, seq: seq, ref: ref, hole: hole, gemSlot: gemSlot})
}

// Unsocket queues taking the gem in hole (0-based) of the item ref points to
// back into the bag. The zone answers seq with UnsocketResp or ErrorResp on
// the player's outbox. It never blocks and reports false if the inbox is
// full.
func (z *Zone) Unsocket(id uint64, seq uint32, ref items.Ref, hole int) bool {
	return z.offer(unsocketCmd{id: id, seq: seq, ref: ref, hole: hole})
}

// forgePlayer is itemPlayer (DEAD first) followed by NPC_TOO_FAR, the
// first two checks of every forge command (phase 3b spec 4.1, phase 3c spec
// 4.2-4.5).
func (z *Zone) forgePlayer(id uint64, seq uint32) *player {
	p := z.itemPlayer(id, seq)
	if p == nil {
		return nil
	}
	if !z.nearRole(p, mapdata.RoleBlacksmith) {
		z.replyError(p, seq, pb.ErrorCode_ERROR_CODE_NPC_TOO_FAR)
		return nil
	}
	return p
}

// forged finishes a forge command that changed the piece ref points to: if
// it is worn, the stats are recomputed with the HP rule of gearChanged (its
// gems and, through resonance, other pieces' lit lines may have changed);
// the character is saved at once. It returns the update for the bag slots
// in bag plus ref's worn slot, if any.
func (z *Zone) forged(p *player, ref items.Ref, bag []int) *pb.InventoryUpdate {
	var worn []int
	if ref.Equipped {
		worn = []int{ref.Slot}
		z.gearChanged(p)
	}
	z.saver.Save(z.state(p), nil)
	return inventoryUpdate(p.inv, bag, worn)
}

// punch runs a punch request (phase 3c spec 4.2): DEAD, NPC_TOO_FAR,
// then the item package's checks; a rejected request costs nothing.
func (z *Zone) punch(c punchCmd) {
	p := z.forgePlayer(c.id, c.seq)
	if p == nil {
		return
	}
	changed, err := p.inv.Punch(z.area.Items, c.ref, &p.xu)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	up := z.forged(p, c.ref, changed)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_PunchResp{PunchResp: &pb.PunchResp{
		Update: up, Stats: z.selfStats(p)}}})
}

// socket runs a socket request (phase 3c spec 4.3) as punch does.
func (z *Zone) socket(c socketCmd) {
	p := z.forgePlayer(c.id, c.seq)
	if p == nil {
		return
	}
	changed, err := p.inv.Socket(z.area.Items, c.ref, c.hole, c.gemSlot)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	up := z.forged(p, c.ref, changed)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_SocketResp{SocketResp: &pb.SocketResp{
		Update: up, Stats: z.selfStats(p)}}})
}

// unsocket runs an unsocket request (phase 3c spec 4.4) as punch does.
func (z *Zone) unsocket(c unsocketCmd) {
	p := z.forgePlayer(c.id, c.seq)
	if p == nil {
		return
	}
	changed, err := p.inv.Unsocket(z.area.Items, c.ref, c.hole, &p.xu)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	up := z.forged(p, c.ref, changed)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_UnsocketResp{UnsocketResp: &pb.UnsocketResp{
		Update: up, Stats: z.selfStats(p)}}})
}

type refineCmd struct {
	id     uint64
	seq    uint32
	ref    items.Ref
	locked []int
}

type refineChooseCmd struct {
	id     uint64
	seq    uint32
	accept bool
}

// pendingRefine is a paid refine result waiting for the player's choice
// (phase 3c spec 4.5): the new sub-lines of the piece ref points to.
type pendingRefine struct {
	ref   items.Ref
	lines []items.Line
}

// Refine queues refining the sub-lines of the item ref points to, keeping
// the lines at the locked indices. The zone answers seq with RefineResp or
// ErrorResp on the player's outbox. It never blocks and reports false if the
// inbox is full.
func (z *Zone) Refine(id uint64, seq uint32, ref items.Ref, locked []int) bool {
	return z.offer(refineCmd{id: id, seq: seq, ref: ref, locked: locked})
}

// RefineChoose queues taking (accept) or declining the waiting refine
// result. The zone answers seq with RefineChooseResp or ErrorResp on the
// player's outbox. It never blocks and reports false if the inbox is full.
func (z *Zone) RefineChoose(id uint64, seq uint32, accept bool) bool {
	return z.offer(refineChooseCmd{id: id, seq: seq, accept: accept})
}

// dropsRefine is the player whose waiting refine result c drops before it
// runs: every item command but a pickup, a buy, a potion or scroll use and
// the choice itself (phase 3c spec 4.5, phase 4b spec 1.2), so the piece's
// slot cannot change while the result waits. A sale and the stash deposit,
// withdraw and sort are not listed: they drop the result themselves, and only
// when they succeed (Zone.sell, Zone.stashDeposit, Zone.stashWithdraw,
// Zone.stashSort). A new refine drops the old result and, if it succeeds,
// leaves its own.
func dropsRefine(c any) (uint64, bool) {
	switch c := c.(type) {
	case equipCmd:
		return c.id, true
	case unequipCmd:
		return c.id, true
	case discardCmd:
		return c.id, true
	case sortBagCmd:
		return c.id, true
	case enhanceCmd:
		return c.id, true
	case punchCmd:
		return c.id, true
	case socketCmd:
		return c.id, true
	case unsocketCmd:
		return c.id, true
	case refineCmd:
		return c.id, true
	}
	return 0, false
}

// refine runs a refine request (phase 3c spec 4.5): DEAD, NPC_TOO_FAR,
// then the item package's checks; a rejected request costs nothing. The
// stones and Xu are paid at the roll, so the character is saved at once; the
// piece keeps its lines and the new set waits in p.refine. The reply lists
// the new set and the bag slots the stones came from.
func (z *Zone) refine(c refineCmd) {
	p := z.forgePlayer(c.id, c.seq)
	if p == nil {
		return
	}
	lines, changed, err := p.inv.RefineRoll(z.rng, z.area.Items, c.ref, c.locked, &p.xu)
	if err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	p.refine = &pendingRefine{ref: c.ref, lines: lines}
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_RefineResp{RefineResp: &pb.RefineResp{
		Lines: pbLines(lines), Update: inventoryUpdate(p.inv, changed, nil), Stats: z.selfStats(p)}}})
}

// refineChoose takes or declines the waiting refine result (phase 3c spec
// 4.5): NO_PENDING_REFINE without one. It needs neither the blacksmith nor a
// live character, as the result is already paid for. Either way the result
// is cleared and the character saved; the reply holds the piece's slot.
func (z *Zone) refineChoose(c refineChooseCmd) {
	p, ok := z.players[c.id]
	if !ok {
		return // left or evicted; see queueAttack
	}
	pr := p.refine
	if pr == nil {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NO_PENDING_REFINE)
		return
	}
	p.refine = nil
	if c.accept {
		if err := p.inv.RefineApply(pr.ref, pr.lines); err != nil {
			z.replyError(p, c.seq, itemErrorCode(err))
			return
		}
	}
	bag, worn := []int{pr.ref.Slot}, []int(nil)
	if pr.ref.Equipped {
		bag, worn = nil, bag
		if c.accept {
			z.gearChanged(p)
		}
	}
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_RefineChooseResp{RefineChooseResp: &pb.RefineChooseResp{
		Update: inventoryUpdate(p.inv, bag, worn), Stats: z.selfStats(p)}}})
}
