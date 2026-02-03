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

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.opentelemetry.io/collector/confmap"
)

// converterWithAgent ensures sane configuration that satisfies the following conditions:
//   - At least one infraattributes processor declared and used with `allow_hostname_override: true`
//   - If no infraattributes processor used, declare & use a minimal infraattributes processor
//   - No resourcedetection configured nor declared
//   - At least one otlphttpexporter with dd-api-key declared & used
//   - Check if used otlphttpexporter has dd-api-key as string, if not string convert it, if not at all notify user
//   - If hostprofiler::symbol_uploader::enabled == true, convert api_key/app_key to strings in each endpoint
//   - If no hostprofiler is used & configured, add minimal one with symbol_uploader: false
type converterWithAgent struct{}

func newConverterWithAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithAgent{}
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

	// If there's no otlphttpexporter configured. We can't infer necessary configurations as it needs URLs and API keys
	// so if nothing is found, notify user
	if err := ensureOtlpHTTPExporterConfig(confStringMap, exporterNames); err != nil {
		return err
	}

	// Determines what components we need to check and ensures at least one infraattributes is configured
	// Deletes any resourcedetection configured in the profiles pipeline
	newProcessorNames, err := c.fixProcessorsPipeline(confStringMap, processorNames)
	if err != nil {
		return err
	}
	profilesPipeline["processors"] = newProcessorNames

	// Ensures at least one hostprofiler is used & configured
	// If not, create a minimal component with symbol uploading disabled
	newReceiverNames, err := fixReceiversPipeline(confStringMap, receiverNames, func(v ...any) { log.Warn(v...) })
	if err != nil {
		return err
	}
	profilesPipeline["receivers"] = newReceiverNames

	// Go through every configured processors to make sure there are no resourcedetections declared that were not in the
	// pipeline
	if err := c.ensureGlobalProcessors(confStringMap); err != nil {
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
		if isComponentType(name, componentTypeResourceDetection) {
			delete(processors, name)
		}
	}
	return nil
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
		if isComponentType(name, componentTypeResourceDetection) {
			delete(processors, name)
			toDelete[name] = true
			continue
		}

		// Track if we have infraattributes
		if isComponentType(name, componentTypeInfraAttributes) {
			// Make sure allow_hostname_override is true
			if err := Set(processors, name+"::"+fieldAllowHostnameOverride, true); err != nil {
				return nil, err
			}
			foundInfraattributes = true
		}
	}

	// Add infraattributes/default if none found
	if !foundInfraattributes {
		if err := Set(processors, defaultInfraAttributesName+"::"+fieldAllowHostnameOverride, true); err != nil {
			return nil, err
		}
		log.Warn("Added minimal infraattributes processor to user configuration")
		processorNames = append(processorNames, defaultInfraAttributesName)
	}

	// Remove processors marked for deletion
	processorNames = slices.DeleteFunc(processorNames, func(processor any) bool {
		name := processor.(string)
		_, exists := toDelete[name]
		return exists
	})

	return processorNames, nil
}
