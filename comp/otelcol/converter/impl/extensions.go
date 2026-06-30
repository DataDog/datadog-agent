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

// findExistingExtensionID returns the ID of the first extension defined in conf
// (under the "extensions" key) whose base component name equals compName.
// Returns "" when no such definition exists.
func findExistingExtensionID(conf *confmap.Conf, compName string) string {
	for id := range findComps(conf.ToStringMap(), compName, "extensions") {
		return id
	}
	return ""
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

// reuseOrAddExtension wires an existing user-defined extension of the same base
// component name into service::extensions when one is present; otherwise it adds
// comp's dd-autoconfigured instance and wires that. Callers are responsible for
// any feature/mode gating and for skipping extensions already in the pipeline.
func reuseOrAddExtension(conf *confmap.Conf, comp component) {
	// User already defined this extension but forgot to wire it into
	// service.extensions — reuse their definition instead of creating a
	// second <name>/dd-autoconfigured.
	if existingID := findExistingExtensionID(conf, comp.Name); existingID != "" {
		wireExtensionIDToPipeline(conf, existingID)
		return
	}
	addComponentToConfig(conf, comp)
	addExtensionToPipeline(conf, comp)
}
