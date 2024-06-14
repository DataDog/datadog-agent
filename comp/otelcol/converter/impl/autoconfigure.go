// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"strings"

	"go.opentelemetry.io/collector/confmap"
)

var ddAutoconfiguredSuffix = "dd-autoconfigured"

type component struct {
	Type         string
	Name         string
	EnhancedName string
	Config       any
}

func enhanceConfig(conf *confmap.Conf) {
	// extensions
	for _, extension := range extensions {
		if extensionIsInServicePipeline(conf, extension) {
			continue
		}
		addComponentToConfig(conf, extension)
		addExtensionToPipeline(conf, extension)
	}

	// infra attributes processor
	addProcessorToPipelinesWithDDExporter(conf, infraAttributesProcessor)

	// prometheus receiver
	addPrometheusReceiver(conf, prometheusReceiver)
}

func componentName(fullName string) string {
	parts := strings.SplitN(fullName, "/", 2)
	return parts[0]
}

// addComponentToPipeline adds comp to the collector config. It supports receivers,
// processors, exporters and extensions.
func addComponentToConfig(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()

	if components, ok := stringMapConf[comp.Type]; ok {
		if componentsMap, ok := components.(map[string]any); ok {
			componentsMap[comp.EnhancedName] = comp.Config
		}
	} else {
		stringMapConf[comp.Type] = map[string]any{
			comp.EnhancedName: comp.Config,
		}
	}

	*conf = *confmap.NewFromStringMap(stringMapConf)
}

// addComponentToPipeline adds comp into pipelineName. If pipelineName does not exist,
// it creates it. It only supports receivers, processors and exporters.
func addComponentToPipeline(conf *confmap.Conf, comp component, pipelineName string) {
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			if pipelines, ok := serviceMap["pipelines"]; ok {
				if pipelinesMap, ok := pipelines.(map[string]any); ok {
					_, ok := pipelinesMap[pipelineName]
					if !ok {
						// create pipeline
						pipelinesMap[pipelineName] = map[string]any{}
					}
					if pipelineMap, ok := pipelinesMap[pipelineName].(map[string]any); ok {
						_, ok := pipelineMap[comp.Type]
						if !ok {
							pipelineMap[comp.Type] = []any{}
						}
						if pipelineOfTypeSlice, ok := pipelineMap[comp.Type].([]any); ok {
							pipelineOfTypeSlice = append(pipelineOfTypeSlice, comp.EnhancedName)
							pipelineMap[comp.Type] = pipelineOfTypeSlice
						}
					}
				}
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}
