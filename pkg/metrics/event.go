// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"sort"

	"github.com/gogo/protobuf/proto"
	jsoniter "github.com/json-iterator/go"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/serializer/jsonstream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// EventPriority represents the priority of an event
type EventPriority string

// Enumeration of the existing event priorities, and their values
const (
	EventPriorityNormal       EventPriority = "normal"
	EventPriorityLow          EventPriority = "low"
	apiKeyJSONField                         = "apiKey"
	eventsJSONField                         = "events"
	internalHostnameJSONField               = "internalHostname"
	defaultSourceType                       = "api"
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
			sourceTypeName = defaultSourceType
		}

		eventsBySourceType[sourceTypeName] = append(eventsBySourceType[sourceTypeName], e)
	}

	hostname, _ := util.GetHostname()
	// Build intake payload containing events and serialize
	data := map[string]interface{}{
		apiKeyJSONField:           "", // legacy field, it isn't actually used by the backend
		eventsJSONField:           eventsBySourceType,
		internalHostnameJSONField: hostname,
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

//// The following methods implement the StreamJSONMarshaler interface
//// for support of the enable_events_stream_payload_serialization option.

// Initialize the data for serialization. Call once before any other methods.
func (events Events) Initialize() error {

	// Sort because events are aggregated by SourceTypeName. See WriteItem.
	sort.Slice(events, func(i, j int) bool { return events[i].SourceTypeName < events[j].SourceTypeName })
	return nil
}

// WriteHeader writes the payload header for this type
func (events Events) WriteHeader(stream *jsoniter.Stream) error {
	stream.WriteObjectStart()
	stream.WriteObjectField(apiKeyJSONField)
	stream.WriteString("")
	stream.WriteMore()

	stream.WriteObjectField(eventsJSONField)
	stream.WriteObjectStart()

	return stream.Flush()
}

// WriteFooter prints the payload footer for this type
func (events Events) WriteFooter(stream *jsoniter.Stream) error {
	// We have at least one event and so we need to close the array
	stream.WriteArrayEnd()
	return events.writeNoEventFooter(stream)
}

// WriteLastFooter writes the last footer. Call once after any other methods.
func (events Events) WriteLastFooter(stream *jsoniter.Stream, itemWrittenCount int) error {
	// If we do not write anything we should not close the JSON array.
	if itemWrittenCount == 0 {
		return events.writeNoEventFooter(stream)
	}

	return events.WriteFooter(stream)
}

func (events Events) writeNoEventFooter(stream *jsoniter.Stream) error {
	stream.WriteObjectEnd()
	stream.WriteMore()

	hostname, _ := util.GetHostname()
	stream.WriteObjectField(internalHostnameJSONField)
	stream.WriteString(hostname)

	stream.WriteObjectEnd()
	return stream.Flush()
}

// WriteItem prints the json representation of an item
func (events Events) WriteItem(stream *jsoniter.Stream, i int, itemIndexInPayload int) error {
	if i < 0 || i > len(events)-1 {
		return errors.New("out of range")
	}

	event := events[i]
	isFirstInPayLoad := itemIndexInPayload == 0
	var startNewSourceType bool

	if isFirstInPayLoad {
		startNewSourceType = true
	} else if i > 0 && events[i].SourceTypeName != events[i-1].SourceTypeName {
		// Close previous source type and open a new one
		stream.WriteArrayEnd()
		stream.WriteMore()
		startNewSourceType = true
	} else {
		// As AddJSONSeparatoraAutomatically returns false, we need the separator between items
		stream.WriteMore()
	}

	if startNewSourceType {
		sourceTypeName := event.SourceTypeName
		if sourceTypeName == "" {
			sourceTypeName = defaultSourceType
		}

		stream.WriteObjectField(sourceTypeName)
		stream.WriteArrayStart()
	}

	if err := writeEvent(event, stream); err != nil {
		return err
	}
	return stream.Flush()
}

// AddJSONSeparatoraAutomatically returns true to add JSON separator automatically between two calls of WriteItem, false otherwise.
func (events Events) AddJSONSeparatoraAutomatically() bool {
	// If AddJSONSeparatoraAutomatically returns true, it leads to this output
	//
	// 	"events": {
	// 		"SourceTypeName1": [  	// First WriteItem
	// 			{ ... } 			// First WriteItem
	//		, 						// ',' added by compressor
	//		],						// Second WriteItem
	//		"SourceTypeName1": [	// Second WriteItem

	return false
}

// Len returns the number of items to marshal
func (events Events) Len() int {
	return len(events)
}

// DescribeItem returns a text description for logs
func (events Events) DescribeItem(i int) string {
	if i < 0 || i > len(events)-1 {
		return "out of range"
	}
	return fmt.Sprintf("Title:%q, Text:%q", events[i].Title, events[i].Text)
}

func writeEvent(event *Event, stream *jsoniter.Stream) error {
	writer := jsonstream.NewJSONRawObjectWriter(stream)
	writer.AddStringField("msg_title", event.Title, jsonstream.AllowEmpty)
	writer.AddStringField("msg_text", event.Text, jsonstream.AllowEmpty)
	writer.AddInt64Field("timestamp", event.Ts)
	writer.AddStringField("priority", string(event.Priority), jsonstream.OmitEmpty)
	writer.AddStringField("host", event.Host, jsonstream.AllowEmpty)

	if len(event.Tags) != 0 {
		writer.StartArrayField("tags")
		for _, tag := range event.Tags {
			writer.AddStringValue(tag)
		}
		writer.FinishArrayField()
	}

	writer.AddStringField("alert_type", string(event.AlertType), jsonstream.OmitEmpty)
	writer.AddStringField("aggregation_key", event.AggregationKey, jsonstream.OmitEmpty)
	writer.AddStringField("source_type_name", event.SourceTypeName, jsonstream.OmitEmpty)
	writer.AddStringField("event_type", event.EventType, jsonstream.OmitEmpty)
	return writer.Close()
}
