// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	// Arbitrary default interval between two events to prevent flooding.
	defaultEvery = 100 * time.Millisecond
	// Default Token bucket size. 40 is meant to handle sudden burst of events while making sure that we prevent
	// flooding.
	defaultBurst = 40

	// maxUniqueToken maximum unique token for the token based rate limiter
	maxUniqueToken = 500
)

var (
	defaultPerRuleLimiters = map[eval.RuleID]rate.Limit{
		RulesetLoadedRuleID:             rate.Inf, // No limit on ruleset loaded
		HeartbeatRuleID:                 rate.Inf, // No limit on heartbeat
		AbnormalPathRuleID:              rate.Every(30 * time.Second),
		NoProcessContextErrorRuleID:     rate.Every(30 * time.Second),
		BrokenProcessLineageErrorRuleID: rate.Every(30 * time.Second),
		EBPFLessHelloMessageRuleID:      rate.Inf, // No limit on hello message
		InternalCoreDumpRuleID:          rate.Every(30 * time.Second),
	}
)

// Limiter defines a limiter interface
type Limiter interface {
	Allow(event Event) bool
	SwapStats() []utils.LimiterStat
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
	for id, rate := range defaultPerRuleLimiters {
		limiters[id] = NewStdLimiter(rate, 1)
	}

	limiter, err := NewAnomalyDetectionLimiter(rl.config.AnomalyDetectionRateLimiterNumKeys, rl.config.AnomalyDetectionRateLimiterNumEventsAllowed, rl.config.AnomalyDetectionRateLimiterPeriod)
	if err != nil {
		// should never happen, fallback to std limiter
		limiters[AnomalyDetectionRuleID] = NewStdLimiter(rate.Every(rl.config.AnomalyDetectionRateLimiterPeriod), rl.config.AnomalyDetectionRateLimiterNumEventsAllowed)
	} else {
		limiters[AnomalyDetectionRuleID] = limiter
	}
}

// Apply a set of rules
func (rl *RateLimiter) Apply(ruleSet *rules.RuleSet, customRuleIDs []eval.RuleID) {
	rl.Lock()
	defer rl.Unlock()

	newLimiters := make(map[string]Limiter)

	for _, id := range customRuleIDs {
		newLimiters[id] = NewStdLimiter(rate.Every(defaultEvery), defaultBurst)
	}

	// override if there is more specific defs
	rl.applyBaseLimitersFromDefault(newLimiters)

	var err error
	for id, rule := range ruleSet.GetRules() {
		every, burst := defaultEvery, defaultBurst

		if rule.Def.Every != 0 {
			every, burst = rule.Def.Every, 1
		}

		if len(rule.Def.RateLimiterToken) > 0 {
			newLimiters[id], err = NewTokenLimiter(maxUniqueToken, burst, every, rule.Def.RateLimiterToken)
			if err != nil {
				seclog.Errorf("unable to use the token based rate limiter, fallback to the standard one: %s", err)
				newLimiters[id] = NewStdLimiter(rate.Every(time.Duration(every)), burst)
			}
		} else {
			newLimiters[id] = NewStdLimiter(rate.Every(time.Duration(every)), burst)
		}
	}

	rl.limiters = newLimiters
}

// Allow returns true if a specific rule shall be allowed to sent a new event
func (rl *RateLimiter) Allow(ruleID string, event Event) bool {
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
