// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializerexporter contains the impleemntation of an exporter which is able
// to serialize OTLP Metrics to an agent demultiplexer.
package serializerexporter

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
)

func newDefaultConfig() component.Config {
	mcfg := MetricsConfig{
		APMStatsReceiverAddr: "http://localhost:8126/v0.6/stats",
		Tags:                 "",
	}
	pkgmcfg := datadogconfig.CreateDefaultConfig().(*datadogconfig.Config)
	mcfg.Metrics = pkgmcfg.Metrics

	return &ExporterConfig{
		// Disable timeout; we don't really do HTTP requests on the ConsumeMetrics call.
		TimeoutConfig: exporterhelper.TimeoutConfig{Timeout: 0},
		// TODO (AP-1294): Fine-tune queue settings and look into retry settings.
		QueueBatchConfig: exporterhelper.NewDefaultQueueConfig(),

		Metrics:      mcfg,
		API:          pkgmcfg.API,
		HostMetadata: pkgmcfg.HostMetadata,
	}
}

func newDefaultConfigForAgent() component.Config {
	cfg := newDefaultConfig().(*ExporterConfig)
	cfg.HostMetadata.Enabled = false
	return cfg
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

	return source.Source{Kind: source.HostnameKind, Identifier: hostnameIdentifier}, nil
}

// Exporter translate OTLP metrics into the Datadog format and sends
// them to the agent serializer.
type Exporter struct {
	tr              *metrics.Translator
	s               serializer.MetricSerializer
	hostGetter      SourceProviderFunc
	extraTags       []string
	apmReceiverAddr string
	createConsumer  createConsumerFunc
	params          exporter.Settings
	hostmetadata    datadogconfig.HostMetadataConfig
	reporter        *inframetadata.Reporter
	gatewayUsage    otel.GatewayUsage
	usageMetric     telemetry.Gauge
}

// TODO: expose the same function in OSS exporter and remove this
func translatorFromConfig(
	set component.TelemetrySettings,
	attributesTranslator *attributes.Translator,
	cfg datadogconfig.MetricsConfig,
	hostGetter SourceProviderFunc,
	statsIn chan []byte,
	extraOptions ...metrics.TranslatorOption,
) (*metrics.Translator, error) {
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

	return metrics.NewTranslator(set, attributesTranslator, options...)
}

// NewExporter creates a new exporter that translates OTLP metrics into the Datadog format and sends
func NewExporter(
	s serializer.MetricSerializer,
	cfg *ExporterConfig,
	hostGetter SourceProviderFunc,
	createConsumer createConsumerFunc,
	tr *metrics.Translator,
	params exporter.Settings,
	reporter *inframetadata.Reporter,
	gatewayUsage otel.GatewayUsage,
	usageMetric telemetry.Gauge,
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
		tr:              tr,
		s:               s,
		hostGetter:      hostGetter,
		apmReceiverAddr: cfg.Metrics.APMStatsReceiverAddr,
		extraTags:       extraTags,
		createConsumer:  createConsumer,
		params:          params,
		hostmetadata:    cfg.HostMetadata,
		reporter:        reporter,
		gatewayUsage:    gatewayUsage,
		usageMetric:     usageMetric,
	}, nil
}

// ConsumeMetrics translates OTLP metrics into the Datadog format and sends
func (e *Exporter) ConsumeMetrics(ctx context.Context, ld pmetric.Metrics) error {
	if e.hostmetadata.Enabled {
		// Consume resources for host metadata
		for i := 0; i < ld.ResourceMetrics().Len(); i++ {
			res := ld.ResourceMetrics().At(i).Resource()
			e.consumeResource(e.reporter, res)
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

	consumer.addTelemetryMetric(hostname, e.params, e.usageMetric)
	consumer.addRuntimeTelemetryMetric(hostname, rmt.Languages)
	consumer.addGatewayUsage(hostname, e.gatewayUsage)
	if err := consumer.Send(e.s); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}
	return nil
}

func (e *Exporter) consumeResource(metadataReporter *inframetadata.Reporter, res pcommon.Resource) {
	if err := metadataReporter.ConsumeResource(res); err != nil {
		e.params.Logger.Warn("failed to consume resource for host metadata", zap.Error(err), zap.Any("resource", res))
	}
}
