// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"regexp"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
)

// addCoreAgentConfig enhances the configuration with information about the core agent.
// For example, if api key is not found in otel config, it can be retrieved from core
// agent config instead.
func addCoreAgentConfig(conf confmaputils.ConfMap, coreCfg config.Component) {
	if coreCfg == nil {
		return
	}
	addAPIKeySite(conf, coreCfg, "exporters", "datadog")
	addEnv(conf, coreCfg)
}

// addEnv adds the env from core agent config to the profiler_options env setting,
// if it is unset.
func addEnv(conf confmaputils.ConfMap, coreCfg config.Component) {
	extensionMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, "extensions")
	if !ok {
		return
	}
	for extension := range extensionMap {
		if confmaputils.IsComponentType(extension, "ddprofiling") {
			ddprofilingMap, err := confmaputils.Ensure[confmaputils.ConfMap](extensionMap, extension)
			if err != nil {
				return
			}
			profilerOptionsMap, ok := confmaputils.Get[confmaputils.ConfMap](ddprofilingMap, "profiler_options")
			if !ok {
				if existing, exists := ddprofilingMap["profiler_options"]; exists && existing != nil {
					return // present but not a map — don't overwrite
				}
				profilerOptionsMap = make(confmaputils.ConfMap)
				ddprofilingMap["profiler_options"] = profilerOptionsMap
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
}

// addAPIKeySite adds the API key and site from core config to github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config.APIConfig.
func addAPIKeySite(conf confmaputils.ConfMap, coreCfg config.Component, compType string, compName string) {
	componentMap, ok := confmaputils.Get[confmaputils.ConfMap](conf, compType)
	if !ok {
		return
	}
	reg, err := regexp.Compile(secretRegex)
	if err != nil {
		return
	}
	for component := range componentMap {
		if confmaputils.IsComponentType(component, compName) {
			datadogMap, err := confmaputils.Ensure[confmaputils.ConfMap](componentMap, component)
			if err != nil {
				return
			}
			apiMap, ok := confmaputils.Get[confmaputils.ConfMap](datadogMap, "api")
			if !ok {
				if existing, exists := datadogMap["api"]; exists && existing != nil {
					return // present but not a map — don't overwrite
				}
				apiMap = make(confmaputils.ConfMap)
				datadogMap["api"] = apiMap
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
			if (apiKey == nil || apiKey == "" || match) && coreCfg.IsConfigured("api_key") {
				apiMap["key"] = coreCfg.GetString("api_key")
			}
		}
	}
}
