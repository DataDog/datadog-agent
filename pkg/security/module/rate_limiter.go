// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var (
	// Arbitrary default limit to prevent flooding.
	defaultLimit = rate.Limit(10)
	// Default Token bucket size. 40 is meant to handle sudden burst of events while making sure that we prevent
	// flooding.
	defaultBurst int = 40

	defaultPerRuleLimiters = map[eval.RuleID]*Limiter{
		events.RulesetLoadedRuleID:       NewLimiter(rate.Inf, 1), // No limit on ruleset loaded
		events.AbnormalPathRuleID:        NewLimiter(rate.Every(30*time.Second), 1),
		events.ProcessContextErrorRuleID: NewLimiter(rate.Every(30*time.Second), 1),
	}
)

// Limiter describes an object that applies limits on
// the rate of triggering of a rule to ensure we don't overflow
// with too permissive rules
type Limiter struct {
	limiter *rate.Limiter

	dropped *atomic.Uint64
	allowed *atomic.Uint64
}

// NewLimiter returns a new rule limiter
func NewLimiter(limit rate.Limit, burst int) *Limiter {
	return &Limiter{
		limiter: rate.NewLimiter(limit, burst),
		dropped: atomic.NewUint64(0),
		allowed: atomic.NewUint64(0),
	}
}

// RateLimiter describes a set of rule rate limiters
type RateLimiter struct {
	sync.RWMutex
	limiters     map[rules.RuleID]*Limiter
	statsdClient statsd.ClientInterface
}

// NewRateLimiter initializes an empty rate limiter
func NewRateLimiter(client statsd.ClientInterface) *RateLimiter {
	rl := &RateLimiter{
		limiters:     make(map[string]*Limiter),
		statsdClient: client,
	}

	return rl
}

func applyBaseLimitersFromDefault(limiters map[string]*Limiter) {
	for id, limiter := range defaultPerRuleLimiters {
		limiters[id] = limiter
	}
}

// Apply a set of rules
func (rl *RateLimiter) Apply(ruleSet *rules.RuleSet, customRuleIDs []eval.RuleID) {
	rl.Lock()
	defer rl.Unlock()

	newLimiters := make(map[string]*Limiter)

	for _, id := range customRuleIDs {
		newLimiters[id] = NewLimiter(defaultLimit, defaultBurst)
	}

	// override if there is more specific defs
	applyBaseLimitersFromDefault(newLimiters)

	for id, rule := range ruleSet.GetRules() {
		if rule.Definition.Every != 0 {
			newLimiters[id] = NewLimiter(rate.Every(rule.Definition.Every), 1)
		} else {
			newLimiters[id] = NewLimiter(defaultLimit, defaultBurst)
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
		ruleLimiter.allowed.Inc()
		return true
	}
	ruleLimiter.dropped.Inc()
	return false
}

// RateLimiterStat represents the rate limiting statistics
type RateLimiterStat struct {
	dropped uint64
	allowed uint64
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were dropped because of the rate limiter
func (rl *RateLimiter) GetStats() map[rules.RuleID]RateLimiterStat {
	rl.Lock()
	defer rl.Unlock()

	stats := make(map[rules.RuleID]RateLimiterStat)
	for ruleID, ruleLimiter := range rl.limiters {
		stats[ruleID] = RateLimiterStat{
			dropped: ruleLimiter.dropped.Swap(0),
			allowed: ruleLimiter.allowed.Swap(0),
		}
	}
	return stats
}

// SendStats sends statistics about the number of sent and drops events
// for the set of rules
func (rl *RateLimiter) SendStats() error {
	for ruleID, counts := range rl.GetStats() {
		tags := []string{fmt.Sprintf("rule_id:%s", ruleID)}
		if counts.dropped > 0 {
			if err := rl.statsdClient.Count(metrics.MetricRateLimiterDrop, int64(counts.dropped), tags, 1.0); err != nil {
				return err
			}
		}
		if counts.allowed > 0 {
			if err := rl.statsdClient.Count(metrics.MetricRateLimiterAllow, int64(counts.allowed), tags, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}
