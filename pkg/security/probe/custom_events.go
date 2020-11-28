// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"time"
)

const (
	// LostEventsRuleID is the rule ID for the lost_events_* events
	LostEventsRuleID = "lost_events"
	// RuleSetLoadedRuleID is the rule ID for the ruleset_loaded events
	RuleSetLoadedRuleID = "ruleset_loaded"
)

// AllCustomRuleIDs returns the list of custom rule IDs
func AllCustomRuleIDs() []string {
	return []string{
		LostEventsRuleID,
		RuleSetLoadedRuleID,
	}
}

type CustomEvent struct {
	eventType   string
	tags        []string
	marshalFunc func() ([]byte, error)
}

func (ce *CustomEvent) GetTags() []string {
	return append(ce.tags, "type:"+ce.GetType())
}

func (ce *CustomEvent) GetType() string {
	return ce.eventType
}

func (ce *CustomEvent) MarshalJSON() ([]byte, error) {
	if ce.marshalFunc != nil {
		return ce.marshalFunc()
	}
	return nil, nil
}

func (ce *CustomEvent) String() string {
	d, err := json.Marshal(ce)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, perCPU map[int]int64) (*eval.Rule, *CustomEvent) {
	return &eval.Rule{
			ID: LostEventsRuleID,
		}, &CustomEvent{
			eventType: "lost_events_read",
			marshalFunc: func() ([]byte, error) {
				return json.Marshal(struct {
					Name string        `json:"map"`
					Lost map[int]int64 `json:"per_cpu"`
				}{
					Name: mapName,
					Lost: perCPU,
				})
			},
		}
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]map[int]uint64) (*eval.Rule, *CustomEvent) {
	return &eval.Rule{
			ID: LostEventsRuleID,
		}, &CustomEvent{
			eventType: "lost_events_write",
			marshalFunc: func() ([]byte, error) {
				return json.Marshal(struct {
					Name string                    `json:"map"`
					Lost map[string]map[int]uint64 `json:"per_event_per_cpu"`
				}{
					Name: mapName,
					Lost: perEventPerCPU,
				})
			},
		}
}

// NewRuleSetLoadedEvent returns the rule and a populated custom event for a new_rules_loaded event
func NewRuleSetLoadedEvent(timestamp time.Time, loadedPolicies map[string]string, loadedRules []rules.RuleID, loadedMacros []rules.MacroID) (*eval.Rule, *CustomEvent) {
	return &eval.Rule{
			ID: RuleSetLoadedRuleID,
		}, &CustomEvent{
			eventType: "ruleset_loaded",
			marshalFunc: func() ([]byte, error) {
				return json.Marshal(struct {
					Timestamp time.Time         `json:"timestamp"`
					Policies  map[string]string `json:"policies"`
					Rules     []rules.RuleID    `json:"rules"`
					Macros    []rules.MacroID   `json:"macros"`
				}{
					Timestamp: timestamp,
					Policies:  loadedPolicies,
					Rules:     loadedRules,
					Macros:    loadedMacros,
				})
			},
		}
}
