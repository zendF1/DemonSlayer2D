package gateway

import (
	"testing"

	"demonslayer2d/internal/pb"
)

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
	for i := 0; i < want; i++ {
		if !o.Send(&pb.Envelope{}) {
			t.Fatalf("send %d of %d was rejected", i+1, want)
		}
	}
	if o.Send(&pb.Envelope{}) {
		t.Fatalf("send %d should report a full queue", want+1)
	}
}
