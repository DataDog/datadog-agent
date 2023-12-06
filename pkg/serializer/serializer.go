// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package serializer

import (
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"strconv"
	"time"

	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	orchestratorForwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

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
	if useV1API {
		return s.serializePayloadJSON(jsonMarshaler, compress)
	}
	return s.serializePayloadProto(protoMarshaler, compress)
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
	adapter := marshaler.NewIterableStreamJSONMarshalerAdapter(payload)
	payloads, err := s.seriesJSONPayloadBuilder.BuildWithOnErrItemTooBigPolicy(adapter, policy)
	return payloads, jsonExtraHeadersWithCompression, err
}

func (s Serializer) serializeIterableStreamablePayload(payload marshaler.IterableStreamJSONMarshaler, policy stream.OnErrItemTooBigPolicy) (transaction.BytesPayloads, http.Header, error) {
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
	eventsSerializer metricsserializer.Events, useV1API bool) (transaction.BytesPayloads, http.Header, error) {
	marshaler := eventsSerializer.CreateSingleMarshaler()
	eventPayloads, extraHeaders, err := s.serializeStreamablePayload(marshaler, stream.FailOnErrItemTooBig)

	if err == stream.ErrItemTooBig {
		expvarsSendEventsErrItemTooBigs.Add(1)

		// Do not use CreateMarshalersBySourceType when there are too many source types (Performance issue).
		if marshaler.Len() > maxItemCountForCreateMarshalersBySourceType {
			expvarsSendEventsErrItemTooBigsFallback.Add(1)
			eventPayloads, extraHeaders, err = s.serializePayload(eventsSerializer, eventsSerializer, true, useV1API)
		} else {
			eventPayloads = nil
			for _, v := range eventsSerializer.CreateMarshalersBySourceType() {
				var eventPayloadsForSourceType transaction.BytesPayloads
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
func (s *Serializer) SendEvents(events event.Events) error {
	if !s.enableEvents {
		log.Debug("events payloads are disabled: dropping it")
		return nil
	}

	var eventPayloads transaction.BytesPayloads
	var extraHeaders http.Header
	var err error

	eventsSerializer := metricsserializer.Events(events)
	if s.enableEventsJSONStream {
		eventPayloads, extraHeaders, err = s.serializeEventsStreamJSONMarshalerPayload(eventsSerializer, true)
	} else {
		eventPayloads, extraHeaders, err = s.serializePayload(eventsSerializer, eventsSerializer, true, true)
	}
	if err != nil {
		return fmt.Errorf("dropping event payload: %s", err)
	}

	return s.Forwarder.SubmitV1Intake(eventPayloads, extraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(serviceChecks servicecheck.ServiceChecks) error {
	if !s.enableServiceChecks {
		log.Debug("service_checks payloads are disabled: dropping it")
		return nil
	}

	serviceChecksSerializer := metricsserializer.ServiceChecks(serviceChecks)
	var serviceCheckPayloads transaction.BytesPayloads
	var extraHeaders http.Header
	var err error

	if s.enableServiceChecksJSONStream {
		serviceCheckPayloads, extraHeaders, err = s.serializeStreamablePayload(serviceChecksSerializer, stream.DropItemOnErrItemTooBig)
	} else {
		serviceCheckPayloads, extraHeaders, err = s.serializePayloadJSON(serviceChecksSerializer, true)
	}
	if err != nil {
		return fmt.Errorf("dropping service check payload: %s", err)
	}

	return s.Forwarder.SubmitV1CheckRuns(serviceCheckPayloads, extraHeaders)
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

func (s *Serializer) sendMetadata(m marshaler.JSONMarshaler, submit func(payload transaction.BytesPayloads, extra http.Header) error) error {
	mustSplit, compressedPayload, payload, err := split.CheckSizeAndSerialize(m, true, split.JSONMarshalFct)
	if err != nil {
		return fmt.Errorf("could not determine size of metadata payload: %s", err)
	}

	log.Debugf("Sending metadata payload, content: %v", string(payload))

	if mustSplit {
		return fmt.Errorf("metadata payload was too big to send (%d bytes compressed, %d bytes uncompressed), metadata payloads cannot be split", len(compressedPayload), len(payload))
	}

	if err := submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), jsonExtraHeadersWithCompression); err != nil {
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
	compressedPayload, err := compression.Compress(payload)
	if err != nil {
		return fmt.Errorf("could not compress processes metadata payload: %s", err)
	}
	if err := s.Forwarder.SubmitV1Intake(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), jsonExtraHeadersWithCompression); err != nil {
		return err
	}

	log.Infof("Sent processes metadata payload, size: %d bytes.", len(payload))
	log.Debugf("Sent processes metadata payload, content: %v", string(payload))
	return nil
}

// SendOrchestratorMetadata serializes & send orchestrator metadata payloads
func (s *Serializer) SendOrchestratorMetadata(msgs []types.ProcessMessageBody, hostName, clusterID string, payloadType int) error {
	orchestratorForwarder, found := s.orchestratorForwarder.Get()

	if !found {
		return errors.New("orchestrator forwarder is not setup")
	}
	for _, m := range msgs {
		payloads, extraHeaders, err := makeOrchestratorPayloads(m, hostName, clusterID)
		if err != nil {
			return log.Errorf("Unable to encode message: %s", err)
		}

		responses, err := orchestratorForwarder.SubmitOrchestratorChecks(payloads, extraHeaders, payloadType)
		if err != nil {
			return log.Errorf("Unable to submit payload: %s", err)
		}

		// Consume the responses so that writers to the channel do not become blocked
		// we don't need the bodies here though
		//nolint:revive // TODO(AML) Fix revive linter
		for range responses {

		}
	}
	return nil
}

// SendOrchestratorManifests serializes & send orchestrator manifest payloads
func (s *Serializer) SendOrchestratorManifests(msgs []types.ProcessMessageBody, hostName, clusterID string) error {
	orchestratorForwarder, found := s.orchestratorForwarder.Get()
	if !found {
		return errors.New("orchestrator forwarder is not setup")
	}
	for _, m := range msgs {
		payloads, extraHeaders, err := makeOrchestratorPayloads(m, hostName, clusterID)
		if err != nil {
			log.Errorf("Unable to encode message: %s", err)
			continue
		}

		responses, err := orchestratorForwarder.SubmitOrchestratorManifests(payloads, extraHeaders)
		if err != nil {
			return log.Errorf("Unable to submit payload: %s", err)
		}

		// Consume the responses so that writers to the channel do not become blocked
		// we don't need the bodies here though
		//nolint:revive // TODO(AML) Fix revive linter
		for range responses {

		}
	}
	return nil
}

func makeOrchestratorPayloads(msg types.ProcessMessageBody, hostName, clusterID string) (transaction.BytesPayloads, http.Header, error) {
	extraHeaders := make(http.Header)
	extraHeaders.Set(headers.HostHeader, hostName)
	extraHeaders.Set(headers.ClusterIDHeader, clusterID)
	extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(time.Now().Unix())))
	extraHeaders.Set(headers.EVPOriginHeader, "agent")
	extraHeaders.Set(headers.EVPOriginVersionHeader, version.AgentVersion)
	extraHeaders.Set(headers.ContentTypeHeader, headers.ProtobufContentType)

	body, err := types.ProcessPayloadEncoder(msg)
	if err != nil {
		return nil, nil, err
	}
	payloads := []*[]byte{&body}
	return transaction.NewBytesPayloadsWithoutMetaData(payloads), extraHeaders, nil
}
