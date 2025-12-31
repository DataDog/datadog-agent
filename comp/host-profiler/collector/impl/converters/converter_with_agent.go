// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"slices"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/go-viper/mapstructure/v2"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/pipeline/xpipeline"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/pipelines"
)

// NewFactoryWithAgent returns a new converterWithAgent factory.
func NewFactoryWithAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithAgent)
}

// converterWithAgent ensures proper configuration when the host profiler runs
// in agent mode.
//
// Key behaviors:
//   - Adds required receivers: hostprofiler, otlp
//   - Adds required exporters: otlphttp with dd-api-key header
//   - Ensures at least one infraattributes processor with allow_hostname_override=true
//   - Removes default resourcedetection processor (replaced by infraattributes)
//     Note: Only the default "resourcedetection" (no name suffix) is removed;
//     custom variants are preserved as they may serve different purposes
//   - Ensures profiles pipeline has all required components
type converterWithAgent struct{}

func newConverterWithAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithAgent{}
}

func (c *converterWithAgent) ensureInfraattributesProcessorConfig(cfg component.Config) *infraattributesprocessor.Config {
	infraattributesConfig := &infraattributesprocessor.Config{}
	if err := mapstructure.Decode(cfg, infraattributesConfig); err != nil {
		log.Warnf("Failed to decode infraattributes config, using defaults: %v", err)
	}
	if !infraattributesConfig.AllowHostnameOverride {
		infraattributesConfig.AllowHostnameOverride = true
		log.Debug("Changed infraattributesprocessor config to ensure sane configuration")
	}
	return infraattributesConfig
}

func (c *converterWithAgent) ensureProcessorsConfig(processors map[component.ID]component.Config) {
	foundInfraattributes := false

	// Ensure all infraattributes processors have allow_hostname_override=true
	for id, cfg := range processors {
		if id.Type() == infraattributesType {
			processors[id] = c.ensureInfraattributesProcessorConfig(cfg)
			foundInfraattributes = true
		}
	}

	// If no infraattributes processor exists at all, add the default one
	if !foundInfraattributes {
		processors[infraattributesID] = c.ensureInfraattributesProcessorConfig(nil)
	}

	// Remove the default resourcedetection processor (used in standalone mode).
	// In agent mode, infraattributes/default takes over the responsibility of
	// resource detection. We only remove the default "resourcedetection" processor
	// (without name suffix), preserving any custom variants like "resourcedetection/custom"
	// that users may have intentionally configured for other purposes.
	delete(processors, resourcedetectionID)
}

func (c *converterWithAgent) ensureReceiversConfig(receivers map[component.ID]component.Config) {
	receivers[hostprofilerID] = ensureHostProfilerConfig(receivers[hostprofilerID])

	if _, ok := receivers[otlpReceiverID]; !ok {
		receivers[otlpReceiverID] = otlpreceiver.NewFactory().CreateDefaultConfig()
		log.Debug("Added otlp receiver default config")
	}
}

func (c *converterWithAgent) ensureExportersConfig(exporters map[component.ID]component.Config) {
	exporters[otlpHTTPExporterID] = ensureOtlpHTTPConfig(exporters[otlpHTTPExporterID])
}

func (c *converterWithAgent) ensureProfilePipeline(profilePipeline *pipelines.PipelineConfig) {
	if !slices.Contains(profilePipeline.Receivers, hostprofilerID) {
		profilePipeline.Receivers = append(profilePipeline.Receivers, hostprofilerID)
		log.Debug("Added hostprofiler to profiles' receiver pipeline")
	}

	// Remove default resourcedetection from pipeline (used in standalone mode).
	// In agent mode, infraattributes/default replaces the default resourcedetection processor.
	// Custom variants (e.g., resourcedetection/custom) are preserved as they may serve different purposes.
	profilePipeline.Processors = slices.DeleteFunc(profilePipeline.Processors, func(comp component.ID) bool {
		return comp == resourcedetectionID
	})

	// Add infraattributes processor if not present
	if !hasProcessorType(profilePipeline.Processors, infraattributesType) {
		profilePipeline.Processors = append(profilePipeline.Processors, infraattributesID)
		log.Debug("Added infraattributes to profiles' processor pipeline")
	}

	// Ensure otlphttp exporter exists
	if !slices.Contains(profilePipeline.Exporters, otlpHTTPExporterID) {
		profilePipeline.Exporters = append(profilePipeline.Exporters, otlpHTTPExporterID)
		log.Debug("Added otlphttp to profiles' exporter pipeline")
	}
}

func (c *converterWithAgent) ensurePipelinesConfig(pipelinesConfig pipelines.Config) {
	profilesPipelineID := pipeline.NewID(xpipeline.SignalProfiles)
	if pipelinesConfig[profilesPipelineID] == nil {
		pipelinesConfig[profilesPipelineID] = &pipelines.PipelineConfig{}
		log.Debug("Created profiles pipeline config")
	}
	c.ensureProfilePipeline(pipelinesConfig[profilesPipelineID])
}

func (c *converterWithAgent) ensureServiceConfig(services *service.Config) {
	if services.Pipelines == nil {
		services.Pipelines = make(pipelines.Config)
	}

	c.ensurePipelinesConfig(services.Pipelines)
}

// Convert visits the whole configuration map and ensures sane settings
func (c *converterWithAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	otelconf := otelcol.Config{
		Receivers:  map[component.ID]component.Config{},
		Exporters:  map[component.ID]component.Config{},
		Processors: map[component.ID]component.Config{},
		Connectors: map[component.ID]component.Config{},
		Extensions: map[component.ID]component.Config{},
		Service:    service.Config{},
	}

	if err := conf.Unmarshal(&otelconf); err != nil {
		log.Errorf("Failed to unmarshal otel config, using defaults: %v", err)
	}
	c.ensureProcessorsConfig(otelconf.Processors)
	c.ensureReceiversConfig(otelconf.Receivers)
	c.ensureExportersConfig(otelconf.Exporters)
	c.ensureServiceConfig(&otelconf.Service)

	// Marshal into a new conf to avoid merging with old data
	newConf := confmap.New()
	if err := newConf.Marshal(otelconf); err != nil {
		return err
	}

	*conf = *newConf
	return nil
}
