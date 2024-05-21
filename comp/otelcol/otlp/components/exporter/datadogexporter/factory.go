// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datadogexporter provides a factory for the Datadog exporter.
package datadogexporter

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/featuregate"

	"go.uber.org/zap"
)

type factory struct {
	onceAttributesTranslator sync.Once
	attributesTranslator     *attributes.Translator
	attributesErr            error

	registry   *featuregate.Registry
	s          serializer.MetricSerializer
	logsAgent  logsagentpipeline.Component
	traceagent *agent.Agent
	h          serializerexporter.SourceProviderFunc

	wg sync.WaitGroup // waits for agent to exit
}

func (f *factory) AttributesTranslator(set component.TelemetrySettings) (*attributes.Translator, error) {
	f.onceAttributesTranslator.Do(func() {
		f.attributesTranslator, f.attributesErr = attributes.NewTranslator(set)
	})
	return f.attributesTranslator, f.attributesErr
}

func newFactoryWithRegistry(
	registry *featuregate.Registry,
	traceagent *agent.Agent,
	s serializer.MetricSerializer,
	logsagent logsagentpipeline.Component,
	h serializerexporter.SourceProviderFunc,
) exporter.Factory {
	f := &factory{
		registry:   registry,
		s:          s,
		logsAgent:  logsagent,
		traceagent: traceagent,
		h:          h,
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

func (t *tagEnricher) SetCardinality(_ string) (err error) {
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
func NewFactory(
	traceagent *agent.Agent,
	s serializer.MetricSerializer,
	logsAgent logsagentpipeline.Component,
	h serializerexporter.SourceProviderFunc,
) exporter.Factory {
	return newFactoryWithRegistry(featuregate.GlobalRegistry(), traceagent, s, logsAgent, h)
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
	cfg := checkAndCastConfig(c, set.TelemetrySettings.Logger)

	var (
		pusher consumer.ConsumeTracesFunc
		stop   component.ShutdownFunc
	)

	ctx, cancel := context.WithCancel(ctx)
	// cancel() runs on shutdown

	if cfg.OnlyMetadata {
		set.Logger.Error("datadog::only_metadata should not be set in OTel Agent")
	}

	// TODO: remove this
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		set.Logger.Info("Starting the trace agent...")
		f.traceagent.Run()
	}()

	tracex := newTracesExporter(ctx, set, cfg, f.traceagent)
	pusher = tracex.consumeTraces
	stop = func(context.Context) error {
		cancel() // first cancel context
		return nil
	}

	return exporterhelper.NewTracesExporter(
		ctx,
		set,
		cfg,
		pusher,
		// explicitly disable since we rely on http.Client timeout logic.
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0 * time.Second}),
		// We don't do retries on traces because of deduping concerns on APM Events.
		exporterhelper.WithRetry(configretry.BackOffConfig{Enabled: false}),
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithShutdown(stop),
	)
}

// createTracesExporter creates a trace exporter based on this config.
func (f *factory) createMetricsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	c component.Config,
) (exporter.Metrics, error) {
	cfg := checkAndCastConfig(c, set.Logger)
	sf := serializerexporter.NewFactory(f.s, &tagEnricher{}, f.h)
	ex := &serializerexporter.ExporterConfig{
		Metrics: cfg.Metrics,
		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: cfg.Timeout,
		},
		QueueSettings: cfg.QueueSettings,
	}
	return sf.CreateMetricsExporter(ctx, set, ex)
}

// createLogsExporter creates a logs exporter based on the config.
func (f *factory) createLogsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	_ component.Config,
) (exporter.Logs, error) {
	var logch chan *message.Message
	if provider := f.logsAgent.GetPipelineProvider(); provider != nil {
		logch = provider.NextPipelineChan()
	}
	lf := logsagentexporter.NewFactory(logch)
	lc := &logsagentexporter.Config{
		OtelSource:    "otel_agent",
		LogSourceName: logsagentexporter.LogSourceName,
	}
	return lf.CreateLogsExporter(ctx, set, lc)
}
