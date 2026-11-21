package gateway

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
	"demonslayer2d/internal/zone"
)

// ______________________TEST OUTBOX_____________________

func TestOutbox(t *testing.T) {
	var kicked []pb.KickReason
	o := newOutbox(2, func(r pb.KickReason) { kicked = append(kicked, r) })
	e := &pb.Envelope{}
	fillOutbox(t, o, 2)
	<-o.ch
	o.closeWith(&pb.Envelope{Seq: 9})
	o.closeWith(nil) // no-op
	if o.Send(e) {
		t.Fatal("send after close must fail")
	}
	var got []*pb.Envelope
	for env := range o.ch {
		got = append(got, env)
	}
	if len(got) != 2 || got[1].Seq != 9 {
		t.Fatalf("drained %v, want the queued envelope then the final one", got)
	}
	o.Kick(pb.KickReason_KICK_REASON_IDLE_TIMEOUT)
	if len(kicked) != 1 || kicked[0] != pb.KickReason_KICK_REASON_IDLE_TIMEOUT {
		t.Fatalf("kicked = %v", kicked)
	}
}

func TestOutboxKeepsRoomForFinalMessage(t *testing.T) {
	o := newOutbox(2, func(pb.KickReason) {})
	fillOutbox(t, o, 2)
	o.closeWith(&pb.Envelope{Payload: &pb.Envelope_Kicked{Kicked: &pb.Kicked{Reason: pb.KickReason_KICK_REASON_SLOW_CONSUMER}}})
	var got []*pb.Envelope
	for env := range o.ch {
		got = append(got, env)
	}
	if len(got) != 3 || got[2].GetKicked().GetReason() != pb.KickReason_KICK_REASON_SLOW_CONSUMER {
		t.Fatalf("a full queue must still deliver the final Kicked message, got %v", got)
	}
}

// fillOutbox sends until the outbox is full and checks it accepted exactly want envelopes.
func fillOutbox(t *testing.T, o *outbox, want int) {
	t.Helper()
	for i := range want {
		if !o.Send(&pb.Envelope{}) {
			t.Fatalf("send %d of %d was rejected", i+1, want)
		}
	}
	if o.Send(&pb.Envelope{}) {
		t.Fatalf("send %d should report a full queue", want+1)
	}
}

// ______________________TEST ERRORS_____________________

func TestErrorCode(t *testing.T) {
	cases := map[error]pb.ErrorCode{
		auth.ErrUsernameInvalid: pb.ErrorCode_ERROR_CODE_USERNAME_INVALID,
		auth.ErrPasswordInvalid: pb.ErrorCode_ERROR_CODE_PASSWORD_INVALID,
		auth.ErrNameInvalid:     pb.ErrorCode_ERROR_CODE_NAME_INVALID,
		auth.ErrAuthFailed:      pb.ErrorCode_ERROR_CODE_AUTH_FAILED,
		auth.ErrRateLimited:     pb.ErrorCode_ERROR_CODE_RATE_LIMITED,
		store.ErrUsernameTaken:  pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN,
		store.ErrNameTaken:      pb.ErrorCode_ERROR_CODE_NAME_TAKEN,
		store.ErrSlotOccupied:   pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED,
		store.ErrNotFound:       pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND,
		zone.ErrZoneFull:        pb.ErrorCode_ERROR_CODE_ZONE_FULL,
		errors.New("boom"):      pb.ErrorCode_ERROR_CODE_INTERNAL,
	}
	for err, want := range cases {
		if got := errorCode(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Errorf("errorCode(%v) = %v, want %v", err, got, want)
		}
	}
}

// A transfer that fails other than by a refusal is answered with a code the
// client can show, or closes the connection with a fitting kick (phase 4a
// spec 3.2): a busy old zone is RATE_LIMITED, a stopping world or zone is
// SERVER_SHUTDOWN, anything else INTERNAL.
func TestTransferFailure(t *testing.T) {
	cases := map[error]struct {
		code pb.ErrorCode
		kick pb.KickReason
	}{
		context.DeadlineExceeded: {code: pb.ErrorCode_ERROR_CODE_RATE_LIMITED},
		context.Canceled:         {code: pb.ErrorCode_ERROR_CODE_RATE_LIMITED},
		world.ErrStopped:         {kick: pb.KickReason_KICK_REASON_SERVER_SHUTDOWN},
		zone.ErrZoneClosed:       {kick: pb.KickReason_KICK_REASON_SERVER_SHUTDOWN},
		zone.ErrNotInZone:        {kick: pb.KickReason_KICK_REASON_SERVER_SHUTDOWN},
		errors.New("boom"):       {code: pb.ErrorCode_ERROR_CODE_INTERNAL},
	}
	for err, want := range cases {
		code, kick := transferFailure(fmt.Errorf("wrapped: %w", err))
		if code != want.code || kick != want.kick {
			t.Errorf("transferFailure(%v) = %v %v, want %v %v", err, code, kick, want.code, want.kick)
		}
	}
}

// A character that left its zone but joined no other is kicked
// SERVER_SHUTDOWN when the world stopped and ZONE_RESTART otherwise, so it
// logs in again instead of being told the server is down.
func TestLostInTransferKick(t *testing.T) {
	cases := map[error]pb.KickReason{
		world.ErrStopped:                     pb.KickReason_KICK_REASON_SERVER_SHUTDOWN,
		zone.ErrZoneFull:                     pb.KickReason_KICK_REASON_ZONE_RESTART,
		errors.New("zone: duplicate entity"): pb.KickReason_KICK_REASON_ZONE_RESTART,
	}
	for err, want := range cases {
		if got := lostKick(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Errorf("lostKick(%v) = %v, want %v", err, got, want)
		}
	}
}

// ______________________TEST CLIENT IP_____________________

func TestClientIP(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.1/32")}
	cases := []struct {
		name    string
		remote  string
		xff     []string
		trusted []netip.Prefix
		want    string
	}{
		{"direct", "203.0.113.7:5000", nil, proxies, "203.0.113.7"},
		{"untrusted peer ignores header", "203.0.113.7:5000", []string{"1.2.3.4"}, proxies, "203.0.113.7"},
		{"no proxies configured", "10.0.0.2:5000", []string{"1.2.3.4"}, nil, "10.0.0.2"},
		{"one proxy", "10.0.0.2:5000", []string{"198.51.100.9"}, proxies, "198.51.100.9"},
		{"spoofed left entry is ignored", "10.0.0.2:5000", []string{"6.6.6.6, 198.51.100.9"}, proxies, "198.51.100.9"},
		{"proxy chain", "127.0.0.1:5000", []string{"198.51.100.9, 10.1.1.1"}, proxies, "198.51.100.9"},
		{"several headers", "10.0.0.2:5000", []string{"6.6.6.6", "198.51.100.9"}, proxies, "198.51.100.9"},
		{"trusted peer without header", "10.0.0.2:5000", nil, proxies, "10.0.0.2"},
		{"malformed hop", "10.0.0.2:5000", []string{"198.51.100.9, junk"}, proxies, "10.0.0.2"},
		{"only proxies", "10.0.0.2:5000", []string{"10.3.3.3"}, proxies, "10.3.3.3"},
		{"ipv6 peer", "[2001:db8::1]:5000", nil, proxies, "2001:db8::1"},
		{"ipv4-mapped peer", "[::ffff:10.0.0.2]:5000", []string{"198.51.100.9"}, proxies, "198.51.100.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.remote, Header: http.Header{}}
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(r, tc.trusted); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// ______________________TEST SERVER_____________________

// A connection that registers after Shutdown has taken its snapshot must kick itself,
// otherwise Shutdown would wait for it until its deadline.
func TestConnTrackedDuringShutdownIsKicked(t *testing.T) {
	s := NewServer(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := s.newConn(nil, "127.0.0.1")
	defer c.idle.Stop()
	defer c.authTimer.Stop()

	s.track(c, true)
	if !c.kicked.Load() {
		t.Fatal("connection tracked after shutdown started was not kicked")
	}
	env := <-c.out.ch
	if env.GetKicked().GetReason() != pb.KickReason_KICK_REASON_SERVER_SHUTDOWN {
		t.Fatalf("final message = %v, want Kicked{SERVER_SHUTDOWN}", env)
	}
}

// Behind a trusted proxy the connection must be keyed on the player's address from
// X-Forwarded-For, not on the proxy's.
func TestConnUsesForwardedIPFromTrustedProxy(t *testing.T) {
	s := NewServer(Deps{
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("::1/128")},
	})
	srv := httptest.NewServer(s)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Forwarded-For": {"198.51.100.9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	var ip string
	for ip == "" && ctx.Err() == nil {
		s.mu.Lock()
		for c := range s.conns {
			ip = c.ip
		}
		s.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	if ip != "198.51.100.9" {
		t.Fatalf("connection ip = %q, want the forwarded player address", ip)
	}
}

// ______________________TEST HANDLERS_____________________

// LearnSkillReq needs a character in the world.
func TestLearnSkillNotInWorld(t *testing.T) {
	s := NewServer(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	c := s.newConn(nil, "127.0.0.1")
	defer c.idle.Stop()
	defer c.authTimer.Stop()
	c.dispatch(context.Background(), &pb.Envelope{Seq: 4, Payload: &pb.Envelope_LearnSkillReq{LearnSkillReq: &pb.LearnSkillReq{SkillId: "nham_1"}}})
	env := <-c.out.ch
	if env.Seq != 4 || env.GetError().GetCode() != pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD {
		t.Fatalf("reply %v, want NOT_IN_WORLD for seq 4", env)
	}
}

// The item requests need a character in the world (phase 3a spec 4.6, 4.7,
// phase 3b spec 4.1, phase 3c spec 4.2-4.5).
func TestItemRequestsNotInWorld(t *testing.T) {
	for name, env := range map[string]*pb.Envelope{
		"pickup":         {Payload: &pb.Envelope_PickupReq{PickupReq: &pb.PickupReq{ItemId: 5}}},
		"equip":          {Payload: &pb.Envelope_EquipReq{EquipReq: &pb.EquipReq{BagSlot: 1}}},
		"unequip":        {Payload: &pb.Envelope_UnequipReq{UnequipReq: &pb.UnequipReq{EquipSlot: 1}}},
		"discard":        {Payload: &pb.Envelope_DiscardReq{DiscardReq: &pb.DiscardReq{BagSlot: 1, Count: 1}}},
		"buy":            {Payload: &pb.Envelope_BuyReq{BuyReq: &pb.BuyReq{NpcId: "shop", Code: "potion_hp_1", Count: 1}}},
		"sell":           {Payload: &pb.Envelope_SellReq{SellReq: &pb.SellReq{NpcId: "shop", BagSlot: 1, Count: 1}}},
		"sort bag":       {Payload: &pb.Envelope_SortBagReq{SortBagReq: &pb.SortBagReq{}}},
		"stash open":     {Payload: &pb.Envelope_StashOpenReq{StashOpenReq: &pb.StashOpenReq{NpcId: "kho"}}},
		"stash deposit":  {Payload: &pb.Envelope_StashDepositReq{StashDepositReq: &pb.StashDepositReq{NpcId: "kho", BagSlot: 1}}},
		"stash withdraw": {Payload: &pb.Envelope_StashWithdrawReq{StashWithdrawReq: &pb.StashWithdrawReq{NpcId: "kho", StashSlot: 1}}},
		"stash expand":   {Payload: &pb.Envelope_StashExpandReq{StashExpandReq: &pb.StashExpandReq{NpcId: "kho"}}},
		"stash sort":     {Payload: &pb.Envelope_StashSortReq{StashSortReq: &pb.StashSortReq{NpcId: "kho"}}},
		"enhance": {Payload: &pb.Envelope_EnhanceReq{EnhanceReq: &pb.EnhanceReq{
			Item: &pb.ItemRef{Equipped: true, Slot: 0}, Times: 1}}},
		"punch":         {Payload: &pb.Envelope_PunchReq{PunchReq: &pb.PunchReq{Item: &pb.ItemRef{Slot: 1}}}},
		"socket":        {Payload: &pb.Envelope_SocketReq{SocketReq: &pb.SocketReq{Item: &pb.ItemRef{Slot: 1}, GemSlot: 2}}},
		"unsocket":      {Payload: &pb.Envelope_UnsocketReq{UnsocketReq: &pb.UnsocketReq{Item: &pb.ItemRef{Slot: 1}, Hole: 1}}},
		"refine":        {Payload: &pb.Envelope_RefineReq{RefineReq: &pb.RefineReq{Item: &pb.ItemRef{Slot: 1}, Locked: []int32{0}}}},
		"refine choose": {Payload: &pb.Envelope_RefineChooseReq{RefineChooseReq: &pb.RefineChooseReq{Accept: true}}},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewServer(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
			c := s.newConn(nil, "127.0.0.1")
			defer c.idle.Stop()
			defer c.authTimer.Stop()
			env.Seq = 4
			c.dispatch(context.Background(), env)
			r := <-c.out.ch
			if r.Seq != 4 || r.GetError().GetCode() != pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD {
				t.Fatalf("reply %v, want NOT_IN_WORLD for seq 4", r)
			}
		})
	}
}

// The transfer and zone list requests need a character in the world (phase
// 4a spec 3.3-3.5, phase 4b spec 4.5-4.6).
func TestTransferRequestsNotInWorld(t *testing.T) {
	for name, env := range map[string]*pb.Envelope{
		"use portal":  {Payload: &pb.Envelope_UsePortalReq{UsePortalReq: &pb.UsePortalReq{PortalId: "gate_east"}}},
		"change zone": {Payload: &pb.Envelope_ChangeZoneReq{ChangeZoneReq: &pb.ChangeZoneReq{Zone: 2}}},
		"return town": {Payload: &pb.Envelope_ReturnTownReq{ReturnTownReq: &pb.ReturnTownReq{BagSlot: 3}}},
		"travel":      {Payload: &pb.Envelope_TravelReq{TravelReq: &pb.TravelReq{NpcId: "xa_phu", Route: 1}}},
		"zone list":   {Payload: &pb.Envelope_ZoneListReq{ZoneListReq: &pb.ZoneListReq{}}},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewServer(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
			c := s.newConn(nil, "127.0.0.1")
			defer c.idle.Stop()
			defer c.authTimer.Stop()
			env.Seq = 4
			c.dispatch(context.Background(), env)
			r := <-c.out.ch
			if r.Seq != 4 || r.GetError().GetCode() != pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD {
				t.Fatalf("reply %v, want NOT_IN_WORLD for seq 4", r)
			}
		})
	}
}

// SetPkReq needs a character in the world (phase 5a spec 4.8).
func TestSetPkNotInWorld(t *testing.T) {
	s := NewServer(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	c := s.newConn(nil, "127.0.0.1")
	defer c.idle.Stop()
	defer c.authTimer.Stop()
	c.dispatch(context.Background(), &pb.Envelope{Seq: 4, Payload: &pb.Envelope_SetPkReq{SetPkReq: &pb.SetPkReq{On: true}}})
	r := <-c.out.ch
	if r.Seq != 4 || r.GetError().GetCode() != pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD {
		t.Fatalf("reply %v, want NOT_IN_WORLD for seq 4", r)
	}
}

// ______________________TEST CLIENT FIXTURES_____________________

var updateFixtures = flag.Bool("update", false, "rewrite testdata/client")

// The Godot client (DemonSlayer2DGame) must encode/decode exactly these bytes; it copies them via tools/sync_server.sh.
func clientFixtures() map[string]*pb.Envelope {
	return map[string]*pb.Envelope{
		"login_req": {Seq: 3, Payload: &pb.Envelope_LoginReq{LoginReq: &pb.LoginReq{
			Username: "tanjiro", Password: "password123", ProtocolVersion: ProtocolVersion}}},
		"create_char_req": {Seq: 5, Payload: &pb.Envelope_CreateCharReq{CreateCharReq: &pb.CreateCharReq{
			Slot: 2, Name: "Hoàng", CharClass: pb.CharClass_CHAR_CLASS_THUY}}},
		"move_input": {Payload: &pb.Envelope_MoveInput{MoveInput: &pb.MoveInput{
			InputSeq: 9, DirX: -0.6, DirY: 0.8}}},
		"char_list_resp": {Seq: 4, Payload: &pb.Envelope_CharListResp{CharListResp: &pb.CharListResp{
			Chars: []*pb.CharSummary{
				{Id: 11, Slot: 1, Name: "Kaito", CharClass: pb.CharClass_CHAR_CLASS_VIEM, Level: 3, MapId: "kumotori_1"},
				{Id: 12, Slot: 3, Name: "Hana", CharClass: pb.CharClass_CHAR_CLASS_PHONG, Level: 1, MapId: "kumotori_1"},
			}}}},
		"enter_world_resp": {Seq: 6, Payload: &pb.Envelope_EnterWorldResp{EnterWorldResp: &pb.EnterWorldResp{
			SelfEntityId: 77, MapId: "kumotori_1", Zone: 2, X: 5.5, Y: 6.25,
			ServerTick: 1234, MoveSpeed: 4, TickRate: 10}}},
		"entity_enter": {Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
			Id: 78, Kind: pb.EntityKind_ENTITY_KIND_PLAYER, Name: "Mai", CharClass: pb.CharClass_CHAR_CLASS_MINH,
			Level: 2, X: 7.5, Y: 8.5, DirX: 0, DirY: -1}}},
		"world_tick": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 42, AckInputSeq: 7, Moves: []*pb.EntityMove{
				{Id: 77, X: 5.5, Y: 6.25, DirX: 1},
				{Id: 78, X: 1.3, Y: 2},
			}}}},
		"attack_req":  {Seq: 8, Payload: &pb.Envelope_AttackReq{AttackReq: &pb.AttackReq{TargetId: 42}}},
		"attack_resp": {Seq: 8, Payload: &pb.Envelope_AttackResp{AttackResp: &pb.AttackResp{}}},
		"allocate_points_req": {Seq: 9, Payload: &pb.Envelope_AllocatePointsReq{AllocatePointsReq: &pb.AllocatePointsReq{
			Strength: 1, Agility: 2, Vitality: 3, Intellect: 4}}},
		"respawn_resp": {Seq: 10, Payload: &pb.Envelope_RespawnResp{RespawnResp: &pb.RespawnResp{X: 5.5, Y: 5.5}}},
		"self_stats": {Payload: &pb.Envelope_SelfStats{SelfStats: &pb.SelfStats{
			Level: 3, Exp: 250, ExpNext: 900, FreePoints: 5, Strength: 6, Agility: 7, Vitality: 8, Intellect: 9,
			Hp: 120, MaxHp: 150, Mp: 30, MaxMp: 40, PhysAttack: 21, MagicAttack: 12, PhysDefense: 10,
			MagicDefense: 8, Accuracy: 95, Evasion: 14, CritRateBp: 350}}},
		"entity_enter_monster": {Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
			Id: 900, Kind: pb.EntityKind_ENTITY_KIND_MONSTER, Name: "Quỷ lang thang", Level: 3,
			X: 12.5, Y: 9.5, DirX: 1, Hp: 100, MaxHp: 160}}},
		"world_tick_combat": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 43, AckInputSeq: 8,
			Moves:     []*pb.EntityMove{{Id: 77, X: 6.5, Y: 7.25, DirX: 1, Snap: true}},
			Hits:      []*pb.Hit{{AttackerId: 77, TargetId: 900, Damage: 18, HpAfter: 82}},
			Deaths:    []*pb.Death{{Id: 901}},
			HpChanges: []*pb.HpChange{{Id: 900, Hp: 82, MaxHp: 160}},
			LevelUps:  []*pb.LevelUp{{Id: 77, Level: 4}}}}},
		"world_tick_ranged": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 44,
			Hits: []*pb.Hit{{AttackerId: 900, TargetId: 77, Damage: 9, HpAfter: 111, Ranged: true}}}}},
		"learn_skill_resp": {Seq: 11, Payload: &pb.Envelope_LearnSkillResp{LearnSkillResp: &pb.LearnSkillResp{
			Stats: &pb.SelfStats{Level: 20, Exp: 100, ExpNext: 5000, SkillPoints: 17, Hp: 300, MaxHp: 300, Mp: 80, MaxMp: 90},
			Skills: []*pb.SkillLevel{
				{SkillId: "viem_1", Level: 2},
				{SkillId: "viem_2", Level: 1},
			}}}},
		"cast_req": {Seq: 12, Payload: &pb.Envelope_CastReq{CastReq: &pb.CastReq{
			SkillId: "viem_1", TargetId: 900, DirX: 0.6, DirY: -0.8}}},
		"world_tick_cast": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 45, AckInputSeq: 9,
			Moves: []*pb.EntityMove{{Id: 77, X: 8.5, Y: 7.25, DirX: 1, Snap: true}},
			Casts: []*pb.Cast{{CasterId: 77, SkillId: "viem_1", TargetId: 900, DirX: 1, Level: 3, WindupMs: 250,
				Impacts: []*pb.CastImpact{{TargetId: 900, ImpactMs: 500}, {TargetId: 902, ImpactMs: 650}}}},
			Hits: []*pb.Hit{
				{AttackerId: 77, TargetId: 900, Damage: 40, HpAfter: 60, SkillId: "viem_1"},
				{AttackerId: 77, TargetId: 902, Damage: 35, HpAfter: 65, SkillId: "viem_1"},
			}}}},
		"world_tick_monster_attack": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 46,
			Casts: []*pb.Cast{{CasterId: 900, TargetId: 77, DirX: -1, WindupMs: 400, Ranged: true,
				Impacts: []*pb.CastImpact{{TargetId: 77, ImpactMs: 700}}}}}}},
		"world_tick_impact": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 47,
			Hits: []*pb.Hit{{AttackerId: 900, TargetId: 77, Damage: 9, HpAfter: 102, Ranged: true}}}}},
		"error_resp": {Seq: 4, Payload: &pb.Envelope_Error{Error: &pb.ErrorResp{
			Code: pb.ErrorCode_ERROR_CODE_AUTH_FAILED, Detail: "wrong username or password"}}},
		"kicked": {Payload: &pb.Envelope_Kicked{Kicked: &pb.Kicked{
			Reason: pb.KickReason_KICK_REASON_DUPLICATE_LOGIN}}},
		"pong": {Payload: &pb.Envelope_Pong{Pong: &pb.Pong{
			ClientTime: 1700000000123, ServerTime: 1700000000456}}},
		"enter_world_resp_inventory": {Seq: 6, Payload: &pb.Envelope_EnterWorldResp{EnterWorldResp: &pb.EnterWorldResp{
			SelfEntityId: 77, MapId: "kumotori_1", Zone: 2, X: 5.5, Y: 6.25,
			ServerTick: 1234, MoveSpeed: 4, TickRate: 10,
			Stats: &pb.SelfStats{Level: 12, Exp: 40, ExpNext: 2000, Hp: 900, MaxHp: 900, Mp: 120, MaxMp: 120,
				PhysAttack: 230, Xu: 1234567},
			Inventory: &pb.Inventory{BagSize: 60,
				Bag: []*pb.ItemSlot{
					{Slot: 0, Item: &pb.ItemInfo{Code: "eq10_armor", Count: 1, Quality: 1,
						Lines: []*pb.ItemLine{{Stat: "hp", Value: 60}}}},
					{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 12}},
				},
				Equipped: []*pb.ItemSlot{
					{Slot: 0, Item: &pb.ItemInfo{Code: "eq10_weapon_viem", Count: 1}},
				}}}}},
		"entity_enter_item": {Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
			Id: 950, Kind: pb.EntityKind_ENTITY_KIND_ITEM, Name: "Áo Lữ Hành", X: 13.1, Y: 9.5,
			Item: &pb.ItemInfo{Code: "eq10_armor", Count: 1, Quality: 2, Element: pb.CharClass_CHAR_CLASS_THUY,
				Lines: []*pb.ItemLine{
					{Stat: "hp", Value: 60},
					{Stat: "phys_defense", Value: 1},
					{Stat: "magic_attack", Value: 3},
				},
				Hidden: []*pb.ItemLine{
					{Stat: "hp", Value: 86},
					{Stat: "phys_attack", Value: 4},
				}}}}},
		"equip_resp": {Seq: 13, Payload: &pb.Envelope_EquipResp{EquipResp: &pb.EquipResp{
			// Worn into an empty slot: bag slot 4 is now empty.
			Update: &pb.InventoryUpdate{
				Bag: []*pb.ItemSlot{{Slot: 4}},
				Equipped: []*pb.ItemSlot{{Slot: 5, Item: &pb.ItemInfo{Code: "eq10_armor", Count: 1, Quality: 1,
					Lines: []*pb.ItemLine{{Stat: "hp", Value: 60}}}}},
			},
			Stats: &pb.SelfStats{Level: 15, Exp: 10, ExpNext: 3000, Hp: 1100, MaxHp: 1200, Mp: 150, MaxMp: 150,
				PhysAttack: 230, PhysDefense: 95, MagicDefense: 90, Xu: 1234567}}}},
		"enhance_req": {Seq: 14, Payload: &pb.Envelope_EnhanceReq{EnhanceReq: &pb.EnhanceReq{
			Item: &pb.ItemRef{Equipped: true, Slot: 5}, Protect: true, Times: 100}}},
		"enhance_resp": {Seq: 14, Payload: &pb.Envelope_EnhanceResp{EnhanceResp: &pb.EnhanceResp{
			Attempts: 3, Success: true, Level: 8,
			// Worn armour now +8; stones from bag slot 3 and the last charm from slot 7.
			Update: &pb.InventoryUpdate{
				Bag: []*pb.ItemSlot{
					{Slot: 3, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 3}},
					{Slot: 7},
				},
				Equipped: []*pb.ItemSlot{{Slot: 5, Item: &pb.ItemInfo{Code: "eq10_armor", Count: 1, Quality: 1, Enhance: 8,
					Lines: []*pb.ItemLine{{Stat: "hp", Value: 60}}}}},
			},
			Stats: &pb.SelfStats{Level: 15, Exp: 10, ExpNext: 3000, Hp: 1150, MaxHp: 1250, Mp: 150, MaxMp: 150,
				PhysAttack: 230, PhysDefense: 95, MagicDefense: 90, Xu: 1232167, HpRegenBp: 5, MpRegen: 1}}}},
		"item_with_gems": {Seq: 15, Payload: &pb.Envelope_EquipResp{EquipResp: &pb.EquipResp{
			// A worn Vàng armour with three holes: two gems and an empty middle hole.
			Update: &pb.InventoryUpdate{
				Bag: []*pb.ItemSlot{{Slot: 2}},
				Equipped: []*pb.ItemSlot{{Slot: 5, Item: &pb.ItemInfo{Code: "eq30_armor", Count: 1, Quality: 2,
					Element: pb.CharClass_CHAR_CLASS_NHAM, Enhance: 3,
					Lines:  []*pb.ItemLine{{Stat: "hp", Value: 200}, {Stat: "phys_defense", Value: 2}},
					Hidden: []*pb.ItemLine{{Stat: "magic_defense", Value: 2}, {Stat: "hp", Value: 210}},
					Holes:  3, Gems: []string{"gem_hp_2", "", "gem_defense_3"}}}},
			},
			Stats: &pb.SelfStats{Level: 30, Exp: 10, ExpNext: 9000, Hp: 2100, MaxHp: 2400, Mp: 300, MaxMp: 300,
				PhysAttack: 400, PhysDefense: 150, MagicDefense: 140, Xu: 50000}}}},
		"refine_resp": {Seq: 16, Payload: &pb.Envelope_RefineResp{RefineResp: &pb.RefineResp{
			// Line 0 locked; lines 1 and 2 redrawn. Three stones left bag slot 7.
			Lines:  []*pb.ItemLine{{Stat: "hp", Value: 200}, {Stat: "magic_attack", Value: 12}, {Stat: "phys_attack", Value: 9}},
			Update: &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 7}}},
			Stats: &pb.SelfStats{Level: 30, Exp: 10, ExpNext: 9000, Hp: 2100, MaxHp: 2400, Mp: 300, MaxMp: 300,
				PhysAttack: 400, PhysDefense: 150, MagicDefense: 140, Xu: 48800}}}},
		"world_tick_look": {Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
			Tick: 48, Looks: []*pb.LookChange{{Id: 78, WeaponEnhance: 10}}}}},
		"entity_enter_glow": {Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
			Id: 78, Kind: pb.EntityKind_ENTITY_KIND_PLAYER, Name: "Mai", CharClass: pb.CharClass_CHAR_CLASS_MINH,
			Level: 20, X: 7.5, Y: 8.5, DirY: -1, Hp: 1500, MaxHp: 1500, WeaponEnhance: 12}}},
		"use_portal_req": {Seq: 17, Payload: &pb.Envelope_UsePortalReq{UsePortalReq: &pb.UsePortalReq{
			PortalId: "to_ubuyashiki"}}},
		"zone_list_resp": {Seq: 18, Payload: &pb.Envelope_ZoneListResp{ZoneListResp: &pb.ZoneListResp{
			Zones: []*pb.ZoneInfo{
				{Zone: 1, Players: 12, Capacity: 50},
				{Zone: 2, Players: 3, Capacity: 50},
			}}}},
		"enter_world_resp_cooldowns": {Seq: 19, Payload: &pb.Envelope_EnterWorldResp{EnterWorldResp: &pb.EnterWorldResp{
			SelfEntityId: 77, MapId: "ubuyashiki", Zone: 1, X: 4.5, Y: 24,
			ServerTick: 1300, MoveSpeed: 4, TickRate: 10,
			Cooldowns: []*pb.Cooldown{
				{Key: "viem_1", RemainingMs: 2000},
				{Key: "zone_change", RemainingMs: 9500},
			}}}},
		"buy_req": {Seq: 20, Payload: &pb.Envelope_BuyReq{BuyReq: &pb.BuyReq{
			NpcId: "kumotori_apothecary", Code: "potion_hp_1", Count: 10}}},
		"use_item_resp": {Seq: 21, Payload: &pb.Envelope_UseItemResp{UseItemResp: &pb.UseItemResp{
			// One potion left bag slot 3; HP restored; the next potion waits 10 s.
			Update: &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 3}}},
			Stats: &pb.SelfStats{Level: 30, Exp: 10, ExpNext: 9000, Hp: 2100, MaxHp: 2400, Mp: 300, MaxMp: 300,
				PhysAttack: 400, PhysDefense: 150, MagicDefense: 140, Xu: 48800},
			CooldownMs: 10000}}},
		"travel_req": {Seq: 22, Payload: &pb.Envelope_TravelReq{TravelReq: &pb.TravelReq{
			NpcId: "ubuyashiki_carriage", Route: 1}}},
		"stash_open_resp": {Seq: 23, Payload: &pb.Envelope_StashOpenResp{StashOpenResp: &pb.StashOpenResp{
			Stash: &pb.Stash{Size: 40, Items: []*pb.ItemSlot{
				{Slot: 0, Item: &pb.ItemInfo{Code: "stone_enhance", Count: 500}},
				{Slot: 4, Item: &pb.ItemInfo{Code: "eq10_armor", Count: 1, Quality: 1, Enhance: 8,
					Lines: []*pb.ItemLine{{Stat: "hp", Value: 60}}}},
			}}}}},
		"stash_deposit_resp": {Seq: 24, Payload: &pb.Envelope_StashDepositResp{StashDepositResp: &pb.StashDepositResp{
			// Bag slot 2 is now empty; the item landed in stash slot 1.
			Update: &pb.InventoryUpdate{Bag: []*pb.ItemSlot{{Slot: 2}}},
			Stash:  []*pb.ItemSlot{{Slot: 1, Item: &pb.ItemInfo{Code: "scroll_protect", Count: 1}}}}}},
		"respawn_req_in_place": {Seq: 25, Payload: &pb.Envelope_RespawnReq{RespawnReq: &pb.RespawnReq{InPlace: true}}},
		"set_pk_req":           {Seq: 26, Payload: &pb.Envelope_SetPkReq{SetPkReq: &pb.SetPkReq{On: true}}},
		"death_notice": {Seq: 0, Payload: &pb.Envelope_DeathNotice{DeathNotice: &pb.DeathNotice{
			KillerId: 78, KillerName: "Mai", ByPlayer: true, ExpLost: 3200}}},
		"entity_enter_pk": {Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
			Id: 78, Kind: pb.EntityKind_ENTITY_KIND_PLAYER, Name: "Mai", CharClass: pb.CharClass_CHAR_CLASS_MINH,
			Level: 30, X: 7.5, Y: 8.5, DirY: -1, Hp: 1500, MaxHp: 1500, PkOn: true}}},
		"party_invite": {Seq: 0, Payload: &pb.Envelope_PartyInvite{PartyInvite: &pb.PartyInvite{
			FromId: 78, FromName: "Mai", FromLevel: 30, FromClass: pb.CharClass_CHAR_CLASS_MINH, ExpiresMs: 30000}}},
		"party_state": {Seq: 0, Payload: &pb.Envelope_PartyState{PartyState: &pb.PartyState{LeaderId: 78, Members: []*pb.PartyMember{
			{Id: 78, Name: "Mai", CharClass: pb.CharClass_CHAR_CLASS_MINH, Level: 30, MapId: "village", Zone: 1},
			{Id: 91, Name: "Tanjiro", CharClass: pb.CharClass_CHAR_CLASS_MINH, Level: 28, MapId: "village", Zone: 1},
			{Id: 104, Name: "Zen", CharClass: pb.CharClass_CHAR_CLASS_MINH, Level: 25, MapId: "forest", Zone: 2}}}}},
		"party_notice": {Seq: 0, Payload: &pb.Envelope_PartyNotice{PartyNotice: &pb.PartyNotice{
			Kind: pb.PartyNoticeKind_PARTY_NOTICE_KIND_NEW_LEADER, Name: "Mai"}}},
		"party_answer_req": {Seq: 27, Payload: &pb.Envelope_PartyAnswerReq{PartyAnswerReq: &pb.PartyAnswerReq{
			FromId: 78, Accept: true}}},
	}
}

// The client pins the same number in its handshake; bump both together.
func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != 13 {
		t.Fatalf("ProtocolVersion = %d, want 13", ProtocolVersion)
	}
}

// readFixture decodes a committed client fixture.
func readFixture(t *testing.T, name string) *pb.Envelope {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "client", name+".hex"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var env pb.Envelope
	if err := proto.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	return &env
}

// The ground item fixture decodes to an ITEM entity with its quality, element,
// lines and hidden lines.
func TestItemFixturesRoundTrip(t *testing.T) {
	e := readFixture(t, "entity_enter_item").GetEntityEnter()
	it := e.GetItem()
	if e.GetKind() != pb.EntityKind_ENTITY_KIND_ITEM || e.GetName() != "Áo Lữ Hành" || e.GetLevel() != 0 || e.GetMaxHp() != 0 {
		t.Fatalf("entity %v, want an ITEM named Áo Lữ Hành with no level or HP", e)
	}
	if it.GetCode() != "eq10_armor" || it.GetCount() != 1 || it.GetQuality() != 2 || it.GetElement() != pb.CharClass_CHAR_CLASS_THUY {
		t.Fatalf("item %v, want one Vàng Thủy eq10_armor", it)
	}
	lines, hidden := it.GetLines(), it.GetHidden()
	if len(lines) != 3 || lines[0].GetStat() != "hp" || lines[0].GetValue() != 60 || lines[2].GetStat() != "magic_attack" ||
		len(hidden) != 2 || hidden[0].GetStat() != "hp" || hidden[0].GetValue() != 86 || hidden[1].GetStat() != "phys_attack" {
		t.Fatalf("lines %v hidden %v", lines, hidden)
	}

	inv := readFixture(t, "enter_world_resp_inventory").GetEnterWorldResp()
	if inv.GetStats().GetXu() != 1234567 || inv.GetInventory().GetBagSize() != 60 ||
		len(inv.GetInventory().GetBag()) != 2 || inv.GetInventory().GetBag()[1].GetItem().GetCount() != 12 ||
		len(inv.GetInventory().GetEquipped()) != 1 {
		t.Fatalf("enter world %v", inv)
	}

	// An updated slot without an item decodes as empty.
	up := readFixture(t, "equip_resp").GetEquipResp()
	if bag := up.GetUpdate().GetBag(); len(bag) != 1 || bag[0].GetSlot() != 4 || bag[0].GetItem() != nil {
		t.Fatalf("bag update %v, want slot 4 empty", bag)
	}
	if eq := up.GetUpdate().GetEquipped(); len(eq) != 1 || eq[0].GetSlot() != 5 || eq[0].GetItem().GetCode() != "eq10_armor" ||
		up.GetStats().GetMaxHp() != 1200 {
		t.Fatalf("equip resp %v", up)
	}
}

// The transfer fixtures decode back to the portal request, a two-zone list and
// an EnterWorldResp carrying a skill and a zone-change cooldown.
func TestTransferFixturesRoundTrip(t *testing.T) {
	env := readFixture(t, "use_portal_req")
	if env.GetSeq() != 17 || env.GetUsePortalReq().GetPortalId() != "to_ubuyashiki" {
		t.Fatalf("use portal %v", env)
	}
	zones := readFixture(t, "zone_list_resp").GetZoneListResp().GetZones()
	if len(zones) != 2 || zones[0].GetZone() != 1 || zones[0].GetPlayers() != 12 || zones[0].GetCapacity() != 50 ||
		zones[1].GetZone() != 2 || zones[1].GetPlayers() != 3 {
		t.Fatalf("zones %v, want 1 (12/50) and 2 (3/50)", zones)
	}
	ew := readFixture(t, "enter_world_resp_cooldowns").GetEnterWorldResp()
	cd := ew.GetCooldowns()
	if ew.GetMapId() != "ubuyashiki" || len(cd) != 2 || cd[0].GetKey() != "viem_1" || cd[0].GetRemainingMs() != 2000 ||
		cd[1].GetKey() != "zone_change" || cd[1].GetRemainingMs() != 9500 {
		t.Fatalf("enter world %v, want viem_1 2000 ms and zone_change 9500 ms", ew)
	}
	if readFixture(t, "enter_world_resp").GetEnterWorldResp().GetCooldowns() != nil {
		t.Fatal("a login EnterWorldResp carries cooldowns")
	}
}

// The shop fixtures decode back to the buy request, the potion reply and the
// carriage request.
func TestShopFixturesRoundTrip(t *testing.T) {
	buy := readFixture(t, "buy_req")
	if b := buy.GetBuyReq(); buy.GetSeq() != 20 || b.GetNpcId() != "kumotori_apothecary" || b.GetCode() != "potion_hp_1" || b.GetCount() != 10 {
		t.Fatalf("buy %v", buy)
	}
	use := readFixture(t, "use_item_resp")
	u := use.GetUseItemResp()
	if use.GetSeq() != 21 || u.GetCooldownMs() != 10000 || u.GetStats().GetHp() != 2100 ||
		len(u.GetUpdate().GetBag()) != 1 || u.GetUpdate().GetBag()[0].GetSlot() != 3 {
		t.Fatalf("use item %v", use)
	}
	tr := readFixture(t, "travel_req")
	if r := tr.GetTravelReq(); tr.GetSeq() != 22 || r.GetNpcId() != "ubuyashiki_carriage" || r.GetRoute() != 1 {
		t.Fatalf("travel %v", tr)
	}
}

// The stash fixtures decode back to a 40-slot stash with two items, a deposit
// that empties one bag slot, and an in-place respawn request.
func TestStashFixturesRoundTrip(t *testing.T) {
	open := readFixture(t, "stash_open_resp")
	st := open.GetStashOpenResp().GetStash()
	if open.GetSeq() != 23 || st.GetSize() != 40 || len(st.GetItems()) != 2 ||
		st.GetItems()[0].GetItem().GetCount() != 500 || st.GetItems()[1].GetSlot() != 4 {
		t.Fatalf("stash open %v", open)
	}
	dep := readFixture(t, "stash_deposit_resp")
	d := dep.GetStashDepositResp()
	if dep.GetSeq() != 24 || len(d.GetUpdate().GetBag()) != 1 || d.GetUpdate().GetBag()[0].GetItem() != nil ||
		len(d.GetStash()) != 1 || d.GetStash()[0].GetSlot() != 1 || d.GetStash()[0].GetItem().GetCode() != "scroll_protect" {
		t.Fatalf("stash deposit %v", dep)
	}
	rs := readFixture(t, "respawn_req_in_place")
	if rs.GetSeq() != 25 || !rs.GetRespawnReq().GetInPlace() {
		t.Fatalf("respawn %v", rs)
	}
}

// The PK fixtures decode back to the toggle request, a death notice from a
// player and a player entity with PK on.
func TestPkFixturesRoundTrip(t *testing.T) {
	req := readFixture(t, "set_pk_req")
	if req.GetSeq() != 26 || !req.GetSetPkReq().GetOn() {
		t.Fatalf("set pk %v", req)
	}
	dn := readFixture(t, "death_notice")
	if d := dn.GetDeathNotice(); dn.GetSeq() != 0 || d.GetKillerId() != 78 || d.GetKillerName() != "Mai" ||
		!d.GetByPlayer() || d.GetExpLost() != 3200 {
		t.Fatalf("death notice %v", dn)
	}
	if e := readFixture(t, "entity_enter_pk").GetEntityEnter(); e.GetKind() != pb.EntityKind_ENTITY_KIND_PLAYER || !e.GetPkOn() {
		t.Fatalf("entity %v, want a player with pk_on", e)
	}
}

// The party fixtures decode back to their invite, three members on two maps,
// a notice and the answer.
func TestPartyFixturesRoundTrip(t *testing.T) {
	inv := readFixture(t, "party_invite")
	if i := inv.GetPartyInvite(); inv.GetSeq() != 0 || i.GetFromId() != 78 || i.GetFromName() != "Mai" ||
		i.GetFromLevel() != 30 || i.GetExpiresMs() != 30000 {
		t.Fatalf("party invite %v", inv)
	}
	st := readFixture(t, "party_state").GetPartyState()
	m := st.GetMembers()
	if st.GetLeaderId() != 78 || len(m) != 3 || m[0].GetMapId() != "village" || m[2].GetMapId() != "forest" ||
		m[2].GetZone() != 2 || m[1].GetName() != "Tanjiro" {
		t.Fatalf("party state %v", st)
	}
	if n := readFixture(t, "party_notice").GetPartyNotice(); n.GetKind() != pb.PartyNoticeKind_PARTY_NOTICE_KIND_NEW_LEADER ||
		n.GetName() != "Mai" {
		t.Fatalf("party notice %v", n)
	}
	ans := readFixture(t, "party_answer_req")
	if a := ans.GetPartyAnswerReq(); ans.GetSeq() != 27 || a.GetFromId() != 78 || !a.GetAccept() {
		t.Fatalf("party answer %v", ans)
	}
}

// The enhancement fixtures decode back to their request, reply, look change and
// glowing player.
func TestEnhanceFixturesRoundTrip(t *testing.T) {
	look := readFixture(t, "world_tick_look").GetWorldTick()
	if l := look.GetLooks(); look.GetTick() != 48 || len(l) != 1 || l[0].GetId() != 78 || l[0].GetWeaponEnhance() != 10 {
		t.Fatalf("world tick %v, want one look of 78 at +10", look)
	}
	if e := readFixture(t, "entity_enter_glow").GetEntityEnter(); e.GetKind() != pb.EntityKind_ENTITY_KIND_PLAYER ||
		e.GetWeaponEnhance() != 12 {
		t.Fatalf("entity %v, want a player with weapon_enhance 12", e)
	}
	req := readFixture(t, "enhance_req").GetEnhanceReq()
	if !req.GetItem().GetEquipped() || req.GetItem().GetSlot() != 5 || !req.GetProtect() || req.GetTimes() != 100 {
		t.Fatalf("enhance req %v", req)
	}
	resp := readFixture(t, "enhance_resp").GetEnhanceResp()
	up := resp.GetUpdate()
	if resp.GetAttempts() != 3 || !resp.GetSuccess() || resp.GetLevel() != 8 ||
		len(up.GetBag()) != 2 || up.GetBag()[1].GetItem() != nil ||
		len(up.GetEquipped()) != 1 || up.GetEquipped()[0].GetItem().GetEnhance() != 8 ||
		resp.GetStats().GetXu() != 1232167 || resp.GetStats().GetHpRegenBp() != 5 || resp.GetStats().GetMpRegen() != 1 {
		t.Fatalf("enhance resp %v", resp)
	}
}

// The socket fixtures decode back to an item with three holes, an empty one in
// the middle, and to a refine result with its lines in order.
func TestSocketFixturesRoundTrip(t *testing.T) {
	eq := readFixture(t, "item_with_gems").GetEquipResp().GetUpdate().GetEquipped()
	if len(eq) != 1 {
		t.Fatalf("equipped %v, want one slot", eq)
	}
	it := eq[0].GetItem()
	if g := it.GetGems(); it.GetHoles() != 3 || len(g) != 3 || g[0] != "gem_hp_2" || g[1] != "" || g[2] != "gem_defense_3" ||
		len(it.GetHidden()) != 2 {
		t.Fatalf("item %v, want 3 holes with gem_hp_2, empty, gem_defense_3", it)
	}
	resp := readFixture(t, "refine_resp").GetRefineResp()
	if l := resp.GetLines(); len(l) != 3 || l[0].GetStat() != "hp" || l[1].GetStat() != "magic_attack" ||
		l[2].GetValue() != 9 || len(resp.GetUpdate().GetBag()) != 1 || resp.GetStats().GetXu() != 48800 {
		t.Fatalf("refine resp %v", resp)
	}
}

// The ranged fixture decodes back to a Hit that carries the flag, and the
// melee Hit of the combat fixture does not.
func TestRangedHitFixtureRoundTrip(t *testing.T) {
	for name, want := range map[string]bool{"world_tick_ranged": true, "world_tick_combat": false} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "client", name+".hex"))
		if err != nil {
			t.Fatal(err)
		}
		b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		var env pb.Envelope
		if err := proto.Unmarshal(b, &env); err != nil {
			t.Fatal(err)
		}
		hits := env.GetWorldTick().GetHits()
		if len(hits) != 1 || hits[0].GetRanged() != want {
			t.Fatalf("%s: hits %v, want one with ranged=%v", name, hits, want)
		}
	}
}

// The cast fixture decodes back to the Cast with its level, both Hits with their skill id and
// the snapping move.
func TestCastFixtureRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "client", "world_tick_cast.hex"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var env pb.Envelope
	if err := proto.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	wt := env.GetWorldTick()
	casts := wt.GetCasts()
	if len(casts) != 1 || casts[0].GetCasterId() != 77 || casts[0].GetSkillId() != "viem_1" || casts[0].GetTargetId() != 900 || casts[0].GetLevel() != 3 {
		t.Fatalf("casts %v, want one level-3 viem_1 cast by 77 at 900", casts)
	}
	c := casts[0]
	if c.GetWindupMs() != 250 || c.GetRanged() || len(c.GetImpacts()) != 2 ||
		c.GetImpacts()[0].GetTargetId() != 900 || c.GetImpacts()[0].GetImpactMs() != 500 ||
		c.GetImpacts()[1].GetTargetId() != 902 || c.GetImpacts()[1].GetImpactMs() != 650 {
		t.Fatalf("cast %v, want windup 250 and impacts 900@500, 902@650", c)
	}
	hits := wt.GetHits()
	if len(hits) != 2 || hits[0].GetSkillId() != "viem_1" || hits[1].GetSkillId() != "viem_1" {
		t.Fatalf("hits %v, want two with skill_id viem_1", hits)
	}
	if mv := wt.GetMoves(); len(mv) != 1 || !mv[0].GetSnap() {
		t.Fatalf("moves %v, want one snap", mv)
	}
}

// The monster attack fixture is a ranged Cast with no skill id and one impact; the impact
// fixture carries only a Hit.
func TestAttackTimingFixturesRoundTrip(t *testing.T) {
	read := func(name string) *pb.WorldTick {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "client", name+".hex"))
		if err != nil {
			t.Fatal(err)
		}
		b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		var env pb.Envelope
		if err := proto.Unmarshal(b, &env); err != nil {
			t.Fatal(err)
		}
		return env.GetWorldTick()
	}
	wt := read("world_tick_monster_attack")
	if len(wt.GetCasts()) != 1 || len(wt.GetHits()) != 0 {
		t.Fatalf("monster attack: casts %v hits %v, want one cast and no hit", wt.GetCasts(), wt.GetHits())
	}
	c := wt.GetCasts()[0]
	if c.GetCasterId() != 900 || c.GetSkillId() != "" || !c.GetRanged() || c.GetWindupMs() != 400 ||
		len(c.GetImpacts()) != 1 || c.GetImpacts()[0].GetTargetId() != 77 || c.GetImpacts()[0].GetImpactMs() != 700 {
		t.Fatalf("monster attack cast %v", c)
	}
	wt = read("world_tick_impact")
	if len(wt.GetCasts()) != 0 || len(wt.GetHits()) != 1 || wt.GetHits()[0].GetDamage() != 9 {
		t.Fatalf("impact: casts %v hits %v, want only one hit", wt.GetCasts(), wt.GetHits())
	}
}

func TestClientFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "client")
	if *updateFixtures {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, env := range clientFixtures() {
		b, err := proto.MarshalOptions{Deterministic: true}.Marshal(env)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		want := hex.EncodeToString(b) + "\n"
		path := filepath.Join(dir, name+".hex")
		if *updateFixtures {
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("%s missing or stale (err=%v); run: go test ./internal/gateway -run TestClientFixtures -update", path, err)
		}
	}
}
