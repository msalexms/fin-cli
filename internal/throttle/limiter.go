// Package throttle provides a context-aware token bucket rate limiter.
package throttle

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// Limiter enforces a maximum rate of operations per minute with a small burst.
type Limiter struct {
	l *rate.Limiter
}

// NewPerMinute builds a Limiter allowing n calls per minute with the given burst.
// If n <= 0 the limiter is effectively disabled (infinite rate).
func NewPerMinute(n int, burst int) *Limiter {
	if n <= 0 {
		return &Limiter{l: rate.NewLimiter(rate.Inf, 0)}
	}
	if burst < 1 {
		burst = 1
	}
	per := rate.Limit(float64(n) / 60.0) // tokens per second
	return &Limiter{l: rate.NewLimiter(per, burst)}
}

// Wait blocks until a token is available or ctx is done.
func (l *Limiter) Wait(ctx context.Context) error {
	return l.l.Wait(ctx)
}

// Reserve returns how long the caller must wait before performing the call.
// Useful for UI feedback. A zero duration means "go now".
func (l *Limiter) Reserve() time.Duration {
	r := l.l.Reserve()
	if !r.OK() {
		return time.Hour
	}
	return r.Delay()
}
