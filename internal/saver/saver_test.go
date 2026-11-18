package saver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"demonslayer2d/internal/store"
)

type fakeWriter struct {
	mu     sync.Mutex
	writes []store.CharState
	fail   int // fail this many writes first
}

func (f *fakeWriter) SaveCharacterState(_ context.Context, st store.CharState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail > 0 {
		f.fail--
		return errors.New("db down")
	}
	f.writes = append(f.writes, st)
	return nil
}

func (f *fakeWriter) snapshot() []store.CharState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.CharState(nil), f.writes...)
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitErr(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for save result")
		return nil
	}
}

func TestSaveNotifiesWaiter(t *testing.T) {
	w := &fakeWriter{}
	s := New(w, quiet())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	done := make(chan error, 1)
	s.Save(store.CharState{ID: 1, X: 2}, done)
	if err := waitErr(t, done); err != nil {
		t.Fatal(err)
	}
	if got := w.snapshot(); len(got) != 1 || got[0].X != 2 {
		t.Fatalf("writes = %+v", got)
	}
}

func TestCoalescesPendingSaves(t *testing.T) {
	w := &fakeWriter{}
	s := New(w, quiet())
	d1, d2 := make(chan error, 1), make(chan error, 1)
	s.Save(store.CharState{ID: 7, X: 1}, d1)
	s.Save(store.CharState{ID: 7, X: 9}, d2) // queued before Run starts
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	if waitErr(t, d1) != nil || waitErr(t, d2) != nil {
		t.Fatal("waiters should both succeed")
	}
	got := w.snapshot()
	if len(got) != 1 || got[0].X != 9 {
		t.Fatalf("want one write with the latest state, got %+v", got)
	}
}

func TestRetriesAfterFailure(t *testing.T) {
	w := &fakeWriter{fail: 2}
	s := New(w, quiet())
	s.MinBackoff, s.MaxBackoff = time.Millisecond, 4*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	done := make(chan error, 1)
	s.Save(store.CharState{ID: 3}, done)
	if err := waitErr(t, done); err != nil {
		t.Fatal(err)
	}
	if n := len(w.snapshot()); n != 1 {
		t.Fatalf("writes = %d, want 1", n)
	}
}

func TestStopFlushesPending(t *testing.T) {
	w := &fakeWriter{}
	s := New(w, quiet())
	for i := int64(1); i <= 3; i++ {
		s.Save(store.CharState{ID: i}, nil)
	}
	s.Stop()
	go s.Run(context.Background())
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after Stop")
	}
	if n := len(w.snapshot()); n != 3 {
		t.Fatalf("writes = %d, want 3", n)
	}
}

func TestCancelFailsWaiters(t *testing.T) {
	w := &fakeWriter{fail: 1 << 30}
	s := New(w, quiet())
	s.MinBackoff, s.MaxBackoff = time.Hour, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)

	done := make(chan error, 1)
	s.Save(store.CharState{ID: 1}, done)
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := waitErr(t, done); err == nil {
		t.Fatal("want an error when the saver is cancelled before writing")
	}
}
