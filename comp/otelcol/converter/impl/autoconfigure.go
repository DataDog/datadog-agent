// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"regexp"
	"strings"

	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

var ddAutoconfiguredSuffix = "dd-autoconfigured"

const secretRegex = "ENC\\[.*\\][ \t]*$"

type component struct {
	Type         string
	Name         string
	EnhancedName string
	Config       any
}

func (c *ddConverter) enhanceConfig(conf *confmap.Conf) {
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

	// datadog connector
	changeDefaultConfigsForDatadogConnector(conf)

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
				stringMapConf[comp.Type] = make(map[string]any)
				componentsMap = stringMapConf[comp.Type].(map[string]any)
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

// addCoreAgentConfig enhances the configuration with information about the core agent.
// For example, if api key is not found in otel config, it can be retrieved from core
// agent config instead.
func addCoreAgentConfig(conf *confmap.Conf, coreCfg config.Component) {
	if coreCfg == nil {
		return
	}
	stringMapConf := conf.ToStringMap()
	exporters, ok := stringMapConf["exporters"]
	if !ok {
		return
	}
	exporterMap, ok := exporters.(map[string]any)
	if !ok {
		return
	}
	reg, err := regexp.Compile(secretRegex)
	if err != nil {
		return
	}
	for exporter := range exporterMap {
		if componentName(exporter) == "datadog" {
			datadog, ok := exporterMap[exporter]
			if !ok {
				return
			}
			datadogMap, ok := datadog.(map[string]any)
			if !ok {
				// datadog section is there, but there is nothing in it. We
				// need to add it so we can add to it.
				exporterMap[exporter] = make(map[string]any)
				datadogMap = exporterMap[exporter].(map[string]any)
			}
			api, ok := datadogMap["api"]
			// ok can be true if api section is there but contains nothing (api == nil).
			// In which case, we need to add it so we can add to it.
			if !ok || api == nil {
				datadogMap["api"] = make(map[string]any, 2)
				api = datadogMap["api"]
			}
			apiMap, ok := api.(map[string]any)
			if !ok {
				return
			}

			// api::site
			apiSite := apiMap["site"]
			if (apiSite == nil || apiSite == "") && coreCfg.Get("site") != nil {
				apiMap["site"] = coreCfg.Get("site")
			} else if (apiSite == nil || apiSite == "") && coreCfg.Get("site") == nil {
				// if site is nil or empty string, and core config site is unset, set default
				// site. Site defaults to an empty string in helm chart:
				// https://github.com/DataDog/helm-charts/blob/datadog-3.86.0/charts/datadog/templates/_otel_agent_config.yaml#L24.
				apiMap["site"] = "datadoghq.com"
			}

			// api::key
			var match bool
			apiKey, ok := apiMap["key"]
			if ok {
				var key string
				if keyString, okString := apiKey.(string); okString {
					key = keyString
				}
				if ok && key != "" {
					match = reg.Match([]byte(key))
					if !match {
						continue
					}
				}
			}
			// TODO: add logic to either fail or log message if api key not found
			if (apiKey == nil || apiKey == "" || match) && coreCfg.Get("api_key") != nil {
				apiMap["key"] = coreCfg.Get("api_key")
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}
