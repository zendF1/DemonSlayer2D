package zone

import (
	"math"
	"slices"
	"sync"
	"time"
)

// TickStats summarises tick durations since the previous TakeStats call.
type TickStats struct {
	Ticks int
	Max   time.Duration
	P99   time.Duration
}

type tickStats struct {
	mu      sync.Mutex
	samples []time.Duration
}

func (s *tickStats) record(d time.Duration) {
	s.mu.Lock()
	s.samples = append(s.samples, d)
	s.mu.Unlock()
}

func (s *tickStats) take() TickStats {
	s.mu.Lock()
	xs := s.samples
	s.samples = nil
	s.mu.Unlock()
	if len(xs) == 0 {
		return TickStats{}
	}
	slices.Sort(xs)
	i := int(math.Ceil(0.99*float64(len(xs)))) - 1
	return TickStats{Ticks: len(xs), Max: xs[len(xs)-1], P99: xs[i]}
}
