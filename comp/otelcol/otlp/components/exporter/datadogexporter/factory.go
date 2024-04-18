// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/internal/hostmetadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
)

const metadataReporterPeriod = 30 * time.Minute

func consumeResource(metadataReporter *inframetadata.Reporter, res pcommon.Resource, logger *zap.Logger) {
	if err := metadataReporter.ConsumeResource(res); err != nil {
		logger.Warn("failed to consume resource for host metadata", zap.Error(err), zap.Any("resource", res))
	}
}

type factory struct {
	onceMetadata sync.Once

	onceProvider   sync.Once
	sourceProvider source.Provider
	providerErr    error

	onceReporter     sync.Once
	onceStopReporter sync.Once
	reporter         *inframetadata.Reporter
	reporterErr      error

	onceAttributesTranslator sync.Once
	attributesTranslator     *attributes.Translator
	attributesErr            error

	wg sync.WaitGroup // waits for agent to exit

	registry *featuregate.Registry
}

func (f *factory) SourceProvider(set component.TelemetrySettings, configHostname string) (source.Provider, error) {
	f.onceProvider.Do(func() {
		f.sourceProvider, f.providerErr = hostmetadata.GetSourceProvider(set, configHostname)
	})
	return f.sourceProvider, f.providerErr
}

func (f *factory) AttributesTranslator(set component.TelemetrySettings) (*attributes.Translator, error) {
	f.onceAttributesTranslator.Do(func() {
		f.attributesTranslator, f.attributesErr = attributes.NewTranslator(set)
	})
	return f.attributesTranslator, f.attributesErr
}

func newFactoryWithRegistry(registry *featuregate.Registry) exporter.Factory {
	f := &factory{registry: registry}
	return exporter.NewFactory(
		metadata.Type,
		f.createDefaultConfig,
		exporter.WithMetrics(f.createMetricsExporter, metadata.MetricsStability),
		exporter.WithTraces(f.createTracesExporter, metadata.TracesStability),
		exporter.WithLogs(f.createLogsExporter, metadata.LogsStability),
	)
}

// NewFactory creates a Datadog exporter factory
func NewFactory() exporter.Factory {
	return newFactoryWithRegistry(featuregate.GlobalRegistry())
}

func defaultClientConfig() confighttp.ClientConfig {
	// do not use NewDefaultClientConfig for backwards-compatibility
	return confighttp.ClientConfig{
		Timeout: 15 * time.Second,
	}
}

// createDefaultConfig creates the default exporter configuration
func (f *factory) createDefaultConfig() component.Config {
	return &Config{
		ClientConfig:  defaultClientConfig(),
		BackOffConfig: configretry.NewDefaultBackOffConfig(),
		QueueSettings: exporterhelper.NewDefaultQueueSettings(),

		API: APIConfig{
			Site: "datadoghq.com",
		},

		Metrics: MetricsConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: "https://api.datadoghq.com",
			},
			DeltaTTL: 3600,
			ExporterConfig: MetricsExporterConfig{
				ResourceAttributesAsTags:           false,
				InstrumentationScopeMetadataAsTags: false,
			},
			HistConfig: HistogramConfig{
				Mode:             "distributions",
				SendAggregations: false,
			},
			SumConfig: SumConfig{
				CumulativeMonotonicMode:        CumulativeMonotonicSumModeToDelta,
				InitialCumulativeMonotonicMode: InitialValueModeAuto,
			},
			SummaryConfig: SummaryConfig{
				Mode: SummaryModeGauges,
			},
		},

		Traces: TracesConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: "https://trace.agent.datadoghq.com",
			},
			IgnoreResources: []string{},
		},

		Logs: LogsConfig{
			TCPAddrConfig: confignet.TCPAddrConfig{
				Endpoint: "https://http-intake.logs.datadoghq.com",
			},
		},

		HostMetadata: HostMetadataConfig{
			Enabled:        true,
			HostnameSource: HostnameSourceConfigOrSystem,
		},
	}
}

// checkAndCastConfig checks the configuration type and its warnings, and casts it to
// the Datadog Config struct.
func checkAndCastConfig(c component.Config, logger *zap.Logger) *Config {
	cfg, ok := c.(*Config)
	if !ok {
		panic("programming error: config structure is not of type *datadogexporter.Config")
	}
	cfg.logWarnings(logger)
	return cfg
}

// createMetricsExporter creates a metrics exporter based on this config.
func (f *factory) createMetricsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Metrics, error) {
	return nil, nil
}

// createTracesExporter creates a trace exporter based on this config.
func (f *factory) createTracesExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Traces, error) {
	// TODO implement
	return nil, nil
}

// createLogsExporter creates a logs exporter based on the config.
func (f *factory) createLogsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Logs, error) {
	return nil, nil
}
