package party

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"demonslayer2d/internal/pb"
)

// fakeClock is a clock the test moves by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// box records what the service pushes to one player.
// It also checks that the receiver's cell is written before each state is
// pushed: 0 with an empty state, a party id otherwise.
type box struct {
	mu   sync.Mutex
	msgs []Msg
	cell atomic.Pointer[atomic.Uint64]
	bad  atomic.Int32
}

func (b *box) Send(m Msg) bool {
	if st, ok := m.(*State); ok {
		if c := b.cell.Load(); c != nil && (len(st.Members) == 0) != (c.Load() == 0) {
			b.bad.Add(1)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msgs = append(b.msgs, m)
	return true
}

// take returns the pushes since the last take and forgets them.
func (b *box) take() []Msg {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.msgs
	b.msgs = nil
	return m
}

type player struct {
	id   uint64
	box  *box
	cell *atomic.Uint64
}

type fixture struct {
	t   *testing.T
	s   *Service
	clk *fakeClock
	p   map[uint64]*player
}

func newFixture(t *testing.T) *fixture {
	clk := &fakeClock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	return &fixture{t: t, s: New(clk.now), clk: clk, p: map[uint64]*player{}}
}

// reg registers player id with name "p<id>", class Viem, level id, map "m", zone 1.
func (f *fixture) reg(ids ...uint64) {
	for _, id := range ids {
		b := &box{}
		cell := f.s.Register(info(id), b)
		if cell == nil || cell.Load() != 0 {
			f.t.Fatalf("Register(%d) cell %v", id, cell)
		}
		b.cell.Store(cell)
		f.p[id] = &player{id: id, box: b, cell: cell}
		f.t.Cleanup(func() {
			if n := b.bad.Load(); n > 0 {
				f.t.Errorf("player %d got %d states that did not match its cell", id, n)
			}
		})
	}
}

func info(id uint64) Info {
	return Info{ID: id, Name: fmt.Sprintf("p%d", id), Class: pb.CharClass_CHAR_CLASS_VIEM, Level: int(id), MapID: "m", Zone: 1}
}

func (f *fixture) code(got, want pb.ErrorCode, what string) {
	f.t.Helper()
	if got != want {
		f.t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// party forms a party led by ids[0] with the others joining in order, then
// clears every box.
func (f *fixture) party(ids ...uint64) {
	f.t.Helper()
	for _, id := range ids[1:] {
		f.code(f.s.Invite(ids[0], id), OK, "Invite")
		f.code(f.s.Answer(id, ids[0], true), OK, "Answer")
	}
	for _, p := range f.p {
		p.box.take()
	}
}

func (f *fixture) take(id uint64) []Msg { return f.p[id].box.take() }

func (f *fixture) cell(id uint64) uint64 { return f.p[id].cell.Load() }

// state is the PartyState a party led by leader with members in join order shows.
func state(leader uint64, members ...uint64) *State {
	st := &State{LeaderID: leader}
	for _, id := range members {
		st.Members = append(st.Members, Member(info(id)))
	}
	return st
}

func notice(k NoticeKind, name string) *Notice { return &Notice{Kind: k, Name: name} }

func (f *fixture) want(id uint64, want ...Msg) {
	f.t.Helper()
	got := f.take(id)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		f.t.Fatalf("player %d got %s, want %s", id, dump(got), dump(want))
	}
}

func dump(ms []Msg) string {
	s := "["
	for _, m := range ms {
		s += fmt.Sprintf(" %T%+v", m, m)
	}
	return s + " ]"
}

// ______________________TEST INVITE_____________________

func TestInviteErrorsInOrder(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3, 4, 5, 6, 7, 8, 9)
	f.code(f.s.Invite(1, 1), InvalidTarget, "invite self")
	f.code(f.s.Invite(1, 99), InvalidTarget, "invite unknown")
	f.code(f.s.Invite(99, 1), NotInWorld, "invite from unknown")

	f.party(1, 2)
	f.code(f.s.Invite(2, 3), NotPartyLeader, "member invites")
	f.code(f.s.Invite(2, 2), InvalidTarget, "self beats not leader")

	f.party(1, 3, 4, 5)
	f.code(f.s.Invite(1, 6), PartyFull, "full party invites")
	f.code(f.s.Invite(1, 2), PartyFull, "full beats already in party")

	f.party(6, 7)
	f.code(f.s.Invite(8, 7), AlreadyInParty, "target in another party")
	f.code(f.s.Invite(6, 7), AlreadyInParty, "target in own party")

	f.code(f.s.Invite(8, 9), OK, "first invite")
	f.code(f.s.Invite(6, 9), InvitePending, "second invite")
	f.code(f.s.Invite(8, 9), InvitePending, "same invite again")
}

func TestInvitePushesAndExpires(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3)
	f.code(f.s.Invite(3, 2), OK, "Invite")
	f.want(2, &Invite{FromID: 3, FromName: "p3", FromLevel: 3, FromClass: pb.CharClass_CHAR_CLASS_VIEM, ExpiresMs: 30000})
	f.want(3)
	if f.cell(3) != 0 || f.cell(2) != 0 {
		t.Fatal("an invite must not form a party")
	}
	f.code(f.s.Invite(3, 1), OK, "inviter may invite several players")

	f.clk.add(29 * time.Second)
	f.code(f.s.Invite(1, 2), InvitePending, "still pending at 29 s")
	f.clk.add(2 * time.Second)
	f.code(f.s.Invite(1, 2), OK, "expired invite no longer blocks")
	f.want(2, &Invite{FromID: 1, FromName: "p1", FromLevel: 1, FromClass: pb.CharClass_CHAR_CLASS_VIEM, ExpiresMs: 30000})
}

// ______________________TEST ANSWER_____________________

func TestAnswerDeclineAndAccept(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3)
	f.code(f.s.Answer(2, 1, true), InviteNotFound, "no invite")
	f.code(f.s.Answer(99, 1, true), NotInWorld, "unknown answerer")

	f.code(f.s.Invite(1, 2), OK, "Invite")
	f.code(f.s.Answer(2, 3, true), InviteNotFound, "invite from someone else")
	f.take(2)
	f.code(f.s.Answer(2, 1, false), OK, "decline")
	f.want(1, notice(NoticeDeclined, "p2"))
	f.want(2)
	f.code(f.s.Answer(2, 1, false), InviteNotFound, "invite gone after decline")

	f.code(f.s.Invite(1, 2), OK, "Invite again")
	f.take(2)
	f.code(f.s.Answer(2, 1, true), OK, "accept")
	if f.cell(1) == 0 || f.cell(1) != f.cell(2) {
		t.Fatalf("cells %d %d", f.cell(1), f.cell(2))
	}
	f.want(1, notice(NoticeJoined, "p2"), state(1, 1, 2))
	f.want(2, state(1, 1, 2))
	f.code(f.s.Answer(2, 1, true), InviteNotFound, "invite gone after accept")

	f.code(f.s.Invite(1, 3), OK, "third")
	f.take(3)
	f.code(f.s.Answer(3, 1, true), OK, "third accepts")
	f.want(1, notice(NoticeJoined, "p3"), state(1, 1, 2, 3))
	f.want(2, notice(NoticeJoined, "p3"), state(1, 1, 2, 3))
	f.want(3, state(1, 1, 2, 3))
	if f.cell(3) != f.cell(1) {
		t.Fatal("third cell")
	}
}

func TestAnswerAcceptErrors(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)

	// Expired.
	f.code(f.s.Invite(1, 2), OK, "Invite")
	f.clk.add(31 * time.Second)
	f.code(f.s.Answer(2, 1, true), InviteNotFound, "accept after expiry")
	f.code(f.s.Answer(2, 1, false), InviteNotFound, "expired invite is gone")

	// The invitee has since formed a party of its own.
	f.code(f.s.Invite(1, 2), OK, "Invite 1->2")
	f.code(f.s.Invite(3, 2), InvitePending, "2 already has an invite")
	f.code(f.s.Invite(2, 3), OK, "Invite 2->3")
	f.code(f.s.Answer(3, 2, true), OK, "3 joins 2")
	f.code(f.s.Answer(2, 1, true), AlreadyInParty, "accept when in a party")
	f.code(f.s.Answer(2, 1, true), InviteNotFound, "invite gone after a failed accept")

	// The inviter is now a non-leader member.
	f.code(f.s.Invite(4, 5), OK, "Invite 4->5")
	f.party(2, 4)
	f.code(f.s.Answer(5, 4, true), InviteNotFound, "inviter is a plain member")

	// The inviter is offline.
	f.code(f.s.Invite(6, 7), OK, "Invite 6->7")
	f.s.Unregister(6)
	f.code(f.s.Answer(7, 6, true), InviteNotFound, "inviter offline")

	// The party filled meanwhile.
	f.party(8, 9)
	f.code(f.s.Invite(8, 10), OK, "Invite 8->10")
	f.reg(11, 12, 13)
	f.party(8, 11, 12) // 8, 9, 11, 12
	f.code(f.s.Invite(8, 13), OK, "Invite 8->13")
	f.code(f.s.Answer(13, 8, true), OK, "fifth joins")
	f.take(10)
	f.code(f.s.Answer(10, 8, true), PartyFull, "sixth accepts")
	f.code(f.s.Answer(10, 8, true), InviteNotFound, "invite gone after PARTY_FULL")
	if f.cell(10) != 0 {
		t.Fatal("sixth got a cell")
	}
	f.want(10)
}

// ______________________TEST LEAVE AND KICK_____________________

func TestLeave(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3)
	f.code(f.s.Leave(1), NotInParty, "leave alone")
	f.code(f.s.Leave(99), NotInWorld, "leave unknown")

	f.party(1, 2, 3)
	pid := f.cell(1)
	f.code(f.s.Leave(1), OK, "leader leaves")
	if f.cell(1) != 0 || f.cell(2) != pid || f.cell(3) != pid {
		t.Fatalf("cells %d %d %d", f.cell(1), f.cell(2), f.cell(3))
	}
	f.want(1, &State{})
	f.want(2, notice(NoticeLeft, "p1"), notice(NoticeNewLeader, "p2"), state(2, 2, 3))
	f.want(3, notice(NoticeLeft, "p1"), notice(NoticeNewLeader, "p2"), state(2, 2, 3))
	f.code(f.s.Invite(2, 1), OK, "the new leader invites")

	f.code(f.s.Leave(3), OK, "member of two leaves")
	if f.cell(2) != 0 || f.cell(3) != 0 {
		t.Fatal("disbanded cells")
	}
	f.want(3, &State{})
	f.want(2, notice(NoticeDisbanded, ""), &State{})
	f.code(f.s.Leave(2), NotInParty, "leave a disbanded party")
}

func TestLeaveNonLeader(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3)
	f.party(1, 2, 3)
	f.code(f.s.Leave(2), OK, "member leaves")
	f.want(2, &State{})
	f.want(1, notice(NoticeLeft, "p2"), state(1, 1, 3))
	f.want(3, notice(NoticeLeft, "p2"), state(1, 1, 3))
}

func TestKick(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3, 4)
	f.code(f.s.Kick(1, 2), NotInParty, "kick alone")
	f.code(f.s.Kick(99, 2), NotInWorld, "kick by unknown")
	f.party(1, 2, 3)
	f.code(f.s.Kick(2, 3), NotPartyLeader, "member kicks")
	f.code(f.s.Kick(2, 99), NotPartyLeader, "not leader beats bad member")
	f.code(f.s.Kick(1, 1), BadRequest, "kick self")
	f.code(f.s.Kick(1, 4), BadRequest, "kick non-member")
	f.code(f.s.Kick(1, 99), BadRequest, "kick unknown")

	f.code(f.s.Kick(1, 3), OK, "kick")
	if f.cell(3) != 0 || f.cell(1) == 0 || f.cell(2) != f.cell(1) {
		t.Fatal("cells after kick")
	}
	f.want(3, notice(NoticeYouWereKicked, ""), &State{})
	f.want(1, notice(NoticeKicked, "p3"), state(1, 1, 2))
	f.want(2, notice(NoticeKicked, "p3"), state(1, 1, 2))

	f.code(f.s.Kick(1, 2), OK, "kick down to one")
	f.want(2, notice(NoticeYouWereKicked, ""), &State{})
	f.want(1, notice(NoticeDisbanded, ""), &State{})
	if f.cell(1) != 0 || f.cell(2) != 0 {
		t.Fatal("cells after disband")
	}
}

// ______________________TEST REGISTRY_____________________

func TestUnregister(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3, 4, 5, 6)
	f.party(1, 2, 3)
	f.code(f.s.Invite(1, 4), OK, "invite from the leaver")
	f.code(f.s.Invite(5, 1), AlreadyInParty, "1 is in a party")
	f.code(f.s.Invite(6, 5), OK, "unrelated invite")
	f.take(4)
	f.take(5)

	f.s.Unregister(1)
	f.want(1)
	f.want(2, notice(NoticeLeft, "p1"), notice(NoticeNewLeader, "p2"), state(2, 2, 3))
	f.want(3, notice(NoticeLeft, "p1"), notice(NoticeNewLeader, "p2"), state(2, 2, 3))
	if f.cell(1) != 0 {
		t.Fatal("the leaver cell")
	}
	f.code(f.s.Invite(2, 4), OK, "the invite from the leaver was dropped")
	f.code(f.s.Invite(6, 5), InvitePending, "unrelated invite kept")
	f.code(f.s.Invite(2, 1), InvalidTarget, "the leaver is gone")
	f.s.Unregister(1) // twice is harmless

	// An invite to the leaver is dropped too.
	f.code(f.s.Invite(3, 6), NotPartyLeader, "")
	f.reg(7)
	f.code(f.s.Invite(6, 7), OK, "invite to 7")
	f.s.Unregister(7)
	f.s.mu.Lock()
	_, ok := f.s.invites[7]
	f.s.mu.Unlock()
	if ok {
		t.Fatal("invite to the leaver kept")
	}
}

func TestRegisterTwiceReplaces(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2)
	f.party(1, 2)
	old := f.p[1]
	f.reg(1)
	if old.cell.Load() != 0 {
		t.Fatal("old cell kept its party")
	}
	f.want(2, notice(NoticeDisbanded, ""), &State{})
	if old.box.take() != nil {
		t.Fatal("the replaced entry was sent something")
	}
	f.code(f.s.Invite(2, 1), OK, "the new entry is free")
	f.want(1, &Invite{FromID: 2, FromName: "p2", FromLevel: 2, FromClass: pb.CharClass_CHAR_CLASS_VIEM, ExpiresMs: 30000})
}

func TestMovedAndLeveled(t *testing.T) {
	f := newFixture(t)
	f.reg(1, 2, 3)
	f.s.Moved(3, "x", 2)
	f.s.Leveled(3, 9)
	f.want(3)

	f.party(1, 2)
	f.s.Moved(2, "sagiri_1", 3)
	want := state(1, 1, 2)
	want.Members[1].MapID, want.Members[1].Zone = "sagiri_1", 3
	f.want(1, want)
	f.want(2, want)
	f.want(3)

	f.s.Leveled(1, 40)
	want = state(1, 1, 2)
	want.Members[1].MapID, want.Members[1].Zone = "sagiri_1", 3
	want.Members[0].Level = 40
	f.want(1, want)
	f.want(2, want)

	f.s.Moved(99, "x", 1) // unknown ids are ignored
	f.s.Leveled(99, 1)

	// The updated info shows in invites too.
	f.code(f.s.Invite(1, 3), OK, "Invite")
	f.want(3, &Invite{FromID: 1, FromName: "p1", FromLevel: 40, FromClass: pb.CharClass_CHAR_CLASS_VIEM, ExpiresMs: 30000})
}

// ______________________TEST CONCURRENCY_____________________

func TestConcurrentInviteAnswerLeave(t *testing.T) {
	f := newFixture(t)
	const n = 24
	for id := uint64(1); id <= n; id++ {
		f.reg(id)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			r := rand.New(rand.NewPCG(seed, 7))
			for i := 0; i < 2000; i++ {
				a, b := uint64(r.IntN(n)+1), uint64(r.IntN(n)+1)
				switch r.IntN(6) {
				case 0, 1:
					f.s.Invite(a, b)
				case 2, 3:
					f.s.Answer(a, b, r.IntN(4) != 0)
				case 4:
					f.s.Leave(a)
				case 5:
					f.s.Kick(a, b)
				}
				if i%500 == 0 {
					f.s.Moved(a, "m", r.IntN(3)+1)
				}
			}
		}(uint64(g))
	}
	wg.Wait()

	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	seen := map[uint64]bool{}
	for id, p := range f.p {
		e := f.s.players[id]
		if e == nil {
			t.Fatalf("player %d gone", id)
		}
		if e.party == nil {
			if p.cell.Load() != 0 {
				t.Fatalf("player %d not in a party has cell %d", id, p.cell.Load())
			}
			continue
		}
		pa := e.party
		if p.cell.Load() != pa.id {
			t.Fatalf("player %d cell %d, party %d", id, p.cell.Load(), pa.id)
		}
		if len(pa.members) < 2 || len(pa.members) > MaxMembers || pa.leader != pa.members[0] {
			t.Fatalf("party %d: leader %d members %v", pa.id, pa.leader, pa.members)
		}
		found := false
		for _, m := range pa.members {
			found = found || m == id
		}
		if !found {
			t.Fatalf("player %d not listed in its party", id)
		}
		seen[pa.id] = true
	}
	t.Logf("%d parties at the end", len(seen))
}
