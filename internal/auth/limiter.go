package auth

import (
	"sync"
	"time"
)

// Limiter counts attempts per key over a sliding window.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
	calls  int
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, now: time.Now, hits: make(map[string][]time.Time)}
}

// Take records one attempt for key if it has fewer than max attempts in the window,
// and reports whether it did. Checking and recording happen under one lock, so
// concurrent callers can never get more than max successful takes.
func (l *Limiter) Take(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	h := l.prune(key)
	if len(h) >= l.max {
		return false
	}
	l.hits[key] = append(h, l.now())
	l.calls++
	if l.calls%1024 == 0 {
		l.sweep()
	}
	return true
}

// Refund gives back the most recent attempt taken for key.
func (l *Limiter) Refund(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	h := l.hits[key]
	if len(h) <= 1 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = h[:len(h)-1]
}

// sweep drops expired attempts for every key. The caller holds mu.
func (l *Limiter) sweep() {
	for k := range l.hits {
		l.prune(k)
	}
}

func (l *Limiter) prune(key string) []time.Time {
	h := l.hits[key]
	cutoff := l.now().Add(-l.window)
	i := 0
	for i < len(h) && !h[i].After(cutoff) {
		i++
	}
	h = h[i:]
	if len(h) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = h
	return h
}
