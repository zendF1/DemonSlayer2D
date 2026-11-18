package auth

import (
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	if !l.Allow("k") {
		t.Fatal("fresh key should be allowed")
	}
	l.Hit("k")
	l.Hit("k")
	if l.Allow("k") {
		t.Fatal("should be blocked after 2 hits")
	}
	if !l.Allow("other") {
		t.Fatal("keys must be independent")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("k") {
		t.Fatal("should be allowed after the window passes")
	}
	if n := len(l.hits); n != 0 {
		t.Fatalf("expired keys should be pruned, %d left", n)
	}
}
