// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"go.opentelemetry.io/collector/confmap"
)

type converterWithAgent struct {
	config config.Component
}

func newConverterWithAgent(_ confmap.ConverterSettings, config config.Component) confmap.Converter {
	return &converterWithAgent{config}
}

// Convert implements the confmap.Converter interface for converterWithAgent.
func (c *converterWithAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()

	profilesPipeline, err := Ensure[confMap](confStringMap, "service::pipelines::profiles")
	if err != nil {
		return err
	}
	processorNames, err := Ensure[[]any](profilesPipeline, "processors")
	if err != nil {
		return err
	}
	receiverNames, err := Ensure[[]any](profilesPipeline, "receivers")
	if err != nil {
		return err
	}
	exporterNames, err := Ensure[[]any](profilesPipeline, "exporters")
	if err != nil {
		return err
	}

	// If there's no otlphttpexporter configured, check if an exporter is configured but not in pipeline, add it
	// We can't infer necessary configurations as it needs URLs, so if nothing is found, notify user
	newExporterNames, err := c.fixExportersPipeline(confStringMap, exporterNames)
	if err != nil {
		return err
	}
	profilesPipeline["exporters"] = newExporterNames

	// Determines what components we need to check and ensures at least one infraattributes is configured
	// Deletes any resourcedetection configured in the profiles pipeline
	newProcessorNames, err := c.fixProcessorsPipeline(confStringMap, processorNames)
	if err != nil {
		return err
	}
	profilesPipeline["processors"] = newProcessorNames

	// Ensures at least one hostprofiler is configured using a minimal default configuration
	newReceiverNames, err := c.fixReceiversPipeline(confStringMap, receiverNames)
	if err != nil {
		return err
	}
	profilesPipeline["receivers"] = newReceiverNames

	// Go through every configured processors to make sure there are no resourcedetections declared that were not in the
	// pipeline
	if err := c.ensureGlobalProcessors(confStringMap); err != nil {
		return err
	}
	if err := c.ensureGlobalReceivers(confStringMap); err != nil {
		return err
	}

	// Go through every exporter and ensure every configured otlphttpexporter has a datadog API key
	if err := c.ensureGlobalExporters(confStringMap); err != nil {
		return err
	}
	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

func (c *converterWithAgent) ensureGlobalProcessors(conf confMap) error {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return err
	}

	for name := range processors {
		if strings.Contains(name, "resourcedetection") {
			delete(processors, name)
		}
	}
	return nil
}

func (c *converterWithAgent) ensureHostProfilerConfig(hostProfiler confMap) error {
	// Normalize symbol uploader endpoint keys if enabled
	if isEnabled, ok := Get[bool](hostProfiler, "symbol_uploader::enabled"); ok && isEnabled {
		endpoints, err := Ensure[[]any](hostProfiler, "symbol_uploader::symbol_endpoints")
		if err != nil {
			return err
		}
		for _, endpoint := range endpoints {
			if endpointMap, ok := endpoint.(confMap); ok {
				c.ensureStringKey(endpointMap, "api_key")
				c.ensureStringKey(endpointMap, "app_key")
			}
		}
	}
	return nil
}

func (c *converterWithAgent) ensureGlobalReceivers(conf confMap) error {
	receivers, err := Ensure[confMap](conf, "receivers")
	if err != nil {
		return err
	}

	for receiver, config := range receivers {
		if strings.Contains(receiver, "hostprofiler") {
			hostProfilerConfig, ok := config.(confMap)
			if !ok {
				return fmt.Errorf("hostprofiler config should be a map")
			}

			if err := c.ensureHostProfilerConfig(hostProfilerConfig); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *converterWithAgent) ensureGlobalExporters(conf confMap) error {
	exporters, err := Ensure[confMap](conf, "exporters")
	if err != nil {
		return err
	}

	// Normalize headers for all otlphttp exporters
	for exporter := range exporters {
		if !strings.Contains(exporter, "otlphttp") {
			continue
		}

		headers, err := Ensure[confMap](exporters, exporter+"::headers")
		if err != nil {
			return err
		}
		c.ensureStringKey(headers, "dd-api-key")
	}
	return nil
}

func (c *converterWithAgent) fixExportersPipeline(conf confMap, exporterNames []any) ([]any, error) {
	// Ensure at least one otlphttp exporter exists
	for _, nameAny := range exporterNames {
		if name, ok := nameAny.(string); ok && strings.Contains(name, "otlphttp") {
			return exporterNames, nil
		}
	}

	// Check if one is configured, add it if so
	exporters, err := Ensure[confMap](conf, "exporters")
	if err != nil {
		return exporterNames, err
	}
	for exporter := range exporters {
		if strings.Contains(exporter, "otlphttp") {
			return append(exporterNames, exporter), nil
		}
	}

	return exporterNames, fmt.Errorf("no otlphttp exporter configured in profiles pipeline")

}

func (c *converterWithAgent) fixProcessorsPipeline(conf confMap, processorNames []any) ([]any, error) {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return nil, err
	}
	foundInfraattributes := false
	toDelete := make(map[string]bool)

	// remove resourcedetection, track & sanitize infraattributes
	for _, nameAny := range processorNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("processor name must be a string, got %T", nameAny)
		}

		// Remove resourcedetection from pipeline and global config
		if strings.Contains(name, "resourcedetection") {
			delete(processors, name)
			toDelete[name] = true
			continue
		}

		// Track if we have infraattributes
		if strings.Contains(name, "infraattributes") {
			// Make sure allow_hostname_override is true
			if err := Set(processors, name+"::allow_hostname_override", true); err != nil {
				return nil, err
			}
			foundInfraattributes = true
		}
	}

	// Add infraattributes/default if none found
	if !foundInfraattributes {
		if err := Set(processors, "infraattributes/default::allow_hostname_override", true); err != nil {
			return nil, err
		}
		processorNames = append(processorNames, "infraattributes/default")
	}

	// Remove processors marked for deletion
	processorNames = slices.DeleteFunc(processorNames, func(processor any) bool {
		name := processor.(string)
		_, exists := toDelete[name]
		return exists
	})

	return processorNames, nil
}

func (c *converterWithAgent) fixReceiversPipeline(conf confMap, receiverNames []any) ([]any, error) {
	// Check if hostprofiler is in the pipeline
	hasHostProfiler := false
	for _, nameAny := range receiverNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("processor name must be a string, got %T", nameAny)
		}

		if strings.Contains(name, "hostprofiler") {
			hasHostProfiler = true
			break
		}
	}

	if hasHostProfiler {
		return receiverNames, nil
	}

	// Ensure default config exists if hostprofiler receiver is not configured
	if err := Set(conf, "receivers::hostprofiler::symbol_uploader::enabled", false); err != nil {
		return nil, err
	}

	return append(receiverNames, "hostprofiler"), nil
}

// ensureStringKey ensures a key exists in the map and is a string.
// If missing, it's left for agent config to fill in.
// If present but not a string, converts it to a string.
func (c *converterWithAgent) ensureStringKey(m confMap, key string) {
	var (
		stringKeyToDatadogAgentKey = map[string]string{
			"api_key":    "api_key",
			"app_key":    "app_key",
			"dd-api-key": "api_key",
		}
	)
	if _, exists := m[key]; !exists {
		m[key] = c.config.GetString(stringKeyToDatadogAgentKey[key])
		return
	}
	if _, isString := m[key].(string); !isString {
		m[key] = fmt.Sprintf("%v", m[key])
	}
}
