// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package rules

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	// Arbitrary default limit to prevent flooding.
	defaultLimit = rate.Limit(10)
	// Default Token bucket size. 40 is meant to handle sudden burst of events while making sure that we prevent
	// flooding.
	defaultBurst = 40
)

var (
	defaultPerRuleLimiters = map[eval.RuleID]Limiter{
		events.RulesetLoadedRuleID:             NewStdLimiter(rate.Inf, 1), // No limit on ruleset loaded
		events.AbnormalPathRuleID:              NewStdLimiter(rate.Every(30*time.Second), 1),
		events.NoProcessContextErrorRuleID:     NewStdLimiter(rate.Every(30*time.Second), 1),
		events.BrokenProcessLineageErrorRuleID: NewStdLimiter(rate.Every(30*time.Second), 1),
	}
)

// Limiter defines a limiter interface
type Limiter interface {
	Allow(event events.Event) bool
	SwapStats() []utils.LimiterStat
}

// StdLimiter describes an object that applies limits on
// the rate of triggering of a rule to ensure we don't overflow
// with too permissive rules
type StdLimiter struct {
	rateLimiter *rate.Limiter

	// stats
	dropped *atomic.Uint64
	allowed *atomic.Uint64
}

// NewStdLimiter returns a new rule limiter
func NewStdLimiter(limit rate.Limit, burst int) *StdLimiter {
	return &StdLimiter{
		rateLimiter: rate.NewLimiter(limit, burst),
		dropped:     atomic.NewUint64(0),
		allowed:     atomic.NewUint64(0),
	}
}

// Allow returns whether the event is allowed
func (l *StdLimiter) Allow(_ events.Event) bool {
	if l.rateLimiter.Allow() {
		l.allowed.Inc()
		return true
	}
	l.dropped.Inc()

	return false
}

// SwapStats returns the dropped and allowed stats, and zeros the stats
func (l *StdLimiter) SwapStats() []utils.LimiterStat {
	return []utils.LimiterStat{
		{
			Dropped: l.dropped.Swap(0),
			Allowed: l.allowed.Swap(0),
		},
	}
}

// AnomalyDetectionLimiter limiter specific to anomaly detection
type AnomalyDetectionLimiter struct {
	limiter *utils.Limiter[string]
}

// Allow returns whether the event is allowed
func (al *AnomalyDetectionLimiter) Allow(event events.Event) bool {
	return al.limiter.Allow(event.GetWorkloadID())
}

// SwapStats return dropped and allowed stats
func (al *AnomalyDetectionLimiter) SwapStats() []utils.LimiterStat {
	return al.limiter.SwapStats()
}

// NewAnomalyDetectionLimiter returns a new rate limiter which is bucketed by workload ID
func NewAnomalyDetectionLimiter(numWorkloads int, numEventsAllowedPerPeriod int, period time.Duration) (*AnomalyDetectionLimiter, error) {
	limiter, err := utils.NewLimiter[string](numWorkloads, numEventsAllowedPerPeriod, period)
	if err != nil {
		return nil, err
	}

	return &AnomalyDetectionLimiter{
		limiter: limiter,
	}, nil
}

// RateLimiter describes a set of rule rate limiters
type RateLimiter struct {
	sync.RWMutex
	limiters     map[rules.RuleID]Limiter
	statsdClient statsd.ClientInterface
	config       *config.RuntimeSecurityConfig
}

// NewRateLimiter initializes an empty rate limiter
func NewRateLimiter(config *config.RuntimeSecurityConfig, client statsd.ClientInterface) *RateLimiter {
	rl := &RateLimiter{
		limiters:     make(map[string]Limiter),
		statsdClient: client,
		config:       config,
	}

	return rl
}

func (rl *RateLimiter) applyBaseLimitersFromDefault(limiters map[string]Limiter) {
	for id, limiter := range defaultPerRuleLimiters {
		limiters[id] = limiter
	}

	limiter, err := NewAnomalyDetectionLimiter(rl.config.AnomalyDetectionRateLimiterNumKeys, rl.config.AnomalyDetectionRateLimiterNumEventsAllowed, rl.config.AnomalyDetectionRateLimiterPeriod)
	if err != nil {
		// should never happen, fallback to std limiter
		limiters[events.AnomalyDetectionRuleID] = NewStdLimiter(rate.Every(rl.config.AnomalyDetectionRateLimiterPeriod), rl.config.AnomalyDetectionRateLimiterNumEventsAllowed)
	} else {
		limiters[events.AnomalyDetectionRuleID] = limiter
	}
}

// Apply a set of rules
func (rl *RateLimiter) Apply(ruleSet *rules.RuleSet, customRuleIDs []eval.RuleID) {
	rl.Lock()
	defer rl.Unlock()

	newLimiters := make(map[string]Limiter)

	for _, id := range customRuleIDs {
		newLimiters[id] = NewStdLimiter(defaultLimit, defaultBurst)
	}

	// override if there is more specific defs
	rl.applyBaseLimitersFromDefault(newLimiters)

	for id, rule := range ruleSet.GetRules() {
		if rule.Definition.Every != 0 {
			newLimiters[id] = NewStdLimiter(rate.Every(rule.Definition.Every), 1)
		} else {
			newLimiters[id] = NewStdLimiter(defaultLimit, defaultBurst)
		}
	}

	rl.limiters = newLimiters
}

// Allow returns true if a specific rule shall be allowed to sent a new event
func (rl *RateLimiter) Allow(ruleID string, event events.Event) bool {
	rl.RLock()
	defer rl.RUnlock()

	limiter, ok := rl.limiters[ruleID]
	if !ok {
		return false
	}
	return limiter.Allow(event)
}

// GetStats returns a map indexed by ids that describes the amount of events
// that were dropped because of the rate limiter
func (rl *RateLimiter) GetStats() map[string][]utils.LimiterStat {
	rl.Lock()
	defer rl.Unlock()

	stats := make(map[string][]utils.LimiterStat)
	for ruleID, limiter := range rl.limiters {
		stats[ruleID] = limiter.SwapStats()
	}
	return stats
}

// SendStats sends statistics about the number of sent and drops events
// for the set of rules
func (rl *RateLimiter) SendStats() error {
	for ruleID, stats := range rl.GetStats() {
		ruleIDTag := fmt.Sprintf("rule_id:%s", ruleID)
		for _, stat := range stats {
			tags := []string{ruleIDTag}
			if len(stat.Tags) > 0 {
				tags = append(tags, stat.Tags...)
			}

			if stat.Dropped > 0 {
				if err := rl.statsdClient.Count(metrics.MetricRateLimiterDrop, int64(stat.Dropped), tags, 1.0); err != nil {
					return err
				}
			}
			if stat.Allowed > 0 {
				if err := rl.statsdClient.Count(metrics.MetricRateLimiterAllow, int64(stat.Allowed), tags, 1.0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
