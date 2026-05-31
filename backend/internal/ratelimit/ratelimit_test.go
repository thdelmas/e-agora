package ratelimit

import (
	"testing"
	"time"
)

// fixedClock lets tests advance time deterministically.
type fixedClock struct{ t time.Time }

func (c *fixedClock) now() time.Time      { return c.t }
func (c *fixedClock) add(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(burst int, rate float64) (*Limiter, *fixedClock) {
	clk := &fixedClock{t: time.Unix(1_700_000_000, 0)}
	l := New(burst, rate)
	l.now = clk.now
	return l, clk
}

func TestAllow_BurstThenReject(t *testing.T) {
	l, _ := newTestLimiter(3, 1) // burst 3, 1/sec
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("s"); !ok {
			t.Fatalf("burst token %d should be allowed", i)
		}
	}
	ok, retry := l.Allow("s")
	if ok {
		t.Error("4th request should be rejected")
	}
	if retry <= 0 || retry > time.Second {
		t.Errorf("retry-after = %v, want ~1s", retry)
	}
}

func TestAllow_RefillsOverTime(t *testing.T) {
	l, clk := newTestLimiter(1, 2) // 2 tokens/sec
	if ok, _ := l.Allow("s"); !ok {
		t.Fatal("first allowed")
	}
	if ok, _ := l.Allow("s"); ok {
		t.Fatal("second should be rejected (bucket empty)")
	}
	clk.add(600 * time.Millisecond) // +1.2 tokens at 2/sec
	if ok, _ := l.Allow("s"); !ok {
		t.Error("should be allowed after refill")
	}
}

func TestAllow_PerKeyIsolation(t *testing.T) {
	l, _ := newTestLimiter(1, 1)
	l.Allow("a") // exhaust a
	if ok, _ := l.Allow("a"); ok {
		t.Error("a should be exhausted")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Error("b must have its own bucket")
	}
}

func TestGC_EvictsIdleBuckets(t *testing.T) {
	l, clk := newTestLimiter(1, 1)
	l.Allow("old")
	clk.add(idleTTL + gcEvery + time.Second)
	l.Allow("new") // triggers GC; "old" is now idle
	if _, exists := l.buckets["old"]; exists {
		t.Error("idle bucket should have been evicted")
	}
	if _, exists := l.buckets["new"]; !exists {
		t.Error("active bucket should remain")
	}
}
