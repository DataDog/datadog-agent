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
	"sort"
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
	"github.com/DataDog/datadog-agent/pkg/serializer/limits"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
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

	pipelines := s.buildPipelines(metricsKindSeries)
	err = seriesSerializer.MarshalSplitCompressPipelines(s.config, s.Strategy, pipelines)
	if err != nil {
		return fmt.Errorf("dropping series payload: %s", err)
	}

	return pipelines.Send(s.Forwarder, s.protobufExtraHeadersWithCompression)
}

func (s *Serializer) getFailoverAllowlist() metricsserializer.Filter {
	failoverActive := s.config.GetBool("multi_region_failover.enabled") && s.config.GetBool("multi_region_failover.failover_metrics")
	if !failoverActive {
		return nil
	}

	var allowlist map[string]struct{}
	if s.config.IsSet("multi_region_failover.metric_allowlist") {
		rawList := s.config.GetStringSlice("multi_region_failover.metric_allowlist")
		allowlist = make(map[string]struct{}, len(rawList))
		for _, allowed := range rawList {
			allowlist[allowed] = struct{}{}
		}
	}

	if len(allowlist) == 0 {
		return metricsserializer.AllowAllFilter{}
	}

	return metricsserializer.NewMapFilter(allowlist)
}

func (s *Serializer) getAutoscalingFailoverMetrics() metricsserializer.Filter {
	autoscalingFailoverEnabled := s.config.GetBool("autoscaling.failover.enabled") && s.config.GetBool("cluster_agent.enabled")
	if !autoscalingFailoverEnabled {
		return nil
	}

	var allowlist map[string]struct{}
	rawList := s.config.GetStringSlice("autoscaling.failover.metrics")
	allowlist = make(map[string]struct{}, len(rawList))
	for _, allowed := range rawList {
		allowlist[allowed] = struct{}{}
	}

	if len(allowlist) == 0 {
		return nil
	}

	return metricsserializer.NewMapFilter(allowlist)
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

	pipelines := s.buildPipelines(metricsKindSketches)
	err := sketchesSerializer.MarshalSplitCompressPipelines(s.config, s.Strategy, pipelines, s.logger)
	if err != nil {
		return fmt.Errorf("dropping sketch payload: %v", err)
	}

	return pipelines.Send(s.Forwarder, s.protobufExtraHeadersWithCompression)
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
	// Serialize the payload to JSON
	payload, err := m.MarshalJSON()
	if err != nil {
		return fmt.Errorf("could not serialize metadata payload: %s", err)
	}

	s.logger.Debugf("Sending metadata payload, content: %v", string(payload))

	// Fast path: if uncompressed payload is under the target batch size, send as-is
	if len(payload) <= limits.MetadataTargetBatch {
		compressedPayload, err := s.Strategy.Compress(payload)
		if err != nil {
			return fmt.Errorf("could not compress metadata payload: %s", err)
		}

		if err := submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), s.jsonExtraHeadersWithCompression); err != nil {
			return err
		}

		s.logger.Debugf("Sent metadata payload, size (raw/compressed): %d/%d bytes.", len(payload), len(compressedPayload))
		return nil
	}

	// Slow path: payload exceeds target size, need to batch
	return s.sendBatchedMetadata(payload, submit)
}

// sendBatchedMetadata splits a large metadata payload into multiple batches and sends them sequentially.
// This is necessary because the intake server enforces a 1MB uncompressed limit on metadata payloads.
//
// Note on partial failures: If batch N fails after batches 1..N-1 have been sent, those earlier batches
// are already submitted to the forwarder. The backend must handle partial batch sets gracefully.
func (s *Serializer) sendBatchedMetadata(payload []byte, submit func(payload transaction.BytesPayloads, extra http.Header) error) error {
	// Parse payload as generic map to access check_metadata
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		return fmt.Errorf("could not parse metadata payload for batching: %s", err)
	}

	// Get check_metadata, which is what we'll batch
	checkMetadataRaw, hasCheckMetadata := payloadMap["check_metadata"]
	if !hasCheckMetadata {
		// No check_metadata to batch. Send as-is and let intake handle it.
		compressedPayload, err := s.Strategy.Compress(payload)
		if err != nil {
			return fmt.Errorf("could not compress metadata payload: %s", err)
		}
		if len(payload) > limits.MetadataMaxUncompressed {
			s.logger.Warnf("Metadata payload exceeds %d bytes (%d bytes) but has no check_metadata to batch",
				limits.MetadataMaxUncompressed, len(payload))
		}
		return submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), s.jsonExtraHeadersWithCompression)
	}

	checkMetadata, ok := checkMetadataRaw.(map[string]interface{})
	if !ok {
		// check_metadata is not a map, can't batch. Send as-is.
		compressedPayload, err := s.Strategy.Compress(payload)
		if err != nil {
			return fmt.Errorf("could not compress metadata payload: %s", err)
		}
		s.logger.Warnf("Metadata payload exceeds limit but check_metadata is not a map, sending as-is")
		return submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), s.jsonExtraHeadersWithCompression)
	}

	// If check_metadata is empty, send the original payload as-is
	if len(checkMetadata) == 0 {
		compressedPayload, err := s.Strategy.Compress(payload)
		if err != nil {
			return fmt.Errorf("could not compress metadata payload: %s", err)
		}
		if len(payload) > limits.MetadataMaxUncompressed {
			s.logger.Warnf("Metadata payload exceeds %d bytes (%d bytes) but check_metadata is empty",
				limits.MetadataMaxUncompressed, len(payload))
		}
		return submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), s.jsonExtraHeadersWithCompression)
	}

	// Calculate base payload size (everything except check_metadata content)
	// Include batch metadata fields in the base size estimate
	basePayloadMap := make(map[string]interface{})
	for k, v := range payloadMap {
		if k != "check_metadata" {
			basePayloadMap[k] = v
		}
	}
	basePayloadMap["check_metadata"] = map[string]interface{}{}
	// Reserve space for batch metadata fields: "_dd_batch_index":N,"_dd_batch_total":N
	// Estimate ~50 bytes for these fields (conservative)
	basePayload, err := json.Marshal(basePayloadMap)
	if err != nil {
		s.logger.Warnf("Failed to marshal base payload for size estimation: %v", err)
		basePayload = []byte("{}")
	}
	baseSize := len(basePayload) + 50 // +50 for batch metadata fields

	// Build batches by iterating over check_metadata in sorted order for deterministic batching
	var batches []map[string]interface{}
	currentBatch := make(map[string]interface{})
	currentBatchSize := baseSize

	// Sort check names for deterministic batch composition
	checkNames := make([]string, 0, len(checkMetadata))
	for checkName := range checkMetadata {
		checkNames = append(checkNames, checkName)
	}
	sort.Strings(checkNames)

	for _, checkName := range checkNames {
		instances := checkMetadata[checkName]
		instancesSlice, ok := instances.([]interface{})
		if !ok {
			// Not a slice, add entire entry to current batch
			entryJSON, err := json.Marshal(map[string]interface{}{checkName: instances})
			if err != nil {
				s.logger.Warnf("Failed to marshal check entry %q for size estimation: %v", checkName, err)
				continue
			}
			entrySize := len(entryJSON)

			// Drop entries that exceed the max payload size - they can never fit in a batch
			if entrySize > limits.MetadataMaxUncompressed {
				s.logger.Warnf("Dropping check entry %q: size %d bytes exceeds maximum %d bytes",
					checkName, entrySize, limits.MetadataMaxUncompressed)
				continue
			}

			if currentBatchSize+entrySize > limits.MetadataTargetBatch && len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = make(map[string]interface{})
				currentBatchSize = baseSize
			}
			currentBatch[checkName] = instances
			currentBatchSize += entrySize
			continue
		}

		// Batch individual instances within this check
		var currentInstances []interface{}
		for _, instance := range instancesSlice {
			instanceJSON, err := json.Marshal(instance)
			if err != nil {
				s.logger.Warnf("Failed to marshal instance in check %q for size estimation: %v", checkName, err)
				continue
			}
			// Add overhead for JSON structure: "checkName":[ and ],
			instanceSize := len(instanceJSON) + len(checkName) + 10

			// Drop instances that exceed the max payload size - they can never fit in a batch
			if instanceSize > limits.MetadataMaxUncompressed {
				s.logger.Warnf("Dropping instance in check %q: size %d bytes exceeds maximum %d bytes",
					checkName, instanceSize, limits.MetadataMaxUncompressed)
				continue
			}

			if currentBatchSize+instanceSize > limits.MetadataTargetBatch {
				// Current batch is full
				if len(currentInstances) > 0 {
					currentBatch[checkName] = currentInstances
				}
				if len(currentBatch) > 0 {
					batches = append(batches, currentBatch)
					currentBatch = make(map[string]interface{})
					currentBatchSize = baseSize
				}
				currentInstances = nil
			}

			currentInstances = append(currentInstances, instance)
			currentBatchSize += instanceSize
		}

		if len(currentInstances) > 0 {
			currentBatch[checkName] = currentInstances
		}
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	// If no batches were created (e.g., all entries failed to marshal), send original payload
	if len(batches) == 0 {
		s.logger.Warnf("No batches created from check_metadata, sending original payload")
		compressedPayload, err := s.Strategy.Compress(payload)
		if err != nil {
			return fmt.Errorf("could not compress metadata payload: %s", err)
		}
		return submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&compressedPayload}), s.jsonExtraHeadersWithCompression)
	}

	// Build all batch payloads first to catch serialization errors before sending any
	type preparedBatch struct {
		json       []byte
		compressed []byte
	}
	preparedBatches := make([]preparedBatch, 0, len(batches))

	for i, batch := range batches {
		// Build the batch payload, preserving the original UUID and adding batch correlation metadata
		batchPayload := make(map[string]interface{})
		for k, v := range payloadMap {
			if k == "check_metadata" {
				batchPayload[k] = batch
			} else {
				// Preserve all original fields including UUID for backend correlation
				batchPayload[k] = v
			}
		}
		// Add batch metadata for backend correlation and reassembly
		// Use _dd_ prefix to avoid collision with user-defined fields
		batchPayload["_dd_batch_index"] = i
		batchPayload["_dd_batch_total"] = len(batches)

		batchJSON, err := json.Marshal(batchPayload)
		if err != nil {
			return fmt.Errorf("could not serialize metadata batch %d: %s", i+1, err)
		}

		batchCompressed, err := s.Strategy.Compress(batchJSON)
		if err != nil {
			return fmt.Errorf("could not compress metadata batch %d: %s", i+1, err)
		}

		if len(batchJSON) > limits.MetadataMaxUncompressed {
			s.logger.Warnf("Metadata batch %d/%d exceeds %d bytes (%d bytes), sending anyway",
				i+1, len(batches), limits.MetadataMaxUncompressed, len(batchJSON))
		}

		preparedBatches = append(preparedBatches, preparedBatch{json: batchJSON, compressed: batchCompressed})
	}

	// Send each batch
	s.logger.Infof("Splitting metadata payload (%d bytes) into %d batches", len(payload), len(batches))

	for i, pb := range preparedBatches {
		if err := submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&pb.compressed}), s.jsonExtraHeadersWithCompression); err != nil {
			return fmt.Errorf("failed to submit metadata batch %d/%d: %w", i+1, len(preparedBatches), err)
		}

		s.logger.Debugf("Sent metadata batch %d/%d, size (raw/compressed): %d/%d bytes", i+1, len(preparedBatches), len(pb.json), len(pb.compressed))
	}

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
