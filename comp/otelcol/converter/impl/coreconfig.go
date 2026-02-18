// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"regexp"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"go.opentelemetry.io/collector/confmap"
)

// addCoreAgentConfig enhances the configuration with information about the core agent.
// For example, if api key is not found in otel config, it can be retrieved from core
// agent config instead.
func addCoreAgentConfig(conf *confmap.Conf, coreCfg config.Component) {
	if coreCfg == nil {
		return
	}
	addAPIKeySite(conf, coreCfg, "exporters", "datadog")
	addEnv(conf, coreCfg)
}

// addEnv adds the env from core agent config to the profiler_options env setting,
// if it is unset.
func addEnv(conf *confmap.Conf, coreCfg config.Component) {
	stringMapConf := conf.ToStringMap()
	extensions, ok := stringMapConf["extensions"]
	if !ok {
		return
	}
	extensionMap, ok := extensions.(map[string]any)
	if !ok {
		return
	}
	for extension := range extensionMap {
		if componentName(extension) == "ddprofiling" {
			ddprofiling, ok := extensionMap[extension]
			if !ok {
				return
			}
			ddprofilingMap, ok := ddprofiling.(map[string]any)
			if !ok {
				ddprofilingMap = make(map[string]any)
				extensionMap[extension] = ddprofilingMap
			}
			profilerOptions, ok := ddprofilingMap["profiler_options"]
			if !ok || profilerOptions == nil {
				profilerOptions = make(map[string]any)
				ddprofilingMap["profiler_options"] = profilerOptions
			}
			profilerOptionsMap, ok := profilerOptions.(map[string]any)
			if !ok {
				return
			}
			if profilerOptionsMap["env"] != nil && profilerOptionsMap["env"] != "" {
				return
			}
			if coreCfg.GetString("env") == "" {
				return
			}
			profilerOptionsMap["env"] = coreCfg.Get("env")
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

// addAPIKeySite adds the API key and site from core config to github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config.APIConfig.
func addAPIKeySite(conf *confmap.Conf, coreCfg config.Component, compType string, compName string) {
	stringMapConf := conf.ToStringMap()
	components, ok := stringMapConf[compType]
	if !ok {
		return
	}
	componentMap, ok := components.(map[string]any)
	if !ok {
		return
	}
	reg, err := regexp.Compile(secretRegex)
	if err != nil {
		return
	}
	for component := range componentMap {
		if componentName(component) == compName {
			datadog, ok := componentMap[component]
			if !ok {
				return
			}
			datadogMap, ok := datadog.(map[string]any)
			if !ok {
				// datadog section is there, but there is nothing in it. We
				// need to add it so we can add to it.
				componentMap[component] = make(map[string]any)
				datadogMap = componentMap[component].(map[string]any)
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
			if apiSite == nil || apiSite == "" {
				if coreCfg.Get("site") != nil && coreCfg.Get("site") != "" {
					apiMap["site"] = coreCfg.Get("site")
				} else {
					// if site is nil or empty string, and core config site is unset, set default
					// site. Site defaults to an empty string in helm chart:
					// https://github.com/DataDog/helm-charts/blob/datadog-3.86.0/charts/datadog/templates/_otel_agent_config.yaml#L24.
					apiMap["site"] = defaultSite
				}
			}

			// api::key
			var match bool
			apiKey, ok := apiMap["key"]
			if ok {
				var key string
				if keyString, okString := apiKey.(string); okString {
					key = keyString
				}
				if key != "" {
					match = reg.Match([]byte(key))
					if !match {
						continue
					}
				}
			}
			// TODO: add logic to either fail or log message if api key not found
			if (apiKey == nil || apiKey == "" || match) && coreCfg.Get("api_key") != nil {
				apiMap["key"] = coreCfg.GetString("api_key")
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}
