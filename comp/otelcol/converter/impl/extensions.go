// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import "go.opentelemetry.io/collector/confmap"

var (
	// pprof
	pProfName         = "pprof"
	pProfEnhancedName = pProfName + "/" + ddAutoconfiguredSuffix
	pProfConfig       any

	// zpages
	zpagesName         = "zpages"
	zpagesEnhancedName = zpagesName + "/" + ddAutoconfiguredSuffix
	zpagesConfig       = map[string]any{
		"endpoint": "localhost:55679",
	}

	// healthcheck
	healthCheckName         = "health_check"
	healthCheckEnhancedName = healthCheckName + "/" + ddAutoconfiguredSuffix
	healthCheckConfig       any

	// components
	extensions = []component{
		{
			Name:         pProfName,
			EnhancedName: pProfEnhancedName,
			Type:         "extensions",
			Config:       pProfConfig,
		},
		{
			Name:         zpagesName,
			EnhancedName: zpagesEnhancedName,
			Type:         "extensions",
			Config:       zpagesConfig,
		},
		{
			Name:         healthCheckName,
			EnhancedName: healthCheckEnhancedName,
			Type:         "extensions",
			Config:       healthCheckConfig,
		},
	}
)

func ExtensionIsInServicePipeline(conf *confmap.Conf, comp component) bool {
	pipelineExtensions := conf.Get("service::extensions")
	if pipelineExtensions == nil {
		return false
	}

	if extensionsSlice, ok := pipelineExtensions.([]any); ok {
		for _, extension := range extensionsSlice {
			if extensionString, ok := extension.(string); ok {
				if componentName(extensionString) == comp.Name {
					return true
				}
			}
		}
	}
	return false
}

func addExtensionToPipeline(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()
	if service, ok := stringMapConf["service"]; ok {
		if serviceMap, ok := service.(map[string]any); ok {
			_, ok := serviceMap["extensions"]
			if !ok {
				serviceMap["extensions"] = []any{}
			}
			if extensionsSlice, ok := serviceMap["extensions"].([]any); ok {
				extensionsSlice = append(extensionsSlice, comp.EnhancedName)
				serviceMap["extensions"] = extensionsSlice
			}
		}
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}
