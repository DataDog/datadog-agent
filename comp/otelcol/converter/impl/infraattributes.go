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
	infraAttributesEnhancedName = infraAttributesName + "/" + ddEnhancedSuffix
	infraAttributesConfig       any
	
	// component
	infraAttributesProcessor    = component{
		Name:         infraAttributesName,
		EnhancedName: infraAttributesEnhancedName,
		Type:         "processors",
		Config:       infraAttributesConfig,
	}
)

func addProcessorToPipelinesWithDDExporter(conf *confmap.Conf, comp component) {
	var componentAddedToConfig bool
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if pipelines, ok := serviceMap["pipelines"]; ok {
				if pipelinesMap, ok := pipelines.(map[string]any); ok {
					for pipelineName, components := range pipelinesMap {
						if componentsMap, ok := components.(map[string]any); ok {
							if exporters, ok := componentsMap["exporters"]; ok {
								if exportersSlice, ok := exporters.([]any); ok {
									for _, exporter := range exportersSlice {
										if exporterString, ok := exporter.(string); ok {
											if componentName(exporterString) == "datadog" {
												// datadog component is an exporter in this pipeline. Need to make sure that processor is also configured.
												_, ok := componentsMap[comp.Type]
												if !ok {
													componentsMap[comp.Type] = []any{}
												}

												infraAttrsInPipeline := false
												if processorsSlice, ok := componentsMap[comp.Type].([]any); ok {
													for _, processor := range processorsSlice {
														if processorString, ok := processor.(string); ok {
															if componentName(processorString) == comp.Name {
																infraAttrsInPipeline = true
															}
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
								}
							}
						}
					}
				}
			}
		}
	}
}
