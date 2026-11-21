package world

import (
	"demonslayer2d/internal/party"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/zone"
)

// partyOutbox is a connection's outbox as the party service sees it: each
// push becomes its envelope with seq 0 (phase 5b spec 5). The service calls
// Send with its lock held; the connection's outbox never blocks. Party pushes
// are one-shot state with no resync, so a push the full queue refuses kicks
// the connection (SLOW_CONSUMER), as a full outbox does everywhere else.
// Kick only takes the connection's kickOnce and outbox mutex, so it is safe
// under the party lock; the unregister runs later on the read goroutine.
type partyOutbox struct{ out zone.Outbox }

func (o partyOutbox) Send(m party.Msg) bool {
	env := partyEnvelope(m)
	if env == nil {
		return false
	}
	if o.out.Send(env) {
		return true
	}
	o.out.Kick(pb.KickReason_KICK_REASON_SLOW_CONSUMER)
	return false
}

// partyEnvelope is the envelope of a party push, or nil for an unknown one.
// It only reads m: one State is shared by every member it is sent to.
func partyEnvelope(m party.Msg) *pb.Envelope {
	switch m := m.(type) {
	case *party.Invite:
		return &pb.Envelope{Payload: &pb.Envelope_PartyInvite{PartyInvite: &pb.PartyInvite{
			FromId: m.FromID, FromName: m.FromName, FromLevel: int32(m.FromLevel), FromClass: m.FromClass, ExpiresMs: m.ExpiresMs,
		}}}
	case *party.State:
		st := &pb.PartyState{LeaderId: m.LeaderID, Members: make([]*pb.PartyMember, len(m.Members))}
		for i, p := range m.Members {
			st.Members[i] = &pb.PartyMember{Id: p.ID, Name: p.Name, CharClass: p.Class, Level: int32(p.Level),
				MapId: p.MapID, Zone: int32(p.Zone)}
		}
		return &pb.Envelope{Payload: &pb.Envelope_PartyState{PartyState: st}}
	case *party.Notice:
		return &pb.Envelope{Payload: &pb.Envelope_PartyNotice{PartyNotice: &pb.PartyNotice{
			Kind: pb.PartyNoticeKind(m.Kind), Name: m.Name,
		}}}
	}
	return nil
}
