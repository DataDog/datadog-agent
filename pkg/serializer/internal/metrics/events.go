// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	utiljson "github.com/DataDog/datadog-agent/pkg/util/json"
)

const (
	apiKeyJSONField           = "apiKey"
	eventsJSONField           = "events"
	internalHostnameJSONField = "internalHostname"
	outOfRangeMsg             = "out of range"
)

var (
	eventExpvar = expvar.NewMap("Event")
	tlmEvent    = telemetry.NewCounter("metrics", "event_split",
		[]string{"action"}, "Events action split")
)

// Events represents a list of events ready to be serialize
type Events struct {
	EventsArr []*event.Event
	Hostname  string
}

func (events Events) getEventsBySourceType() map[string][]*event.Event {
	eventsBySourceType := make(map[string][]*event.Event)
	for _, e := range events.EventsArr {
		sourceTypeName := e.SourceTypeName
		if sourceTypeName == "" {
			sourceTypeName = "api"
		}

		eventsBySourceType[sourceTypeName] = append(eventsBySourceType[sourceTypeName], e)
	}
	return eventsBySourceType
}

// MarshalJSON serializes events to JSON so it can be sent to the Agent 5 intake
// (we don't use the v1 event endpoint because it only supports 1 event per payload)
// FIXME(olivier): to be removed when v2 endpoints are available
func (events Events) MarshalJSON() ([]byte, error) {
	// Regroup events by their source type name
	eventsBySourceType := events.getEventsBySourceType()
	// Build intake payload containing events and serialize
	data := map[string]interface{}{
		apiKeyJSONField:           "", // legacy field, it isn't actually used by the backend
		eventsJSONField:           eventsBySourceType,
		internalHostnameJSONField: events.Hostname,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into times number of pieces
func (events Events) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	eventExpvar.Add("TimesSplit", 1)
	tlmEvent.Inc("times_split")
	// An individual event cannot be split,
	// we can only split up the events

	// only split as much as possible
	if len(events.EventsArr) < times {
		eventExpvar.Add("EventsShorter", 1)
		tlmEvent.Inc("shorter")
		times = len(events.EventsArr)
	}
	splitPayloads := make([]marshaler.AbstractMarshaler, times)

	batchSize := len(events.EventsArr) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// the batchSize won't be perfect, in most cases there will be more or less in the last one than the others
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(events.EventsArr)
		}
		newEvents := events.EventsArr[n:end]
		splitPayloads[i] = Events{
			EventsArr: newEvents,
			Hostname:  events.Hostname,
		}
		n += batchSize
	}
	return splitPayloads, nil
}

// Implements StreamJSONMarshaler.
// Each item in StreamJSONMarshaler is composed of all events for a specific source type name.
type eventsSourceType struct {
	sourceType string
	events     []*event.Event
}

type eventsBySourceTypeMarshaler struct {
	Events             // Required to avoid implementing Marshaler methods
	eventsBySourceType []eventsSourceType
}

func (*eventsBySourceTypeMarshaler) WriteHeader(stream *jsoniter.Stream) error {
	writeEventsHeader(stream)
	return stream.Flush()
}

func writeEventsHeader(stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(apiKeyJSONField)
	stream.WriteString("")
	stream.WriteMore()

	stream.WriteObjectField(eventsJSONField)
	stream.WriteObjectStart()
}

func (e *eventsBySourceTypeMarshaler) WriteFooter(stream *jsoniter.Stream) error {
	return writeEventsFooter(stream, e.Hostname)
}

func writeEventsFooter(stream *jsoniter.Stream, hname string) error {
	stream.WriteObjectEnd()
	stream.WriteMore()

	stream.WriteObjectField(internalHostnameJSONField)
	stream.WriteString(hname)

	stream.WriteObjectEnd()
	return stream.Flush()
}

func (e *eventsBySourceTypeMarshaler) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > len(e.eventsBySourceType)-1 {
		return errors.New(outOfRangeMsg)
	}

	writer := utiljson.NewRawObjectWriter(stream)
	eventSourceType := e.eventsBySourceType[i]
	if err := writer.StartArrayField(eventSourceType.sourceType); err != nil {
		return err
	}
	for _, v := range eventSourceType.events {
		if err := writeEvent(v, writer); err != nil {
			return err
		}
	}
	return writer.FinishArrayField()
}

func (e *eventsBySourceTypeMarshaler) Len() int { return len(e.eventsBySourceType) }

func (e *eventsBySourceTypeMarshaler) DescribeItem(i int) string {
	if i < 0 || i > len(e.eventsBySourceType)-1 {
		return outOfRangeMsg
	}
	return fmt.Sprintf("Source type: %s, events count: %d", e.eventsBySourceType[i].sourceType, len(e.eventsBySourceType[i].events))
}

func writeEvent(event *event.Event, writer *utiljson.RawObjectWriter) error {
	if err := writer.StartObject(); err != nil {
		return err
	}
	writer.AddStringField("msg_title", event.Title, utiljson.AllowEmpty)
	writer.AddStringField("msg_text", event.Text, utiljson.AllowEmpty)
	writer.AddInt64Field("timestamp", event.Ts)
	writer.AddStringField("priority", string(event.Priority), utiljson.OmitEmpty)
	writer.AddStringField("host", event.Host, utiljson.AllowEmpty)

	if len(event.Tags) != 0 {
		if err := writer.StartArrayField("tags"); err != nil {
			return err
		}
		for _, tag := range event.Tags {
			writer.AddStringValue(tag)
		}
		if err := writer.FinishArrayField(); err != nil {
			return err
		}
	}

	writer.AddStringField("alert_type", string(event.AlertType), utiljson.OmitEmpty)
	writer.AddStringField("aggregation_key", event.AggregationKey, utiljson.OmitEmpty)
	writer.AddStringField("source_type_name", event.SourceTypeName, utiljson.OmitEmpty)
	writer.AddStringField("event_type", event.EventType, utiljson.OmitEmpty)
	if err := writer.FinishObject(); err != nil {
		return err
	}
	return writer.Flush()
}

// CreateSingleMarshaler creates marshaler.StreamJSONMarshaler where each item
// is composed of all events for a specific source type name.
func (events Events) CreateSingleMarshaler() marshaler.StreamJSONMarshaler {
	eventsBySourceType := events.getEventsBySourceType()
	values := make([]eventsSourceType, 0, len(eventsBySourceType))
	for sourceType, events := range eventsBySourceType {
		values = append(values, eventsSourceType{sourceType, events})
	}
	return &eventsBySourceTypeMarshaler{events, values}
}

// Implements a *collection* of StreamJSONMarshaler.
// Each collection is composed of all events for a specific source type name.
// Items returned by CreateMarshalerBySourceType can be too big. In this case,
// we use a collection of StreamJSONMarshaler each by source type.
type eventsMarshaler struct {
	sourceTypeName string
	Events
}

func (e *eventsMarshaler) WriteHeader(stream *jsoniter.Stream) error {
	writeEventsHeader(stream)
	stream.WriteObjectField(e.sourceTypeName)
	stream.WriteArrayStart()
	return stream.Flush()
}

func (e *eventsMarshaler) WriteFooter(stream *jsoniter.Stream) error {
	stream.WriteArrayEnd()
	return writeEventsFooter(stream, e.Hostname)
}

func (e *eventsMarshaler) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > len(e.EventsArr)-1 {
		return errors.New(outOfRangeMsg)
	}

	event := e.EventsArr[i]
	writer := utiljson.NewRawObjectWriter(stream)
	if err := writeEvent(event, writer); err != nil {
		return err
	}

	return writer.Flush()
}

func (e *eventsMarshaler) Len() int { return len(e.EventsArr) }

func (e *eventsMarshaler) DescribeItem(i int) string {
	if i < 0 || i > len(e.Events.EventsArr)-1 {
		return outOfRangeMsg
	}
	event := e.EventsArr[i]
	return fmt.Sprintf("Title: %s, Text: %s, Source Type: %s", event.Title, event.Text, event.SourceTypeName)
}

// CreateMarshalersBySourceType creates a collection of marshaler.StreamJSONMarshaler.
// Each StreamJSONMarshaler is composed of all events for a specific source type name.
func (events Events) CreateMarshalersBySourceType() []marshaler.StreamJSONMarshaler {
	e := events.getEventsBySourceType()

	// Make sure we return at least one marshaler to have non-empty JSON.
	if len(e) == 0 {
		return []marshaler.StreamJSONMarshaler{&eventsBySourceTypeMarshaler{events, nil}}
	}

	values := make([]marshaler.StreamJSONMarshaler, 0, len(e))
	for k, v := range e {
		values = append(values, &eventsMarshaler{k, Events{
			EventsArr: v,
			Hostname:  events.Hostname,
		}})
	}
	return values
}
