// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
)

var resourceDetectionDefaultConfig confMap = confMap{
	"detectors": []any{"system"},
	"system": confMap{
		"resource_attributes": confMap{
			"host.arch": confMap{
				"enabled": true,
			},
			"host.name": confMap{
				"enabled": false,
			},
			"os.type": confMap{
				"enabled": false,
			},
		},
	},
}

// TODO: currently converter helpers use datadog-agent's logger which isn't setup when in Standalone mode
var standaloneLogger = zap.L().Sugar()

// converterWithoutAgent ensures sane configuration that satisfies the following conditions:
//   - At least one resourcedetection processor declared and used with required defaults
//   - If no resourcedetection processor used, declare & use a minimal resourcedetection processor
//   - No infraattributes processor configured nor declared
//   - remove infraattributes processor from metrics processors pipeline
//   - At least one otlphttpexporter with dd-api-key declared & used
//   - Check if used otlphttpexporter has dd-api-key as string, if not string convert it, if not at all notify user
//   - If hostprofiler::symbol_uploader::enabled == true, convert api_key/app_key to strings in each endpoint
//   - If no hostprofiler is used & configured, add minimal one with symbol_uploader: false
//   - remove ddprofiling & hpflare extensions
type converterWithoutAgent struct{}

func newConverterWithoutAgent(settings confmap.ConverterSettings) confmap.Converter {
	return &converterWithoutAgent{}
}

func (c *converterWithoutAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()

	profilesPipeline, err := Ensure[confMap](confStringMap, "service::pipelines::profiles")
	if err != nil {
		return err
	}

	// no need to check for errors here as we directly depend on profilesPipeline that had to be valid for this code
	// path to be executed
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

	// If there's no otlphttpexporter configured. We can't infer necessary configurations as it needs URLs and API keys
	// so if nothing is found, notify user
	if err := ensureOtlpHTTPExporterConfig(confStringMap, exporterNames); err != nil {
		return err
	}

	// Determines what components we need to check and ensures at least one resourcedetection is configured
	// Deletes any infraattributes configured in the profiles pipeline
	newProcessorNames, err := c.fixProcessorsPipeline(confStringMap, processorNames)
	if err != nil {
		return err
	}
	profilesPipeline["processors"] = newProcessorNames

	// Ensures at least one hostprofiler is used & configured
	// If not, create a minimal component with symbol uploading disabled
	newReceiverNames, err := c.fixReceiversPipeline(confStringMap, receiverNames)
	if err != nil {
		return err
	}
	profilesPipeline["receivers"] = newReceiverNames

	// Go through every configured processor to make sure there are no infraattributes declared that were not in the
	// pipeline
	if err := c.ensureGlobalProcessors(confStringMap); err != nil {
		return err
	}

	// Remove agent-only extensions
	if err := c.removeAgentOnlyExtensions(confStringMap); err != nil {
		return err
	}

	// infraattributes processor can also be used in metrics pipeline
	if err := c.ensureMetricsPipeline(confStringMap); err != nil {
		return err
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}

func (c *converterWithoutAgent) ensureMetricsPipeline(conf confMap) error {
	metrics, err := Ensure[confMap](conf, "service::pipelines::metrics")
	if err != nil {
		return err
	}

	processors, err := Ensure[[]any](metrics, "processors")
	if err != nil {
		return err
	}

	filteredProcessors := make([]any, 0, len(metrics))
	for _, processorAny := range processors {
		processor, ok := processorAny.(string)
		if !ok {
			return errors.New("extensions in profiles service pipeline should be strings")
		}

		if isComponentType(processor, "infraattributes") {
			continue
		}

		filteredProcessors = append(filteredProcessors, processorAny)
	}

	metrics["processors"] = filteredProcessors

	// infraattributes processor should've already been deleted by now, no need to check
	return nil
}

func (c *converterWithoutAgent) ensureGlobalProcessors(conf confMap) error {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return err
	}

	for name := range processors {
		if isComponentType(name, "infraattributes") {
			delete(processors, name)
		}
	}
	return nil
}

func (c *converterWithoutAgent) fixProcessorsPipeline(conf confMap, processorNames []any) ([]any, error) {
	processors, err := Ensure[confMap](conf, "processors")
	if err != nil {
		return nil, err
	}
	foundResourcedetection := false
	toDelete := make(map[string]bool)

	// remove infraattributes, track & sanitize resourcedetection
	for _, nameAny := range processorNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("processor name must be a string, got %T", nameAny)
		}

		// Remove infraattributes from pipeline and global config
		if isComponentType(name, "infraattributes") {
			delete(processors, name)
			toDelete[name] = true
			continue
		}

		// Track if we have resourcedetection
		if isComponentType(name, "resourcedetection") {
			if resourceDetectionConfig, ok := Get[confMap](conf, "processors::"+name); ok {
				c.ensureResourceDetectionConfig(resourceDetectionConfig)
			}
			foundResourcedetection = true
		}
	}

	// Add resourcedetection/default if none found
	if !foundResourcedetection {
		if err := Set(processors, "resourcedetection/default", resourceDetectionDefaultConfig); err != nil {
			return nil, err
		}
		standaloneLogger.Warn("Added minimal resourcedetection processor to user configuration")
		processorNames = append(processorNames, "resourcedetection/default")
	}

	// Remove processors marked for deletion
	processorNames = slices.DeleteFunc(processorNames, func(processor any) bool {
		name := processor.(string)
		_, exists := toDelete[name]
		return exists
	})

	return processorNames, nil
}

func (c *converterWithoutAgent) ensureResourceDetectionConfig(resourceDetection confMap) error {
	detectors, err := Ensure[[]any](resourceDetection, "detectors")
	if err != nil {
		return err
	}

	var resourceAttributes map[string]bool
	if !slices.ContainsFunc(detectors, func(detector any) bool {
		if detector, ok := detector.(string); ok {
			return detector == "system"
		}
		return false
	}) {
		resourceDetection["detectors"] = append(detectors, "system")
		resourceAttributes = map[string]bool{
			"host.arch": true,
			"host.name": false,
			"os.type":   false,
		}
	} else {
		resourceAttributes = map[string]bool{"host.arch": true}
	}

	for attribute, value := range resourceAttributes {
		if err := Set(resourceDetection, "system::resource_attributes::"+attribute+"::enabled", value); err != nil {
			return err
		}
	}
	return nil
}

func (c *converterWithoutAgent) fixReceiversPipeline(conf confMap, receiverNames []any) ([]any, error) {
	// Check if hostprofiler is in the pipeline
	hasHostProfiler := false
	for _, nameAny := range receiverNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("receiver name must be a string, got %T", nameAny)
		}

		if !isComponentType(name, "hostprofiler") {
			continue
		}

		hasHostProfiler = true

		if hostProfilerConfig, ok := Get[confMap](conf, "receivers::"+name); ok {
			if err := c.checkHostProfilerReceiverConfig(hostProfilerConfig); err != nil {
				return nil, err
			}
		}
	}

	if hasHostProfiler {
		return receiverNames, nil
	}

	// Ensure default config exists if hostprofiler receiver is not configured
	if err := Set(conf, "receivers::hostprofiler::symbol_uploader::enabled", false); err != nil {
		return nil, err
	}

	standaloneLogger.Warn("Added minimal hostprofiler receiver to user configuration")
	return append(receiverNames, "hostprofiler"), nil
}

func (c *converterWithoutAgent) checkHostProfilerReceiverConfig(hostProfiler confMap) error {
	if isEnabled, ok := Get[bool](hostProfiler, "symbol_uploader::enabled"); !ok || !isEnabled {
		return nil
	}

	endpoints, ok := Get[[]any](hostProfiler, "symbol_uploader::symbol_endpoints")

	if !ok {
		return errors.New("hostprofiler's symbol_endpoints should be a list")
	}

	if len(endpoints) == 0 {
		return errors.New("hostprofiler's symbol_endpoints cannot be empty when symbol_uploader is enabled")
	}

	for _, epAny := range endpoints {
		// Skip non-map endpoints - validation happens at unmarshal time, not here.
		// Converter's job is transformation, not validation.
		if ep, ok := epAny.(confMap); ok {
			ensureKeyStringValue(ep, "api_key")
			ensureKeyStringValue(ep, "app_key")
		}
	}
	return nil
}

func (c *converterWithoutAgent) removeAgentOnlyExtensions(conf confMap) error {
	service, err := Ensure[confMap](conf, "service")
	if err != nil {
		return err
	}

	extensions, ok := Get[[]any](service, "extensions")
	if !ok {
		return nil
	}

	// Filter out agent-only extensions
	filteredExtensions := make([]any, 0, len(extensions))
	for _, extAny := range extensions {
		ext, ok := extAny.(string)
		if !ok {
			return errors.New("extensions in profiles service pipeline should be strings")
		}

		// Skip ddprofiling and hpflare extensions
		if isComponentType(ext, "ddprofiling") || isComponentType(ext, "hpflare") {
			continue
		}

		filteredExtensions = append(filteredExtensions, extAny)
	}

	service["extensions"] = filteredExtensions

	// Also remove the extension definitions from global config
	extensionsConf, ok := Get[confMap](conf, "extensions")
	if ok {
		for name := range extensionsConf {
			if isComponentType(name, "ddprofiling") || isComponentType(name, "hpflare") {
				delete(extensionsConf, name)
			}
		}
	}

	return nil
}
