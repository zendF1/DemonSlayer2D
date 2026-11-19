package gateway

import (
	"sync"

	"demonslayer2d/internal/pb"
)

// outbox is a connection's bounded send queue. It implements zone.Outbox.
// The channel has one spare slot beyond size so the final message (Kicked) always fits.
type outbox struct {
	mu     sync.Mutex
	ch     chan *pb.Envelope
	size   int
	closed bool
	kick   func(pb.KickReason)
}

func newOutbox(size int, kick func(pb.KickReason)) *outbox {
	return &outbox{ch: make(chan *pb.Envelope, size+1), size: size, kick: kick}
}

// Send queues env without blocking; it returns false when the queue is full or closed.
func (o *outbox) Send(env *pb.Envelope) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || len(o.ch) >= o.size {
		return false
	}
	o.ch <- env // cannot block: len < size < cap and only senders hold mu
	return true
}

func (o *outbox) Kick(reason pb.KickReason) { o.kick(reason) }

// closeWith queues final (there is always room for it), then closes the queue. Later calls do nothing.
func (o *outbox) closeWith(final *pb.Envelope) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	if final != nil {
		o.ch <- final // the spare slot guarantees this does not block
	}
	o.closed = true
	close(o.ch)
}
