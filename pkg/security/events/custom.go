// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	// ServiceName is the service tag of the custom event types defined in this package
	ServiceName = "runtime-security-agent"

	// LostEventsRuleID is the rule ID for the lost_events_* events
	LostEventsRuleID = "lost_events"
	//LostEventsRuleDesc is the rule description for the lost_events_* events
	LostEventsRuleDesc = "Lost events"

	// RulesetLoadedRuleID is the rule ID for the ruleset_loaded events
	RulesetLoadedRuleID = "ruleset_loaded"
	// RulesetLoadedRuleDesc is the rule description for the ruleset_loaded events
	RulesetLoadedRuleDesc = "New ruleset loaded"

	// HeartbeatRuleID is the rule ID for the heartbeat events
	HeartbeatRuleID = "heartbeat"
	// HeartbeatRuleDesc is the rule description for the heartbeat events
	HeartbeatRuleDesc = "Heartbeat"

	// AbnormalPathRuleID is the rule ID for the abnormal_path events
	AbnormalPathRuleID = "abnormal_path"
	// AbnormalPathRuleDesc is the rule description for the abnormal_path events
	AbnormalPathRuleDesc = "Abnormal path detected"

	// SelfTestRuleID is the rule ID for the self_test events
	SelfTestRuleID = "self_test"
	// SelfTestRuleDesc is the rule description for the self_test events
	SelfTestRuleDesc = "Self tests result"

	// AnomalyDetectionRuleID is the rule ID for anomaly_detection events
	AnomalyDetectionRuleID = "anomaly_detection"
	// AnomalyDetectionRuleDesc is the rule description for anomaly_detection events
	AnomalyDetectionRuleDesc = "Anomaly detection"

	// NoProcessContextErrorRuleID is the rule ID for events without process context
	NoProcessContextErrorRuleID = "no_process_context"
	// NoProcessContextErrorRuleDesc is the rule description for events without process context
	NoProcessContextErrorRuleDesc = "No process context detected"

	// BrokenProcessLineageErrorRuleID is the rule ID for events with a broken process lineage
	BrokenProcessLineageErrorRuleID = "broken_process_lineage"
	// BrokenProcessLineageErrorRuleDesc is the rule description for events with a broken process lineage
	BrokenProcessLineageErrorRuleDesc = "Broken process lineage detected"

	// RefreshUserCacheRuleID is the rule ID used to refresh users and groups cache
	RefreshUserCacheRuleID = "refresh_user_cache"
)

// CustomEventCommonFields represents the fields common to all custom events
type CustomEventCommonFields struct {
	Timestamp time.Time `json:"date"`
	Service   string    `json:"service"`
}

// FillCustomEventCommonFields fills the common fields with default values
func (commonFields *CustomEventCommonFields) FillCustomEventCommonFields() {
	commonFields.Service = ServiceName
	commonFields.Timestamp = time.Now()
}

// NewCustomRule returns a new custom rule
func NewCustomRule(id eval.RuleID, description string) *rules.Rule {
	return &rules.Rule{
		Rule:       &eval.Rule{ID: id},
		Definition: &rules.RuleDefinition{ID: id, Description: description},
	}
}

// AllCustomRuleIDs returns the list of custom rule IDs
func AllCustomRuleIDs() []string {
	return []string{
		LostEventsRuleID,
		RulesetLoadedRuleID,
		AbnormalPathRuleID,
		SelfTestRuleID,
		AnomalyDetectionRuleID,
		NoProcessContextErrorRuleID,
		BrokenProcessLineageErrorRuleID,
	}
}

// NewCustomEventLazy returns a new custom event
func NewCustomEventLazy(eventType model.EventType, marshalerCtor func() EventMarshaler, tags ...string) *CustomEvent {
	return &CustomEvent{
		eventType:     eventType,
		marshalerCtor: marshalerCtor,
		tags:          tags,
	}
}

// NewCustomEvent returns a new custom event
func NewCustomEvent(eventType model.EventType, marshaler EventMarshaler, tags ...string) *CustomEvent {
	return NewCustomEventLazy(eventType, func() EventMarshaler {
		return marshaler
	}, tags...)
}

// CustomEvent is used to send custom security events to Datadog
type CustomEvent struct {
	eventType     model.EventType
	tags          []string
	marshalerCtor func() EventMarshaler
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

// GetActions returns the triggred actions
func (ce *CustomEvent) GetActions() []*model.ActionTriggered {
	return nil
}

// GetWorkloadID returns the workload id
func (ce *CustomEvent) GetWorkloadID() string {
	return ""
}

// GetEventType returns the event type
func (ce *CustomEvent) GetEventType() model.EventType {
	return ce.eventType
}

// MarshalJSON marshals the custom event to JSON using easyJSON
func (ce *CustomEvent) MarshalJSON() ([]byte, error) {
	return ce.marshalerCtor().ToJSON()
}
