// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	logsagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/featuregate"

	"go.uber.org/zap"
)

type factory struct {
	onceAttributesTranslator sync.Once
	attributesTranslator     *attributes.Translator
	attributesErr            error

	registry  *featuregate.Registry
	s         serializer.MetricSerializer
	logsAgent logsagent.Component
	h         hostname.Component
}

func (f *factory) AttributesTranslator(set component.TelemetrySettings) (*attributes.Translator, error) {
	f.onceAttributesTranslator.Do(func() {
		f.attributesTranslator, f.attributesErr = attributes.NewTranslator(set)
	})
	return f.attributesTranslator, f.attributesErr
}

func newFactoryWithRegistry(registry *featuregate.Registry, s serializer.MetricSerializer, logsagent logsagent.Component, h hostname.Component) exporter.Factory {
	f := &factory{
		registry:  registry,
		s:         s,
		logsAgent: logsagent,
		h:         h,
	}

	return exporter.NewFactory(
		Type,
		f.createDefaultConfig,
		exporter.WithMetrics(f.createMetricsExporter, MetricsStability),
		exporter.WithTraces(f.createTracesExporter, TracesStability),
		exporter.WithLogs(f.createLogsExporter, LogsStability),
	)
}

type tagEnricher struct{}

func (t *tagEnricher) SetCardinality(cardinality string) (err error) {
	return nil
}

// Enrich of a given dimension.
func (t *tagEnricher) Enrich(_ context.Context, extraTags []string, dimensions *otlpmetrics.Dimensions) []string {
	enrichedTags := make([]string, 0, len(extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)
	return enrichedTags
}

// NewFactory creates a Datadog exporter factory
func NewFactory(s serializer.MetricSerializer, logsAgent logsagent.Component, h hostname.Component) exporter.Factory {
	return newFactoryWithRegistry(featuregate.GlobalRegistry(), s, logsAgent, h)
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

		Metrics: serializerexporter.MetricsConfig{
			DeltaTTL: 3600,
			ExporterConfig: serializerexporter.MetricsExporterConfig{
				ResourceAttributesAsTags:           false,
				InstrumentationScopeMetadataAsTags: false,
			},
			HistConfig: serializerexporter.HistogramConfig{
				Mode:             "distributions",
				SendAggregations: false,
			},
			SumConfig: serializerexporter.SumConfig{
				CumulativeMonotonicMode:        serializerexporter.CumulativeMonotonicSumModeToDelta,
				InitialCumulativeMonotonicMode: serializerexporter.InitialValueModeAuto,
			},
			SummaryConfig: serializerexporter.SummaryConfig{
				Mode: serializerexporter.SummaryModeGauges,
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

// createTracesExporter creates a trace exporter based on this config.
func (f *factory) createTracesExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Traces, error) {
	// TODO implement
	return nil, nil
}

// createTracesExporter creates a trace exporter based on this config.
func (f *factory) createMetricsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Metrics, error) {
	cfg := checkAndCastConfig(c, set.Logger)
	sf := serializerexporter.NewFactory(f.s, &tagEnricher{}, f.h.Get)
	return sf.CreateMetricsExporter(ctx, set, cfg.Metrics)
}

// createLogsExporter creates a logs exporter based on the config.
func (f *factory) createLogsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Logs, error) {
	var logch chan *message.Message
	if provider := f.logsAgent.GetPipelineProvider(); provider != nil {
		logch = provider.NextPipelineChan()
	}
	lf := logsagentexporter.NewFactory(logch)
	return lf.CreateLogsExporter(ctx, set, c)
}
