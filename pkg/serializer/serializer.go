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
	if s.config.IsConfigured("multi_region_failover.metric_allowlist") {
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

// DirectMetricsOptions carries producer-side callbacks that normally run from
// metrics.IterableSeries/IterableSketches.Append. The direct serializer
// experiment uses them before serializing each row so host tags, logging, and
// telemetry remain comparable with the current path.
type DirectMetricsOptions struct {
	SeriesCallback           func(*metrics.Serie)
	SeriesRowCallback        func(*metrics.SerieRow)
	V3MetricPointRowCallback func(*metrics.V3MetricPointRow)
	SketchCallback           func(*metrics.SketchSeries)
}

// DirectMetricsResult summarizes an experimental direct series/sketch flush.
type DirectMetricsResult struct {
	SeriesEnabled   bool
	SketchesEnabled bool
	SeriesCount     uint64
	SketchesCount   uint64
	SeriesErr       error
	SketchesErr     error
}

type directSeriesCallbackSink struct {
	sink               metrics.SerieSink
	callback           func(*metrics.Serie)
	rowCallback        func(*metrics.SerieRow)
	v3PointRowCallback func(*metrics.V3MetricPointRow)
}

func (s directSeriesCallbackSink) Append(serie *metrics.Serie) {
	if s.callback != nil {
		s.callback(serie)
	}
	s.sink.Append(serie)
}

func (s directSeriesCallbackSink) AppendSerieRow(row metrics.SerieRow) {
	if s.rowCallback != nil {
		s.rowCallback(&row)
	} else if s.callback != nil {
		serie := row.ToSerie()
		s.callback(serie)
		row = metrics.SerieRowFromSerie(serie)
	}

	if rowSink, ok := s.sink.(metrics.SerieRowSink); ok {
		rowSink.AppendSerieRow(row)
		return
	}
	s.sink.Append(row.ToSerie())
}

func (s directSeriesCallbackSink) AppendV3MetricPointRow(row *metrics.V3MetricPointRow) {
	if row == nil {
		return
	}
	if s.v3PointRowCallback != nil {
		s.v3PointRowCallback(row)
	} else if s.rowCallback != nil {
		serieRow := row.ToSerieRow()
		s.rowCallback(&serieRow)
		updateV3MetricPointRowFromSerieRow(row, serieRow)
	} else if s.callback != nil {
		serieRow := row.ToSerieRow()
		serie := serieRow.ToSerie()
		s.callback(serie)
		serieRow = metrics.SerieRowFromSerie(serie)
		updateV3MetricPointRowFromSerieRow(row, serieRow)
	}

	if pointSink, ok := s.sink.(metrics.V3MetricPointRowSink); ok {
		pointSink.AppendV3MetricPointRow(row)
		return
	}
	if rowSink, ok := s.sink.(metrics.SerieRowSink); ok {
		rowSink.AppendSerieRow(row.ToSerieRow())
		return
	}
	s.sink.Append(row.ToSerieRow().ToSerie())
}

func updateV3MetricPointRowFromSerieRow(row *metrics.V3MetricPointRow, serieRow metrics.SerieRow) {
	if row == nil {
		return
	}
	row.Timestamps = row.Timestamps[:0]
	row.Values = row.Values[:0]
	if len(serieRow.Points) > 0 {
		row.Timestamp = int64(serieRow.Points[0].Ts)
		row.Value = serieRow.Points[0].Value
	}
	if len(serieRow.Points) > 1 {
		for _, point := range serieRow.Points {
			row.Timestamps = append(row.Timestamps, int64(point.Ts))
			row.Values = append(row.Values, point.Value)
		}
	}
	row.Tags = serieRow.Tags
	row.Host = serieRow.Host
	row.Device = serieRow.Device
	row.MType = serieRow.MType
	row.Interval = serieRow.Interval
	row.SourceTypeName = serieRow.SourceTypeName
	row.Unit = serieRow.Unit
	row.NoIndex = serieRow.NoIndex
	row.Resources = serieRow.Resources
	row.Source = serieRow.Source
}

type directSketchCallbackSink struct {
	sink     metrics.SketchesSink
	callback func(*metrics.SketchSeries)
}

func (s directSketchCallbackSink) Append(sketch *metrics.SketchSeries) {
	if s.callback != nil {
		s.callback(sketch)
	}
	s.sink.Append(sketch)
}

type directNoopSeriesSink struct{}

func (directNoopSeriesSink) Append(*metrics.Serie) {}

type directNoopSketchSink struct{}

func (directNoopSketchSink) Append(*metrics.SketchSeries) {}

func onlyV3Pipelines(pipelines metricsserializer.PipelineSet) metricsserializer.PipelineSet {
	v3Pipelines := metricsserializer.PipelineSet{}
	for conf, ctx := range pipelines {
		if conf.V3 {
			v3Pipelines[conf] = ctx
		}
	}
	return v3Pipelines
}

// SendDirectV3SeriesRows is an intentionally experimental local-only path for
// the DogStatsD columnar v3 vertical slice. The producer emits serializer-visible
// rows directly into v3 protobuf builders; v2 and JSON series payloads are not
// produced by this method.
func (s *Serializer) SendDirectV3SeriesRows(
	producer func(metrics.SerieRowSink),
	options DirectMetricsOptions,
) DirectMetricsResult {
	result := DirectMetricsResult{SeriesEnabled: s.AreSeriesEnabled()}
	if !result.SeriesEnabled {
		return result
	}
	if !s.config.GetBool("use_v2_api.series") {
		result.SeriesErr = fmt.Errorf("direct v3 row serializer experiment requires use_v2_api.series=true")
		return result
	}

	seriesPipelines := onlyV3Pipelines(s.buildPipelines(metricsKindSeries))
	if len(seriesPipelines) == 0 {
		result.SeriesErr = fmt.Errorf("direct v3 row serializer experiment requires at least one v3 series pipeline")
		return result
	}

	seriesDirectSink, err := metricsserializer.NewDirectSeriesSink(s.config, s.Strategy, seriesPipelines)
	if err != nil {
		result.SeriesErr = fmt.Errorf("creating direct v3 series row sink: %w", err)
		return result
	}

	var seriesSink metrics.SerieRowSink = seriesDirectSink
	if options.SeriesCallback != nil || options.SeriesRowCallback != nil || options.V3MetricPointRowCallback != nil {
		seriesSink = directSeriesCallbackSink{sink: seriesDirectSink, callback: options.SeriesCallback, rowCallback: options.SeriesRowCallback, v3PointRowCallback: options.V3MetricPointRowCallback}
	}

	producer(seriesSink)

	result.SeriesCount, result.SeriesErr = seriesDirectSink.Finish()
	if result.SeriesErr == nil {
		result.SeriesErr = seriesPipelines.Send(s.Forwarder, s.protobufExtraHeadersWithCompression)
	}
	return result
}

// SendDirectV3MetricPointRows is an intentionally experimental local-only path
// for the DogStatsD columnar v3 vertical slice. The producer emits single-point
// v3 rows directly into v3 protobuf builders; v2 and JSON series payloads are
// not produced by this method.
func (s *Serializer) SendDirectV3MetricPointRows(
	producer func(metrics.V3MetricPointRowSink),
	options DirectMetricsOptions,
) DirectMetricsResult {
	result := DirectMetricsResult{SeriesEnabled: s.AreSeriesEnabled()}
	if !result.SeriesEnabled {
		return result
	}
	if !s.config.GetBool("use_v2_api.series") {
		result.SeriesErr = fmt.Errorf("direct v3 metric point row serializer experiment requires use_v2_api.series=true")
		return result
	}

	seriesPipelines := onlyV3Pipelines(s.buildPipelines(metricsKindSeries))
	if len(seriesPipelines) == 0 {
		result.SeriesErr = fmt.Errorf("direct v3 metric point row serializer experiment requires at least one v3 series pipeline")
		return result
	}

	seriesDirectSink, err := metricsserializer.NewDirectSeriesSink(s.config, s.Strategy, seriesPipelines)
	if err != nil {
		result.SeriesErr = fmt.Errorf("creating direct v3 metric point row sink: %w", err)
		return result
	}

	var seriesSink metrics.V3MetricPointRowSink = seriesDirectSink
	if options.SeriesCallback != nil || options.SeriesRowCallback != nil || options.V3MetricPointRowCallback != nil {
		seriesSink = directSeriesCallbackSink{sink: seriesDirectSink, callback: options.SeriesCallback, rowCallback: options.SeriesRowCallback, v3PointRowCallback: options.V3MetricPointRowCallback}
	}

	producer(seriesSink)

	result.SeriesCount, result.SeriesErr = seriesDirectSink.Finish()
	if result.SeriesErr == nil {
		result.SeriesErr = seriesPipelines.Send(s.Forwarder, s.protobufExtraHeadersWithCompression)
	}
	return result
}

// SendDirectSeriesAndSketches is an intentionally experimental local-only path
// that lets the aggregator produce directly into serializer pipeline builders,
// bypassing IterableSeries/IterableSketches channels and consumer goroutines.
// It currently supports the v2/v3 protobuf metric APIs; JSON v1 series are not
// implemented for this experiment.
func (s *Serializer) SendDirectSeriesAndSketches(
	producer func(metrics.SerieSink, metrics.SketchesSink),
	options DirectMetricsOptions,
) DirectMetricsResult {
	result := DirectMetricsResult{
		SeriesEnabled:   s.AreSeriesEnabled(),
		SketchesEnabled: s.AreSketchesEnabled(),
	}

	var seriesSink metrics.SerieSink = directNoopSeriesSink{}
	var seriesDirectSink *metricsserializer.DirectSeriesSink
	var seriesPipelines metricsserializer.PipelineSet
	if result.SeriesEnabled {
		if !s.config.GetBool("use_v2_api.series") {
			result.SeriesErr = fmt.Errorf("direct series serializer experiment requires use_v2_api.series=true")
			return result
		}

		seriesPipelines = s.buildPipelines(metricsKindSeries)
		var err error
		seriesDirectSink, err = metricsserializer.NewDirectSeriesSink(s.config, s.Strategy, seriesPipelines)
		if err != nil {
			result.SeriesErr = fmt.Errorf("creating direct series sink: %w", err)
			return result
		}
		seriesSink = seriesDirectSink
	}
	if options.SeriesCallback != nil || options.SeriesRowCallback != nil || options.V3MetricPointRowCallback != nil {
		seriesSink = directSeriesCallbackSink{sink: seriesSink, callback: options.SeriesCallback, rowCallback: options.SeriesRowCallback, v3PointRowCallback: options.V3MetricPointRowCallback}
	}

	var sketchesSink metrics.SketchesSink = directNoopSketchSink{}
	var sketchesDirectSink *metricsserializer.DirectSketchSink
	var sketchesPipelines metricsserializer.PipelineSet
	if result.SketchesEnabled {
		sketchesPipelines = s.buildPipelines(metricsKindSketches)
		var err error
		sketchesDirectSink, err = metricsserializer.NewDirectSketchSink(s.config, s.Strategy, sketchesPipelines, s.logger)
		if err != nil {
			result.SketchesErr = fmt.Errorf("creating direct sketches sink: %w", err)
			return result
		}
		sketchesSink = sketchesDirectSink
	}
	if options.SketchCallback != nil {
		sketchesSink = directSketchCallbackSink{sink: sketchesSink, callback: options.SketchCallback}
	}

	producer(seriesSink, sketchesSink)

	if seriesDirectSink != nil {
		result.SeriesCount, result.SeriesErr = seriesDirectSink.Finish()
		if result.SeriesErr == nil {
			result.SeriesErr = seriesPipelines.Send(s.Forwarder, s.protobufExtraHeadersWithCompression)
		}
	}
	if sketchesDirectSink != nil {
		result.SketchesCount, result.SketchesErr = sketchesDirectSink.Finish()
		if result.SketchesErr == nil {
			result.SketchesErr = sketchesPipelines.Send(s.Forwarder, s.protobufExtraHeadersWithCompression)
		}
	}

	return result
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
