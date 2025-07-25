// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datadogexporter provides a factory for the Datadog exporter.
package datadogexporter

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	tracepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/featuregate"
	"google.golang.org/protobuf/proto"

	"go.uber.org/zap"
)

type factory struct {
	setupErr               error
	onceSetupTraceAgentCmp sync.Once

	registry       *featuregate.Registry
	s              serializer.MetricSerializer
	logsAgent      logsagentpipeline.Component
	h              serializerexporter.SourceProviderFunc
	traceagentcmp  traceagent.Component
	mclientwrapper *metricsclient.StatsdClientWrapper
	gatewayUsage   otel.GatewayUsage
	store          serializerexporter.TelemetryStore
}

// setupTraceAgentCmp sets up the trace agent component.
// It is needed in trace exporter to send trace and in metrics exporter to send apm stats.
// The set up happens only once, subsequent calls are no-op.
func (f *factory) setupTraceAgentCmp(set component.TelemetrySettings) error {
	f.onceSetupTraceAgentCmp.Do(func() {
		var attributesTranslator *attributes.Translator
		attributesTranslator, f.setupErr = attributes.NewTranslator(set)
		if f.setupErr != nil {
			return
		}
		f.traceagentcmp.SetOTelAttributeTranslator(attributesTranslator)
	})
	return f.setupErr
}

func newFactoryWithRegistry(
	registry *featuregate.Registry,
	traceagentcmp traceagent.Component,
	s serializer.MetricSerializer,
	logsagent logsagentpipeline.Component,
	h serializerexporter.SourceProviderFunc,
	mclientwrapper *metricsclient.StatsdClientWrapper,
	gatewayUsage otel.GatewayUsage,
	store serializerexporter.TelemetryStore,
) exporter.Factory {
	f := &factory{
		registry:       registry,
		s:              s,
		logsAgent:      logsagent,
		traceagentcmp:  traceagentcmp,
		h:              h,
		mclientwrapper: mclientwrapper,
		gatewayUsage:   gatewayUsage,
		store:          store,
	}

	return exporter.NewFactory(
		Type,
		CreateDefaultConfig,
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
	traceagentcmp traceagent.Component,
	s serializer.MetricSerializer,
	logsAgent logsagentpipeline.Component,
	h serializerexporter.SourceProviderFunc,
	mclientwrapper *metricsclient.StatsdClientWrapper,
	gatewayUsage otel.GatewayUsage,
	store serializerexporter.TelemetryStore,
) exporter.Factory {
	return newFactoryWithRegistry(featuregate.GlobalRegistry(), traceagentcmp, s, logsAgent, h, mclientwrapper, gatewayUsage, store)
}

// CreateDefaultConfig creates the default exporter configuration
func CreateDefaultConfig() component.Config {
	ddcfg := datadogconfig.CreateDefaultConfig().(*datadogconfig.Config)
	ddcfg.Traces.TracesConfig.ComputeTopLevelBySpanKind = true
	ddcfg.Logs.Endpoint = "https://agent-http-intake.logs.datadoghq.com"
	ddcfg.HostMetadata.Enabled = false
	return ddcfg
}

// checkAndCastConfig checks the configuration type and its warnings, and casts it to
// the Datadog Config struct.
func checkAndCastConfig(c component.Config, logger *zap.Logger) *datadogconfig.Config {
	cfg, ok := c.(*datadogconfig.Config)
	if !ok {
		panic("programming error: config structure is not of type *datadogconfig.Config")
	}
	logWarnings(cfg, logger)
	return cfg
}

// logWarnings logs warning messages found during configuration loading.
func logWarnings(cfg *datadogconfig.Config, logger *zap.Logger) {
	cfg.LogWarnings(logger)
	if cfg.Hostname != "" {
		logger.Warn(fmt.Sprintf("hostname \"%s\" is ignored in the embedded collector", cfg.Hostname))
	}
	if cfg.HostMetadata.Enabled {
		logger.Warn("host_metadata should not be enabled and is ignored in the embedded collector")
	}
	if cfg.OnlyMetadata {
		logger.Warn("only_metadata should not be enabled and is ignored in the embedded collector")
	}
	if cfg.Traces.ComputeStatsBySpanKind || cfg.Traces.PeerServiceAggregation || cfg.Traces.PeerTagsAggregation || len(cfg.Traces.PeerTags) > 0 {
		logger.Warn("inferred service related configs (compute_stats_by_span_kind, peer_service_aggregation, peer_tags_aggregation, peer_tags) should only be set in datadog connector rather than datadog exporter in the embedded collector")
	}
}

// createTracesExporter creates a trace exporter based on this config.
func (f *factory) createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	c component.Config,
) (exporter.Traces, error) {
	cfg := checkAndCastConfig(c, set.TelemetrySettings.Logger)

	err := f.setupTraceAgentCmp(set.TelemetrySettings)
	if err != nil {
		return nil, fmt.Errorf("failed to set up trace agent component: %w", err)
	}

	otelmclient, err := metricsclient.InitializeMetricClient(set.MeterProvider, metricsclient.ExporterSourceTag)
	if err != nil {
		return nil, err
	}
	f.mclientwrapper.SetDelegate(otelmclient)

	if cfg.OnlyMetadata {
		return nil, fmt.Errorf("datadog::only_metadata should not be set in OTel Agent")
	}

	tracex := newTracesExporter(ctx, set, cfg, f.traceagentcmp, f.gatewayUsage, f.store.DDOTTraces)

	return exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
		tracex.consumeTraces,
		// explicitly disable since we rely on http.Client timeout logic.
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0 * time.Second}),
		// We don't do retries on traces because of deduping concerns on APM Events.
		exporterhelper.WithRetry(configretry.BackOffConfig{Enabled: false}),
		exporterhelper.WithQueue(cfg.QueueSettings),
	)
}

// createMetricsExporter creates a metrics exporter based on this config.
func (f *factory) createMetricsExporter(
	ctx context.Context,
	set exporter.Settings,
	c component.Config,
) (exporter.Metrics, error) {
	cfg := checkAndCastConfig(c, set.Logger)
	if err := f.setupTraceAgentCmp(set.TelemetrySettings); err != nil {
		return nil, fmt.Errorf("failed to set up trace agent component: %w", err)
	}
	otelmclient, err := metricsclient.InitializeMetricClient(set.MeterProvider, metricsclient.ExporterSourceTag)
	if err != nil {
		return nil, err
	}
	f.mclientwrapper.SetDelegate(otelmclient)
	var wg sync.WaitGroup // waits for consumeStatsPayload to exit
	statsIn := make(chan []byte, 1000)
	statsv := set.BuildInfo.Command + set.BuildInfo.Version
	ctx, cancel := context.WithCancel(ctx) // cancel() runs on shutdown
	f.consumeStatsPayload(ctx, &wg, statsIn, statsv, fmt.Sprintf("datadogexporter-%s-%s", set.BuildInfo.Command, set.BuildInfo.Version), set.Logger)
	sf := serializerexporter.NewFactoryForOTelAgent(f.s, &tagEnricher{}, f.h, statsIn, f.gatewayUsage, f.store)
	ex := &serializerexporter.ExporterConfig{
		Metrics: serializerexporter.MetricsConfig{
			Metrics: cfg.Metrics,
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: cfg.Timeout,
		},
		QueueBatchConfig: cfg.QueueSettings,
		ShutdownFunc: func(context.Context) error {
			cancel()  // first cancel context
			wg.Wait() // then wait for shutdown
			close(statsIn)
			return nil
		},
	}
	return sf.CreateMetrics(ctx, set, ex)
}

func (f *factory) consumeStatsPayload(ctx context.Context, wg *sync.WaitGroup, statsIn <-chan []byte, tracerVersion string, agentVersion string, logger *zap.Logger) {
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-statsIn:
					sp := &tracepb.StatsPayload{}

					err := proto.Unmarshal(msg, sp)
					if err != nil {
						logger.Error("failed to unmarshal stats payload", zap.Error(err))
						continue
					}
					for _, csp := range sp.Stats {
						if csp.TracerVersion == "" {
							csp.TracerVersion = tracerVersion
						}
					}
					// The DD Connector doesn't set the agent version, so we'll set it here
					sp.AgentVersion = agentVersion
					f.traceagentcmp.SendStatsPayload(sp)
				}
			}
		}()
	}
}

// createLogsExporter creates a logs exporter based on the config.
func (f *factory) createLogsExporter(
	ctx context.Context,
	set exporter.Settings,
	c component.Config,
) (exporter.Logs, error) {
	cfg := checkAndCastConfig(c, set.Logger)
	var logch chan *message.Message
	if provider := f.logsAgent.GetPipelineProvider(); provider != nil {
		logch = provider.NextPipelineChan()
	}
	lf := logsagentexporter.NewFactoryWithType(logch, Type, f.gatewayUsage)
	lc := &logsagentexporter.Config{
		OtelSource:    "otel_agent",
		LogSourceName: logsagentexporter.LogSourceName,
		QueueSettings: cfg.QueueSettings,
	}
	return lf.CreateLogs(ctx, set, lc)
}
