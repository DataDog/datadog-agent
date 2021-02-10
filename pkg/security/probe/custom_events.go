//go:generate go run github.com/mailru/easyjson/easyjson -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/hashicorp/go-multierror"
)

const (
	// LostEventsRuleID is the rule ID for the lost_events_* events
	LostEventsRuleID = "lost_events"
	// RulesetLoadedRuleID is the rule ID for the ruleset_loaded events
	RulesetLoadedRuleID = "ruleset_loaded"
	// NoisyProcessRuleID is the rule ID for the noisy_process events
	NoisyProcessRuleID = "noisy_process"
	// AbnormalPathRuleID is the rule ID for the abnormal_path events
	AbnormalPathRuleID = "abnormal_path"
)

// AllCustomRuleIDs returns the list of custom rule IDs
func AllCustomRuleIDs() []string {
	return []string{
		LostEventsRuleID,
		RulesetLoadedRuleID,
		NoisyProcessRuleID,
		AbnormalPathRuleID,
	}
}

func newCustomEvent(eventType model.EventType, marshalFunc func() ([]byte, error)) *CustomEvent {
	return &CustomEvent{
		eventType:   eventType,
		marshalFunc: marshalFunc,
	}
}

// CustomEvent is used to send custom security events to Datadog
type CustomEvent struct {
	eventType   model.EventType
	tags        []string
	marshalFunc func() ([]byte, error)
}

// Clone returns a copy of the current CustomEvent
func (ce *CustomEvent) Clone() CustomEvent {
	return CustomEvent{
		eventType:   ce.eventType,
		tags:        ce.tags,
		marshalFunc: ce.marshalFunc,
	}
}

// GetTags returns the tags of the custom event
func (ce *CustomEvent) GetTags() []string {
	return append(ce.tags, "type:"+ce.GetType())
}

// GetType returns the type of the custom event as a string
func (ce *CustomEvent) GetType() string {
	return ce.eventType.String()
}

// GetEventType returns the event type
func (ce *CustomEvent) GetEventType() model.EventType {
	return ce.eventType
}

// MarshalJSON is the JSON marshaller function of the custom event
func (ce *CustomEvent) MarshalJSON() ([]byte, error) {
	return ce.marshalFunc()
}

// String returns the string representation of a custom event
func (ce *CustomEvent) String() string {
	d, err := json.Marshal(ce)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

func newRule(ruleDef *rules.RuleDefinition) *rules.Rule {
	return &rules.Rule{
		Rule:       &eval.Rule{ID: ruleDef.ID},
		Definition: ruleDef,
	}
}

// EventLostRead is the event used to report lost events detected from user space
// easyjson:json
type EventLostRead struct {
	Timestamp time.Time `json:"date"`
	Name      string    `json:"map"`
	Lost      int64     `json:"lost"`
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, lost int64) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: LostEventsRuleID,
		}), newCustomEvent(model.CustomLostReadEventType, EventLostRead{
			Name:      mapName,
			Lost:      lost,
			Timestamp: time.Now(),
		}.MarshalJSON)
}

// EventLostWrite is the event used to report lost events detected from kernel space
// easyjson:json
type EventLostWrite struct {
	Timestamp time.Time         `json:"date"`
	Name      string            `json:"map"`
	Lost      map[string]uint64 `json:"per_event"`
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]uint64) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: LostEventsRuleID,
		}), newCustomEvent(model.CustomLostWriteEventType, EventLostWrite{
			Name:      mapName,
			Lost:      perEventPerCPU,
			Timestamp: time.Now(),
		}.MarshalJSON)
}

// RulesIgnored holds the errors
type RulesIgnored struct {
	Errors *multierror.Error
}

// MarshalJSON custom marshaller
func (r *RulesIgnored) MarshalJSON() ([]byte, error) {
	if r.Errors == nil {
		return nil, nil
	}

	var errs []interface{}

	for _, err := range r.Errors.Errors {
		if rerr, ok := err.(*rules.ErrRuleLoad); ok {
			errs = append(errs,
				struct {
					ID     string `json:"id"`
					Reason string `json:"reason"`
				}{
					ID:     rerr.Definition.ID,
					Reason: rerr.Err.Error(),
				})
		}
	}

	return json.Marshal(errs)
}

// UnmarshalJSON empty unmarshaller
func (r *RulesIgnored) UnmarshalJSON(data []byte) error {
	return nil
}

// PoliciesIgnored holds the errors
type PoliciesIgnored struct {
	Errors *multierror.Error
}

// MarshalJSON custom marshaller
func (r *PoliciesIgnored) MarshalJSON() ([]byte, error) {
	if r.Errors == nil {
		return nil, nil
	}

	var errs []interface{}

	for _, err := range r.Errors.Errors {
		if perr, ok := err.(*rules.ErrPolicyLoad); ok {
			errs = append(errs,
				struct {
					Name   string `json:"name"`
					Reason string `json:"reason"`
				}{
					Name:   perr.Name,
					Reason: perr.Err.Error(),
				})
		}
	}

	return json.Marshal(errs)
}

// UnmarshalJSON empty unmarshaller
func (r *PoliciesIgnored) UnmarshalJSON(data []byte) error {
	return nil
}

// RuleSetLoaded holds the rules
type RuleSetLoaded struct {
	Rules map[eval.RuleID]*eval.Rule
}

// MarshalJSON custom marshaller
func (r *RuleSetLoaded) MarshalJSON() ([]byte, error) {
	var loaded []interface{}

	for id, rule := range r.Rules {
		loaded = append(loaded,
			struct {
				ID         string `json:"id"`
				Expression string `json:"expression"`
			}{
				ID:         id,
				Expression: rule.Expression,
			})
	}

	return json.Marshal(loaded)
}

// UnmarshalJSON empty unmarshaller
func (r *RuleSetLoaded) UnmarshalJSON(data []byte) error {
	return nil
}

// RulesetLoadedEvent is used to report that a new ruleset was loaded
// easyjson:json
type RulesetLoadedEvent struct {
	Timestamp       time.Time         `json:"date"`
	Policies        map[string]string `json:"policies"`
	PoliciesIgnored *PoliciesIgnored  `json:"policies_ignored,omitempty"`
	Macros          []rules.MacroID   `json:"macros_loaded"`
	Rules           *RuleSetLoaded    `json:"rules_loaded"`
	RulesIgnored    *RulesIgnored     `json:"rules_ignored,omitempty"`
}

// NewRuleSetLoadedEvent returns the rule and a populated custom event for a new_rules_loaded event
func NewRuleSetLoadedEvent(rs *rules.RuleSet, err *multierror.Error) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: RulesetLoadedRuleID,
		}), newCustomEvent(model.CustomRulesetLoadedEventType, RulesetLoadedEvent{
			Timestamp:       time.Now(),
			Policies:        rs.ListPolicies(),
			PoliciesIgnored: &PoliciesIgnored{Errors: err},
			Rules:           &RuleSetLoaded{Rules: rs.GetRules()},
			Macros:          rs.ListMacroIDs(),
			RulesIgnored:    &RulesIgnored{Errors: err},
		}.MarshalJSON)
}

// NoisyProcessEvent is used to report that a noisy process was temporarily discarded
// easyjson:json
type NoisyProcessEvent struct {
	Timestamp      time.Time                 `json:"date"`
	Event          string                    `json:"event_type"`
	Count          uint64                    `json:"pid_count"`
	Threshold      int64                     `json:"threshold"`
	ControlPeriod  time.Duration             `json:"control_period"`
	DiscardedUntil time.Time                 `json:"discarded_until"`
	Process        *ProcessContextSerializer `json:"process"`
}

// NewNoisyProcessEvent returns the rule and a populated custom event for a noisy_process event
func NewNoisyProcessEvent(eventType model.EventType,
	count uint64,
	threshold int64,
	controlPeriod time.Duration,
	discardedUntil time.Time,
	process *model.ProcessCacheEntry,
	resolvers *Resolvers,
	timestamp time.Time) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: NoisyProcessRuleID,
		}), newCustomEvent(model.CustomNoisyProcessEventType, NoisyProcessEvent{
			Timestamp:      timestamp,
			Event:          eventType.String(),
			Count:          count,
			Threshold:      threshold,
			ControlPeriod:  controlPeriod,
			DiscardedUntil: discardedUntil,
			Process:        newProcessContextSerializer(process, nil, resolvers),
		}.MarshalJSON)
}

func resolutionErrorToEventType(err error) model.EventType {
	switch err.(type) {
	case ErrTruncatedParents:
		return model.CustomTruncatedParentsEventType
	case ErrTruncatedSegment:
		return model.CustomTruncatedSegmentEventType
	default:
		return model.UnknownEventType
	}
}

// AbnormalPathEvent is used to report that a path resolution failed for a suspicious reason
// easyjson:json
type AbnormalPathEvent struct {
	Timestamp           time.Time        `json:"date"`
	Event               *EventSerializer `json:"triggering_event"`
	PathResolutionError string           `json:"path_resolution_error"`
}

// NewAbnormalPathEvent returns the rule and a populated custom event for a abnormal_path event
func NewAbnormalPathEvent(event *Event, pathResolutionError error) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: AbnormalPathRuleID,
		}), newCustomEvent(resolutionErrorToEventType(event.GetPathResolutionError()), AbnormalPathEvent{
			Timestamp:           event.ResolveEventTimestamp(),
			Event:               newEventSerializer(event),
			PathResolutionError: pathResolutionError.Error(),
		}.MarshalJSON)
}
