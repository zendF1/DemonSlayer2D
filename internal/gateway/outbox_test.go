package gateway

import (
	"testing"

	"demonslayer2d/internal/pb"
)

func TestOutbox(t *testing.T) {
	var kicked []pb.KickReason
	o := newOutbox(2, func(r pb.KickReason) { kicked = append(kicked, r) })
	e := &pb.Envelope{}
	if !o.Send(e) || !o.Send(e) {
		t.Fatal("first two sends should fit")
	}
	if o.Send(e) {
		t.Fatal("third send should report a full queue")
	}
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
	e := &pb.Envelope{}
	if !o.Send(e) || !o.Send(e) || o.Send(e) {
		t.Fatal("queue of 2 should accept exactly 2 envelopes")
	}
	o.closeWith(&pb.Envelope{Payload: &pb.Envelope_Kicked{Kicked: &pb.Kicked{Reason: pb.KickReason_KICK_REASON_SLOW_CONSUMER}}})
	var got []*pb.Envelope
	for env := range o.ch {
		got = append(got, env)
	}
	if len(got) != 3 || got[2].GetKicked().GetReason() != pb.KickReason_KICK_REASON_SLOW_CONSUMER {
		t.Fatalf("a full queue must still deliver the final Kicked message, got %v", got)
	}
}
