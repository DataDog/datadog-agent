// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"bytes"
	"expvar"
	"fmt"

	json "github.com/json-iterator/go"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// EventPriority represents the priority of an event
type EventPriority string

// Enumeration of the existing event priorities, and their values
const (
	EventPriorityNormal EventPriority = "normal"
	EventPriorityLow    EventPriority = "low"
)

var eventExpvar = expvar.NewMap("Event")

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
	Title          string         `json:"msg_title"`
	Text           string         `json:"msg_text"`
	Ts             int64          `json:"timestamp"`
	Priority       EventPriority  `json:"priority,omitempty"`
	Host           string         `json:"host"`
	Tags           []string       `json:"tags,omitempty"`
	AlertType      EventAlertType `json:"alert_type,omitempty"`
	AggregationKey string         `json:"aggregation_key,omitempty"`
	SourceTypeName string         `json:"source_type_name,omitempty"`
	EventType      string         `json:"event_type,omitempty"`
}

// Return a JSON string or "" in case of error during the Marshaling
func (e *Event) String() string {
	s, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(s)
}

// Events represents a list of events ready to be serialize
type Events []*Event

// Marshal serialize events using agent-payload definition
func (events Events) Marshal() ([]byte, error) {
	payload := &agentpayload.EventsPayload{
		Events:   []*agentpayload.EventsPayload_Event{},
		Metadata: &agentpayload.CommonMetadata{},
	}

	for _, e := range events {
		payload.Events = append(payload.Events,
			&agentpayload.EventsPayload_Event{
				Title:          e.Title,
				Text:           e.Text,
				Ts:             e.Ts,
				Priority:       string(e.Priority),
				Host:           e.Host,
				Tags:           e.Tags,
				AlertType:      string(e.AlertType),
				AggregationKey: e.AggregationKey,
				SourceTypeName: e.SourceTypeName,
			})
	}

	return proto.Marshal(payload)
}

// MarshalJSON serializes events to JSON so it can be sent to the Agent 5 intake
// (we don't use the v1 event endpoint because it only supports 1 event per payload)
//FIXME(olivier): to be removed when v2 endpoints are available
func (events Events) MarshalJSON() ([]byte, error) {
	// Regroup events by their source type name
	eventsBySourceType := make(map[string][]*Event)
	for _, e := range events {
		sourceTypeName := e.SourceTypeName
		if sourceTypeName == "" {
			sourceTypeName = "api"
		}

		eventsBySourceType[sourceTypeName] = append(eventsBySourceType[sourceTypeName], e)
	}

	hostname, _ := util.GetHostname()
	// Build intake payload containing events and serialize
	data := map[string]interface{}{
		"apiKey":           "", // legacy field, it isn't actually used by the backend
		"events":           eventsBySourceType,
		"internalHostname": hostname,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into times number of pieces
func (events Events) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	eventExpvar.Add("TimesSplit", 1)
	// An individual event cannot be split,
	// we can only split up the events

	// only split as much as possible
	if len(events) < times {
		eventExpvar.Add("EventsShorter", 1)
		times = len(events)
	}
	splitPayloads := make([]marshaler.Marshaler, times)

	batchSize := len(events) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// the batchSize won't be perfect, in most cases there will be more or less in the last one than the others
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(events)
		}
		newEvents := Events(events[n:end])
		splitPayloads[i] = newEvents
		n += batchSize
	}
	return splitPayloads, nil
}
