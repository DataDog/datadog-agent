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
	secprofModel "github.com/DataDog/datadog-agent/pkg/security/security_profile/model"
)

// booleanTagEquals returns true if the given rule has the given tag set to a boolean and its value matches the given value
func booleanTagEquals(rule *rules.Rule, tag string, value bool) bool {
	if val, ok := rule.Definition.GetTag(tag); ok {
		b, err := strconv.ParseBool(val)
		return err == nil && b == value
	}
	return false
}

func isAllowAutosuppressionRule(rule *rules.Rule) bool {
	return booleanTagEquals(rule, "allow_autosuppression", true)
}

func isWorkloadDriftOnlyRule(rule *rules.Rule) bool {
	return booleanTagEquals(rule, "workload_drift_only", true)
}

const (
	securityProfileSuppressionType = "security_profile"
	activityDumpSuppressionType    = "activity_dump"
	noWorkloadDriftSuppressionType = "no_workload_drift"
)

// Opts holds options for auto suppression
type Opts struct {
	SecurityProfileAutoSuppressionEnabled bool
	ActivityDumpAutoSuppressionEnabled    bool
	EventTypes                            []model.EventType
}

// StatsTags holds tags for auto suppression stats
type StatsTags struct {
	RuleID          string
	SuppressionType string
}

// AutoSuppression is a struct that encapsulates the auto suppression logic
type AutoSuppression struct {
	once      sync.Once
	opts      Opts
	statsLock sync.RWMutex
	stats     map[StatsTags]*atomic.Int64
}

// Init initializes the auto suppression with the given options
func (as *AutoSuppression) Init(opts Opts) {
	as.once.Do(func() {
		as.opts = opts
		as.stats = make(map[StatsTags]*atomic.Int64)
	})
}

// Suppresses returns true if the event should be suppressed for the given rule, false otherwise. It also counts statistics depending on this result
func (as *AutoSuppression) Suppresses(rule *rules.Rule, event *model.Event) bool {
	if isAllowAutosuppressionRule(rule) && slices.Contains(as.opts.EventTypes, event.GetEventType()) {
		if as.opts.SecurityProfileAutoSuppressionEnabled {
			if event.IsInProfile() {
				as.count(rule.ID, securityProfileSuppressionType)
				return true
			} else if isWorkloadDriftOnlyRule(rule) && event.SecurityProfileContext.EventTypeState != secprofModel.StableEventType {
				as.count(rule.ID, noWorkloadDriftSuppressionType)
				return true
			}
		}
		if as.opts.ActivityDumpAutoSuppressionEnabled && event.HasActiveActivityDump() {
			as.count(rule.ID, activityDumpSuppressionType)
			return true
		}
	}
	return false
}

// Apply resets the auto suppression stats based on the given ruleset
func (as *AutoSuppression) Apply(ruleSet *rules.RuleSet) {
	var enabledSuppressionTypes []string
	if as.opts.SecurityProfileAutoSuppressionEnabled {
		enabledSuppressionTypes = append(enabledSuppressionTypes, securityProfileSuppressionType)
		enabledSuppressionTypes = append(enabledSuppressionTypes, noWorkloadDriftSuppressionType)
	}
	if as.opts.ActivityDumpAutoSuppressionEnabled {
		enabledSuppressionTypes = append(enabledSuppressionTypes, activityDumpSuppressionType)
	}

	tags := StatsTags{}
	newStats := make(map[StatsTags]*atomic.Int64)
	for _, rule := range ruleSet.GetRules() {
		if isAllowAutosuppressionRule(rule) {
			tags.RuleID = rule.ID
			for _, suppressionType := range enabledSuppressionTypes {
				tags.SuppressionType = suppressionType
				newStats[tags] = atomic.NewInt64(0)
			}
		}
	}

	as.statsLock.Lock()
	as.stats = newStats
	as.statsLock.Unlock()
}

func (as *AutoSuppression) count(ruleID string, suppressionType string) {
	as.statsLock.RLock()
	defer as.statsLock.RUnlock()

	tags := StatsTags{
		RuleID:          ruleID,
		SuppressionType: suppressionType,
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
