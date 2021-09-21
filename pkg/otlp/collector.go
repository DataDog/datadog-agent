// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func getComponents() (
	component.Factories,
	error,
) {
	var errs []error

	extensions, err := component.MakeExtensionFactoryMap()
	if err != nil {
		errs = append(errs, err)
	}

	receivers, err := component.MakeReceiverFactoryMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	exporters, err := component.MakeExporterFactoryMap(
		otlpexporter.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	processors, err := component.MakeProcessorFactoryMap()
	if err != nil {
		errs = append(errs, err)
	}

	factories := component.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}

	return factories, consumererror.Combine(errs)
}

func getBuildInfo() (component.BuildInfo, error) {
	version, err := version.Agent()
	if err != nil {
		return component.BuildInfo{}, err
	}

	return component.BuildInfo{
		Command:     flavor.GetFlavor(),
		Description: flavor.GetFlavor(),
		Version:     version.String(),
	}, nil
}

// PipelineConfig is the config struct for an OTLP pipeline.
type PipelineConfig struct {
	// BindHost is the bind host for the OTLP receiver.
	BindHost string
	// GRPCPort is the OTLP receiver gRPC port.
	GRPCPort uint
	// HTTPPort is the OTLP receiver HTTP port.
	HTTPPort uint
	// TracePort is the trace Agent OTLP port.
	TracePort uint
}

// Pipeline is an OTLP pipeline.
type Pipeline struct {
	col *service.Collector
}

// NewPipeline defines a new OTLP pipeline.
func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	buildInfo, err := getBuildInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get build info: %w", err)
	}

	factories, err := getComponents()
	if err != nil {
		return nil, fmt.Errorf("failed to get components: %w", err)
	}

	// Replace default core to use Agent logger
	options := []zap.Option{zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapAgent.NewZapCore()
	}),
	}

	parser, err := newParser(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build parser: %w", err)
	}

	col, err := service.New(service.CollectorSettings{
		Factories:               factories,
		BuildInfo:               buildInfo,
		DisableGracefulShutdown: true,
		ParserProvider:          parserProvider(*parser),
		LoggingOptions:          options,
	})
	if err != nil {
		return nil, err
	}
	return &Pipeline{col}, nil
}

// Run the OTLP pipeline.
func (p *Pipeline) Run(_ context.Context) error {
	// TODO (AP-1254): Avoid this workaround
	// See https://github.com/open-telemetry/opentelemetry-collector/issues/3957
	cmd := p.col.Command()
	return cmd.RunE(cmd, nil)
}

// Stop the OTLP pipeline.
func (p *Pipeline) Stop() {
	p.col.Shutdown()
}

// BuildAndStart builds and starts an OTLP pipeline
func BuildAndStart(ctx context.Context, cfg config.Config) (*Pipeline, error) {
	pcfg, err := FromAgentConfig(config.Datadog)
	if err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	p, err := NewPipeline(pcfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build pipeline: %w", err)
	}

	go func() {
		err = p.Run(ctx)
		if err != nil {
			log.Errorf("Error running the OTLP pipeline: %s", err)
		}
	}()

	return p, nil
}
