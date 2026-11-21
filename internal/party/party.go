// Package party holds every party in memory under one lock (phase 5b spec
// 4.1). It never calls into the world or a zone: it writes each member's
// party-id cell, which zones only read, and pushes invites, party states and
// notices straight into the members' outboxes.
//
// The pushes are package-local messages (Invite, State, Notice) and the error
// codes are pb.ErrorCode values with the numbers of protocol 13. The protocol
// messages and the named error codes arrive with protocol 13; until then the
// caller adapts its connection outbox to Outbox and converts each Msg into the
// matching envelope.
package party

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/pb"
)

// Party limits (phase 5b spec 2).
const (
	MaxMembers = 5
	InviteTTL  = 30 * time.Second
)

// Error codes the service returns; OK on success. The party codes carry the
// protocol 13 numbers (phase 5b spec 5).
const (
	OK             = pb.ErrorCode_ERROR_CODE_UNSPECIFIED
	BadRequest     = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	NotInWorld     = pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD
	InvalidTarget  = pb.ErrorCode_ERROR_CODE_INVALID_TARGET
	PartyFull      = pb.ErrorCode_ERROR_CODE_PARTY_FULL
	AlreadyInParty = pb.ErrorCode_ERROR_CODE_ALREADY_IN_PARTY
	NotPartyLeader = pb.ErrorCode_ERROR_CODE_NOT_PARTY_LEADER
	NotInParty     = pb.ErrorCode_ERROR_CODE_NOT_IN_PARTY
	InviteNotFound = pb.ErrorCode_ERROR_CODE_INVITE_NOT_FOUND
	InvitePending  = pb.ErrorCode_ERROR_CODE_INVITE_PENDING
)

// NoticeKind is a party event shown to a member; the values are those of
// PartyNoticeKind in protocol 13.
type NoticeKind int32

const (
	NoticeJoined        NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_JOINED)          // Name joined the party
	NoticeLeft          NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_LEFT)            // Name left the party
	NoticeKicked        NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_KICKED)          // the leader removed Name
	NoticeYouWereKicked NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_YOU_WERE_KICKED) // the leader removed you
	NoticeDeclined      NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_DECLINED)        // Name declined your invite
	NoticeDisbanded     NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_DISBANDED)       // you were the last member
	NoticeNewLeader     NoticeKind = NoticeKind(pb.PartyNoticeKind_PARTY_NOTICE_KIND_NEW_LEADER)      // Name now leads the party
)

// Msg is one push: *Invite, *State or *Notice. Receivers must not modify it;
// one State is shared by every member it is sent to.
type Msg interface{ isMsg() }

// Invite tells a player who invites it (PartyInvite).
type Invite struct {
	FromID    uint64
	FromName  string
	FromLevel int
	FromClass pb.CharClass
	ExpiresMs int32
}

// Member is one party member as every member sees it (PartyMember).
type Member Info

// State is a whole party (PartyState); no members means not in a party.
type State struct {
	LeaderID uint64
	Members  []Member // join order
}

// Notice is a party event (PartyNotice).
type Notice struct {
	Kind NoticeKind
	Name string
}

func (*Invite) isMsg() {}
func (*State) isMsg()  {}
func (*Notice) isMsg() {}

// Outbox receives a player's pushes. Send is called with the service lock
// held, so it must not block or call back into the service (the gateway
// outbox drops the message when its queue is full).
type Outbox interface {
	Send(Msg) bool
}

// Info is what the service knows of a player in the world. ID is the entity
// id, kept across zone changes.
type Info struct {
	ID    uint64
	Name  string
	Class pb.CharClass
	Level int
	MapID string
	Zone  int
}

type party struct {
	id      uint64
	leader  uint64
	members []uint64 // join order
}

// invite is a pending invite, keyed by the invitee.
type invite struct {
	from  uint64
	until time.Time
}

type entry struct {
	info  Info
	out   Outbox
	cell  *atomic.Uint64
	party *party
}

// Service holds the players in the world, their parties and the pending invites.
type Service struct {
	now func() time.Time

	mu      sync.Mutex
	lastID  uint64
	players map[uint64]*entry
	invites map[uint64]invite
}

// New returns an empty service reading the time from now.
func New(now func() time.Time) *Service {
	return &Service{now: now, players: map[uint64]*entry{}, invites: map[uint64]invite{}}
}

// Register adds a player entering the world and returns its party-id cell,
// at 0. Registering an id again first removes the old entry as Unregister does.
func (s *Service) Register(info Info, out Outbox) *atomic.Uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unregister(info.ID)
	e := &entry{info: info, out: out, cell: new(atomic.Uint64)}
	s.players[info.ID] = e
	return e.cell
}

// Unregister removes a player leaving the world: it leaves its party as Leave
// does, without any push to it, and every invite from or to it is dropped.
func (s *Service) Unregister(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unregister(id)
}

func (s *Service) unregister(id uint64) {
	e := s.players[id]
	if e == nil {
		return
	}
	if e.party != nil {
		s.remove(e, false, false)
	}
	delete(s.invites, id)
	for to, inv := range s.invites {
		if inv.from == id {
			delete(s.invites, to)
		}
	}
	delete(s.players, id)
}

// Invite records an invite from one player to another and pushes it to the
// invitee (phase 5b spec 4.2).
func (s *Service) Invite(from, to uint64) pb.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.players[from]
	if i == nil {
		return NotInWorld
	}
	t := s.players[to]
	if to == from || t == nil {
		return InvalidTarget
	}
	if i.party != nil && i.party.leader != from {
		return NotPartyLeader
	}
	if i.party != nil && len(i.party.members) >= MaxMembers {
		return PartyFull
	}
	if t.party != nil {
		return AlreadyInParty
	}
	if _, ok := s.pending(to); ok {
		return InvitePending
	}
	s.invites[to] = invite{from: from, until: s.now().Add(InviteTTL)}
	s.push(to, &Invite{
		FromID:    from,
		FromName:  i.info.Name,
		FromLevel: i.info.Level,
		FromClass: i.info.Class,
		ExpiresMs: int32(InviteTTL.Milliseconds()),
	})
	return OK
}

// pending returns the unexpired invite to a player, dropping an expired one.
func (s *Service) pending(to uint64) (invite, bool) {
	inv, ok := s.invites[to]
	if !ok {
		return invite{}, false
	}
	if !s.now().Before(inv.until) {
		delete(s.invites, to)
		return invite{}, false
	}
	return inv, true
}

// Answer accepts or declines the invite from one player to another (phase 5b
// spec 4.3). The invite is gone after any answer that finds it.
func (s *Service) Answer(to, from uint64, accept bool) pb.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.players[to]
	if t == nil {
		return NotInWorld
	}
	inv, ok := s.pending(to)
	if !ok || inv.from != from {
		return InviteNotFound
	}
	delete(s.invites, to)
	i := s.players[from]
	if !accept {
		if i != nil {
			s.push(from, &Notice{Kind: NoticeDeclined, Name: t.info.Name})
		}
		return OK
	}
	if t.party != nil {
		return AlreadyInParty
	}
	if i == nil || (i.party != nil && i.party.leader != from) {
		return InviteNotFound
	}
	if i.party != nil && len(i.party.members) >= MaxMembers {
		return PartyFull
	}
	p := i.party
	if p == nil {
		s.lastID++
		p = &party{id: s.lastID, leader: from, members: []uint64{from}}
		i.party = p
		i.cell.Store(p.id)
	}
	old := slices.Clone(p.members)
	p.members = append(p.members, to)
	t.party = p
	t.cell.Store(p.id)
	joined := &Notice{Kind: NoticeJoined, Name: t.info.Name}
	for _, id := range old {
		s.push(id, joined)
	}
	s.pushState(p)
	return OK
}

// Leave takes a player out of its party (phase 5b spec 4.4).
func (s *Service) Leave(id uint64) pb.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.players[id]
	if e == nil {
		return NotInWorld
	}
	if e.party == nil {
		return NotInParty
	}
	s.remove(e, true, false)
	return OK
}

// Kick lets a party leader remove a member (phase 5b spec 4.5).
func (s *Service) Kick(leader, member uint64) pb.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.players[leader]
	if l == nil {
		return NotInWorld
	}
	if l.party == nil {
		return NotInParty
	}
	if l.party.leader != leader {
		return NotPartyLeader
	}
	m := s.players[member]
	if member == leader || m == nil || m.party != l.party {
		return BadRequest
	}
	s.remove(m, true, true)
	return OK
}

// remove takes e out of its party. tell sends e its pushes (an empty state,
// after YOU_WERE_KICKED when kicked); Unregister passes false. Every cell is
// written before the first push.
func (s *Service) remove(e *entry, tell, kicked bool) {
	p := e.party
	p.members = slices.DeleteFunc(p.members, func(id uint64) bool { return id == e.info.ID })
	e.party = nil
	e.cell.Store(0)
	disband := len(p.members) == 1
	var last *entry
	if disband {
		last = s.players[p.members[0]]
		last.party = nil
		last.cell.Store(0)
	}
	newLeader := !disband && p.leader == e.info.ID
	if newLeader {
		p.leader = p.members[0]
	}

	if tell {
		if kicked {
			s.push(e.info.ID, &Notice{Kind: NoticeYouWereKicked})
		}
		s.push(e.info.ID, &State{})
	}
	if disband {
		s.push(last.info.ID, &Notice{Kind: NoticeDisbanded})
		s.push(last.info.ID, &State{})
		return
	}
	kind := NoticeLeft
	if kicked {
		kind = NoticeKicked
	}
	gone := &Notice{Kind: kind, Name: e.info.Name}
	for _, id := range p.members {
		s.push(id, gone)
	}
	if newLeader {
		n := &Notice{Kind: NoticeNewLeader, Name: s.players[p.leader].info.Name}
		for _, id := range p.members {
			s.push(id, n)
		}
	}
	s.pushState(p)
}

// Moved records a player's new map and zone and pushes the party state.
func (s *Service) Moved(id uint64, mapID string, zone int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.players[id]
	if e == nil {
		return
	}
	e.info.MapID, e.info.Zone = mapID, zone
	if e.party != nil {
		s.pushState(e.party)
	}
}

// Leveled records a player's new level and pushes the party state.
func (s *Service) Leveled(id uint64, level int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.players[id]
	if e == nil {
		return
	}
	e.info.Level = level
	if e.party != nil {
		s.pushState(e.party)
	}
}

// stateOf is the PartyState of p, members in join order with their current info.
func (s *Service) stateOf(p *party) *State {
	st := &State{LeaderID: p.leader, Members: make([]Member, 0, len(p.members))}
	for _, id := range p.members {
		st.Members = append(st.Members, Member(s.players[id].info))
	}
	return st
}

// pushState sends one fresh state of p to every member.
func (s *Service) pushState(p *party) {
	st := s.stateOf(p)
	for _, id := range p.members {
		s.push(id, st)
	}
}

// push sends m to a registered player; a full outbox drops it.
func (s *Service) push(id uint64, m Msg) {
	if e := s.players[id]; e != nil {
		e.out.Send(m)
	}
}
