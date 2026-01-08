// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datadogexporter provides a factory for the Datadog exporter.
package datadogexporter

import (
	"context"
	"errors"
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
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	tracepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
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

	onceReporter sync.Once
	reporter     *inframetadata.Reporter
	reporterErr  error

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
	ddcfg.QueueSettings = configoptional.Some(exporterhelper.NewDefaultQueueConfig()) // TODO: remove this line with next collector version upgrade
	return ddcfg
}

func addEmbeddedCollectorConfigWarnings(cfg *datadogconfig.Config) {
	if cfg.Hostname != "" {
		cfg.AddWarningf("hostname \"%s\" is ignored in the embedded collector", cfg.Hostname)
	}
	if cfg.OnlyMetadata {
		cfg.AddWarningf("only_metadata should not be enabled and is ignored in the embedded collector")
	}
}

// createTracesExporter creates a trace exporter based on this config.
func (f *factory) createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	c component.Config,
) (exporter.Traces, error) {
	cfg, err := datadogconfig.CheckAndCastConfig(c)
	if err != nil {
		return nil, err
	}
	// add warnings for embedded collector-specific checks to config
	addEmbeddedCollectorConfigWarnings(cfg)
	// log all warnings found during configuration loading
	cfg.LogWarnings(set.Logger)

	err = f.setupTraceAgentCmp(set.TelemetrySettings)
	if err != nil {
		return nil, fmt.Errorf("failed to set up trace agent component: %w", err)
	}

	otelmclient, err := metricsclient.InitializeMetricClient(set.MeterProvider, metricsclient.ExporterSourceTag)
	if err != nil {
		return nil, err
	}
	f.mclientwrapper.SetDelegate(otelmclient)

	if _, err = f.Reporter(set, cfg.HostMetadata.ReporterPeriod, cfg.HostMetadata.Enabled); err != nil {
		return nil, err
	}

	if cfg.OnlyMetadata {
		return nil, errors.New("datadog::only_metadata should not be set in OTel Agent")
	}

	tracex := newTracesExporter(ctx, set, cfg, f.traceagentcmp, f.gatewayUsage, f.store.DDOTTraces, f.store.DDOTGWUsage, f.reporter)

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
	cfg, err := datadogconfig.CheckAndCastConfig(c)
	if err != nil {
		return nil, err
	}
	// add warnings for embedded collector-specific checks to config
	addEmbeddedCollectorConfigWarnings(cfg)
	// log all warnings found during configuration loading
	cfg.LogWarnings(set.Logger)

	if err := f.setupTraceAgentCmp(set.TelemetrySettings); err != nil {
		return nil, fmt.Errorf("failed to set up trace agent component: %w", err)
	}
	otelmclient, err := metricsclient.InitializeMetricClient(set.MeterProvider, metricsclient.ExporterSourceTag)
	if err != nil {
		return nil, err
	}
	f.mclientwrapper.SetDelegate(otelmclient)

	if _, err = f.Reporter(set, cfg.HostMetadata.ReporterPeriod, cfg.HostMetadata.Enabled); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup // waits for consumeStatsPayload to exit
	statsIn := make(chan []byte, 1000)
	statsv := set.BuildInfo.Command + set.BuildInfo.Version
	ctx, cancel := context.WithCancel(ctx) // cancel() runs on shutdown
	f.consumeStatsPayload(ctx, &wg, statsIn, statsv, fmt.Sprintf("datadogexporter-%s-%s", set.BuildInfo.Command, set.BuildInfo.Version), set.Logger)

	sf := serializerexporter.NewFactoryForOTelAgent(f.s, f.h, statsIn, f.gatewayUsage, f.store, f.reporter)
	ex := &serializerexporter.ExporterConfig{
		Metrics: serializerexporter.MetricsConfig{
			Metrics: cfg.Metrics,
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: cfg.Timeout,
		},
		HostMetadata:     cfg.HostMetadata,
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
	cfg, err := datadogconfig.CheckAndCastConfig(c)
	if err != nil {
		return nil, err
	}
	// add warnings for embedded collector-specific checks to config
	addEmbeddedCollectorConfigWarnings(cfg)
	// log all warnings found during configuration loading
	cfg.LogWarnings(set.Logger)

	var logch chan *message.Message
	if provider := f.logsAgent.GetPipelineProvider(); provider != nil {
		logch = provider.NextPipelineChan()
	}

	if _, err := f.Reporter(set, cfg.HostMetadata.ReporterPeriod, cfg.HostMetadata.Enabled); err != nil {
		return nil, err
	}

	lf := logsagentexporter.NewFactoryWithType(logch, Type, f.gatewayUsage, f.store.DDOTGWUsage, f.reporter)
	lc := &logsagentexporter.Config{
		OtelSource:    "otel_agent",
		LogSourceName: logsagentexporter.LogSourceName,
		QueueSettings: cfg.QueueSettings,
		HostMetadata:  cfg.HostMetadata,
	}
	return lf.CreateLogs(ctx, set, lc)
}

// Reporter builds and returns an *inframetadata.Reporter.
func (f *factory) Reporter(params exporter.Settings, reporterPeriod time.Duration, enableHostMetadata bool) (*inframetadata.Reporter, error) {
	if !enableHostMetadata {
		return nil, nil
	}
	f.onceReporter.Do(func() {
		r, err := inframetadata.NewReporter(params.Logger, serializerexporter.NewPusher(f.s), reporterPeriod)
		if err != nil {
			f.reporterErr = fmt.Errorf("failed to build host metadata reporter: %w", err)
		} else {
			f.reporter = r
		}
		// No need to do f.reporter.Run() in DDOT because DDOT only *pushes* host metadata from OTel resource attributes.
		// DDOT should never periodically report host metadata from source providers, unlike in OSS.
	})
	return f.reporter, f.reporterErr
}
