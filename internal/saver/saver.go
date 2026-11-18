// Package saver writes character state to the database off the game loop.
// Saves for the same character are coalesced; failed writes are retried with backoff.
package saver

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"demonslayer2d/internal/store"
)

type Writer interface {
	SaveCharacterState(ctx context.Context, st store.CharState) error
}

type entry struct {
	st      store.CharState
	waiters []chan<- error
}

type Saver struct {
	w   Writer
	log *slog.Logger

	MinBackoff   time.Duration
	MaxBackoff   time.Duration
	WriteTimeout time.Duration

	mu       sync.Mutex
	pending  map[int64]*entry
	wake     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

func New(w Writer, log *slog.Logger) *Saver {
	return &Saver{
		w: w, log: log,
		MinBackoff: time.Second, MaxBackoff: 30 * time.Second, WriteTimeout: 5 * time.Second,
		pending: make(map[int64]*entry),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Save queues st and never blocks. If done is non-nil it receives exactly one value:
// nil once the state (or a newer one) is written, or an error if the saver is cancelled first.
// done must have a buffer of at least 1.
func (s *Saver) Save(st store.CharState, done chan<- error) {
	s.mu.Lock()
	e, ok := s.pending[st.ID]
	if !ok {
		e = &entry{}
		s.pending[st.ID] = e
	}
	e.st = st
	if done != nil {
		e.waiters = append(e.waiters, done)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stop asks Run to write everything pending and return.
func (s *Saver) Stop() { s.stopOnce.Do(func() { close(s.stop) }) }

// Done is closed when Run returns.
func (s *Saver) Done() <-chan struct{} { return s.done }

// Run writes queued states until Stop (after flushing) or ctx is cancelled (pending waiters get ctx.Err()).
func (s *Saver) Run(ctx context.Context) {
	defer close(s.done)
	var backoff time.Duration
	for {
		batch := s.take()
		if len(batch) == 0 {
			select {
			case <-s.wake:
				continue
			case <-s.stop:
				if s.empty() {
					return
				}
				continue
			case <-ctx.Done():
				s.failAll(ctx.Err())
				return
			}
		}
		failed := s.write(ctx, batch)
		if len(failed) == 0 {
			backoff = 0
			continue
		}
		s.requeue(failed)
		if backoff == 0 {
			backoff = s.MinBackoff
		} else {
			backoff = min(2*backoff, s.MaxBackoff)
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			s.failAll(ctx.Err())
			return
		}
	}
}

func (s *Saver) take() map[int64]*entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.pending
	s.pending = make(map[int64]*entry)
	return b
}

func (s *Saver) empty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending) == 0
}

func (s *Saver) write(ctx context.Context, batch map[int64]*entry) []*entry {
	var failed []*entry
	for _, e := range batch {
		wctx, cancel := context.WithTimeout(ctx, s.WriteTimeout)
		err := s.w.SaveCharacterState(wctx, e.st)
		cancel()
		if err != nil {
			s.log.Error("save character failed", "char_id", e.st.ID, "err", err)
			failed = append(failed, e)
			continue
		}
		notify(e.waiters, nil)
	}
	return failed
}

// requeue puts failed entries back. If a newer state was queued meanwhile, it wins and keeps both sets of waiters.
func (s *Saver) requeue(failed []*entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range failed {
		if cur, ok := s.pending[f.st.ID]; ok {
			cur.waiters = append(f.waiters, cur.waiters...)
		} else {
			s.pending[f.st.ID] = f
		}
	}
}

func (s *Saver) failAll(err error) {
	for _, e := range s.take() {
		notify(e.waiters, err)
	}
}

func notify(waiters []chan<- error, err error) {
	for _, w := range waiters {
		select {
		case w <- err:
		default:
		}
	}
}
