// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/internal/logsagentexporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/loggingexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.uber.org/atomic"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/DataDog/datadog-agent/pkg/version"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
)

var pipelineError = atomic.NewError(nil)

type tagEnricher struct {
	cardinality collectors.TagCardinality
}

func (t *tagEnricher) SetCardinality(cardinality string) (err error) {
	t.cardinality, err = collectors.StringToTagCardinality(cardinality)
	if err != nil {
		return err
	}
	return nil
}

// enrichedTags of a given dimension.
// In the OTLP pipeline, 'contexts' are kept within the translator and function differently than DogStatsD/check metrics.
// TODO: we need to move this to TagEnricher processor
func (t *tagEnricher) Enrich(_ context.Context, extraTags []string, dimensions *otlpmetrics.Dimensions) []string {
	enrichedTags := make([]string, 0, len(extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)

	entityTags, err := tagger.Tag(dimensions.OriginID(), t.cardinality)
	if err != nil {
		log.Tracef("Cannot get tags for entity %s: %s", dimensions.OriginID(), err)
	} else {
		enrichedTags = append(enrichedTags, entityTags...)
	}

	globalTags, err := tagger.GlobalTags(t.cardinality)
	if err != nil {
		log.Trace(err.Error())
	} else {
		enrichedTags = append(enrichedTags, globalTags...)
	}

	return enrichedTags
}

func getComponents(s serializer.MetricSerializer, logsAgentChannel chan *message.Message) (
	otelcol.Factories,
	error,
) {
	var errs []error

	extensions, err := extension.MakeFactoryMap()
	if err != nil {
		errs = append(errs, err)
	}

	receivers, err := receiver.MakeFactoryMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	exporterFactories := []exporter.Factory{
		otlpexporter.NewFactory(),
		serializerexporter.NewFactory(s, &tagEnricher{cardinality: collectors.LowCardinality}, hostname.Get),
		loggingexporter.NewFactory(),
	}

	if logsAgentChannel != nil {
		exporterFactories = append(exporterFactories, logsagentexporter.NewFactory(logsAgentChannel))
	}

	exporters, err := exporter.MakeFactoryMap(exporterFactories...)
	if err != nil {
		errs = append(errs, err)
	}

	processors, err := processor.MakeFactoryMap(
		batchprocessor.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	factories := otelcol.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}

	return factories, multierr.Combine(errs...)
}

func getBuildInfo() (component.BuildInfo, error) {
	return component.BuildInfo{
		Command:     flavor.GetFlavor(),
		Description: flavor.GetFlavor(),
		Version:     version.AgentVersion,
	}, nil
}

// PipelineConfig is the config struct for an OTLP pipeline.
type PipelineConfig struct {
	// OTLPReceiverConfig is the OTLP receiver configuration.
	OTLPReceiverConfig map[string]interface{}
	// TracePort is the trace Agent OTLP port.
	TracePort uint
	// MetricsEnabled states whether OTLP metrics support is enabled.
	MetricsEnabled bool
	// TracesEnabled states whether OTLP traces support is enabled.
	TracesEnabled bool
	// LogsEnabled states whether OTLP logs support is enabled.
	LogsEnabled bool
	// Debug contains debug configurations.
	Debug map[string]interface{}
	// Metrics contains configuration options for the serializer metrics exporter
	Metrics map[string]interface{}
}

// valid values for debug log level.
var debugLogLevelMap = map[string]struct{}{
	"disabled": {},
	"debug":    {},
	"info":     {},
	"warn":     {},
	"error":    {},
}

// shouldSetLoggingSection returns whether debug logging is enabled.
// If an invalid loglevel value is set, it assumes debug logging is disabled.
// If the special 'disabled' value is set, it returns false.
// Otherwise it returns true and lets the Collector handle the rest.
func (p *PipelineConfig) shouldSetLoggingSection() bool {
	// Legacy behavior: keep it so that we support `loglevel: disabled`.
	if v, ok := p.Debug["loglevel"]; ok {
		if s, ok := v.(string); ok {
			_, ok := debugLogLevelMap[s]
			return ok && s != "disabled"
		}
	}

	// If the legacy behavior does not apply, we always want to set the logging section.
	return true
}

// Pipeline is an OTLP pipeline.
type Pipeline struct {
	col *otelcol.Collector
}

// CollectorStatus is the status struct for an OTLP pipeline's collector
type CollectorStatus struct {
	Status       string
	ErrorMessage string
}

// NewPipeline defines a new OTLP pipeline.
func NewPipeline(cfg PipelineConfig, s serializer.MetricSerializer, logsAgentChannel chan *message.Message) (*Pipeline, error) {
	buildInfo, err := getBuildInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get build info: %w", err)
	}

	// Replace default core to use Agent logger
	options := []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapAgent.NewZapCore()
		}),
	}

	configProvider, err := newMapProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build configuration provider: %w", err)
	}

	col, err := otelcol.NewCollector(otelcol.CollectorSettings{
		Factories: func() (otelcol.Factories, error) {
			return getComponents(s, logsAgentChannel)
		},
		BuildInfo:               buildInfo,
		DisableGracefulShutdown: true,
		ConfigProvider:          configProvider,
		LoggingOptions:          options,
		// see https://github.com/DataDog/datadog-agent/commit/3f4a78e5f2e276c8cdd90fa7e60455a2374d41d0
		SkipSettingGRPCLogger: true,
	})
	if err != nil {
		return nil, err
	}

	return &Pipeline{col}, nil
}

func recoverAndStoreError() {
	if r := recover(); r != nil {
		err := fmt.Errorf("OTLP pipeline had a panic: %v", r)
		pipelineError.Store(err)
		log.Errorf(err.Error())
	}
}

// Run the OTLP pipeline.
func (p *Pipeline) Run(ctx context.Context) error {
	defer recoverAndStoreError()
	return p.col.Run(ctx)
}

// Stop the OTLP pipeline.
func (p *Pipeline) Stop() {
	p.col.Shutdown()
}

// NewPipelineFromAgentConfig creates a new pipeline from the given agent configuration, metric serializer and logs channel. It returns
// any potential failure.
func NewPipelineFromAgentConfig(cfg config.Component, s serializer.MetricSerializer, logsAgentChannel chan *message.Message) (*Pipeline, error) {
	pcfg, err := FromAgentConfig(cfg)
	if err != nil {
		pipelineError.Store(fmt.Errorf("config error: %w", err))
		return nil, pipelineError.Load()
	}
	if err := checkAndUpdateCfg(cfg, pcfg, logsAgentChannel); err != nil {
		return nil, err
	}
	p, err := NewPipeline(pcfg, s, logsAgentChannel)
	if err != nil {
		pipelineError.Store(fmt.Errorf("failed to build pipeline: %w", err))
		return nil, pipelineError.Load()
	}

	return p, nil
}

// GetCollectorStatus get the collector status and error message (if there is one)
func (p *Pipeline) GetCollectorStatus() CollectorStatus {
	statusMessage, errMessage := "Failed to start", ""
	if p != nil {
		statusMessage = p.col.GetState().String()
	}
	err := pipelineError.Load()
	if err != nil {
		// If the pipeline is nil then it failed to start so we return the error.
		errMessage = err.Error()
	}
	return CollectorStatus{Status: statusMessage, ErrorMessage: errMessage}
}
