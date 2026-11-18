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

// Allow reports whether key has fewer than max hits in the window. It does not record a hit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) < l.max
}

// Hit records one attempt for key.
func (l *Limiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.prune(key), l.now())
	l.calls++
	if l.calls%1024 == 0 {
		for k := range l.hits {
			l.prune(k)
		}
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
