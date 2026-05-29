package gemini

import (
	"context"
	"sync"
	"time"
)

// RateLimiter throttles outbound API calls to stay under a requests-per-minute ceiling.
// A single limiter is meant to be shared across all clients (flash + pro) since the
// Gemini free tier counts requests globally per project.
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// NewRateLimiter creates a limiter that allows `rpm` requests per minute.
// rpm <= 0 disables limiting.
func NewRateLimiter(rpm int) *RateLimiter {
	if rpm <= 0 {
		return &RateLimiter{}
	}
	return &RateLimiter{interval: time.Minute / time.Duration(rpm)}
}

// Wait blocks until the next request slot is available (or ctx is canceled).
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r.interval == 0 {
		return nil
	}
	r.mu.Lock()
	now := time.Now()
	if r.next.Before(now) {
		r.next = now
	}
	sleep := r.next.Sub(now)
	r.next = r.next.Add(r.interval)
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
