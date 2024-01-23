// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package serializer

import (
	"expvar"
	"fmt"
	"net/http"

	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	orchestratorForwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/benbjohnson/clock"
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

// MetricSerializer represents the interface of method needed by the aggregator to serialize its data
type MetricSerializer interface {
	SendEvents(e event.Events) error
	SendServiceChecks(serviceChecks servicecheck.ServiceChecks) error
	SendIterableSeries(serieSource metrics.SerieSource) error
	AreSeriesEnabled() bool
	SendSketch(sketches metrics.SketchesSource) error
	AreSketchesEnabled() bool

	SendMetadata(m marshaler.JSONMarshaler) error
	SendHostMetadata(m marshaler.JSONMarshaler) error
	SendProcessesMetadata(data interface{}) error
	SendAgentchecksMetadata(m marshaler.JSONMarshaler) error
	SendOrchestratorMetadata(msgs []types.ProcessMessageBody, hostName, clusterID string, payloadType int) error
	SendOrchestratorManifests(msgs []types.ProcessMessageBody, hostName, clusterID string) error
}

// Serializer serializes metrics to the correct format and routes the payloads to the correct endpoint in the Forwarder
type Serializer struct {
	clock                 clock.Clock
	Forwarder             forwarder.Forwarder
	orchestratorForwarder orchestratorForwarder.Component

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
func NewSerializer(forwarder forwarder.Forwarder, orchestratorForwarder orchestratorForwarder.Component) *Serializer {
	s := &Serializer{
		clock:                         clock.New(),
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
	if !s.AreSeriesEnabled() {
		log.Warn("series payloads are disabled: all series will be dropped")
	}
	if !s.AreSketchesEnabled() {
		log.Warn("service_checks payloads are disabled: all service_checks will be dropped")
	}
	if !s.enableSketches {
		log.Warn("sketches payloads are disabled: all sketches will be dropped")
	}
	if !s.enableJSONToV1Intake {
		log.Warn("JSON to V1 intake is disabled: all payloads to that endpoint will be dropped")
	}

	if !config.Datadog.GetBool("enable_sketch_stream_payload_serialization") {
		log.Warn("'enable_sketch_stream_payload_serialization' is set to false which is not recommended. This option is deprecated and will removed in the future. If you need this option, please reach out to support")
	}

	return s
}

func (s Serializer) serializePayload(
	jsonMarshaler marshaler.JSONMarshaler,
	protoMarshaler marshaler.ProtoMarshaler,
	compress bool,
	useV1API bool) (transaction.BytesPayloads, http.Header, error) {
	panic("not called")
}

func (s Serializer) serializePayloadJSON(payload marshaler.JSONMarshaler, compress bool) (transaction.BytesPayloads, http.Header, error) {
	var extraHeaders http.Header

	if compress {
		extraHeaders = jsonExtraHeadersWithCompression
	} else {
		extraHeaders = jsonExtraHeaders
	}

	return s.serializePayloadInternal(payload, compress, extraHeaders, split.JSONMarshalFct)
}

func (s Serializer) serializePayloadProto(payload marshaler.ProtoMarshaler, compress bool) (transaction.BytesPayloads, http.Header, error) {
	var extraHeaders http.Header
	if compress {
		extraHeaders = protobufExtraHeadersWithCompression
	} else {
		extraHeaders = protobufExtraHeaders
	}
	return s.serializePayloadInternal(payload, compress, extraHeaders, split.ProtoMarshalFct)
}

func (s Serializer) serializePayloadInternal(payload marshaler.AbstractMarshaler, compress bool, extraHeaders http.Header, marshalFct split.MarshalFct) (transaction.BytesPayloads, http.Header, error) {
	payloads, err := split.Payloads(payload, compress, marshalFct)

	if err != nil {
		return nil, nil, fmt.Errorf("could not split payload into small enough chunks: %s", err)
	}

	return payloads, extraHeaders, nil
}

func (s Serializer) serializeStreamablePayload(payload marshaler.StreamJSONMarshaler, policy stream.OnErrItemTooBigPolicy) (transaction.BytesPayloads, http.Header, error) {
	panic("not called")
}

func (s Serializer) serializeIterableStreamablePayload(payload marshaler.IterableStreamJSONMarshaler, policy stream.OnErrItemTooBigPolicy) (transaction.BytesPayloads, http.Header, error) {
	panic("not called")
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
	eventsSerializer metricsserializer.Events, useV1API bool) (transaction.BytesPayloads, http.Header, error) {
	panic("not called")
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(events event.Events) error {
	panic("not called")
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(serviceChecks servicecheck.ServiceChecks) error {
	panic("not called")
}

// AreSeriesEnabled returns whether series are enabled for serialization
func (s *Serializer) AreSeriesEnabled() bool {
	return s.enableSeries
}

// SendIterableSeries serializes a list of series and sends the payload to the forwarder
func (s *Serializer) SendIterableSeries(serieSource metrics.SerieSource) error {
	if !s.AreSeriesEnabled() {
		log.Debug("series payloads are disabled: dropping it")
		return nil
	}

	seriesSerializer := metricsserializer.CreateIterableSeries(serieSource)
	useV1API := !config.Datadog.GetBool("use_v2_api.series")

	var seriesBytesPayloads transaction.BytesPayloads
	var extraHeaders http.Header
	var err error

	if useV1API && s.enableJSONStream {
		seriesBytesPayloads, extraHeaders, err = s.serializeIterableStreamablePayload(seriesSerializer, stream.DropItemOnErrItemTooBig)
	} else if useV1API && !s.enableJSONStream {
		seriesBytesPayloads, extraHeaders, err = s.serializePayloadJSON(seriesSerializer, true)
	} else {
		seriesBytesPayloads, err = seriesSerializer.MarshalSplitCompress(marshaler.NewBufferContext())
		extraHeaders = protobufExtraHeadersWithCompression
	}

	if err != nil {
		return fmt.Errorf("dropping series payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1Series(seriesBytesPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitSeries(seriesBytesPayloads, extraHeaders)
}

// AreSketchesEnabled returns whether sketches are enabled for serialization
func (s *Serializer) AreSketchesEnabled() bool {
	return s.enableSketches
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches metrics.SketchesSource) error {
	if !s.AreSketchesEnabled() {
		log.Debug("sketches payloads are disabled: dropping it")
		return nil
	}
	sketchesSerializer := metricsserializer.SketchSeriesList{SketchesSource: sketches}
	if s.enableSketchProtobufStream {
		payloads, err := sketchesSerializer.MarshalSplitCompress(marshaler.NewBufferContext())
		if err != nil {
			return fmt.Errorf("dropping sketch payload: %v", err)
		}

		return s.Forwarder.SubmitSketchSeries(payloads, protobufExtraHeadersWithCompression)
	} else {
		//nolint:revive // TODO(AML) Fix revive linter
		compress := true
		splitSketches, extraHeaders, err := s.serializePayloadProto(sketchesSerializer, compress)
		if err != nil {
			return fmt.Errorf("dropping sketch payload: %s", err)
		}

		return s.Forwarder.SubmitSketchSeries(splitSketches, extraHeaders)
	}
}

// SendMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendMetadata(m marshaler.JSONMarshaler) error {
	panic("not called")
}

// SendHostMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendHostMetadata(m marshaler.JSONMarshaler) error {
	panic("not called")
}

// SendAgentchecksMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendAgentchecksMetadata(m marshaler.JSONMarshaler) error {
	panic("not called")
}

func (s *Serializer) sendMetadata(m marshaler.JSONMarshaler, submit func(payload transaction.BytesPayloads, extra http.Header) error) error {
	panic("not called")
}

// SendProcessesMetadata serializes a payload and sends it to the forwarder.
// Used only by the legacy processes metadata collector.
func (s *Serializer) SendProcessesMetadata(data interface{}) error {
	panic("not called")
}

// SendOrchestratorMetadata serializes & send orchestrator metadata payloads
func (s *Serializer) SendOrchestratorMetadata(msgs []types.ProcessMessageBody, hostName, clusterID string, payloadType int) error {
	panic("not called")
}

// SendOrchestratorManifests serializes & send orchestrator manifest payloads
func (s *Serializer) SendOrchestratorManifests(msgs []types.ProcessMessageBody, hostName, clusterID string) error {
	panic("not called")
}

func makeOrchestratorPayloads(msg types.ProcessMessageBody, hostName, clusterID string) (transaction.BytesPayloads, http.Header, error) {
	panic("not called")
}
