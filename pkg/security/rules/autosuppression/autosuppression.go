// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autosuppression holds auto suppression related files
package autosuppression

import (
	"strconv"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// IsAllowAutosuppressionRule returns true if the given rule allows auto suppression
func IsAllowAutosuppressionRule(rule *rules.Rule) bool {
	if val, ok := rule.Definition.GetTag("allow_autosuppression"); ok {
		b, err := strconv.ParseBool(val)
		return err == nil && b
	}
	return false
}

// AutoSuppression is an interface for auto suppression
type AutoSuppression interface {
	// CanSuppress returns true if the given event can be suppressed
	CanSuppress(event *model.Event) bool
	// Apply resets the auto suppression stats based on the given ruleset
	Apply(ruleSet *rules.RuleSet)
	// Inc increments the auto suppression stats for the given rule ID
	Inc(ruleID string)
	// GetStats returns auto suppressions stats, if enabled
	GetStats() map[string]int64
}

type autoSuppression struct {
	eventTypes map[model.EventType]struct{}
	statsLock  sync.RWMutex
	stats      map[string]*atomic.Int64
}

func (s *autoSuppression) CanSuppress(event *model.Event) bool {
	_, ok := s.eventTypes[event.GetEventType()]
	return ok
}

func (s *autoSuppression) Apply(ruleSet *rules.RuleSet) {
	newStats := make(map[string]*atomic.Int64)
	for _, rule := range ruleSet.GetRules() {
		if IsAllowAutosuppressionRule(rule) {
			newStats[rule.ID] = atomic.NewInt64(0)
		}
	}

	s.statsLock.Lock()
	s.stats = newStats
	s.statsLock.Unlock()
}

func (s *autoSuppression) Inc(ruleID string) {
	s.statsLock.RLock()
	defer s.statsLock.RUnlock()

	if stat, ok := s.stats[ruleID]; ok {
		stat.Inc()
	}
}

func (s *autoSuppression) GetStats() map[string]int64 {
	s.statsLock.RLock()
	defer s.statsLock.RUnlock()

	stats := make(map[string]int64, len(s.stats))
	for ruleID, stat := range s.stats {
		stats[string(ruleID)] = stat.Swap(0)
	}
	return stats
}

// Enable returns an AutoSuppression that can suppress the given event types
func Enable(eventTypes []model.EventType) AutoSuppression {
	as := &autoSuppression{
		eventTypes: make(map[model.EventType]struct{}, len(eventTypes)),
		stats:      make(map[string]*atomic.Int64),
	}

	for _, eventType := range eventTypes {
		as.eventTypes[eventType] = struct{}{}
	}

	return as
}

type noOpAutoSuppression struct{}

func (s *noOpAutoSuppression) CanSuppress(_ *model.Event) bool {
	return false
}

func (s *noOpAutoSuppression) Apply(_ *rules.RuleSet) {}

func (s *noOpAutoSuppression) Inc(_ string) {}

func (s *noOpAutoSuppression) GetStats() map[string]int64 {
	return nil
}

// Disable returns an AutoSuppression that does nothing,
// it is used when auto suppression is disabled
func Disable() AutoSuppression {
	return &noOpAutoSuppression{}
}
