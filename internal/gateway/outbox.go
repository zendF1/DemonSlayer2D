package gateway

import (
	"sync"

	"demonslayer2d/internal/pb"
)

// outbox is a connection's bounded send queue. It implements zone.Outbox.
type outbox struct {
	mu     sync.Mutex
	ch     chan *pb.Envelope
	closed bool
	kick   func(pb.KickReason)
}

func newOutbox(size int, kick func(pb.KickReason)) *outbox {
	return &outbox{ch: make(chan *pb.Envelope, size), kick: kick}
}

// Send queues env without blocking; it returns false when the queue is full or closed.
func (o *outbox) Send(env *pb.Envelope) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return false
	}
	select {
	case o.ch <- env:
		return true
	default:
		return false
	}
}

func (o *outbox) Kick(reason pb.KickReason) { o.kick(reason) }

// closeWith queues final if there is room, then closes the queue. Later calls do nothing.
func (o *outbox) closeWith(final *pb.Envelope) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	if final != nil {
		select {
		case o.ch <- final:
		default:
		}
	}
	o.closed = true
	close(o.ch)
}
