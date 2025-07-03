// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"slices"
	"strings"

	"go.opentelemetry.io/collector/confmap"
)

var ddAutoconfiguredSuffix = "dd-autoconfigured"

const secretRegex = "ENC\\[.*\\][ \t]*$"

type component struct {
	Type         string
	Name         string
	EnhancedName string
	Config       any
}

// Applies selected feature changes
func (c *ddConverter) enhanceConfig(conf *confmap.Conf) {
	var enabledFeatures []string

	// If not specified, assume all features are enabled (ocb tests will not have coreConfig)
	if c.coreConfig != nil {
		enabledFeatures = c.coreConfig.GetStringSlice("otelcollector.converter.features")
	} else {
		enabledFeatures = []string{"infraattributes", "prometheus", "pprof", "zpages", "health_check", "ddflare"}
	}

	// extensions (pprof, zpages, health_check, ddflare)
	for _, extension := range extensions {
		if !slices.Contains(enabledFeatures, extension.Name) || extensionIsInServicePipeline(conf, extension) {
			continue
		}

		addComponentToConfig(conf, extension)
		addExtensionToPipeline(conf, extension)
	}

	// infra attributes processor
	if slices.Contains(enabledFeatures, "infraattributes") {
		addProcessorToPipelinesWithDDExporter(conf, infraAttributesProcessor)
	}
	// prometheus receiver
	if slices.Contains(enabledFeatures, "prometheus") {
		addPrometheusReceiver(conf, findInternalMetricsAddress(conf))
	}

	// add datadog agent sourced config
	addCoreAgentConfig(conf, c.coreConfig)
}

func componentName(fullName string) string {
	parts := strings.SplitN(fullName, "/", 2)
	return parts[0]
}

// addComponentToConfig adds comp to the collector config. It supports receivers,
// processors, exporters and extensions.
func addComponentToConfig(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()

	components, present := stringMapConf[comp.Type]
	if present {
		componentsMap, ok := components.(map[string]any)
		if !ok {
			if components == nil {
				// components map is nil. It is defined but section is empty.
				// need to create map manually

				componentsMap = make(map[string]any)
				stringMapConf[comp.Type] = componentsMap
			} else {
				return
			}
		}
		componentsMap[comp.EnhancedName] = comp.Config
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
	_, ok = pipelinesMap[pipelineName]
	if !ok {
		pipelinesMap[pipelineName] = map[string]any{}
	}
	pipelineMap, ok := pipelinesMap[pipelineName].(map[string]any)
	if !ok {
		return
	}

	_, ok = pipelineMap[comp.Type]
	if !ok {
		pipelineMap[comp.Type] = []any{}
	}
	if pipelineOfTypeSlice, ok := pipelineMap[comp.Type].([]any); ok {
		pipelineOfTypeSlice = append(pipelineOfTypeSlice, comp.EnhancedName)
		pipelineMap[comp.Type] = pipelineOfTypeSlice
	}

	*conf = *confmap.NewFromStringMap(stringMapConf)
}

// findComps finds and returns the matching components and their configs in a string conf map.
// Component can be receivers, processors, connectors or exporters.
func findComps(stringMapConf map[string]any, compName string, compType string) map[string]map[string]any {
	comps, ok := stringMapConf[compType]
	if !ok {
		return nil
	}
	compsMap, ok := comps.(map[string]any)
	if !ok {
		return nil
	}
	cfgsByRecv := make(map[string]map[string]any)
	for name, cfg := range compsMap {
		if componentName(name) != compName {
			continue
		}
		cfgMap, ok := cfg.(map[string]any)
		if !ok {
			cfgMap = nil // some components like debug exporter can leave configs empty and use defaults
		}
		cfgsByRecv[name] = cfgMap
	}
	return cfgsByRecv
}
