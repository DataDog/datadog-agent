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
	orchestratorForwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"

	"github.com/DataDog/datadog-agent/comp/core/config"
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

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	protobufContentType                         = "application/x-protobuf"
	jsonContentType                             = "application/json"
	payloadVersionHTTPHeader                    = "DD-Agent-Payload"
	maxItemCountForCreateMarshalersBySourceType = 100
)

var (
	expvars                                 = expvar.NewMap("serializer")
	expvarsSendEventsErrItemTooBigs         = expvar.Int{}
	expvarsSendEventsErrItemTooBigsFallback = expvar.Int{}
)

func init() {
	expvars.Set("SendEventsErrItemTooBigs", &expvarsSendEventsErrItemTooBigs)
	expvars.Set("SendEventsErrItemTooBigsFallback", &expvarsSendEventsErrItemTooBigsFallback)
}

// initExtraHeaders initializes the global extraHeaders variables.
// Not part of the `init` function body to ease testing
func initExtraHeaders(s *Serializer) {

	s.jsonExtraHeaders.Set("Content-Type", jsonContentType)

	for k := range s.jsonExtraHeaders {
		s.jsonExtraHeadersWithCompression.Set(k, s.jsonExtraHeaders.Get(k))
	}

	s.protobufExtraHeaders.Set("Content-Type", protobufContentType)
	s.protobufExtraHeaders.Set(payloadVersionHTTPHeader, version.AgentPayloadVersion)

	s.protobufExtraHeadersWithCompression = make(http.Header)
	for k := range s.protobufExtraHeaders {
		s.protobufExtraHeadersWithCompression.Set(k, s.protobufExtraHeaders.Get(k))
	}

	encoding := s.Strategy.ContentEncoding()

	if encoding != "" {
		s.jsonExtraHeadersWithCompression.Set("Content-Encoding", encoding)
		s.protobufExtraHeadersWithCompression.Set("Content-Encoding", encoding)
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
	Forwarder             forwarder.Forwarder
	orchestratorForwarder orchestratorForwarder.Component
	config                config.Component

	Strategy                            compression.Compressor
	seriesJSONPayloadBuilder            *stream.JSONPayloadBuilder
	jsonExtraHeaders                    http.Header
	protobufExtraHeaders                http.Header
	jsonExtraHeadersWithCompression     http.Header
	protobufExtraHeadersWithCompression http.Header

	// Those variables allow users to blacklist any kind of payload
	// from being sent by the agent. This was introduced for
	// environment where, for example, events or serviceChecks
	// might collect data considered too sensitive (database IP and
	// such). By default every kind of payload is enabled since
	// almost every user won't fall into this use case.
	enableEvents         bool
	enableSeries         bool
	enableServiceChecks  bool
	enableSketches       bool
	enableJSONToV1Intake bool
	hostname             string
	logger               log.Component
}

// NewSerializer returns a new Serializer initialized
func NewSerializer(forwarder forwarder.Forwarder, orchestratorForwarder orchestratorForwarder.Component, compressor compression.Compressor, config config.Component, logger log.Component, hostName string) *Serializer {
	s := &Serializer{
		Forwarder:                           forwarder,
		orchestratorForwarder:               orchestratorForwarder,
		config:                              config,
		seriesJSONPayloadBuilder:            stream.NewJSONPayloadBuilder(config.GetBool("enable_json_stream_shared_compressor_buffers"), config, compressor, logger),
		enableEvents:                        config.GetBool("enable_payloads.events"),
		enableSeries:                        config.GetBool("enable_payloads.series"),
		enableServiceChecks:                 config.GetBool("enable_payloads.service_checks"),
		enableSketches:                      config.GetBool("enable_payloads.sketches"),
		enableJSONToV1Intake:                config.GetBool("enable_payloads.json_to_v1_intake"),
		hostname:                            hostName,
		Strategy:                            compressor,
		jsonExtraHeaders:                    make(http.Header),
		protobufExtraHeaders:                make(http.Header),
		jsonExtraHeadersWithCompression:     make(http.Header),
		protobufExtraHeadersWithCompression: make(http.Header),
		logger:                              logger,
	}

	initExtraHeaders(s)

	if !s.enableEvents {
		logger.Warn("event payloads are disabled: all events will be dropped")
	}
	if !s.AreSeriesEnabled() {
		logger.Warn("series payloads are disabled: all series will be dropped")
	}
	if !s.AreSketchesEnabled() {
		logger.Warn("service_checks payloads are disabled: all service_checks will be dropped")
	}
	if !s.enableSketches {
		logger.Warn("sketches payloads are disabled: all sketches will be dropped")
	}
	if !s.enableJSONToV1Intake {
		logger.Warn("JSON to V1 intake is disabled: all payloads to that endpoint will be dropped")
	}

	return s
}

func (s Serializer) serializeStreamablePayload(payload marshaler.StreamJSONMarshaler, policy stream.OnErrItemTooBigPolicy) (transaction.BytesPayloads, http.Header, error) {
	adapter := marshaler.NewIterableStreamJSONMarshalerAdapter(payload)
	payloads, err := s.seriesJSONPayloadBuilder.BuildWithOnErrItemTooBigPolicy(adapter, policy)
	return payloads, s.jsonExtraHeadersWithCompression, err
}

func (s Serializer) serializeIterableStreamablePayload(payload marshaler.IterableStreamJSONMarshaler, policy stream.OnErrItemTooBigPolicy) (transaction.BytesPayloads, http.Header, error) {
	payloads, err := s.seriesJSONPayloadBuilder.BuildWithOnErrItemTooBigPolicy(payload, policy)
	return payloads, s.jsonExtraHeadersWithCompression, err
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(events event.Events) error {
	if !s.enableEvents {
		s.logger.Debug("events payloads are disabled: dropping it")
		return nil
	}

	payloads, err := metricsserializer.MarshalEvents(events, s.hostname, s.config, s.logger, s.Strategy)
	if err != nil {
		return fmt.Errorf("dropping event payloads: %v", err)
	}
	if len(payloads) == 0 {
		return nil
	}
	return s.Forwarder.SubmitV1Intake(payloads, transaction.Events, s.jsonExtraHeadersWithCompression)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(serviceChecks servicecheck.ServiceChecks) error {
	if !s.enableServiceChecks {
		s.logger.Debug("service_checks payloads are disabled: dropping it")
		return nil
	}

	serviceChecksSerializer := metricsserializer.ServiceChecks(serviceChecks)

	serviceCheckPayloads, extraHeaders, err := s.serializeStreamablePayload(serviceChecksSerializer, stream.DropItemOnErrItemTooBig)
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
		s.logger.Debug("series payloads are disabled: dropping it")
		return nil
	}

	seriesSerializer := metricsserializer.CreateIterableSeries(serieSource)
	useV1API := !s.config.GetBool("use_v2_api.series")

	var seriesBytesPayloads transaction.BytesPayloads
	var extraHeaders http.Header
	var err error

	if useV1API {
		seriesBytesPayloads, extraHeaders, err = s.serializeIterableStreamablePayload(seriesSerializer, stream.DropItemOnErrItemTooBig)
		if err != nil {
			return fmt.Errorf("dropping series payload: %s", err)
		}
		return s.Forwarder.SubmitV1Series(seriesBytesPayloads, extraHeaders)
	}

	pipelines := s.buildPipelines()
	seriesBytesPayloads, err = seriesSerializer.MarshalSplitCompressPipelines(s.config, s.Strategy, pipelines)
	extraHeaders = s.protobufExtraHeadersWithCompression

	if err != nil {
		return fmt.Errorf("dropping series payload: %s", err)
	}

	return s.Forwarder.SubmitSeries(seriesBytesPayloads, extraHeaders)
}

func (s *Serializer) getFailoverAllowlist() (bool, map[string]struct{}) {
	failoverActive := s.config.GetBool("multi_region_failover.enabled") && s.config.GetBool("multi_region_failover.failover_metrics")
	var allowlist map[string]struct{}
	if failoverActive && s.config.IsSet("multi_region_failover.metric_allowlist") {
		rawList := s.config.GetStringSlice("multi_region_failover.metric_allowlist")
		allowlist = make(map[string]struct{}, len(rawList))
		for _, allowed := range rawList {
			allowlist[allowed] = struct{}{}
		}
	}
	return failoverActive, allowlist
}

func (s *Serializer) buildPipelines() []metricsserializer.Pipeline {
	failoverActiveForMRF, allowlistForMRF := s.getFailoverAllowlist()
	failoverActiveForAutoscaling, allowlistForAutoscaling := s.getAutoscalingFailoverMetrics()
	failoverActive := (failoverActiveForMRF && len(allowlistForMRF) > 0) || (failoverActiveForAutoscaling && len(allowlistForAutoscaling) > 0)

	if failoverActive {
		return []metricsserializer.Pipeline{
			{
				FilterFunc:  func(metric metricsserializer.Filterable) bool { return true },
				Destination: transaction.PrimaryOnly,
			},
			{
				FilterFunc: func(metric metricsserializer.Filterable) bool {
					_, allowed := allowlistForMRF[metric.GetName()]
					return allowed
				},
				Destination: transaction.SecondaryOnly,
			},
			{
				FilterFunc: func(metric metricsserializer.Filterable) bool {
					_, allowed := allowlistForAutoscaling[metric.GetName()]
					return allowed
				},
				Destination: transaction.LocalOnly,
			},
		}
	}

	// Default: all metrics to AllRegions
	return []metricsserializer.Pipeline{
		{
			FilterFunc:  func(metric metricsserializer.Filterable) bool { return true },
			Destination: transaction.AllRegions,
		},
	}
}

func (s *Serializer) getAutoscalingFailoverMetrics() (bool, map[string]struct{}) {
	autoscalingFailoverEnabled := s.config.GetBool("autoscaling.failover.enabled") && s.config.GetBool("cluster_agent.enabled")
	var allowlist map[string]struct{}
	if autoscalingFailoverEnabled {
		rawList := s.config.GetStringSlice("autoscaling.failover.metrics")
		allowlist = make(map[string]struct{}, len(rawList))
		for _, allowed := range rawList {
			allowlist[allowed] = struct{}{}
		}
	}
	return autoscalingFailoverEnabled, allowlist
}

// AreSketchesEnabled returns whether sketches are enabled for serialization
func (s *Serializer) AreSketchesEnabled() bool {
	return s.enableSketches
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches metrics.SketchesSource) error {
	if !s.AreSketchesEnabled() {
		s.logger.Debug("sketches payloads are disabled: dropping it")
		return nil
	}
	sketchesSerializer := metricsserializer.SketchSeriesList{SketchesSource: sketches}

	pipelines := s.buildPipelines()
	payloads, err := sketchesSerializer.MarshalSplitCompressPipelines(s.config, s.Strategy, pipelines, s.logger)
	if err != nil {
		return fmt.Errorf("dropping sketch payload: %v", err)
	}

	return s.Forwarder.SubmitSketchSeries(payloads, s.protobufExtraHeadersWithCompression)
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
	mustSplit, compressedPayload, payload, err := split.CheckSizeAndSerialize(m, true, s.Strategy)
	if err != nil {
		return fmt.Errorf("could not determine size of metadata payload: %s", err)
	}

	s.logger.Debugf("Sending metadata payload, content: %v", string(payload))

	if mustSplit {
		return fmt.Errorf("metadata payload was too big to send (%d bytes compressed, %d bytes uncompressed), metadata payloads cannot be split", len(compressedPayload), len(payload))
	}

	if err := submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), s.jsonExtraHeadersWithCompression); err != nil {
		return err
	}

	s.logger.Debugf("Sent metadata payload, size (raw/compressed): %d/%d bytes.", len(payload), len(compressedPayload))
	return nil
}

// SendProcessesMetadata serializes a payload and sends it to the forwarder.
// Used only by the legacy processes metadata collector.
func (s *Serializer) SendProcessesMetadata(data interface{}) error {
	if !s.enableJSONToV1Intake {
		s.logger.Debug("JSON to V1 intake endpoint payloads are disabled: dropping it")
		return nil
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not serialize processes metadata payload: %s", err)
	}
	compressedPayload, err := s.Strategy.Compress(payload)
	if err != nil {
		return fmt.Errorf("could not compress processes metadata payload: %s", err)
	}
	if err := s.Forwarder.SubmitV1Intake(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}),
		transaction.Events, s.jsonExtraHeadersWithCompression); err != nil {
		return err
	}

	s.logger.Debugf("Sent processes metadata payload, size: %d bytes, content: %v", len(payload), string(payload))
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
			return s.logger.Errorf("Unable to encode message: %s", err)
		}

		err = orchestratorForwarder.SubmitOrchestratorChecks(payloads, extraHeaders, payloadType)
		if err != nil {
			return s.logger.Errorf("Unable to submit payload: %s", err)
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
			s.logger.Errorf("Unable to encode message: %s", err)
			continue
		}

		err = orchestratorForwarder.SubmitOrchestratorManifests(payloads, extraHeaders)
		if err != nil {
			return s.logger.Errorf("Unable to submit payload: %s", err)
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
