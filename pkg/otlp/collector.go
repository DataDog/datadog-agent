// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp
// +build otlp

package otlp

import (
	"context"
	"fmt"

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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/otlp/internal/serializerexporter"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	pipelineError = atomic.NewError(nil)
)

func getComponents(s serializer.MetricSerializer) (
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

	exporters, err := exporter.MakeFactoryMap(
		otlpexporter.NewFactory(),
		serializerexporter.NewFactory(s),
		loggingexporter.NewFactory(),
	)
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
func NewPipeline(cfg PipelineConfig, s serializer.MetricSerializer) (*Pipeline, error) {
	buildInfo, err := getBuildInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get build info: %w", err)
	}

	factories, err := getComponents(s)
	if err != nil {
		return nil, fmt.Errorf("failed to get components: %w", err)
	}

	// Replace default core to use Agent logger
	options := []zap.Option{zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapAgent.NewZapCore()
	}),
	}

	configProvider, err := newMapProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build configuration provider: %w", err)
	}

	col, err := otelcol.NewCollector(otelcol.CollectorSettings{
		Factories:               factories,
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

// Run the OTLP pipeline.
func (p *Pipeline) Run(ctx context.Context) error {
	return p.col.Run(ctx)
}

// Stop the OTLP pipeline.
func (p *Pipeline) Stop() {
	p.col.Shutdown()
}

// BuildAndStart builds and starts an OTLP pipeline
func BuildAndStart(ctx context.Context, cfg config.Config, s serializer.MetricSerializer) (*Pipeline, error) {
	p, err := NewPipelineFromAgentConfig(cfg, s)
	if err != nil {
		return nil, err
	}
	go func() {
		err := p.Run(ctx)
		if err != nil {
			pipelineError.Store(fmt.Errorf("Error running the OTLP pipeline: %w", err))
			log.Errorf(pipelineError.Load().Error())
		}
	}()
	return p, nil
}

func NewPipelineFromAgentConfig(cfg config.Config, s serializer.MetricSerializer) (*Pipeline, error) {
	pcfg, err := FromAgentConfig(cfg)
	if err != nil {
		pipelineError.Store(fmt.Errorf("config error: %w", err))
		return nil, pipelineError.Load()
	}

	p, err := NewPipeline(pcfg, s)
	if err != nil {
		pipelineError.Store(fmt.Errorf("failed to build pipeline: %w", err))
		return nil, pipelineError.Load()
	}

	return p, nil
}

// GetCollectorStatus get the collector status and error message (if there is one)
func GetCollectorStatus(p *Pipeline) CollectorStatus {
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
