// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package module

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-go/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

const (
	// Arbitrary default limit to prevent flooding.
	defaultLimit = rate.Limit(10)
	// Default Token bucket size. 40 is meant to handle sudden burst of events while making sure that we prevent
	// flooding.
	defaultBurst int = 40
)

// Limit defines rate limiter limit
type Limit struct {
	Limit int
	Burst int
}

// LimiterOpts rate limiter options
type LimiterOpts struct {
	Limits map[rules.RuleID]Limit
}

// Limiter describes an object that applies limits on
// the rate of triggering of a rule to ensure we don't overflow
// with too permissive rules
type Limiter struct {
	limiter *rate.Limiter

	// https://github.com/golang/go/issues/36606
	padding int32 //nolint:structcheck,unused
	dropped int64
	allowed int64
}

// NewLimiter returns a new rule limiter
func NewLimiter(limit rate.Limit, burst int) *Limiter {
	return &Limiter{
		limiter: rate.NewLimiter(limit, burst),
	}
}

// RateLimiter describes a set of rule rate limiters
type RateLimiter struct {
	sync.RWMutex
	opts         LimiterOpts
	limiters     map[rules.RuleID]*Limiter
	statsdClient *statsd.Client
}

// NewRateLimiter initializes an empty rate limiter
func NewRateLimiter(client *statsd.Client, opts LimiterOpts) *RateLimiter {
	return &RateLimiter{
		limiters:     make(map[string]*Limiter),
		statsdClient: client,
		opts:         opts,
	}
}

// Apply a set of rules
func (rl *RateLimiter) Apply(rules []rules.RuleID) {
	rl.Lock()
	defer rl.Unlock()

	newLimiters := make(map[string]*Limiter)
	for _, id := range rules {
		if limiter, found := rl.limiters[id]; found {
			newLimiters[id] = limiter
		} else {
			limit := defaultLimit
			burst := defaultBurst

			if l, exists := rl.opts.Limits[id]; exists {
				limit = rate.Limit(l.Limit)
				burst = l.Burst
			}
			newLimiters[id] = NewLimiter(limit, burst)
		}
	}
	rl.limiters = newLimiters
}

// Allow returns true if a specific rule shall be allowed to sent a new event
func (rl *RateLimiter) Allow(ruleID string) bool {
	rl.RLock()
	defer rl.RUnlock()

	ruleLimiter, ok := rl.limiters[ruleID]
	if !ok {
		return false
	}
	if ruleLimiter.limiter.Allow() {
		ruleLimiter.allowed++
		return true
	}
	ruleLimiter.dropped++
	return false
}

// RateLimiterStat represents the rate limiting statistics
type RateLimiterStat struct {
	dropped int64
	allowed int64
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were dropped because of the rate limiter
func (rl *RateLimiter) GetStats() map[rules.RuleID]RateLimiterStat {
	rl.Lock()
	defer rl.Unlock()

	stats := make(map[rules.RuleID]RateLimiterStat)
	for ruleID, ruleLimiter := range rl.limiters {
		stats[ruleID] = RateLimiterStat{
			dropped: ruleLimiter.dropped,
			allowed: ruleLimiter.allowed,
		}
		ruleLimiter.dropped = 0
		ruleLimiter.allowed = 0
	}
	return stats
}

// SendStats sends statistics about the number of sent and drops events
// for the set of rules
func (rl *RateLimiter) SendStats() error {
	for ruleID, counts := range rl.GetStats() {
		tags := []string{fmt.Sprintf("rule_id:%s", ruleID)}
		if counts.dropped > 0 {
			if err := rl.statsdClient.Count(metrics.MetricRateLimiterDrop, counts.dropped, tags, 1.0); err != nil {
				return err
			}
		}
		if counts.allowed > 0 {
			if err := rl.statsdClient.Count(metrics.MetricRateLimiterAllow, counts.allowed, tags, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}
