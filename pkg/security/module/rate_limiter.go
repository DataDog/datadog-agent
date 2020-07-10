package module

import (
	"sync/atomic"

	"golang.org/x/time/rate"
)

const (
	defaultLimit = rate.Limit(50)
	defaultBurst int = 1
)

type RuleLimiter struct {
	limiter *rate.Limiter
	dropped int64
}

type RateLimiter struct {
	limiters map[string]*RuleLimiter
}

// NewRateLimiter - Initializes an empty rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*RuleLimiter),
	}
}

// Allow - Returns true if a specific rule shall be allowed to sent a new event
func (rl *RateLimiter) Allow(ruleID string) bool {
	ruleLimiter, ok := rl.limiters[ruleID]
	if !ok {
		// Create a new rate limiter for this rule
		rl.limiters[ruleID] = &RuleLimiter{
			limiter: rate.NewLimiter(defaultLimit, defaultBurst),
		}
		ruleLimiter = rl.limiters[ruleID]
	}
	if ruleLimiter.limiter.Allow() {
		return true
	}
	atomic.AddInt64(&ruleLimiter.dropped, 1)
	return false
}

// GetStats - Returns a map indexed by ruleIDs that describes the amount of events that were dropped because of the rate
// limiter
func (rl *RateLimiter) GetStats() map[string]int64 {
	stats := make(map[string]int64)
	for ruleID, ruleLimiter := range rl.limiters {
		if ruleLimiter.dropped > 0 {
			stats[ruleID] = ruleLimiter.dropped
			// It's ok if we missed an event between the previous line and the next one, as long as we subtract the
			// value read. The missed events will show up on the next call to GetStats
			atomic.AddInt64(&ruleLimiter.dropped, -stats[ruleID])
		}
	}
	return stats
}
