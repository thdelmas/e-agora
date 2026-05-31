package ratelimit

import (
	"sync"
	"time"
)

const (
	idleTTL = 10 * time.Minute // evict buckets unused this long
	gcEvery = time.Minute      // sweep cadence
)

// Limiter is a per-key token-bucket rate limiter (R11). A key is typically an
// anonymous session id. Buckets live in memory and are swept when idle; for
// multi-instance deployments swap this for a shared store (same Allow contract).
// Safe for concurrent use.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	burst   float64
	rate    float64          // tokens refilled per second
	now     func() time.Time // injectable clock (tests)
	lastGC  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New builds a limiter allowing bursts up to `burst` and a sustained `ratePerSec`.
func New(burst int, ratePerSec float64) *Limiter {
	return &Limiter{
		buckets: make(map[string]*bucket),
		burst:   float64(burst),
		rate:    ratePerSec,
		now:     time.Now,
	}
}

// Allow consumes one token for key. It returns whether the request is permitted
// and, when not, how long until a token is available (for Retry-After).
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	} else {
		b.tokens = minf(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
		b.last = now
	}

	l.maybeGC(now)

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	retry := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, retry
}

// maybeGC evicts idle buckets at most once per gcEvery. Caller holds the lock.
func (l *Limiter) maybeGC(now time.Time) {
	if now.Sub(l.lastGC) < gcEvery {
		return
	}
	for k, b := range l.buckets {
		if now.Sub(b.last) > idleTTL {
			delete(l.buckets, k)
		}
	}
	l.lastGC = now
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
