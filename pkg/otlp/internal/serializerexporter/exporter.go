package serializerexporter

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/model/pdata"
	"go.uber.org/zap"
)

var _ config.Exporter = (*exporterConfig)(nil)

// exporterConfig is the exporter configuration.
type exporterConfig struct {
	config.ExporterSettings `mapstructure:",squash"`
}

func newDefaultConfig() config.Exporter {
	return &exporterConfig{}
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

func newExporter(logger *zap.Logger, s serializer.MetricSerializer) (*exporter, error) {
	// TODO (AP-1267): Expose these settings in datadog.yaml.
	tr, err := translator.New(logger,
		translator.WithFallbackHostnameProvider(hostnameProviderFunc(util.GetHostname)),
		translator.WithHistogramMode(translator.HistogramModeDistributions),
		translator.WithNumberMode(translator.NumberModeCumulativeToDelta),
		translator.WithQuantiles(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build translator: %w", err)
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
