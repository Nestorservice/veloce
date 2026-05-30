package openrouter

import (
	"context"
	"sync"
	"time"
)

// RateLimiter throttles outbound API calls. A single limiter is meant to be
// shared between the Worker (DeepSeek) and Architect (Llama) since OpenRouter
// counts requests globally per account.
//
// Two knobs:
//   - rpm:           hard ceiling per minute (free tier: ~10)
//   - minInterval:   forced pause between consecutive calls (covers bursty
//                    accounts that exceed instantaneous limits even under rpm)
//
// On a 429, callers should invoke Penalize with the server-suggested delay
// so subsequent waiters honor the backoff.
type RateLimiter struct {
	mu          sync.Mutex
	interval    time.Duration // derived from rpm
	minInterval time.Duration // floor between any two calls
	next        time.Time
}

// NewRateLimiter returns a limiter allowing rpm requests per minute, with an
// additional floor of minInterval between any two calls.
//
//	rpm <= 0          : disables rpm cap
//	minInterval == 0  : no extra floor
func NewRateLimiter(rpm int, minInterval time.Duration) *RateLimiter {
	r := &RateLimiter{minInterval: minInterval}
	if rpm > 0 {
		r.interval = time.Minute / time.Duration(rpm)
	}
	return r
}

// Wait blocks until the next request slot is available (or ctx is canceled).
func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	now := time.Now()
	if r.next.Before(now) {
		r.next = now
	}
	sleep := r.next.Sub(now)

	// advance the cursor by the larger of (rpm interval, min floor)
	step := r.interval
	if r.minInterval > step {
		step = r.minInterval
	}
	r.next = r.next.Add(step)
	r.mu.Unlock()

	if sleep <= 0 {
		return nil
	}
	select {
	case <-time.After(sleep):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Penalize pushes the next allowed slot forward by d (after a 429).
func (r *RateLimiter) Penalize(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := time.Now().Add(d)
	if t.After(r.next) {
		r.next = t
	}
}
