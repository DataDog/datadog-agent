package ratelimiter

import (
	"math"

	"golang.org/x/time/rate"
)

// SingleRateLimiter is a wrapper on top of golang.org/x/time/rate which implements a rate limiter but also
// returns the effective rate of allowance.
type SingleRateLimiter struct {
	rate             float64
	limiter          *rate.Limiter
	droppedEvents    int64
	successfulEvents int64
}

type MultiProbeRateLimiter struct {
	defaultRate float64
	x           map[string]*SingleRateLimiter
}

func NewMultiProbeRateLimiter(defaultRatePerSecond float64) *MultiProbeRateLimiter {
	return &MultiProbeRateLimiter{
		defaultRate: defaultRatePerSecond,
		x:           map[string]*SingleRateLimiter{},
	}
}

// SetRate sets the rate for events with a specific ID. Specify mps=0 to
// disable rate limiting.
func (mr *MultiProbeRateLimiter) SetRate(id string, mps float64) {
	mr.x[id] = NewSingleEventRateLimiter(mps)
}

func (mr *MultiProbeRateLimiter) AllowOneEvent(id string) (bool, int64, int64) {
	rateLimiter, ok := mr.x[id]
	if !ok {
		mr.SetRate(id, mr.defaultRate)
		rateLimiter = mr.x[id]
	}
	return rateLimiter.AllowOneEvent(),
		rateLimiter.droppedEvents, rateLimiter.successfulEvents
}

// NewSingleEventRateLimiter returns a rate limiter which restricts the number of single events sampled per second.
// This defaults to infinite, allow all behaviour. The MaxPerSecond value of the rule may override the default.
func NewSingleEventRateLimiter(mps float64) *SingleRateLimiter {
	limit := math.MaxFloat64
	if mps > 0 {
		limit = mps
	}
	return &SingleRateLimiter{
		rate:    mps,
		limiter: rate.NewLimiter(rate.Limit(limit), int(math.Ceil(limit))),
	}
}

// AllowOneEvent returns the rate limiter's decision to allow an event to be processed, and the
// effective rate at the time it is called. The effective rate is computed by averaging the rate
// for the previous second with the current rate
func (r *SingleRateLimiter) AllowOneEvent() bool {

	if r.rate == 0 {
		return true
	}

	var sampled bool = false
	if r.limiter.Allow() {
		sampled = true
		r.successfulEvents++
	} else {
		r.droppedEvents++
	}

	return sampled
}
