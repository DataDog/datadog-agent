// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package metrics

import (
	"slices"
	"sort"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	apiKeyJSONField           = "apiKey"
	eventsJSONField           = "events"
	internalHostnameJSONField = "internalHostname"
	outOfRangeMsg             = "out of range"
)

var (
	tlmEventsSent          = telemetry.NewSimpleCounter("metrics", "events_sent", "number of events successfully serialized")
	tlmEventsDropped       = telemetry.NewSimpleCounter("metrics", "events_dropped", "number of events dropped for any reason")
	tlmEventsPayloads      = telemetry.NewSimpleCounter("metrics", "events_payloads", "number of events payloads produced")
	tlmEventsEmptyPayloads = telemetry.NewSimpleCounter("metrics", "events_payload_empty", "number of empty payloads produced due to too big events")
)

func writeEventsHeader(stream *jsoniter.Stream) {
	stream.WriteObjectStart()
	stream.WriteObjectField(apiKeyJSONField)
	stream.WriteString("")
	stream.WriteMore()

	stream.WriteObjectField(eventsJSONField)
	stream.WriteObjectStart()
}

func writeEventsFooter(stream *jsoniter.Stream, hname string) {
	stream.WriteObjectEnd()
	stream.WriteMore()

	stream.WriteObjectField(internalHostnameJSONField)
	stream.WriteString(hname)

	stream.WriteObjectEnd()
}

var jsonConfig = jsoniter.Config{}.Froze()

// eventsMarshaler2 serializes events to json grouped by source type name. namely:
//
//	{ "events": { "source type": [ { event }, ... ], ... }, ... }
type eventsMarshaler2 struct {
	logger      log.Component
	compression compression.Component

	events []*event.Event
	stream *jsoniter.Stream

	bufferContext  *marshaler.BufferContext
	compressor     *stream.Compressor
	header, footer []byte

	// last source type name successfully added to the compressor
	sourceTypeName string
	// if any events were added to the compressor since starting new payload
	eventsInPayload bool

	maxPayloadSize      int
	maxUncompressedSize int

	payloads []*transaction.BytesPayload
}

// MarshalEvents serializes an array of events into one or more compressed intake payloads.
func MarshalEvents(
	events event.Events,
	hostname string,
	config config.Component,
	logger log.Component,
	compression compression.Component,
) ([]*transaction.BytesPayload, error) {
	m := createMarshaler2(events, hostname, config, logger, compression)
	return m.marshal()
}

func createMarshaler2(
	events event.Events,
	hostname string,
	config config.Component,
	logger log.Component,
	compression compression.Component,
) *eventsMarshaler2 {
	events = slices.Clone(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].SourceTypeName < events[j].SourceTypeName
	})

	stream := jsoniter.NewStream(jsonConfig, nil, 4096)

	writeEventsHeader(stream)
	header := slices.Clone(stream.Buffer())
	stream.Reset(nil)

	stream.WriteArrayEnd()
	writeEventsFooter(stream, hostname)
	footer := slices.Clone(stream.Buffer())
	stream.Reset(nil)

	return &eventsMarshaler2{
		events:      events,
		logger:      logger,
		compression: compression,

		stream: stream,

		bufferContext: marshaler.NewBufferContext(),
		header:        header,
		footer:        footer,

		maxPayloadSize:      config.GetInt("serializer_max_payload_size"),
		maxUncompressedSize: config.GetInt("serializer_max_uncompressed_payload_size"),
	}
}

func (e *eventsMarshaler2) flushPayload() error {
	err := e.closePayload()
	if err != nil {
		return err
	}

	e.compressor, err = stream.NewCompressor(
		e.bufferContext.CompressorInput,
		e.bufferContext.CompressorOutput,
		e.maxPayloadSize,
		e.maxUncompressedSize,
		e.header,
		e.footer,
		nil,
		e.compression,
	)

	return err
}

func (e *eventsMarshaler2) closePayload() error {
	if e.compressor != nil {
		payload, err := e.compressor.Close()
		if err != nil {
			return err
		}

		if e.eventsInPayload {
			tlmEventsPayloads.Inc()
			e.payloads = append(e.payloads, transaction.NewBytesPayload(payload, 0))
		} else {
			tlmEventsEmptyPayloads.Inc()
		}
	}

	e.bufferContext.CompressorInput.Reset()
	e.bufferContext.CompressorOutput.Reset()

	return nil
}

func (e *eventsMarshaler2) writeEvent(ev *event.Event) {
	s := e.stream
	s.WriteObjectStart()
	s.WriteObjectField("msg_title")
	s.WriteString(ev.Title)
	s.WriteMore()
	s.WriteObjectField("msg_text")
	s.WriteString(ev.Text)
	s.WriteMore()
	s.WriteObjectField("timestamp")
	s.WriteInt64(ev.Ts)
	if ev.Priority != "" {
		s.WriteMore()
		s.WriteObjectField("priority")
		s.WriteString(string(ev.Priority))
	}
	s.WriteMore()
	s.WriteObjectField("host")
	s.WriteString(ev.Host)
	if len(ev.Tags) > 0 {
		s.WriteMore()
		s.WriteObjectField("tags")
		s.WriteArrayStart()
		for i, t := range ev.Tags {
			if i > 0 {
				s.WriteMore()
			}
			s.WriteString(t)
		}
		s.WriteArrayEnd()
	}
	if ev.AlertType != "" {
		s.WriteMore()
		s.WriteObjectField("alert_type")
		s.WriteString(string(ev.AlertType))
	}
	if ev.AggregationKey != "" {
		s.WriteMore()
		s.WriteObjectField("aggregation_key")
		s.WriteString(ev.AggregationKey)
	}
	if ev.SourceTypeName != "" {
		s.WriteMore()
		s.WriteObjectField("source_type_name")
		s.WriteString(ev.SourceTypeName)
	}
	if ev.EventType != "" {
		s.WriteMore()
		s.WriteObjectField("event_type")
		s.WriteString(ev.EventType)
	}
	s.WriteObjectEnd()
}

// returns true if item was processed or dropped, false if we need to try again
func (e *eventsMarshaler2) writeItem(ev *event.Event) (bool, error) {
	s := e.stream
	s.Reset(nil)

	if !e.eventsInPayload || e.sourceTypeName != ev.SourceTypeName {
		if e.eventsInPayload {
			s.WriteArrayEnd()
			s.WriteMore()
		}
		if ev.SourceTypeName != "" {
			s.WriteObjectField(ev.SourceTypeName)
		} else {
			s.WriteObjectField("api")
		}
		s.WriteArrayStart()
	} else {
		s.WriteMore()
	}

	e.writeEvent(ev)

	err := e.compressor.AddItem(s.Buffer())
	switch err {
	case nil:
		e.sourceTypeName = ev.SourceTypeName
		e.eventsInPayload = true
		tlmEventsSent.Inc()
		return true, nil
	case stream.ErrPayloadFull:
		if err := e.flushPayload(); err != nil {
			return false, err
		}
		e.eventsInPayload = false
		return false, nil
	default:
		tlmEventsDropped.Inc()
		e.logger.Warnf("Dropping event: title=%q text=%q source_type_name=%q: %v", ev.Title, ev.Text, ev.SourceTypeName, err)
		return true, nil
	}
}

func (e *eventsMarshaler2) marshal() ([]*transaction.BytesPayload, error) {
	if err := e.flushPayload(); err != nil {
		return nil, err
	}
	for _, ev := range e.events {
		for {
			ok, err := e.writeItem(ev)
			if err != nil {
				return nil, err
			}
			if ok {
				break
			}
		}
	}
	if err := e.closePayload(); err != nil {
		return nil, err
	}

	return e.payloads, nil
}
