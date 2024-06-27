package eventparser

import (
	"math"

	"golang.org/x/time/rate"
)

// rateLimiter is a wrapper on top of golang.org/x/time/rate which implements a rate limiter but also
// returns the effective rate of allowance.
type rateLimiter struct {
	limiter          *rate.Limiter
	droppedEvents    float64
	successfulEvents float64
}

// newSingleEventRateLimiter returns a rate limiter which restricts the number of single events sampled per second.
// This defaults to infinite, allow all behaviour. The MaxPerSecond value of the rule may override the default.
func newSingleEventRateLimiter(mps float64) *rateLimiter {
	limit := math.MaxFloat64
	if mps > 0 {
		limit = mps
	}
	return &rateLimiter{
		limiter: rate.NewLimiter(rate.Limit(limit), int(math.Ceil(limit))),
	}
}

// allowOneEvent returns the rate limiter's decision to allow an event to be processed, and the
// effective rate at the time it is called. The effective rate is computed by averaging the rate
// for the previous second with the current rate
func (r *rateLimiter) allowOneEvent() bool {

	var sampled bool = false
	if r.limiter.Allow() {
		sampled = true
		r.successfulEvents++
	} else {
		r.droppedEvents++
	}

	return sampled
}
