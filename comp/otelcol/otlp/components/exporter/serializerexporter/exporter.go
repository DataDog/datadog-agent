// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializerexporter contains the impleemntation of an exporter which is able
// to serialize OTLP Metrics to an agent demultiplexer.
package serializerexporter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
)

// Legacy DefaultForwarder defaults reproduced at the OTel exporterhelper layer
// so that turning on the UseSyncForwarder feature gate preserves the in-flight
// concurrency, queue depth, retry budget, and per-request timeout that the
// async forwarder enforced before. Tweak in lockstep with the corresponding
// forwarder_* settings in serializer.go::setupForwarder if those ever change.
const (
	// legacyForwarderQueueSize matches the sum of the legacy forwarder's
	// high_prio + low_prio + requeue buffers (100 each).
	legacyForwarderQueueSize = 300
	// legacyForwarderNumConsumers matches forwarder_num_workers = 1.
	legacyForwarderNumConsumers = 1
	// LegacyForwarderTimeout matches forwarder_timeout = 20 seconds. Exported
	// so callers outside this package (e.g. the DDOT otel-agent command) can
	// reuse the same fallback instead of duplicating the literal.
	LegacyForwarderTimeout = 20 * time.Second
	// legacyForwarderBackoffInitial / Max / MaxElapsed mirror the
	// forwarder_backoff_* and forwarder_retry_queue_capacity_time_interval_sec
	// settings: base 2s, factor 2, cap 64s, total 15 min budget.
	legacyForwarderBackoffInitial    = 2 * time.Second
	legacyForwarderBackoffMultiplier = 2
	legacyForwarderBackoffMax        = 64 * time.Second
	legacyForwarderRetryMaxElapsed   = 15 * time.Minute
)

func newDefaultConfig() component.Config {
	mcfg := MetricsConfig{
		APMStatsReceiverAddr: "http://localhost:8126/v0.6/stats",
		Tags:                 "",
	}
	pkgmcfg := datadogconfig.CreateDefaultConfig().(*datadogconfig.Config)
	mcfg.Metrics = pkgmcfg.Metrics

	return &ExporterConfig{
		QueueBatchConfig: configoptional.Some(exporterhelper.NewDefaultQueueConfig()),
		RetryConfig:      configretry.NewDefaultBackOffConfig(),
		HTTPConfig:       confighttp.NewDefaultClientConfig(),

		Metrics:      mcfg,
		API:          pkgmcfg.API,
		HostMetadata: pkgmcfg.HostMetadata,
	}
}

func newDefaultConfigForAgent() component.Config {
	cfg := newDefaultConfig().(*ExporterConfig)
	cfg.HostMetadata.Enabled = false

	// Mirror the legacy DefaultForwarder defaults so switching to the sync
	// forwarder path preserves in-flight concurrency, queue depth, backoff
	// budget, and per-request timeout. These are DDOT/agent-specific; the
	// OSS exporter uses OTel's own defaults.
	queue := exporterhelper.NewDefaultQueueConfig()
	queue.QueueSize = legacyForwarderQueueSize
	queue.NumConsumers = legacyForwarderNumConsumers
	cfg.QueueBatchConfig = configoptional.Some(queue)

	cfg.RetryConfig.InitialInterval = legacyForwarderBackoffInitial
	cfg.RetryConfig.Multiplier = legacyForwarderBackoffMultiplier
	cfg.RetryConfig.MaxInterval = legacyForwarderBackoffMax
	cfg.RetryConfig.MaxElapsedTime = legacyForwarderRetryMaxElapsed

	// TimeoutConfig controls the exporterhelper per-call deadline;
	// HTTPConfig.Timeout controls the underlying http.Client, which bounds the
	// TCP round-trip when the sync forwarder is in use (sendHTTPTransactions
	// does not propagate the caller context to t.Process).
	cfg.TimeoutConfig = exporterhelper.TimeoutConfig{Timeout: LegacyForwarderTimeout}
	cfg.HTTPConfig.Timeout = LegacyForwarderTimeout

	return cfg
}

// DefaultAgentRetryConfig returns a retry configuration that mirrors the
// legacy DefaultForwarder budget (2-64s exponential backoff over 15 min).
// DDOT uses this as the base when the user has not customized retry_on_failure,
// preserving the pre-sync-forwarder behavior.
func DefaultAgentRetryConfig() configretry.BackOffConfig {
	cfg := configretry.NewDefaultBackOffConfig()
	cfg.InitialInterval = legacyForwarderBackoffInitial
	cfg.Multiplier = float64(legacyForwarderBackoffMultiplier)
	cfg.MaxInterval = legacyForwarderBackoffMax
	cfg.MaxElapsedTime = legacyForwarderRetryMaxElapsed
	return cfg
}

// allSendsPermanent reports whether every constituent error in err wraps
// ErrPermanentHTTPError. consumer.Send returns a multierr-combined error
// (series + sketches + APM stats sends); we mark the whole flush permanent
// only when every sub-send failed with a non-retryable status code so that
// a transient failure in one pipeline does not suppress retries for another.
func allSendsPermanent(err error) bool {
	errs := multierr.Errors(err)
	if len(errs) == 0 {
		return errors.Is(err, defaultforwarderimpl.ErrPermanentHTTPError)
	}
	for _, e := range errs {
		if !errors.Is(e, defaultforwarderimpl.ErrPermanentHTTPError) {
			return false
		}
	}
	return true
}

var _ source.Provider = (*SourceProviderFunc)(nil)

// SourceProviderFunc is an adapter to allow the use of a function as a metrics.HostnameProvider.
type SourceProviderFunc func(context.Context) (string, error)

// Source calls f and wraps in a source struct.
func (f SourceProviderFunc) Source(ctx context.Context) (source.Source, error) {
	hostnameIdentifier, err := f(ctx)
	if err != nil {
		return source.Source{}, err
	}

	return source.Source{Kind: source.HostnameKind, Identifier: source.Identifier{Primary: hostnameIdentifier}}, nil
}

// Exporter translate OTLP metrics into the Datadog format and sends
// them to the agent serializer.
type Exporter struct {
	tr                metrics.Provider
	s                 serializer.MetricSerializer
	hostGetter        SourceProviderFunc
	extraTags         []string
	apmReceiverAddr   string
	createConsumer    createConsumerFunc
	params            exporter.Settings
	hostmetadata      datadogconfig.HostMetadataConfig
	reporter          *inframetadata.Reporter
	gatewayUsage      otel.GatewayUsage
	coatUsageMetric   telemetry.Gauge
	coatGWUsageMetric telemetry.Gauge
	ipath             ingestionPath
}

// TODO: expose the same function in OSS exporter and remove this
func translatorFromConfig(
	set component.TelemetrySettings,
	attributesTranslator *attributes.Translator,
	cfg datadogconfig.MetricsConfig,
	hostGetter SourceProviderFunc,
	statsIn chan []byte,
	extraOptions ...metrics.TranslatorOption,
) (metrics.Provider, error) {
	histogramMode := metrics.HistogramMode(cfg.HistConfig.Mode)
	switch histogramMode {
	case metrics.HistogramModeCounters, metrics.HistogramModeNoBuckets, metrics.HistogramModeDistributions:
		// Do nothing
	default:
		return nil, fmt.Errorf("invalid `mode` %q", cfg.HistConfig.Mode)
	}
	options := []metrics.TranslatorOption{
		metrics.WithFallbackSourceProvider(hostGetter),
		metrics.WithHistogramMode(histogramMode),
		metrics.WithDeltaTTL(cfg.DeltaTTL),
	}
	options = append(options, extraOptions...)

	if statsIn != nil {
		options = append(options, metrics.WithStatsOut(statsIn))
	}

	if cfg.HistConfig.SendAggregations {
		options = append(options, metrics.WithHistogramAggregations())
	}

	switch cfg.SummaryConfig.Mode {
	case datadogconfig.SummaryModeGauges:
		options = append(options, metrics.WithQuantiles())
	}

	if cfg.ExporterConfig.InstrumentationScopeMetadataAsTags {
		options = append(options, metrics.WithInstrumentationScopeMetadataAsTags())
	}

	var numberMode metrics.NumberMode
	switch cfg.SumConfig.CumulativeMonotonicMode {
	case datadogconfig.CumulativeMonotonicSumModeRawValue:
		numberMode = metrics.NumberModeRawValue
	case datadogconfig.CumulativeMonotonicSumModeToDelta:
		numberMode = metrics.NumberModeCumulativeToDelta
	}
	options = append(options, metrics.WithNumberMode(numberMode))
	options = append(options, metrics.WithInitialCumulMonoValueMode(
		metrics.InitialCumulMonoValueMode(cfg.SumConfig.InitialCumulativeMonotonicMode)))

	return metrics.NewDefaultTranslator(set, attributesTranslator, options...)
}

// NewExporter creates a new exporter that translates OTLP metrics into the Datadog format and sends
func NewExporter(
	s serializer.MetricSerializer,
	cfg *ExporterConfig,
	hostGetter SourceProviderFunc,
	createConsumer createConsumerFunc,
	tr metrics.Provider,
	params exporter.Settings,
	reporter *inframetadata.Reporter,
	gatewayUsage otel.GatewayUsage,
	coatUsageMetric telemetry.Gauge,
	coatGWUsageMetric telemetry.Gauge,
	ipath ingestionPath,
) (*Exporter, error) {
	var extraTags []string
	if cfg.Metrics.Tags != "" {
		extraTags = strings.Split(cfg.Metrics.Tags, ",")
	}
	params.Logger.Info("serializer exporter configuration", zap.Bool("host_metadata_enabled", cfg.HostMetadata.Enabled),
		zap.Strings("extra_tags", extraTags),
		zap.String("apm_receiver_url", cfg.Metrics.APMStatsReceiverAddr),
		zap.String("histogram_mode", fmt.Sprintf("%v", cfg.Metrics.Metrics.HistConfig.Mode)))
	return &Exporter{
		tr:                tr,
		s:                 s,
		hostGetter:        hostGetter,
		apmReceiverAddr:   cfg.Metrics.APMStatsReceiverAddr,
		extraTags:         extraTags,
		createConsumer:    createConsumer,
		params:            params,
		hostmetadata:      cfg.HostMetadata,
		reporter:          reporter,
		gatewayUsage:      gatewayUsage,
		coatUsageMetric:   coatUsageMetric,
		coatGWUsageMetric: coatGWUsageMetric,
		ipath:             ipath,
	}, nil
}

// ConsumeMetrics translates OTLP metrics into the Datadog format and sends
func (e *Exporter) ConsumeMetrics(ctx context.Context, ld pmetric.Metrics) error {

	// Track requests based on ingestion path
	switch e.ipath {
	case agentOTLPIngest:
		OTLPIngestAgentMetricsRequests.Inc()
		OTLPIngestAgentMetricsEvents.Add(float64(ld.MetricCount()))
	case ddot:
		OTLPIngestDDOTMetricsRequests.Inc()
		OTLPIngestDDOTMetricsEvents.Add(float64(ld.MetricCount()))
	}
	if e.hostmetadata.Enabled {
		if err := e.reporter.ConsumeMetrics(ld); err != nil {
			e.params.Logger.Warn("failed to consume metrics for host metadata", zap.Error(err))
		}
	}
	consumer := e.createConsumer(e.extraTags, e.apmReceiverAddr, e.params.BuildInfo)
	rmt, err := e.tr.MapMetrics(ctx, ld, consumer, e.gatewayUsage.GetHostFromAttributesHandler())
	if err != nil {
		return err
	}
	hostname, err := e.hostGetter(ctx)
	if err != nil {
		return err
	}

	consumer.addTelemetryMetric(hostname, e.params, e.coatUsageMetric)
	consumer.addRuntimeTelemetryMetric(hostname, rmt.Languages)
	consumer.addGatewayUsage(hostname, e.params, e.gatewayUsage, e.coatGWUsageMetric)
	if err := consumer.Send(e.s); err != nil {
		errFlush := fmt.Errorf("failed to flush metrics: %w", err)
		if allSendsPermanent(err) {
			// All constituent send errors are permanent (400/413/403). Signal
			// exporterhelper to drop rather than queue and retry.
			// When consumer.Send combines permanent and transient errors (e.g.
			// series→400 but sketches→503), we fall through so exporterhelper
			// retries the transient failures.
			return consumererror.NewPermanent(errFlush)
		}
		return errFlush
	}
	return nil
}
