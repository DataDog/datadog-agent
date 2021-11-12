package serializerexporter

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/model/pdata"
	"go.uber.org/zap"
)

var _ config.Exporter = (*exporterConfig)(nil)

func newDefaultConfig() config.Exporter {
	return &exporterConfig{
		ExporterSettings: config.NewExporterSettings(config.NewComponentID(TypeStr)),
		// Disable timeout; we don't really do HTTP requests on the ConsumeMetrics call.
		TimeoutSettings: exporterhelper.TimeoutSettings{Timeout: 0},
		// TODO (AP-1294): Fine-tune queue settings and look into retry settings.
		QueueSettings: exporterhelper.DefaultQueueSettings(),

		Metrics: metricsConfig{
			SendMonotonic: true,
			DeltaTTL:      3600,
			Quantiles:     true,
			ExporterConfig: metricsExporterConfig{
				ResourceAttributesAsTags:             false,
				InstrumentationLibraryMetadataAsTags: false,
			},
			HistConfig: histogramConfig{
				Mode:         "distributions",
				SendCountSum: false,
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
	tr *translator.Translator
	s  serializer.MetricSerializer
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

	if cfg.Metrics.Quantiles {
		options = append(options, translator.WithQuantiles())
	}

	if cfg.Metrics.ExporterConfig.ResourceAttributesAsTags {
		options = append(options, translator.WithResourceAttributesAsTags())
	}

	if cfg.Metrics.ExporterConfig.InstrumentationLibraryMetadataAsTags {
		options = append(options, translator.WithInstrumentationLibraryMetadataAsTags())
	}

	var numberMode translator.NumberMode
	if cfg.Metrics.SendMonotonic {
		numberMode = translator.NumberModeCumulativeToDelta
	} else {
		numberMode = translator.NumberModeRawValue
	}
	options = append(options, translator.WithNumberMode(numberMode))

	return translator.New(logger, options...)
}

func newExporter(logger *zap.Logger, s serializer.MetricSerializer, cfg *exporterConfig) (*exporter, error) {
	tr, err := translatorFromConfig(logger, cfg)
	if err != nil {
		return nil, fmt.Errorf("incorrect OTLP metrics configuration: %w", err)
	}

	return &exporter{tr, s}, nil
}

func (e *exporter) ConsumeMetrics(ctx context.Context, ld pdata.Metrics) error {
	consumer := &serializerConsumer{}
	err := e.tr.MapMetrics(ctx, ld, consumer)
	if err != nil {
		return err
	}

	if err := consumer.flush(e.s); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}
	return nil
}
