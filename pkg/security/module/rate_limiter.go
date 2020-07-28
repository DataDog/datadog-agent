// +build linux_bpf

package module

import (
	"sync/atomic"

	"github.com/DataDog/datadog-go/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

const (
	// Arbitrary default limit to prevent flooding.
	defaultLimit = rate.Limit(10)
	// Default Token bucket size. 40 is meant to handle sudden burst of events while making sure that we prevent
	// flooding.
	defaultBurst int = 40
)

// RuleLimiter describes an object that applies limits on
// the rate of triggering of a rule to ensure we don't overflow
// with too permissive rules
type RuleLimiter struct {
	limiter *rate.Limiter
	dropped int64
	allowed int64
}

// NewRuleLimiter returns a new rule limiter
func NewRuleLimiter(limit rate.Limit, burst int) *RuleLimiter {
	return &RuleLimiter{
		limiter: rate.NewLimiter(limit, burst),
	}
}

// RateLimiter describes a set of rule rate limiters
type RateLimiter struct {
	limiters map[string]*RuleLimiter
}

// NewRateLimiter initializes an empty rate limiter
func NewRateLimiter(ids []string) *RateLimiter {
	limiters := make(map[string]*RuleLimiter)
	for _, id := range ids {
		limiters[id] = NewRuleLimiter(defaultLimit, defaultBurst)
	}
	return &RateLimiter{
		limiters: limiters,
	}
}

// Allow returns true if a specific rule shall be allowed to sent a new event
func (rl *RateLimiter) Allow(ruleID string) bool {
	ruleLimiter, ok := rl.limiters[ruleID]
	if !ok {
		return false
	}
	if ruleLimiter.limiter.Allow() {
		atomic.AddInt64(&ruleLimiter.allowed, 1)
		return true
	}
	atomic.AddInt64(&ruleLimiter.dropped, 1)
	return false
}

// RateLimiterStat represents the rate limiting statistics
type RateLimiterStat struct {
	dropped int64
	allowed int64
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were dropped because of the rate limiter
func (rl *RateLimiter) GetStats() map[string]RateLimiterStat {
	stats := make(map[string]RateLimiterStat)
	for ruleID, ruleLimiter := range rl.limiters {
		stats[ruleID] = RateLimiterStat{
			dropped: atomic.SwapInt64(&ruleLimiter.dropped, 0),
			allowed: atomic.SwapInt64(&ruleLimiter.allowed, 0),
		}
	}
	return stats
}

// SendStats sends statistics about the number of sent and drops events
// for the set of rules
func (rl *RateLimiter) SendStats(client *statsd.Client) error {
	for ruleID, counts := range rl.GetStats() {
		if err := client.Count(probe.MetricPrefix+".rules."+ruleID+".rate_limiter.drop", counts.dropped, nil, 1.0); err != nil {
			return err
		}
		if err := client.Count(probe.MetricPrefix+".rules."+ruleID+".rate_limiter.allow", counts.allowed, nil, 1.0); err != nil {
			return err
		}
	}
	return nil
}
