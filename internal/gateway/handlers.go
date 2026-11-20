package gateway

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/text/unicode/norm"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
)

func (c *conn) dispatch(ctx context.Context, env *pb.Envelope) {
	seq := env.Seq
	switch p := env.Payload.(type) {
	case *pb.Envelope_Ping:
		c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Pong{Pong: &pb.Pong{
			ClientTime: p.Ping.ClientTime, ServerTime: time.Now().UnixMilli(),
		}}})
	case *pb.Envelope_MoveInput:
		if c.state != stateInWorld || !c.moves.Allow() {
			return
		}
		c.member.Move(p.MoveInput.InputSeq, p.MoveInput.DirX, p.MoveInput.DirY)
	case *pb.Envelope_RegisterReq:
		if c.expect(seq, stateConnected) {
			c.handleRegister(ctx, seq, p.RegisterReq)
		}
	case *pb.Envelope_LoginReq:
		if c.expect(seq, stateConnected) {
			c.handleLogin(ctx, seq, p.LoginReq)
		}
	case *pb.Envelope_CharListReq:
		if c.expect(seq, stateAuthed) {
			c.handleCharList(ctx, seq)
		}
	case *pb.Envelope_CreateCharReq:
		if c.expect(seq, stateAuthed) {
			c.handleCreateChar(ctx, seq, p.CreateCharReq)
		}
	case *pb.Envelope_DeleteCharReq:
		if c.expect(seq, stateAuthed) {
			c.handleDeleteChar(ctx, seq, p.DeleteCharReq)
		}
	case *pb.Envelope_EnterWorldReq:
		if c.state == stateInWorld {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_ALREADY_IN_WORLD, "already in world")
			return
		}
		if c.expect(seq, stateAuthed) {
			c.handleEnterWorld(ctx, seq, p.EnterWorldReq)
		}
	case *pb.Envelope_LeaveWorldReq:
		if c.state != stateInWorld {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD, "not in world")
			return
		}
		c.handleLeaveWorld(seq)
	case *pb.Envelope_AttackReq:
		if !c.inWorld(seq) {
			return
		}
		if !c.attacks.Allow() {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_RATE_LIMITED, "too many attacks")
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Attack(seq, p.AttackReq.TargetId))
	case *pb.Envelope_CastReq:
		if !c.inWorld(seq) {
			return
		}
		if !c.casts.Allow() {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_RATE_LIMITED, "too many casts")
			return
		}
		r := p.CastReq
		// The zone turns a non-finite direction into zero.
		c.zoneBusyIfDropped(seq, c.member.Cast(seq, r.SkillId, r.TargetId, r.DirX, r.DirY))
	case *pb.Envelope_AllocatePointsReq:
		if !c.inWorld(seq) {
			return
		}
		r := p.AllocatePointsReq
		if total := uint64(r.Strength) + uint64(r.Agility) + uint64(r.Vitality) + uint64(r.Intellect); total > maxPointsTotal {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "too many points")
			return
		}
		pts := combat.Points{Str: int(r.Strength), Agi: int(r.Agility), Vit: int(r.Vitality), Int: int(r.Intellect)}
		c.zoneBusyIfDropped(seq, c.member.Allocate(seq, pts))
	case *pb.Envelope_RespawnReq:
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Respawn(seq))
	case *pb.Envelope_LearnSkillReq:
		// No own limiter: only the packet limit applies (phase 2 spec 4.7).
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Learn(seq, p.LearnSkillReq.SkillId))
	case *pb.Envelope_PickupReq:
		// The item requests have only the packet limit (phase 3a spec 4.8).
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Pickup(seq, p.PickupReq.ItemId))
	case *pb.Envelope_EquipReq:
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Equip(seq, int(p.EquipReq.BagSlot)))
	case *pb.Envelope_UnequipReq:
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Unequip(seq, int(p.UnequipReq.EquipSlot)))
	case *pb.Envelope_DiscardReq:
		if !c.inWorld(seq) {
			return
		}
		r := p.DiscardReq
		c.zoneBusyIfDropped(seq, c.member.Discard(seq, int(r.BagSlot), int(r.Count)))
	case *pb.Envelope_SortBagReq:
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.SortBag(seq))
	case *pb.Envelope_EnhanceReq:
		// Only the packet limit: one request holds up to max_times attempts
		// (phase 3b spec 1.2). The zone checks the slot and times.
		if !c.inWorld(seq) {
			return
		}
		r := p.EnhanceReq
		c.zoneBusyIfDropped(seq, c.member.Enhance(seq, itemRef(r.GetItem()), r.Protect, int(r.Times)))
	case *pb.Envelope_PunchReq:
		// The forge requests have only the packet limit, like enhancement;
		// the zone checks the slots and holes (phase 3c spec 4.2-4.4).
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.Punch(seq, itemRef(p.PunchReq.GetItem())))
	case *pb.Envelope_SocketReq:
		if !c.inWorld(seq) {
			return
		}
		r := p.SocketReq
		c.zoneBusyIfDropped(seq, c.member.Socket(seq, itemRef(r.GetItem()), int(r.Hole), int(r.GemSlot)))
	case *pb.Envelope_UnsocketReq:
		if !c.inWorld(seq) {
			return
		}
		r := p.UnsocketReq
		c.zoneBusyIfDropped(seq, c.member.Unsocket(seq, itemRef(r.GetItem()), int(r.Hole)))
	case *pb.Envelope_RefineReq:
		// The zone checks the locks; their count is bounded by the packet
		// size (phase 3c spec 4.5).
		if !c.inWorld(seq) {
			return
		}
		r := p.RefineReq
		locked := make([]int, len(r.Locked))
		for i, l := range r.Locked {
			locked[i] = int(l)
		}
		c.zoneBusyIfDropped(seq, c.member.Refine(seq, itemRef(r.GetItem()), locked))
	case *pb.Envelope_RefineChooseReq:
		if !c.inWorld(seq) {
			return
		}
		c.zoneBusyIfDropped(seq, c.member.RefineChoose(seq, p.RefineChooseReq.Accept))
	default:
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "unexpected message")
	}
}

// itemRef is r as an items.Ref; a missing ref is bag slot 0.
func itemRef(r *pb.ItemRef) items.Ref {
	return items.Ref{Equipped: r.GetEquipped(), Slot: int(r.GetSlot())}
}

// inWorld answers NOT_IN_WORLD and returns false when the connection has no character in the world.
func (c *conn) inWorld(seq uint32) bool {
	if c.state == stateInWorld {
		return true
	}
	c.fail(seq, pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD, "not in world")
	return false
}

// zoneBusyIfDropped answers INTERNAL when the zone did not accept the request (queued is false).
func (c *conn) zoneBusyIfDropped(seq uint32, queued bool) {
	if !queued {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_INTERNAL, "zone busy")
	}
}

func (c *conn) expect(seq uint32, want connState) bool {
	if c.state == want {
		return true
	}
	c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "not allowed in the current state")
	return false
}

func (c *conn) handleRegister(ctx context.Context, seq uint32, r *pb.RegisterReq) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if _, err := c.srv.deps.Auth.Register(rctx, c.ip, r.Username, r.Password); err != nil {
		c.failErr(seq, err)
		return
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_RegisterResp{RegisterResp: &pb.RegisterResp{}}})
}

func (c *conn) handleLogin(ctx context.Context, seq uint32, r *pb.LoginReq) {
	if r.ProtocolVersion != ProtocolVersion {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_VERSION_MISMATCH, fmt.Sprintf("server speaks protocol %d", ProtocolVersion))
		return
	}
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	id, err := c.srv.deps.Auth.Login(rctx, c.ip, r.Username, r.Password)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	// Kick any session already online for this account and wait until it has saved,
	// so this session never reads an older position than the one being written.
	tctx, tcancel := context.WithTimeout(ctx, requestTimeout)
	err = c.srv.deps.World.Takeover(tctx, id, c)
	tcancel()
	if err != nil {
		c.log.Warn("previous session did not close in time", "account_id", id, "err", err)
		c.fail(seq, pb.ErrorCode_ERROR_CODE_INTERNAL, "previous session is still closing, try again")
		return
	}
	c.authTimer.Stop()
	c.accountID = id
	c.state = stateAuthed
	c.log.Info("login", "account_id", id)
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_LoginResp{LoginResp: &pb.LoginResp{AccountId: id}}})
}

func summary(ch store.Character) *pb.CharSummary {
	return &pb.CharSummary{
		Id: ch.ID, Slot: int32(ch.Slot), Name: ch.Name, CharClass: classes.ToPB(ch.Class),
		Level: int32(ch.Level), MapId: ch.MapID,
	}
}

func (c *conn) handleCharList(ctx context.Context, seq uint32) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	chars, err := c.srv.deps.Chars.ListCharacters(rctx, c.accountID)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	resp := &pb.CharListResp{}
	for _, ch := range chars {
		resp.Chars = append(resp.Chars, summary(ch))
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_CharListResp{CharListResp: resp}})
}

func (c *conn) handleCreateChar(ctx context.Context, seq uint32, r *pb.CreateCharReq) {
	if r.Slot < 1 || r.Slot > 3 {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "slot must be 1-3")
		return
	}
	class, ok := classes.FromPB(r.CharClass)
	if !ok {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "invalid class")
		return
	}
	name, key, err := auth.NormalizeCharName(r.Name)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	mapID, pos := c.srv.deps.World.StartPosition()
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ch, err := c.srv.deps.Chars.CreateCharacter(rctx, store.NewCharacter{
		AccountID: c.accountID, Slot: int(r.Slot), Name: name, NameKey: key, Class: class,
		MapID: mapID, X: pos.X, Y: pos.Y,
	})
	if err != nil {
		c.failErr(seq, err)
		return
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_CreateCharResp{CreateCharResp: &pb.CreateCharResp{Char: summary(ch)}}})
}

func (c *conn) handleDeleteChar(ctx context.Context, seq uint32, r *pb.DeleteCharReq) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ch, err := c.srv.deps.Chars.GetCharacter(rctx, c.accountID, r.CharId)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	if norm.NFC.String(r.ConfirmName) != ch.Name {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "confirm_name does not match the character name")
		return
	}
	if err := c.srv.deps.Chars.DeleteCharacter(rctx, c.accountID, r.CharId); err != nil {
		c.failErr(seq, err)
		return
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_DeleteCharResp{DeleteCharResp: &pb.DeleteCharResp{}}})
}

func (c *conn) handleEnterWorld(ctx context.Context, seq uint32, r *pb.EnterWorldReq) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ch, err := c.srv.deps.Chars.GetCharacter(rctx, c.accountID, r.CharId)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	m, err := c.srv.deps.World.Enter(rctx, world.EnterRequest{Char: ch, Out: c.out, RespSeq: seq})
	if err != nil {
		c.failErr(seq, err)
		return
	}
	// The zone has already queued EnterWorldResp for this seq.
	c.member = m
	c.state = stateInWorld
	c.log.Info("enter world", "account_id", c.accountID, "char_id", ch.ID, "map", m.MapID, "zone", m.ZoneNo)
}

func (c *conn) handleLeaveWorld(seq uint32) {
	c.waitSaved(c.member.Leave(), "save on leave")
	c.member = nil
	c.state = stateAuthed
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_LeaveWorldResp{LeaveWorldResp: &pb.LeaveWorldResp{}}})
}
