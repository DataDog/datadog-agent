// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"maps"
	"slices"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/go-viper/mapstructure/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/pipeline/xpipeline"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/pipelines"
)

// converterWithoutAgent ensures proper configuration when the host profiler runs
// in standalone mode (without the Datadog Agent).
//
// Key behaviors:
//   - Adds required receivers: hostprofiler, otlp
//   - Adds required exporters: otlphttp with dd-api-key header
//   - Ensures resourcedetection processor with system detector enabled
//   - Removes all infraattributes processors (replaced by resourcedetection)
//     Note: All infraattributes variants are removed as they require agent integration
//   - Removes ddprofiling and hpflare extensions (require agent integration)
//   - Ensures profiles pipeline has all required components
type converterWithoutAgent struct{}

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

func newConverterWithoutAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithoutAgent{}
}

func (c *converterWithoutAgent) ensureResourceDetectionConfig(resourceDetection component.Config) *resourcedetectionprocessor.Config {
	resourceDetectionConfig := resourcedetectionprocessor.NewFactory().CreateDefaultConfig().(*resourcedetectionprocessor.Config)
	if err := mapstructure.Decode(resourceDetection, resourceDetectionConfig); err != nil {
		log.Warnf("Failed to decode resourcedetection config, using defaults: %v", err)
	}

	systemConfig := &resourceDetectionConfig.DetectorConfig.SystemConfig
	systemConfig.ResourceAttributes.HostArch.Enabled = true
	// if user hasn't configured `system` detector, disable useless default
	// attributes
	if !slices.Contains(resourceDetectionConfig.Detectors, "system") {
		resourceDetectionConfig.Detectors = append(resourceDetectionConfig.Detectors, "system")
		systemConfig.ResourceAttributes.HostName.Enabled = false
		systemConfig.ResourceAttributes.OsType.Enabled = false
	}

	return resourceDetectionConfig
}

func (c *converterWithoutAgent) ensureProcessorsConfig(processors map[component.ID]component.Config) {
	processors[resourcedetectionID] = c.ensureResourceDetectionConfig(processors[resourcedetectionID])

	// delete all infraattributes processors
	maps.DeleteFunc(processors, func(comp component.ID, _ component.Config) bool {
		return comp.Type() == infraattributesType
	})
}

func (c *converterWithoutAgent) ensureHostProfilerConfig(cfg component.Config) *receiver.Config {
	hostProfilerConfig := receiver.NewFactory().CreateDefaultConfig().(receiver.Config)
	if err := mapstructure.Decode(cfg, &hostProfilerConfig); err != nil {
		log.Warnf("Failed to decode hostprofiler config, using defaults: %v", err)
	}

	if hostProfilerConfig.SymbolUploader.Enabled {
		for _, endpoint := range hostProfilerConfig.SymbolUploader.SymbolEndpoints {
			if len(endpoint.APIKey) == 0 {
				log.Warnf("Symbol Uploader: Unable to infer any API key in standalone mode for %s", endpoint.Site)
			}

			if len(endpoint.AppKey) == 0 {
				log.Warnf("Unable to infer any App key in standalone mode for %s", endpoint.Site)
			}
		}
	}

	return &hostProfilerConfig
}

func (c *converterWithoutAgent) ensureReceiversConfig(receivers map[component.ID]component.Config) {
	receivers[hostprofilerID] = c.ensureHostProfilerConfig(receivers[hostprofilerID])

	if _, ok := receivers[otlpReceiverID]; !ok {
		receivers[otlpReceiverID] = otlpreceiver.NewFactory().CreateDefaultConfig()
		log.Debug("Added otlp receiver default config")
	}
}

func (c *converterWithoutAgent) ensureOtlpHTTPConfig(otlpHTTP component.Config) component.Config {
	// When working with the typed struct, header values are stored in a
	// configopaque.String which do not survive a marshalling.
	// Values stored get turned into [REDACTED], we lose every header
	var configMap map[string]any
	if otlpHTTP != nil {
		configMap = otlpHTTP.(map[string]any)
	} else {
		configMap = make(map[string]any)
	}

	// In standalone mode, check if API key is present in headers
	if headers, ok := configMap["headers"].(map[string]any); ok {
		if key, ok := headers[ddAPIKey].(string); !ok || key == "" {
			log.Warn("Cannot infer any API key in standalone mode")
		}
	}

	return configMap
}

func (c *converterWithoutAgent) ensureExportersConfig(exporters map[component.ID]component.Config) {
	exporters[otlpHTTPExporterID] = c.ensureOtlpHTTPConfig(exporters[otlpHTTPExporterID])
}

func (c *converterWithoutAgent) ensureExtensionsConfig(extensions map[component.ID]component.Config, serviceConfig *service.Config) {
	// Remove ddprofiling and hpflare extensions in standalone mode
	delete(extensions, ddprofilingID)
	delete(extensions, hpflareID)

	// Remove from service extensions list
	serviceConfig.Extensions = slices.DeleteFunc(serviceConfig.Extensions, func(id component.ID) bool {
		return id == ddprofilingID || id == hpflareID
	})
}

func (c *converterWithoutAgent) ensureProfilePipeline(profilePipeline *pipelines.PipelineConfig) {
	if !slices.Contains(profilePipeline.Receivers, hostprofilerID) {
		profilePipeline.Receivers = append(profilePipeline.Receivers, hostprofilerID)
		log.Debug("Added hostprofiler to profiles' receiver pipeline")
	}

	// Remove all infraattributes processors from pipeline (used in agent mode).
	profilePipeline.Processors = slices.DeleteFunc(profilePipeline.Processors, func(comp component.ID) bool {
		return comp.Type() == infraattributesType
	})

	// Add resourcedetection processor if not present
	if !hasProcessorType(profilePipeline.Processors, resourcedetectionType) {
		profilePipeline.Processors = append(profilePipeline.Processors, resourcedetectionID)
		log.Debug("Added resourcedetection to profiles' processor pipeline")
	}

	// Ensure otlphttp exporter exists
	if !slices.Contains(profilePipeline.Exporters, otlpHTTPExporterID) {
		profilePipeline.Exporters = append(profilePipeline.Exporters, otlpHTTPExporterID)
		log.Debug("Added otlphttp to profiles' exporter pipeline")
	}
}

func (c *converterWithoutAgent) ensureMetricsPipeline(metricsPipeline *pipelines.PipelineConfig) {
	metricsPipeline.Processors = slices.DeleteFunc(metricsPipeline.Processors, func (comp component.ID) bool {
		return comp.Type() == infraattributesType
	})
}

func (c *converterWithoutAgent) ensurePipelinesConfig(pipelinesConfig pipelines.Config) {
	profilesPipelineID := pipeline.NewID(xpipeline.SignalProfiles)
	if pipelinesConfig[profilesPipelineID] == nil {
		pipelinesConfig[profilesPipelineID] = &pipelines.PipelineConfig{}
		log.Debug("Created profiles pipeline config")
	}
	c.ensureProfilePipeline(pipelinesConfig[profilesPipelineID])

	metricsPipelineID := pipeline.NewID(pipeline.SignalMetrics)
	// if there is a metrics pipeline, ensure there are no infraattributes
	// components
	if metricsPipeline, ok := pipelinesConfig[metricsPipelineID]; ok {
		c.ensureMetricsPipeline(metricsPipeline)
	}
}

func (c *converterWithoutAgent) ensureServiceConfig(services *service.Config) {
	if services.Pipelines == nil {
		services.Pipelines = make(pipelines.Config)
	}

	c.ensurePipelinesConfig(services.Pipelines)
}

// Convert visits the whole configuration map and ensures sane settings
func (c *converterWithoutAgent) Convert(_ context.Context, conf *confmap.Conf) error {
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
	c.ensureExtensionsConfig(otelconf.Extensions, &otelconf.Service)
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

