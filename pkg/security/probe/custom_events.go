//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/hashicorp/go-multierror"
	"github.com/mailru/easyjson"
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
	// SelfTestRuleID is the rule ID for the self_test events
	SelfTestRuleID = "self_test"
)

// AllCustomRuleIDs returns the list of custom rule IDs
func AllCustomRuleIDs() []string {
	return []string{
		LostEventsRuleID,
		RulesetLoadedRuleID,
		NoisyProcessRuleID,
		AbnormalPathRuleID,
		SelfTestRuleID,
	}
}

func newCustomEvent(eventType model.EventType, marshaler easyjson.Marshaler) *CustomEvent {
	return &CustomEvent{
		eventType: eventType,
		marshaler: marshaler,
	}
}

// CustomEvent is used to send custom security events to Datadog
type CustomEvent struct {
	eventType model.EventType
	tags      []string
	marshaler easyjson.Marshaler
}

// Clone returns a copy of the current CustomEvent
func (ce *CustomEvent) Clone() CustomEvent {
	return CustomEvent{
		eventType: ce.eventType,
		tags:      ce.tags,
		marshaler: ce.marshaler,
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
	return easyjson.Marshal(ce.marshaler)
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
	Lost      float64   `json:"lost"`
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, lost float64) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: LostEventsRuleID,
		}), newCustomEvent(model.CustomLostReadEventType, EventLostRead{
			Name:      mapName,
			Lost:      lost,
			Timestamp: time.Now(),
		})
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
		})
}

// RuleIgnored defines a ignored
// easyjson:json
type RuleIgnored struct {
	ID         string `json:"id"`
	Version    string `json:"version,omitempty"`
	Expression string `json:"expression"`
	Reason     string `json:"reason"`
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

// RuleLoaded defines a loaded rule
// easyjson:json
type RuleLoaded struct {
	ID         string `json:"id"`
	Version    string `json:"version,omitempty"`
	Expression string `json:"expression"`
}

// PolicyLoaded is used to report policy was loaded
// easyjson:json
type PolicyLoaded struct {
	Version      string
	RulesLoaded  []*RuleLoaded  `json:"rules_loaded"`
	RulesIgnored []*RuleIgnored `json:"rules_ignored,omitempty"`
}

// RulesetLoadedEvent is used to report that a new ruleset was loaded
// easyjson:json
type RulesetLoadedEvent struct {
	Timestamp       time.Time        `json:"date"`
	PoliciesLoaded  []*PolicyLoaded  `json:"policies"`
	PoliciesIgnored *PoliciesIgnored `json:"policies_ignored,omitempty"`
	MacrosLoaded    []rules.MacroID  `json:"macros_loaded"`
}

// NewRuleSetLoadedEvent returns the rule and a populated custom event for a new_rules_loaded event
func NewRuleSetLoadedEvent(rs *rules.RuleSet, err *multierror.Error) (*rules.Rule, *CustomEvent) {
	mp := make(map[string]*PolicyLoaded)

	var policy *PolicyLoaded
	var exists bool

	// rule successfully loaded
	for _, rule := range rs.GetRules() {
		policyName := rule.Definition.Policy.Name

		if policy, exists = mp[policyName]; !exists {
			policy = &PolicyLoaded{Version: rule.Definition.Policy.Version}
			mp[policyName] = policy
		}
		policy.RulesLoaded = append(policy.RulesLoaded, &RuleLoaded{
			ID:         rule.ID,
			Version:    rule.Definition.Version,
			Expression: rule.Definition.Expression,
		})
	}

	// rules ignored due to errors
	if err != nil && err.Errors != nil {
		for _, err := range err.Errors {
			if rerr, ok := err.(*rules.ErrRuleLoad); ok {
				policyName := rerr.Definition.Policy.Name

				if policy, exists = mp[policyName]; !exists {
					policy = &PolicyLoaded{}
					mp[policyName] = policy
				}
				policy.RulesIgnored = append(policy.RulesIgnored, &RuleIgnored{
					ID:         rerr.Definition.ID,
					Version:    rerr.Definition.Version,
					Expression: rerr.Definition.Expression,
					Reason:     rerr.Err.Error(),
				})
			}
		}
	}

	var policies []*PolicyLoaded
	for _, policy := range mp {
		policies = append(policies, policy)
	}

	return newRule(&rules.RuleDefinition{
			ID: RulesetLoadedRuleID,
		}), newCustomEvent(model.CustomRulesetLoadedEventType, RulesetLoadedEvent{
			Timestamp:       time.Now(),
			PoliciesLoaded:  policies,
			PoliciesIgnored: &PoliciesIgnored{Errors: err},
			MacrosLoaded:    rs.ListMacroIDs(),
		})
}

// NoisyProcessEvent is used to report that a noisy process was temporarily discarded
// easyjson:json
type NoisyProcessEvent struct {
	Timestamp      time.Time                 `json:"date"`
	Count          uint64                    `json:"pid_count"`
	Threshold      int64                     `json:"threshold"`
	ControlPeriod  time.Duration             `json:"control_period"`
	DiscardedUntil time.Time                 `json:"discarded_until"`
	ProcessContext *ProcessContextSerializer `json:"process"`
}

// NewNoisyProcessEvent returns the rule and a populated custom event for a noisy_process event
func NewNoisyProcessEvent(count uint64,
	threshold int64,
	controlPeriod time.Duration,
	discardedUntil time.Time,
	pce *model.ProcessCacheEntry,
	resolvers *Resolvers,
	timestamp time.Time) (*rules.Rule, *CustomEvent) {

	processContextSerializer := newProcessContextSerializer(&pce.ProcessContext, nil, resolvers)
	return newRule(&rules.RuleDefinition{
			ID: NoisyProcessRuleID,
		}), newCustomEvent(model.CustomNoisyProcessEventType, NoisyProcessEvent{
			Timestamp:      timestamp,
			Count:          count,
			Threshold:      threshold,
			ControlPeriod:  controlPeriod,
			DiscardedUntil: discardedUntil,
			ProcessContext: processContextSerializer,
		})
}

func resolutionErrorToEventType(err error) model.EventType {
	switch err.(type) {
	case ErrTruncatedParents, ErrTruncatedParentsERPC:
		return model.CustomTruncatedParentsEventType
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
			Event:               NewEventSerializer(event),
			PathResolutionError: pathResolutionError.Error(),
		})
}

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	Timestamp time.Time `json:"date"`
	Success   []string  `json:"succeeded_tests"`
	Fails     []string  `json:"failed_tests"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string) (*rules.Rule, *CustomEvent) {
	return newRule(&rules.RuleDefinition{
			ID: SelfTestRuleID,
		}), newCustomEvent(model.CustomSelfTestEventType, SelfTestEvent{
			Timestamp: time.Now(),
			Success:   success,
			Fails:     fails,
		})
}
