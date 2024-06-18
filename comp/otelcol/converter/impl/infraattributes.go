// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import "go.opentelemetry.io/collector/confmap"

var (
	// infraattributes
	infraAttributesName         = "infraattributes"
	infraAttributesEnhancedName = infraAttributesName + "/" + ddAutoconfiguredSuffix
	infraAttributesConfig       any

	// component
	infraAttributesProcessor = component{
		Name:         infraAttributesName,
		EnhancedName: infraAttributesEnhancedName,
		Type:         "processors",
		Config:       infraAttributesConfig,
	}
)

func addProcessorToPipelinesWithDDExporter(conf *confmap.Conf, comp component) {
	var componentAddedToConfig bool
	stringMapConf := conf.ToStringMap()
	service, ok := stringMapConf["service"]
	if !ok {
		return
	}
	serviceMap, ok := service.(map[string]any)
	if !ok {
		return
	}
	pipelines, ok := serviceMap["pipelines"]
	if !ok {
		return
	}
	pipelinesMap, ok := pipelines.(map[string]any)
	if !ok {
		return
	}
	for pipelineName, components := range pipelinesMap {
		componentsMap, ok := components.(map[string]any)
		if !ok {
			return
		}
		exporters, ok := componentsMap["exporters"]
		if !ok {
			continue
		}
		exportersSlice, ok := exporters.([]any)
		if !ok {
			return
		}
		for _, exporter := range exportersSlice {
			exporterString, ok := exporter.(string)
			if !ok {
				return
			}
			if componentName(exporterString) == "datadog" {
				// datadog component is an exporter in this pipeline. Need to make sure that processor is also configured.
				_, ok := componentsMap[comp.Type]
				if !ok {
					componentsMap[comp.Type] = []any{}
				}

				infraAttrsInPipeline := false
				processorsSlice, ok := componentsMap[comp.Type].([]any)
				if !ok {
					return
				}
				for _, processor := range processorsSlice {
					processorString, ok := processor.(string)
					if !ok {
						return
					}
					if componentName(processorString) == comp.Name {
						infraAttrsInPipeline = true
					}

				}
				if !infraAttrsInPipeline {
					// no processors are defined
					if !componentAddedToConfig {
						addComponentToConfig(conf, comp)
						componentAddedToConfig = true
					}
					addComponentToPipeline(conf, comp, pipelineName)
				}
			}

		}

	}

}
