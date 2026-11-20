package zone

import (
	"demonslayer2d/internal/items"
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
// NOT_IN_SAFE_ZONE, then the item package's checks in the spec's order; a
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

// forgePlayer is itemPlayer (DEAD first) followed by NOT_IN_SAFE_ZONE, the
// first two checks of every forge command (phase 3b spec 4.1, phase 3c spec
// 4.2-4.5).
func (z *Zone) forgePlayer(id uint64, seq uint32) *player {
	p := z.itemPlayer(id, seq)
	if p == nil {
		return nil
	}
	if !z.area.InSafe(p.pos) {
		z.replyError(p, seq, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE)
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

// punch runs a punch request (phase 3c spec 4.2): DEAD, NOT_IN_SAFE_ZONE,
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
