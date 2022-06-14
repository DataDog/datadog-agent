// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

var _ config.Exporter = (*exporterConfig)(nil)

func newDefaultConfig() config.Exporter {
	return &exporterConfig{
		ExporterSettings: config.NewExporterSettings(config.NewComponentID(TypeStr)),
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
			TagCardinality: collectors.LowCardinalityString,
			HistConfig: histogramConfig{
				Mode:         "distributions",
				SendCountSum: false,
			},
			SumConfig: sumConfig{
				CumulativeMonotonicMode: CumulativeMonotonicSumModeToDelta,
			},
			SummaryConfig: summaryConfig{
				Mode: SummaryModeGauges,
			},
		},
	}
}

var _ translator.HostnameProvider = (*hostnameProviderFunc)(nil)

// hostnameProviderFunc is an adapter to allow the use of a function as a translator.HostnameProvider.
type hostnameProviderFunc func(context.Context) (string, error)

// Hostname calls f.
func (f hostnameProviderFunc) Hostname(ctx context.Context) (string, error) {
	return f(ctx)
}

// exporter translate OTLP metrics into the Datadog format and sends
// them to the agent serializer.
type exporter struct {
	tr          *translator.Translator
	s           serializer.MetricSerializer
	hostname    string
	extraTags   []string
	cardinality collectors.TagCardinality
}

func translatorFromConfig(logger *zap.Logger, cfg *exporterConfig) (*translator.Translator, error) {
	histogramMode := translator.HistogramMode(cfg.Metrics.HistConfig.Mode)
	switch histogramMode {
	case translator.HistogramModeCounters, translator.HistogramModeNoBuckets, translator.HistogramModeDistributions:
		// Do nothing
	default:
		return nil, fmt.Errorf("invalid `mode` %q", cfg.Metrics.HistConfig.Mode)
	}

	options := []translator.Option{
		translator.WithFallbackHostnameProvider(hostnameProviderFunc(util.GetHostname)),
		translator.WithHistogramMode(histogramMode),
		translator.WithDeltaTTL(cfg.Metrics.DeltaTTL),
	}

	if cfg.Metrics.HistConfig.SendCountSum {
		options = append(options, translator.WithCountSumMetrics())
	}

	switch cfg.Metrics.SummaryConfig.Mode {
	case SummaryModeGauges:
		options = append(options, translator.WithQuantiles())
	}

	if cfg.Metrics.ExporterConfig.ResourceAttributesAsTags {
		options = append(options, translator.WithResourceAttributesAsTags())
	}

	if cfg.Metrics.ExporterConfig.InstrumentationLibraryMetadataAsTags && cfg.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags {
		return nil, fmt.Errorf("cannot use both instrumentation_library_metadata_as_tags(deprecated) and instrumentation_scope_metadata_as_tags")
	}

	if cfg.Metrics.ExporterConfig.InstrumentationLibraryMetadataAsTags {
		options = append(options, translator.WithInstrumentationLibraryMetadataAsTags())
	}

	if cfg.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags {
		options = append(options, translator.WithInstrumentationLibraryMetadataAsTags())
	}

	var numberMode translator.NumberMode
	switch cfg.Metrics.SumConfig.CumulativeMonotonicMode {
	case CumulativeMonotonicSumModeRawValue:
		numberMode = translator.NumberModeRawValue
	case CumulativeMonotonicSumModeToDelta:
		numberMode = translator.NumberModeCumulativeToDelta
	}
	options = append(options, translator.WithNumberMode(numberMode))

	return translator.New(logger, options...)
}

func newExporter(logger *zap.Logger, s serializer.MetricSerializer, cfg *exporterConfig) (*exporter, error) {
	tr, err := translatorFromConfig(logger, cfg)
	if err != nil {
		return nil, fmt.Errorf("incorrect OTLP metrics configuration: %w", err)
	}

	hostname, err := util.GetHostname(context.TODO())
	if err != nil {
		return nil, err
	}

	cardinality, err := collectors.StringToTagCardinality(cfg.Metrics.TagCardinality)
	if err != nil {
		return nil, err
	}

	var extraTags []string

	// if the server is running in a context where static tags are required, add those
	// to extraTags.
	if tags := util.GetStaticTagsSlice(context.TODO()); tags != nil {
		extraTags = append(extraTags, tags...)
	}

	return &exporter{
		tr:          tr,
		s:           s,
		hostname:    hostname,
		extraTags:   extraTags,
		cardinality: cardinality,
	}, nil
}

func (e *exporter) ConsumeMetrics(ctx context.Context, ld pmetric.Metrics) error {
	consumer := &serializerConsumer{cardinality: e.cardinality, extraTags: e.extraTags}
	err := e.tr.MapMetrics(ctx, ld, consumer)
	if err != nil {
		return err
	}

	consumer.addTelemetryMetric(e.hostname)
	if err := consumer.flush(e.s); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}
	return nil
}
