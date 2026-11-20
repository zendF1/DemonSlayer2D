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
	p := z.itemPlayer(c.id, c.seq)
	if p == nil {
		return
	}
	if !z.area.InSafe(p.pos) {
		z.replyError(p, c.seq, pb.ErrorCode_ERROR_CODE_NOT_IN_SAFE_ZONE)
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
