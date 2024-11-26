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

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func newDefaultConfig() component.Config {
	mcfg := MetricsConfig{
		TagCardinality:       "low",
		APMStatsReceiverAddr: "http://localhost:8126/v0.6/stats",
		Tags:                 "",
	}
	pkgmcfg := datadogconfig.CreateDefaultConfig().(*datadogconfig.Config).Metrics
	mcfg.Metrics = pkgmcfg

	return &ExporterConfig{
		// Disable timeout; we don't really do HTTP requests on the ConsumeMetrics call.
		TimeoutConfig: exporterhelper.TimeoutConfig{Timeout: 0},
		// TODO (AP-1294): Fine-tune queue settings and look into retry settings.
		QueueConfig: exporterhelper.NewDefaultQueueConfig(),

		Metrics: mcfg,
	}
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
	enricher        tagenricher
	apmReceiverAddr string
}

// TODO: expose the same function in OSS exporter and remove this
func translatorFromConfig(
	set component.TelemetrySettings,
	attributesTranslator *attributes.Translator,
	cfg datadogconfig.MetricsConfig,
	hostGetter SourceProviderFunc,
	statsIn chan []byte,
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
		metrics.WithOTelPrefix(),
	}

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
		options = append(options, metrics.WithInstrumentationLibraryMetadataAsTags())
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
	set component.TelemetrySettings,
	attributesTranslator *attributes.Translator,
	s serializer.MetricSerializer,
	cfg *ExporterConfig,
	enricher tagenricher,
	hostGetter SourceProviderFunc,
	statsIn chan []byte,
) (*Exporter, error) {
	tr, err := translatorFromConfig(set, attributesTranslator, cfg.Metrics.Metrics, hostGetter, statsIn)
	if err != nil {
		return nil, fmt.Errorf("incorrect OTLP metrics configuration: %w", err)
	}

	err = enricher.SetCardinality(cfg.Metrics.TagCardinality)
	if err != nil {
		return nil, err
	}
	var extraTags []string
	if cfg.Metrics.Tags != "" {
		extraTags = strings.Split(cfg.Metrics.Tags, ",")
	}
	return &Exporter{
		tr:              tr,
		s:               s,
		hostGetter:      hostGetter,
		enricher:        enricher,
		apmReceiverAddr: cfg.Metrics.APMStatsReceiverAddr,
		extraTags:       extraTags,
	}, nil
}

// ConsumeMetrics translates OTLP metrics into the Datadog format and sends
func (e *Exporter) ConsumeMetrics(ctx context.Context, ld pmetric.Metrics) error {
	consumer := &serializerConsumer{enricher: e.enricher, extraTags: e.extraTags, apmReceiverAddr: e.apmReceiverAddr}
	rmt, err := e.tr.MapMetrics(ctx, ld, consumer)
	if err != nil {
		return err
	}
	hostname, err := e.hostGetter(ctx)
	if err != nil {
		return err
	}

	consumer.addTelemetryMetric(hostname)
	consumer.addRuntimeTelemetryMetric(hostname, rmt.Languages)
	if err := consumer.Send(e.s); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}
	return nil
}
