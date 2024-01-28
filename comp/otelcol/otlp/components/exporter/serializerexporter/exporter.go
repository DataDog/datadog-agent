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

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

var _ component.Config = (*exporterConfig)(nil)

func newDefaultConfig() component.Config {
	return &exporterConfig{
		// Disable timeout; we don't really do HTTP requests on the ConsumeMetrics call.
		TimeoutSettings: exporterhelper.TimeoutSettings{Timeout: 0},
		// TODO (AP-1294): Fine-tune queue settings and look into retry settings.
		QueueSettings: exporterhelper.NewDefaultQueueSettings(),

		Metrics: metricsConfig{
			DeltaTTL: 3600,
			ExporterConfig: metricsExporterConfig{
				ResourceAttributesAsTags:             false,
				InstrumentationLibraryMetadataAsTags: false,
				InstrumentationScopeMetadataAsTags:   false,
			},
			TagCardinality: "low",
			HistConfig: histogramConfig{
				Mode:             "distributions",
				SendAggregations: false,
			},
			SumConfig: sumConfig{
				CumulativeMonotonicMode:        CumulativeMonotonicSumModeToDelta,
				InitialCumulativeMonotonicMode: InitialValueModeAuto,
			},
			SummaryConfig: summaryConfig{
				Mode: SummaryModeGauges,
			},
		},
	}
}

var _ source.Provider = (*sourceProviderFunc)(nil)

// sourceProviderFunc is an adapter to allow the use of a function as a metrics.HostnameProvider.
type sourceProviderFunc func(context.Context) (string, error)

// Source calls f and wraps in a source struct.
func (f sourceProviderFunc) Source(ctx context.Context) (source.Source, error) {
	hostnameIdentifier, err := f(ctx)
	if err != nil {
		return source.Source{}, err
	}

	return source.Source{Kind: source.HostnameKind, Identifier: hostnameIdentifier}, nil
}

// exporter translate OTLP metrics into the Datadog format and sends
// them to the agent serializer.
type exporter struct {
	tr              *metrics.Translator
	s               serializer.MetricSerializer
	hostname        string
	extraTags       []string
	enricher        tagenricher
	apmReceiverAddr string
}

func translatorFromConfig(set component.TelemetrySettings, attributesTranslator *attributes.Translator, cfg *exporterConfig, hostname string) (*metrics.Translator, error) {
	histogramMode := metrics.HistogramMode(cfg.Metrics.HistConfig.Mode)
	switch histogramMode {
	case metrics.HistogramModeCounters, metrics.HistogramModeNoBuckets, metrics.HistogramModeDistributions:
		// Do nothing
	default:
		return nil, fmt.Errorf("invalid `mode` %q", cfg.Metrics.HistConfig.Mode)
	}

	options := []metrics.TranslatorOption{
		metrics.WithFallbackSourceProvider(sourceProviderFunc(func(_ context.Context) (string, error) {
			return hostname, nil
		})),
		metrics.WithHistogramMode(histogramMode),
		metrics.WithDeltaTTL(cfg.Metrics.DeltaTTL),
	}

	if cfg.Metrics.HistConfig.SendAggregations {
		options = append(options, metrics.WithHistogramAggregations())
	}

	switch cfg.Metrics.SummaryConfig.Mode {
	case SummaryModeGauges:
		options = append(options, metrics.WithQuantiles())
	}

	if cfg.Metrics.ExporterConfig.InstrumentationLibraryMetadataAsTags && cfg.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags {
		return nil, fmt.Errorf("cannot use both instrumentation_library_metadata_as_tags(deprecated) and instrumentation_scope_metadata_as_tags")
	}

	if cfg.Metrics.ExporterConfig.InstrumentationLibraryMetadataAsTags {
		options = append(options, metrics.WithInstrumentationLibraryMetadataAsTags())
	}

	if cfg.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags {
		options = append(options, metrics.WithInstrumentationLibraryMetadataAsTags())
	}

	var numberMode metrics.NumberMode
	switch cfg.Metrics.SumConfig.CumulativeMonotonicMode {
	case CumulativeMonotonicSumModeRawValue:
		numberMode = metrics.NumberModeRawValue
	case CumulativeMonotonicSumModeToDelta:
		numberMode = metrics.NumberModeCumulativeToDelta
	}
	options = append(options, metrics.WithNumberMode(numberMode))
	options = append(options, metrics.WithInitialCumulMonoValueMode(
		metrics.InitialCumulMonoValueMode(cfg.Metrics.SumConfig.InitialCumulativeMonotonicMode)))

	return metrics.NewTranslator(set, attributesTranslator, options...)
}

func newExporter(set component.TelemetrySettings, attributesTranslator *attributes.Translator, s serializer.MetricSerializer, cfg *exporterConfig, enricher tagenricher, apmReceiverAddr string, extraTags []string, hostname string) (*exporter, error) {
	// Log any warnings from unmarshaling.
	for _, warning := range cfg.warnings {
		set.Logger.Warn(warning)
	}

	tr, err := translatorFromConfig(set, attributesTranslator, cfg, hostname)
	if err != nil {
		return nil, fmt.Errorf("incorrect OTLP metrics configuration: %w", err)
	}

	err = enricher.SetCardinality(cfg.Metrics.TagCardinality)
	if err != nil {
		return nil, err
	}

	return &exporter{
		tr:              tr,
		s:               s,
		hostname:        hostname,
		enricher:        enricher,
		apmReceiverAddr: apmReceiverAddr,
		extraTags:       extraTags,
	}, nil
}

func (e *exporter) ConsumeMetrics(ctx context.Context, ld pmetric.Metrics) error {
	consumer := &serializerConsumer{enricher: e.enricher, extraTags: e.extraTags, apmReceiverAddr: e.apmReceiverAddr}
	rmt, err := e.tr.MapMetrics(ctx, ld, consumer)
	if err != nil {
		return err
	}

	consumer.addTelemetryMetric(e.hostname)
	consumer.addRuntimeTelemetryMetric(e.hostname, rmt.Languages)
	if err := consumer.Send(e.s); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}
	return nil
}
