// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"strconv"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// EventsAutoSuppressions is a struct to hold the auto suppression stats
type EventsAutoSuppressions struct {
	lock    sync.RWMutex
	enabled bool
	stats   map[string]*atomic.Int64
}

// GetStats returns auto suppressions stats, if enabled
func (s *EventsAutoSuppressions) GetStats() map[string]int64 {
	if !s.enabled {
		return nil
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	stats := make(map[string]int64, len(s.stats))
	for ruleID, stat := range s.stats {
		stats[string(ruleID)] = stat.Swap(0)
	}
	return stats
}

func (s *EventsAutoSuppressions) apply(ruleSet *rules.RuleSet) {
	if !s.enabled {
		return
	}

	newStats := make(map[string]*atomic.Int64)
	for _, rule := range ruleSet.GetRules() {
		if isAllowAutosuppressionRule(rule) {
			newStats[rule.ID] = atomic.NewInt64(0)
		}
	}

	s.lock.Lock()
	s.stats = newStats
	s.lock.Unlock()
}

func (s *EventsAutoSuppressions) inc(ruleID string) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if stat, ok := s.stats[ruleID]; ok {
		stat.Inc()
	}
}

func isAllowAutosuppressionRule(rule *rules.Rule) bool {
	if val, ok := rule.Definition.GetTag("allow_autosuppression"); ok {
		b, err := strconv.ParseBool(val)
		return err == nil && b
	}
	return false
}
