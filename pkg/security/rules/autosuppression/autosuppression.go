// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autosuppression holds auto suppression related files
package autosuppression

import (
	"slices"
	"strconv"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// isAllowAutosuppressionRule returns true if the given rule allows auto suppression
func isAllowAutosuppressionRule(rule *rules.Rule) bool {
	if val, ok := rule.Definition.GetTag("allow_autosuppression"); ok {
		b, err := strconv.ParseBool(val)
		return err == nil && b
	}
	return false
}

const (
	securityProfileTreeType = "security_profile"
	activityDumpTreeType    = "activity_dump"
)

// Opts holds options for auto suppression
type Opts struct {
	SecurityProfileAutoSuppressionEnabled bool
	ActivityDumpAutoSuppressionEnabled    bool
	EventTypes                            []model.EventType
}

// StatsTags holds tags for auto suppression stats
type StatsTags struct {
	RuleID   string
	TreeType string
}

// AutoSuppression is a struct that encapsulates the auto suppression logic
type AutoSuppression struct {
	once             sync.Once
	opts             Opts
	statsLock        sync.RWMutex
	stats            map[StatsTags]*atomic.Int64
	enabledTreeTypes []string
}

// Init initializes the auto suppression with the given options
func (as *AutoSuppression) Init(opts Opts) {
	as.once.Do(func() {
		as.opts = opts
		as.stats = make(map[StatsTags]*atomic.Int64)
		if opts.SecurityProfileAutoSuppressionEnabled {
			as.enabledTreeTypes = append(as.enabledTreeTypes, securityProfileTreeType)
		}
		if opts.ActivityDumpAutoSuppressionEnabled {
			as.enabledTreeTypes = append(as.enabledTreeTypes, activityDumpTreeType)
		}
	})
}

// Suppresses returns true if the event should be suppressed for the given rule, false otherwise. It also counts statistics depending on this result
func (as *AutoSuppression) Suppresses(rule *rules.Rule, event *model.Event) bool {
	if as.opts.SecurityProfileAutoSuppressionEnabled &&
		event.IsInProfile() &&
		slices.Contains(as.opts.EventTypes, event.GetEventType()) &&
		isAllowAutosuppressionRule(rule) {
		as.count(rule.ID, securityProfileTreeType)
		return true
	} else if as.opts.ActivityDumpAutoSuppressionEnabled &&
		event.HasActiveActivityDump() &&
		slices.Contains(as.opts.EventTypes, event.GetEventType()) &&
		isAllowAutosuppressionRule(rule) {
		as.count(rule.ID, activityDumpTreeType)
		return true
	}
	return false
}

// Apply resets the auto suppression stats based on the given ruleset
func (as *AutoSuppression) Apply(ruleSet *rules.RuleSet) {
	tags := StatsTags{}
	newStats := make(map[StatsTags]*atomic.Int64)
	for _, rule := range ruleSet.GetRules() {
		if isAllowAutosuppressionRule(rule) {
			tags.RuleID = rule.ID
			for _, treeType := range as.enabledTreeTypes {
				tags.TreeType = treeType
				newStats[tags] = atomic.NewInt64(0)
			}
		}
	}

	as.statsLock.Lock()
	as.stats = newStats
	as.statsLock.Unlock()
}

func (as *AutoSuppression) count(ruleID string, treeType string) {
	as.statsLock.RLock()
	defer as.statsLock.RUnlock()

	tags := StatsTags{
		RuleID:   ruleID,
		TreeType: treeType,
	}

	if stat, ok := as.stats[tags]; ok {
		stat.Inc()
	}
}

// GetStats returns the auto suppressions stats
func (as *AutoSuppression) GetStats() map[StatsTags]int64 {
	as.statsLock.RLock()
	defer as.statsLock.RUnlock()

	stats := make(map[StatsTags]int64, len(as.stats))
	for tags, stat := range as.stats {
		stats[tags] = stat.Swap(0)
	}
	return stats
}
