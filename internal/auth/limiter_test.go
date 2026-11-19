package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	for i := range 2 {
		if !l.Take("k") {
			t.Fatalf("take %d of 2 should succeed", i+1)
		}
	}
	if l.Take("k") {
		t.Fatal("should be blocked after 2 takes")
	}
	if !l.Take("other") {
		t.Fatal("keys must be independent")
	}
	now = now.Add(61 * time.Second)
	if !l.Take("k") {
		t.Fatal("should be allowed after the window passes")
	}
	now = now.Add(61 * time.Second)
	l.sweep()
	if n := len(l.hits); n != 0 {
		t.Fatalf("expired keys should be pruned, %d left", n)
	}
}

func TestLimiterRefund(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	if !l.Take("k") {
		t.Fatal("first take should succeed")
	}
	l.Refund("k")
	if !l.Take("k") {
		t.Fatal("a refunded take must free its slot")
	}
	l.Refund("missing") // refunding an unknown key is a no-op
}

func TestLimiterTakeIsAtomic(t *testing.T) {
	l := NewLimiter(5, time.Minute)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Take("k") {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := ok.Load(); got != 5 {
		t.Fatalf("%d concurrent takes succeeded, want exactly 5", got)
	}
}
