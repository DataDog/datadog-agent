// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	protobufContentType                         = "application/x-protobuf"
	jsonContentType                             = "application/json"
	payloadVersionHTTPHeader                    = "DD-Agent-Payload"
	maxItemCountForCreateMarshalersBySourceType = 100
)

var (
	// AgentPayloadVersion is the versions of the agent-payload repository
	// used to serialize to protobuf
	AgentPayloadVersion string

	jsonExtraHeaders                    http.Header
	protobufExtraHeaders                http.Header
	jsonExtraHeadersWithCompression     http.Header
	protobufExtraHeadersWithCompression http.Header

	expvars                                 = expvar.NewMap("serializer")
	expvarsSendEventsErrItemTooBigs         = expvar.Int{}
	expvarsSendEventsErrItemTooBigsFallback = expvar.Int{}
)

func init() {
	expvars.Set("SendEventsErrItemTooBigs", &expvarsSendEventsErrItemTooBigs)
	expvars.Set("SendEventsErrItemTooBigsFallback", &expvarsSendEventsErrItemTooBigsFallback)
	initExtraHeaders()
}

// initExtraHeaders initializes the global extraHeaders variables.
// Not part of the `init` function body to ease testing
func initExtraHeaders() {
	jsonExtraHeaders = make(http.Header)
	jsonExtraHeaders.Set("Content-Type", jsonContentType)

	jsonExtraHeadersWithCompression = make(http.Header)
	for k := range jsonExtraHeaders {
		jsonExtraHeadersWithCompression.Set(k, jsonExtraHeaders.Get(k))
	}

	protobufExtraHeaders = make(http.Header)
	protobufExtraHeaders.Set("Content-Type", protobufContentType)
	protobufExtraHeaders.Set(payloadVersionHTTPHeader, AgentPayloadVersion)

	protobufExtraHeadersWithCompression = make(http.Header)
	for k := range protobufExtraHeaders {
		protobufExtraHeadersWithCompression.Set(k, protobufExtraHeaders.Get(k))
	}

	if compression.ContentEncoding != "" {
		jsonExtraHeadersWithCompression.Set("Content-Encoding", compression.ContentEncoding)
		protobufExtraHeadersWithCompression.Set("Content-Encoding", compression.ContentEncoding)
	}
}

// EventsStreamJSONMarshaler handles two serialization logics.
type EventsStreamJSONMarshaler interface {
	marshaler.Marshaler

	// Create a single marshaler.
	CreateSingleMarshaler() marshaler.StreamJSONMarshaler

	// If the single marshaler cannot serialize, use smaller marshalers.
	CreateMarshalersBySourceType() []marshaler.StreamJSONMarshaler
}

// MetricSerializer represents the interface of method needed by the aggregator to serialize its data
type MetricSerializer interface {
	SendEvents(e EventsStreamJSONMarshaler) error
	SendServiceChecks(sc marshaler.StreamJSONMarshaler) error
	SendSeries(series marshaler.StreamJSONMarshaler) error
	SendSketch(sketches marshaler.Marshaler) error
	SendMetadata(m marshaler.JSONMarshaler) error
	SendHostMetadata(m marshaler.JSONMarshaler) error
	SendProcessesMetadata(data interface{}) error
	SendOrchestratorMetadata(msgs []ProcessMessageBody, hostName, clusterID string, payloadType int) error
}

// Serializer serializes metrics to the correct format and routes the payloads to the correct endpoint in the Forwarder
type Serializer struct {
	Forwarder             forwarder.Forwarder
	orchestratorForwarder forwarder.Forwarder

	seriesJSONPayloadBuilder *stream.JSONPayloadBuilder

	// Those variables allow users to blacklist any kind of payload
	// from being sent by the agent. This was introduced for
	// environment where, for example, events or serviceChecks
	// might collect data considered too sensitive (database IP and
	// such). By default every kind of payload is enabled since
	// almost every user won't fall into this use case.
	enableEvents                  bool
	enableSeries                  bool
	enableServiceChecks           bool
	enableSketches                bool
	enableJSONToV1Intake          bool
	enableJSONStream              bool
	enableServiceChecksJSONStream bool
	enableEventsJSONStream        bool
	enableSketchProtobufStream    bool
}

// NewSerializer returns a new Serializer initialized
func NewSerializer(forwarder forwarder.Forwarder, orchestratorForwarder forwarder.Forwarder) *Serializer {
	s := &Serializer{
		Forwarder:                     forwarder,
		orchestratorForwarder:         orchestratorForwarder,
		seriesJSONPayloadBuilder:      stream.NewJSONPayloadBuilder(config.Datadog.GetBool("enable_json_stream_shared_compressor_buffers")),
		enableEvents:                  config.Datadog.GetBool("enable_payloads.events"),
		enableSeries:                  config.Datadog.GetBool("enable_payloads.series"),
		enableServiceChecks:           config.Datadog.GetBool("enable_payloads.service_checks"),
		enableSketches:                config.Datadog.GetBool("enable_payloads.sketches"),
		enableJSONToV1Intake:          config.Datadog.GetBool("enable_payloads.json_to_v1_intake"),
		enableJSONStream:              stream.Available && config.Datadog.GetBool("enable_stream_payload_serialization"),
		enableServiceChecksJSONStream: stream.Available && config.Datadog.GetBool("enable_service_checks_stream_payload_serialization"),
		enableEventsJSONStream:        stream.Available && config.Datadog.GetBool("enable_events_stream_payload_serialization"),
		enableSketchProtobufStream:    stream.Available && config.Datadog.GetBool("enable_sketch_stream_payload_serialization"),
	}

	if !s.enableEvents {
		log.Warn("event payloads are disabled: all events will be dropped")
	}
	if !s.enableSeries {
		log.Warn("series payloads are disabled: all series will be dropped")
	}
	if !s.enableServiceChecks {
		log.Warn("service_checks payloads are disabled: all service_checks will be dropped")
	}
	if !s.enableSketches {
		log.Warn("sketches payloads are disabled: all sketches will be dropped")
	}
	if !s.enableJSONToV1Intake {
		log.Warn("JSON to V1 intake is disabled: all payloads to that endpoint will be dropped")
	}

	return s
}

func (s Serializer) serializePayload(payload marshaler.Marshaler, compress bool, useV1API bool) (forwarder.Payloads, http.Header, error) {
	if useV1API {
		return s.serializePayloadJSON(payload, compress)
	}
	return s.serializePayloadProto(payload, compress)
}

func (s Serializer) serializePayloadJSON(payload marshaler.JSONMarshaler, compress bool) (forwarder.Payloads, http.Header, error) {
	var extraHeaders http.Header

	if compress {
		extraHeaders = jsonExtraHeadersWithCompression
	} else {
		extraHeaders = jsonExtraHeaders
	}

	return s.serializePayloadInternal(payload, compress, extraHeaders, split.JSONMarshalFct)
}

func (s Serializer) serializePayloadProto(payload marshaler.ProtoMarshaler, compress bool) (forwarder.Payloads, http.Header, error) {
	var extraHeaders http.Header
	if compress {
		extraHeaders = protobufExtraHeadersWithCompression
	} else {
		extraHeaders = protobufExtraHeaders
	}
	return s.serializePayloadInternal(payload, compress, extraHeaders, split.ProtoMarshalFct)
}

func (s Serializer) serializePayloadInternal(payload marshaler.AbstractMarshaler, compress bool, extraHeaders http.Header, marshalFct split.MarshalFct) (forwarder.Payloads, http.Header, error) {
	payloads, err := split.Payloads(payload, compress, marshalFct)

	if err != nil {
		return nil, nil, fmt.Errorf("could not split payload into small enough chunks: %s", err)
	}

	return payloads, extraHeaders, nil
}

func (s Serializer) serializeStreamablePayload(payload marshaler.StreamJSONMarshaler, policy stream.OnErrItemTooBigPolicy) (forwarder.Payloads, http.Header, error) {
	payloads, err := s.seriesJSONPayloadBuilder.BuildWithOnErrItemTooBigPolicy(payload, policy)
	return payloads, jsonExtraHeadersWithCompression, err
}

// As events are gathered by SourceType, the serialization logic is more complex than for the other serializations.
// We first try to use JSONPayloadBuilder where a single item is the list of all events for the same source type.

// This method may lead to item than can be too big to be serialized. In this case we try the following method.
// If the count of source type is less than maxItemCountForCreateMarshalersBySourceType then we use a
// of JSONPayloadBuilder for each source type where an item is a single event. We limit to maxItemCountForCreateMarshalersBySourceType
// for performance reasons.
//
// If none of the previous methods work, we fallback to the old serialization method (Serializer.serializePayload).
func (s Serializer) serializeEventsStreamJSONMarshalerPayload(
	eventsStreamJSONMarshaler EventsStreamJSONMarshaler, useV1API bool) (forwarder.Payloads, http.Header, error) {
	marshaler := eventsStreamJSONMarshaler.CreateSingleMarshaler()
	eventPayloads, extraHeaders, err := s.serializeStreamablePayload(marshaler, stream.FailOnErrItemTooBig)

	if err == stream.ErrItemTooBig {
		expvarsSendEventsErrItemTooBigs.Add(1)

		// Do not use CreateMarshalersBySourceType when there are too many source types (Performance issue).
		if marshaler.Len() > maxItemCountForCreateMarshalersBySourceType {
			expvarsSendEventsErrItemTooBigsFallback.Add(1)
			eventPayloads, extraHeaders, err = s.serializePayload(eventsStreamJSONMarshaler, true, useV1API)
		} else {
			eventPayloads = nil
			for _, v := range eventsStreamJSONMarshaler.CreateMarshalersBySourceType() {
				var eventPayloadsForSourceType forwarder.Payloads
				eventPayloadsForSourceType, extraHeaders, err = s.serializeStreamablePayload(v, stream.DropItemOnErrItemTooBig)
				if err != nil {
					return nil, nil, err
				}
				eventPayloads = append(eventPayloads, eventPayloadsForSourceType...)
			}
		}
	}
	return eventPayloads, extraHeaders, err
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(e EventsStreamJSONMarshaler) error {
	if !s.enableEvents {
		log.Debug("events payloads are disabled: dropping it")
		return nil
	}

	var eventPayloads forwarder.Payloads
	var extraHeaders http.Header
	var err error

	if s.enableEventsJSONStream {
		eventPayloads, extraHeaders, err = s.serializeEventsStreamJSONMarshalerPayload(e, true)
	} else {
		eventPayloads, extraHeaders, err = s.serializePayload(e, true, true)
	}
	if err != nil {
		return fmt.Errorf("dropping event payload: %s", err)
	}

	return s.Forwarder.SubmitV1Intake(eventPayloads, extraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(sc marshaler.StreamJSONMarshaler) error {
	if !s.enableServiceChecks {
		log.Debug("service_checks payloads are disabled: dropping it")
		return nil
	}

	var serviceCheckPayloads forwarder.Payloads
	var extraHeaders http.Header
	var err error

	if s.enableServiceChecksJSONStream {
		serviceCheckPayloads, extraHeaders, err = s.serializeStreamablePayload(sc, stream.DropItemOnErrItemTooBig)
	} else {
		serviceCheckPayloads, extraHeaders, err = s.serializePayloadJSON(sc, true)
	}
	if err != nil {
		return fmt.Errorf("dropping service check payload: %s", err)
	}

	return s.Forwarder.SubmitV1CheckRuns(serviceCheckPayloads, extraHeaders)
}

// SendSeries serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendSeries(series marshaler.StreamJSONMarshaler) error {
	if !s.enableSeries {
		log.Debug("series payloads are disabled: dropping it")
		return nil
	}

	useV1API := !config.Datadog.GetBool("use_v2_api.series")

	var seriesPayloads forwarder.Payloads
	var extraHeaders http.Header
	var err error

	if useV1API && s.enableJSONStream {
		seriesPayloads, extraHeaders, err = s.serializeStreamablePayload(series, stream.DropItemOnErrItemTooBig)
	} else if useV1API && !s.enableJSONStream {
		seriesPayloads, extraHeaders, err = s.serializePayloadJSON(series, true)
	} else {
		seriesPayloads, err = series.MarshalSplitCompress(marshaler.DefaultBufferContext())
		extraHeaders = protobufExtraHeadersWithCompression
	}

	if err != nil {
		return fmt.Errorf("dropping series payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1Series(seriesPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitSeries(seriesPayloads, extraHeaders)
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches marshaler.Marshaler) error {
	if !s.enableSketches {
		log.Debug("sketches payloads are disabled: dropping it")
		return nil
	}

	if s.enableSketchProtobufStream {
		payloads, err := sketches.MarshalSplitCompress(marshaler.DefaultBufferContext())
		if err == nil {
			return s.Forwarder.SubmitSketchSeries(payloads, protobufExtraHeadersWithCompression)
		}
		log.Warnf("Error: %v trying to stream compress SketchSeriesList - falling back to split/compress method", err)
	}

	compress := true
	useV1API := false // Sketches only have a v2 endpoint
	splitSketches, extraHeaders, err := s.serializePayload(sketches, compress, useV1API)
	if err != nil {
		return fmt.Errorf("dropping sketch payload: %s", err)
	}

	return s.Forwarder.SubmitSketchSeries(splitSketches, extraHeaders)
}

// SendMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendMetadata(m marshaler.JSONMarshaler) error {
	return s.sendMetadata(m, s.Forwarder.SubmitMetadata)
}

// SendHostMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendHostMetadata(m marshaler.JSONMarshaler) error {
	return s.sendMetadata(m, s.Forwarder.SubmitHostMetadata)
}

// SendAgentchecksMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendAgentchecksMetadata(m marshaler.JSONMarshaler) error {
	return s.sendMetadata(m, s.Forwarder.SubmitAgentChecksMetadata)
}

func (s *Serializer) sendMetadata(m marshaler.JSONMarshaler, submit func(payload forwarder.Payloads, extra http.Header) error) error {
	mustSplit, compressedPayload, payload, err := split.CheckSizeAndSerialize(m, true, split.JSONMarshalFct)
	if err != nil {
		return fmt.Errorf("could not determine size of metadata payload: %s", err)
	}

	log.Debugf("Sending metadata payload, content: %v", string(payload))

	if mustSplit {
		return fmt.Errorf("metadata payload was too big to send (%d bytes compressed, %d bytes uncompressed), metadata payloads cannot be split", len(compressedPayload), len(payload))
	}

	if err := submit(forwarder.Payloads{&compressedPayload}, jsonExtraHeadersWithCompression); err != nil {
		return err
	}

	log.Infof("Sent metadata payload, size (raw/compressed): %d/%d bytes.", len(payload), len(compressedPayload))
	return nil
}

// SendProcessesMetadata serializes a payload and sends it to the forwarder.
// Used only by the legacy processes metadata collector.
func (s *Serializer) SendProcessesMetadata(data interface{}) error {
	if !s.enableJSONToV1Intake {
		log.Debug("JSON to V1 intake endpoint payloads are disabled: dropping it")
		return nil
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not serialize processes metadata payload: %s", err)
	}
	compressedPayload, err := compression.Compress(nil, payload)
	if err != nil {
		return fmt.Errorf("could not compress processes metadata payload: %s", err)
	}
	if err := s.Forwarder.SubmitV1Intake(forwarder.Payloads{&compressedPayload}, jsonExtraHeadersWithCompression); err != nil {
		return err
	}

	log.Infof("Sent processes metadata payload, size: %d bytes.", len(payload))
	log.Debugf("Sent processes metadata payload, content: %v", string(payload))
	return nil
}

// SendOrchestratorMetadata serializes & send orchestrator metadata payloads
func (s *Serializer) SendOrchestratorMetadata(msgs []ProcessMessageBody, hostName, clusterID string, payloadType int) error {
	if s.orchestratorForwarder == nil {
		return errors.New("orchestrator forwarder is not setup")
	}
	for _, m := range msgs {
		extraHeaders := make(http.Header)
		extraHeaders.Set(headers.HostHeader, hostName)
		extraHeaders.Set(headers.ClusterIDHeader, clusterID)
		extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(time.Now().Unix())))

		body, err := processPayloadEncoder(m)
		if err != nil {
			return log.Errorf("Unable to encode message: %s", err)
		}

		payloads := forwarder.Payloads{&body}
		responses, err := s.orchestratorForwarder.SubmitOrchestratorChecks(payloads, extraHeaders, payloadType)
		if err != nil {
			return log.Errorf("Unable to submit payload: %s", err)
		}

		// Consume the responses so that writers to the channel do not become blocked
		// we don't need the bodies here though
		for range responses {

		}
	}
	return nil
}
