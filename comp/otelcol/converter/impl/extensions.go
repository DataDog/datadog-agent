// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"go.opentelemetry.io/collector/confmap"
)

var (
	// pprof
	pProfName         = "pprof"
	pProfEnhancedName = pProfName + "/" + ddAutoconfiguredSuffix
	pProfConfig       any
	pProfComponent    = component{
		Name:         pProfName,
		EnhancedName: pProfEnhancedName,
		Type:         "extensions",
		Config:       pProfConfig,
	}

	// zpages
	zpagesName         = "zpages"
	zpagesEnhancedName = zpagesName + "/" + ddAutoconfiguredSuffix
	zpagesConfig       = map[string]any{
		"endpoint": "localhost:55679",
	}
	zpagesComponent = component{
		Name:         zpagesName,
		EnhancedName: zpagesEnhancedName,
		Type:         "extensions",
		Config:       zpagesConfig,
	}

	// healthcheck
	healthCheckName         = "health_check"
	healthCheckEnhancedName = healthCheckName + "/" + ddAutoconfiguredSuffix
	healthCheckConfig       any
	healthCheckComponent    = component{
		Name:         healthCheckName,
		EnhancedName: healthCheckEnhancedName,
		Type:         "extensions",
		Config:       healthCheckConfig,
	}

	// datadog
	ddflareName         = "ddflare"
	ddflareEnhancedName = ddflareName + "/" + ddAutoconfiguredSuffix
	ddflareConfig       any
	ddflareComponent    = component{
		Name:         ddflareName,
		EnhancedName: ddflareEnhancedName,
		Type:         "extensions",
		Config:       ddflareConfig,
	}

	// datadog OSS
	datadogName         = "datadog"
	datadogEnhancedName = datadogName + "/" + ddAutoconfiguredSuffix
	datadogConfig       any
	datadogComponent    = component{
		Name:         datadogName,
		EnhancedName: datadogEnhancedName,
		Type:         "extensions",
		Config:       datadogConfig,
	}

	// dogtel (standalone mode only)
	dogtelName         = "dogtel"
	dogtelEnhancedName = dogtelName + "/" + ddAutoconfiguredSuffix
	dogtelConfig       any
	dogtelComponent    = component{
		Name:         dogtelName,
		EnhancedName: dogtelEnhancedName,
		Type:         "extensions",
		Config:       dogtelConfig,
	}
)

func createExtensions(enabledFeatures []string) []component {
	components := []component{
		pProfComponent,
		zpagesComponent,
		healthCheckComponent,
	}

	for _, feature := range enabledFeatures {
		if feature == "ddflare" {
			components = append(components, ddflareComponent)
		} else if feature == "datadog" {
			components = append(components, datadogComponent)
		}
	}

	return components
}

func extensionIsInServicePipeline(conf *confmap.Conf, comp component) bool {
	pipelineExtensions := conf.Get("service::extensions")
	if pipelineExtensions == nil {
		return false
	}

	extensionsSlice, ok := pipelineExtensions.([]any)
	if !ok {
		return false
	}
	for _, extension := range extensionsSlice {
		extensionString, ok := extension.(string)
		if !ok {
			return false
		}
		if componentName(extensionString) == comp.Name {
			return true
		}
	}

	return false
}

// findExistingExtensionID returns the ID of an extension in conf whose base
// component name equals compName, or "" if none exists. When several instances
// share the base name it is deterministic: the canonical instance (ID == base
// name) wins, otherwise the lexicographically-first ID.
func findExistingExtensionID(conf *confmap.Conf, compName string) string {
	minID := ""
	for id := range findComps(conf.ToStringMap(), compName, "extensions") {
		// Canonical instance wins; its ID is unique, so return early.
		if id == compName {
			return id
		}
		if minID == "" || id < minID {
			minID = id
		}
	}
	return minID
}

// wireExtensionIDToPipeline appends extensionID verbatim to service::extensions.
func wireExtensionIDToPipeline(conf *confmap.Conf, extensionID string) {
	stringMapConf := conf.ToStringMap()
	service, ok := stringMapConf["service"]
	if !ok {
		return
	}
	serviceMap, ok := service.(map[string]any)
	if !ok {
		return
	}
	if _, ok = serviceMap["extensions"]; !ok {
		serviceMap["extensions"] = []any{}
	}
	if extensionsSlice, ok := serviceMap["extensions"].([]any); ok {
		extensionsSlice = append(extensionsSlice, extensionID)
		serviceMap["extensions"] = extensionsSlice
	}
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

func addExtensionToPipeline(conf *confmap.Conf, comp component) {
	stringMapConf := conf.ToStringMap()
	service, ok := stringMapConf["service"]
	if !ok {
		return
	}
	serviceMap, ok := service.(map[string]any)
	if !ok {
		return
	}
	_, ok = serviceMap["extensions"]
	if !ok {
		serviceMap["extensions"] = []any{}
	}
	if extensionsSlice, ok := serviceMap["extensions"].([]any); ok {
		extensionsSlice = append(extensionsSlice, comp.EnhancedName)
		serviceMap["extensions"] = extensionsSlice
	}

	*conf = *confmap.NewFromStringMap(stringMapConf)
}

// reuseExtension wires a user-defined-but-unwired extension with the given base
// component name into service::extensions, reporting whether one was found. When
// none exists it leaves conf untouched so the caller can add its own instance.
func reuseExtension(conf *confmap.Conf, compName string) bool {
	existingID := findExistingExtensionID(conf, compName)
	if existingID == "" {
		return false
	}
	wireExtensionIDToPipeline(conf, existingID)
	return true
}
