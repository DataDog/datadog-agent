// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service"
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

func getComponents(s serializer.MetricSerializer) (
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
		serializerexporter.NewFactory(s),
	)
	if err != nil {
		errs = append(errs, err)
	}

	processors, err := component.MakeProcessorFactoryMap(
		batchprocessor.NewFactory(),
	)
	if err != nil {
		errs = append(errs, err)
	}

	factories := component.Factories{
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
	// BindHost is the bind host for the OTLP receiver.
	BindHost string
	// GRPCPort is the OTLP receiver gRPC port.
	GRPCPort uint
	// HTTPPort is the OTLP receiver HTTP port.
	HTTPPort uint
	// TracePort is the trace Agent OTLP port.
	TracePort uint
	// MetricsEnabled states whether OTLP metrics support is enabled.
	MetricsEnabled bool
	// TracesEnabled states whether OTLP traces support is enabled.
	TracesEnabled bool

	// Metrics contains configuration options for the serializer metrics exporter
	Metrics MetricsConfig
}

// MetricsConfig defines the metrics exporter specific configuration options
type MetricsConfig struct {
	// Quantiles states whether to report quantiles from summary metrics.
	// By default, the minimum, maximum and average are reported.
	Quantiles bool

	// SendMonotonic states whether to report cumulative monotonic metrics as counters
	// or gauges
	SendMonotonic bool

	// DeltaTTL defines the time that previous points of a cumulative monotonic
	// metric are kept in memory to calculate deltas
	DeltaTTL int64

	ExporterConfig MetricsExporterConfig

	// HistConfig defines the export of OTLP Histograms.
	HistConfig HistogramConfig
}

// MetricsExporterConfig provides options for a user to customize the behavior of the
// metrics exporter
type MetricsExporterConfig struct {
	// ResourceAttributesAsTags, if set to true, will use the exporterhelper feature to transform all
	// resource attributes into metric labels, which are then converted into tags
	ResourceAttributesAsTags bool

	// InstrumentationLibraryMetadataAsTags, if set to true, adds the name and version of the
	// instrumentation library that created a metric to the metric tags
	InstrumentationLibraryMetadataAsTags bool
}

// HistogramConfig customizes export of OTLP Histograms.
type HistogramConfig struct {
	// Mode for exporting histograms. Valid values are 'distributions', 'counters' or 'nobuckets'.
	//  - 'distributions' sends histograms as Datadog distributions (recommended).
	//  - 'counters' sends histograms as Datadog counts, one metric per bucket.
	//  - 'nobuckets' sends no bucket histogram metrics. .sum and .count metrics will still be sent
	//    if `send_count_sum_metrics` is enabled.
	//
	// The current default is 'distributions'.
	Mode string

	// SendCountSum states if the export should send .sum and .count metrics for histograms.
	// The current default is false.
	SendCountSum bool
}

// Pipeline is an OTLP pipeline.
type Pipeline struct {
	col *service.Collector
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

	col, err := service.New(service.CollectorSettings{
		Factories:               factories,
		BuildInfo:               buildInfo,
		DisableGracefulShutdown: true,
		ConfigMapProvider:       newMapProvider(cfg),
		LoggingOptions:          options,
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
	pcfg, err := FromAgentConfig(config.Datadog)
	if err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	p, err := NewPipeline(pcfg, s)
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
