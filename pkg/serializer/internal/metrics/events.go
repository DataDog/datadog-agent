// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package metrics

import (
	"expvar"

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
type Events []*event.Event

// Marshal serialize events using agent-payload definition
func (events Events) Marshal() ([]byte, error) {
	panic("not called")
}

func (events Events) getEventsBySourceType() map[string][]*event.Event {
	panic("not called")
}

// MarshalJSON serializes events to JSON so it can be sent to the Agent 5 intake
// (we don't use the v1 event endpoint because it only supports 1 event per payload)
// FIXME(olivier): to be removed when v2 endpoints are available
func (events Events) MarshalJSON() ([]byte, error) {
	panic("not called")
}

// SplitPayload breaks the payload into times number of pieces
func (events Events) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	panic("not called")
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
	panic("not called")
}

func writeEventsHeader(stream *jsoniter.Stream) {
	panic("not called")
}

func (*eventsBySourceTypeMarshaler) WriteFooter(stream *jsoniter.Stream) error {
	panic("not called")
}

func writeEventsFooter(stream *jsoniter.Stream) error {
	panic("not called")
}

func (e *eventsBySourceTypeMarshaler) WriteItem(stream *jsoniter.Stream, i int) error {
	panic("not called")
}

func (e *eventsBySourceTypeMarshaler) Len() int {
	panic("not called")
}

func (e *eventsBySourceTypeMarshaler) DescribeItem(i int) string {
	panic("not called")
}

func writeEvent(event *event.Event, writer *utiljson.RawObjectWriter) error {
	panic("not called")
}

// CreateSingleMarshaler creates marshaler.StreamJSONMarshaler where each item
// is composed of all events for a specific source type name.
func (events Events) CreateSingleMarshaler() marshaler.StreamJSONMarshaler {
	panic("not called")
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
	panic("not called")
}

func (e *eventsMarshaler) WriteFooter(stream *jsoniter.Stream) error {
	panic("not called")
}

func (e *eventsMarshaler) WriteItem(stream *jsoniter.Stream, i int) error {
	panic("not called")
}

func (e *eventsMarshaler) Len() int {
	panic("not called")
}

func (e *eventsMarshaler) DescribeItem(i int) string {
	panic("not called")
}

// CreateMarshalersBySourceType creates a collection of marshaler.StreamJSONMarshaler.
// Each StreamJSONMarshaler is composed of all events for a specific source type name.
func (events Events) CreateMarshalersBySourceType() []marshaler.StreamJSONMarshaler {
	panic("not called")
}
