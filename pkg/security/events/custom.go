// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"time"

	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	// ServiceName is the service tag of the custom event types defined in this package
	ServiceName = "runtime-security-agent"

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
	// AnomalyDetectionRuleID is the rule ID for anomaly_detection events
	AnomalyDetectionRuleID = "anomaly_detection"
	// ProcessContextErrorRuleID is the rule ID for events without process context
	ProcessContextErrorRuleID = "no_process_context"
)

type CustomEventCommonFields struct {
	Timestamp time.Time `json:"date"`
	Service   string    `json:"service"`
}

func (commonFields *CustomEventCommonFields) FillCustomEventCommonFields() {
	commonFields.Service = ServiceName
	commonFields.Timestamp = time.Now()
}

// NewCustomRule returns a new custom rule
func NewCustomRule(id eval.RuleID) *rules.Rule {
	return &rules.Rule{
		Rule:       &eval.Rule{ID: id},
		Definition: &rules.RuleDefinition{ID: id},
	}
}

// AllCustomRuleIDs returns the list of custom rule IDs
func AllCustomRuleIDs() []string {
	return []string{
		LostEventsRuleID,
		RulesetLoadedRuleID,
		NoisyProcessRuleID,
		AbnormalPathRuleID,
		SelfTestRuleID,
		AnomalyDetectionRuleID,
		ProcessContextErrorRuleID,
	}
}

// NewCustomEvent returns a new custom event
func NewCustomEventLazy(eventType model.EventType, marshalerCtor func() easyjson.Marshaler) *CustomEvent {
	return &CustomEvent{
		eventType:     eventType,
		marshalerCtor: marshalerCtor,
	}
}

func NewCustomEvent(eventType model.EventType, marshaler easyjson.Marshaler) *CustomEvent {
	return NewCustomEventLazy(eventType, func() easyjson.Marshaler {
		return marshaler
	})
}

// CustomEvent is used to send custom security events to Datadog
type CustomEvent struct {
	eventType     model.EventType
	tags          []string
	marshalerCtor func() easyjson.Marshaler
}

// Clone returns a copy of the current CustomEvent
func (ce *CustomEvent) Clone() CustomEvent {
	return CustomEvent{
		eventType:     ce.eventType,
		tags:          ce.tags,
		marshalerCtor: ce.marshalerCtor,
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

func (ce *CustomEvent) MarshalEasyJSON(w *jwriter.Writer) {
	ce.marshalerCtor().MarshalEasyJSON(w)
}
