// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package event

import (
	"encoding/json"
	"fmt"

	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

// EventPriority represents the priority of an event
//
//nolint:revive // TODO(AML) Fix revive linter
type EventPriority string

// Enumeration of the existing event priorities, and their values
const (
	EventPriorityNormal EventPriority = "normal"
	EventPriorityLow    EventPriority = "low"
)

// GetEventPriorityFromString returns the EventPriority from its string representation
func GetEventPriorityFromString(val string) (EventPriority, error) {
	switch val {
	case string(EventPriorityNormal):
		return EventPriorityNormal, nil
	case string(EventPriorityLow):
		return EventPriorityLow, nil
	default:
		return "", fmt.Errorf("Invalid event priority: '%s'", val)
	}
}

// EventAlertType represents the alert type of an event
//
//nolint:revive // TODO(AML) Fix revive linter
type EventAlertType string

// Enumeration of the existing event alert types, and their values
const (
	EventAlertTypeError   EventAlertType = "error"
	EventAlertTypeWarning EventAlertType = "warning"
	EventAlertTypeInfo    EventAlertType = "info"
	EventAlertTypeSuccess EventAlertType = "success"
)

// GetAlertTypeFromString returns the EventAlertType from its string representation
func GetAlertTypeFromString(val string) (EventAlertType, error) {
	switch val {
	case string(EventAlertTypeError):
		return EventAlertTypeError, nil
	case string(EventAlertTypeWarning):
		return EventAlertTypeWarning, nil
	case string(EventAlertTypeInfo):
		return EventAlertTypeInfo, nil
	case string(EventAlertTypeSuccess):
		return EventAlertTypeSuccess, nil
	default:
		return EventAlertTypeInfo, fmt.Errorf("Invalid alert type: '%s'", val)
	}
}

// Event holds an event (w/ serialization to DD agent 5 intake format)
type Event struct {
	Title          string                 `json:"msg_title"`
	Text           string                 `json:"msg_text"`
	Ts             int64                  `json:"timestamp"`
	Priority       EventPriority          `json:"priority,omitempty"`
	Host           string                 `json:"host"`
	Tags           []string               `json:"tags,omitempty"`
	AlertType      EventAlertType         `json:"alert_type,omitempty"`
	AggregationKey string                 `json:"aggregation_key,omitempty"`
	SourceTypeName string                 `json:"source_type_name,omitempty"`
	EventType      string                 `json:"event_type,omitempty"`
	OriginInfo     taggertypes.OriginInfo `json:"-"` // OriginInfo is not serialized, it's used for origin detection
}

// Return a JSON string or "" in case of error during the Marshaling
func (e *Event) String() string {
	s, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(s)
}

// Events is a collection of Event.
type Events []*Event
