// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package event provides the event type and its serialization to the DD agent 5 intake format.
package event

import (
	"encoding/json"
	"fmt"

	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

// Priority represents the priority of an event
type Priority string

// Enumeration of the existing event priorities, and their values
const (
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

// GetEventPriorityFromString returns the Priority from its string representation
func GetEventPriorityFromString(val string) (Priority, error) {
	switch val {
	case string(PriorityNormal):
		return PriorityNormal, nil
	case string(PriorityLow):
		return PriorityLow, nil
	default:
		return "", fmt.Errorf("Invalid event priority: '%s'", val)
	}
}

// AlertType represents the alert type of an event
type AlertType string

// Enumeration of the existing event alert types, and their values
const (
	AlertTypeError   AlertType = "error"
	AlertTypeWarning AlertType = "warning"
	AlertTypeInfo    AlertType = "info"
	AlertTypeSuccess AlertType = "success"
)

// GetAlertTypeFromString returns the AlertType from its string representation
func GetAlertTypeFromString(val string) (AlertType, error) {
	switch val {
	case string(AlertTypeError):
		return AlertTypeError, nil
	case string(AlertTypeWarning):
		return AlertTypeWarning, nil
	case string(AlertTypeInfo):
		return AlertTypeInfo, nil
	case string(AlertTypeSuccess):
		return AlertTypeSuccess, nil
	default:
		return AlertTypeInfo, fmt.Errorf("Invalid alert type: '%s'", val)
	}
}

// Event holds an event (w/ serialization to DD agent 5 intake format)
type Event struct {
	Title          string                 `json:"msg_title"`
	Text           string                 `json:"msg_text"`
	Ts             int64                  `json:"timestamp"`
	Priority       Priority               `json:"priority,omitempty"`
	Host           string                 `json:"host"`
	Tags           []string               `json:"tags,omitempty"`
	AlertType      AlertType              `json:"alert_type,omitempty"`
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
