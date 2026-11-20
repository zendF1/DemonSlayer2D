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
	}
}

// The client pins the same number in its handshake; bump both together.
func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != 5 {
		t.Fatalf("ProtocolVersion = %d, want 5", ProtocolVersion)
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
