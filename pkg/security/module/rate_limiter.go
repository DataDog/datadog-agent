package module

import (
	"sync/atomic"

	"golang.org/x/time/rate"
)

const (
	// Arbitrary default limit to prevent flooding. Might change in the future.
	defaultLimit = rate.Limit(20)
	// We can only reserve one token at a time.
	defaultBurst int = 1
)

type RuleLimiter struct {
	limiter *rate.Limiter
	dropped int64
}

func NewRuleLimiter(limit rate.Limit, burst int) *RuleLimiter {
	return &RuleLimiter{
		limiter: rate.NewLimiter(limit, burst),
	}
}

type RateLimiter struct {
	limiters map[string]*RuleLimiter
}

// NewRateLimiter - Initializes an empty rate limiter
func NewRateLimiter(ids []string) *RateLimiter {
	limiters := make(map[string]*RuleLimiter)
	for _, id := range ids {
		limiters[id] = NewRuleLimiter(defaultLimit, defaultBurst)
	}
	return &RateLimiter{
		limiters: limiters,
	}
}

// Allow - Returns true if a specific rule shall be allowed to sent a new event
func (rl *RateLimiter) Allow(ruleID string) bool {
	ruleLimiter, ok := rl.limiters[ruleID]
	if !ok {
		return false
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
		stats[ruleID] = atomic.SwapInt64(&ruleLimiter.dropped, 0)
	}
	return stats
}
