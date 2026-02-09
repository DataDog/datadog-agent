// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.opentelemetry.io/collector/confmap"
)

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

// NewFactoryWithAgent returns a new converterWithAgent factory.
func NewFactoryWithAgent(c config.Component) confmap.ConverterFactory {
	newConverterWithAgentWrapper := func(settings confmap.ConverterSettings) confmap.Converter {
		return newConverterWithAgent(settings, c)
	}

	return confmap.NewConverterFactory(newConverterWithAgentWrapper)
}

type confMap = map[string]any

// Component type names for OTEL configuration
const (
	componentTypeInfraAttributes   = "infraattributes"
	componentTypeResourceDetection = "resourcedetection"
	componentTypeHostProfiler      = "hostprofiler"
	componentTypeOtlpHTTP          = "otlphttp"
	componentTypeDDProfiling       = "ddprofiling"
	componentTypeHPFlare           = "hpflare"
)

// Default component names
const (
	defaultInfraAttributesName   = "infraattributes/default"
	defaultResourceDetectionName = "resourcedetection/default"
	defaultHostProfilerName      = "hostprofiler"
)

// Configuration paths used multiple times across converters
const (
	pathSymbolUploaderEnabled = "symbol_uploader::enabled"
	pathSymbolEndpoints       = "symbol_uploader::symbol_endpoints"
)

// Configuration field names used multiple times
const (
	fieldAllowHostnameOverride = "allow_hostname_override"
	fieldDDAPIKey              = "dd-api-key"
	fieldAPIKey                = "api_key"
	fieldAppKey                = "app_key"
)

// OTEL config path prefixes
const (
	pathPrefixReceivers  = "receivers::"
	pathPrefixExporters  = "exporters::"
	pathPrefixProcessors = "processors::"
)

// isComponentType checks if a component name matches a specific type.
// OTEL components follow the naming convention: "type" or "type/id"
// Examples: "otlphttp", "otlphttp/prod", "hostprofiler/custom"
func isComponentType(name, componentType string) bool {
	return name == componentType || strings.HasPrefix(name, componentType+"/")
}

// Get retrieves a value of type T from the confMap at the given path.
// Path segments are separated by "::".
// Returns the value and true if found and of correct type, zero value and false otherwise.
// Does not modify the map.
// Get only logs errors because they are recoverable (ie. Ensure)
func Get[T any](c confMap, path string) (T, bool) {
	var zero T
	currentMap := c
	pathSlice := strings.Split(path, "::")

	target := pathSlice[len(pathSlice)-1]
	for _, key := range pathSlice[:len(pathSlice)-1] {
		childConfMap, exists := currentMap[key]
		if !exists {
			log.Debugf("Non existent %s intermediate map in %s", key, path)
			return zero, false
		}

		childMap, isMap := childConfMap.(confMap)
		if !isMap {
			log.Debugf("Intermediate node %s in %s is not a map", key, path)
			return zero, false
		}

		currentMap = childMap
	}

	obj, exists := currentMap[target]
	if !exists {
		log.Debugf("leaf element in %s doesn't exist", path)
		return zero, false
	}

	val, ok := obj.(T)
	return val, ok
}

// Ensure retrieves a value of type T from the confMap at the given path.
// If the path doesn't exist, it creates the path with a zero value of type T.
// Path segments are separated by "::".
// Returns an error if an intermediate path element exists but is not a map.
func Ensure[T any](c confMap, path string) (T, error) {
	if val, ok := Get[T](c, path); ok {
		return val, nil
	}
	var zero T
	// Special handling for map types
	// create an empty map instead of nil
	switch any(zero).(type) {
	case map[string]any:
		zero = any(make(map[string]any)).(T)
	}
	if err := Set(c, path, zero); err != nil {
		return zero, fmt.Errorf("failed to ensure path %q: %w", path, err)
	}
	return zero, nil
}

// Set sets a value of type T in the confMap at the given path.
// Path segments are separated by "::".
// Creates intermediate maps as needed.
// Returns an error if an intermediate path element exists but is not a map.
func Set[T any](c confMap, path string, value T) error {
	currentMap := c
	pathSlice := strings.Split(path, "::")

	target := pathSlice[len(pathSlice)-1]
	for _, key := range pathSlice[:len(pathSlice)-1] {
		childConfMap, exists := currentMap[key]
		if !exists {
			currentMap[key] = make(confMap)
			childConfMap = currentMap[key]
		}

		childMap, isMap := childConfMap.(map[string]any)
		if !isMap {
			return fmt.Errorf("path element %q is not a map", key)
		}

		currentMap = childMap
	}

	if existingValue, exists := currentMap[target]; exists {
		log.Debugf("Overwriting config at %s: %v -> %v", path, existingValue, value)
	}
	currentMap[target] = value
	return nil
}

// ensureKeyStringValue checks if a key exists in the config and converts it to a string if needed.
// Only converts primitive numeric types; rejects complex types like maps, slices, or structs.
// Returns true if the key exists and is (or was converted to) a string, false otherwise.
func ensureKeyStringValue(config confMap, key string) bool {
	val, ok := config[key]
	if !ok {
		return false
	}

	if _, isString := val.(string); isString {
		return true
	}

	// Only convert primitive numeric types
	switch v := val.(type) {
	case int, int32, int64, float32, float64, uint, uint32, uint64:
		log.Debugf("converting %s value from %T to string", key, val)
		config[key] = fmt.Sprintf("%v", v)
		return true
	default:
		log.Warnf("API key %s has unexpected type %T, cannot convert", key, val)
		return false
	}
}

// fixReceiversPipeline ensures at least one hostprofiler receiver is configured in the pipeline
// If none exists, it adds a minimal hostprofiler receiver with symbol_uploader disabled
// warnFunc is called if a default hostprofiler is added
func fixReceiversPipeline(conf confMap, receiverNames []any, warnFunc func(...any)) ([]any, error) {
	// Check if hostprofiler is in the pipeline
	hasHostProfiler := false
	for _, nameAny := range receiverNames {
		name, ok := nameAny.(string)
		if !ok {
			return nil, fmt.Errorf("receiver name must be a string, got %T", nameAny)
		}

		if !isComponentType(name, componentTypeHostProfiler) {
			continue
		}

		hasHostProfiler = true

		if hostProfilerConfig, ok := Get[confMap](conf, "receivers::"+name); ok {
			if err := checkHostProfilerReceiverConfig(hostProfilerConfig); err != nil {
				return nil, err
			}
		}
	}

	if hasHostProfiler {
		return receiverNames, nil
	}

	// Ensure default config exists if hostprofiler receiver is not configured
	if err := Set(conf, "receivers::"+defaultHostProfilerName+"::"+pathSymbolUploaderEnabled, false); err != nil {
		return nil, err
	}

	warnFunc("Added minimal hostprofiler receiver to user configuration")
	return append(receiverNames, defaultHostProfilerName), nil
}

// checkHostProfilerReceiverConfig validates and normalizes hostprofiler receiver configuration
// It ensures that if symbol_uploader is enabled, symbol_endpoints is properly configured
// and all api_key/app_key values are strings
func checkHostProfilerReceiverConfig(hostProfiler confMap) error {
	if isEnabled, ok := Get[bool](hostProfiler, pathSymbolUploaderEnabled); !ok || !isEnabled {
		return nil
	}

	endpoints, ok := Get[[]any](hostProfiler, pathSymbolEndpoints)

	if !ok {
		return errors.New("symbol_endpoints must be a list")
	}

	if len(endpoints) == 0 {
		return errors.New("symbol_endpoints cannot be empty when symbol_uploader is enabled")
	}

	for _, epAny := range endpoints {
		if ep, ok := epAny.(confMap); ok {
			ensureKeyStringValue(ep, fieldAPIKey)
			ensureKeyStringValue(ep, fieldAppKey)
		}
	}
	return nil
}

func ensureOtlpHTTPExporterConfig(conf confMap, exporterNames []any) error {
	// for each otlphttpexporter used, check if necessary api key is present
	hasOtlpHTTP := false
	for _, nameAny := range exporterNames {
		if name, ok := nameAny.(string); ok && isComponentType(name, componentTypeOtlpHTTP) {
			hasOtlpHTTP = true

			headers, err := Ensure[confMap](conf, "exporters::"+name+"::headers")
			if err != nil {
				return err
			}

			if !ensureKeyStringValue(headers, fieldDDAPIKey) {
				return fmt.Errorf("%s exporter missing required dd-api-key header", name)
			}
		}
	}

	if !hasOtlpHTTP {
		return errors.New("no otlphttp exporter configured in profiles pipeline")
	}

	return nil
}

// addProfilerMetadataTags always creates a dedicated resource/profiler-metadata processor
// without searching for existing resource processors.
func addProfilerMetadataTags(conf confMap) error {
	const resourceProcessorName = "resource/profiler-metadata"

	resourceProcessor, err := Ensure[confMap](conf, "processors::"+resourceProcessorName)
	if err != nil {
		return err
	}

	attributes, err := Ensure[[]any](resourceProcessor, "attributes")
	if err != nil {
		return err
	}
	if len(attributes) != 0 {
		log.Warnf("%s already exists! appending profiler_name and profiler_version", resourceProcessorName)
	}

	profilerNameElement := confMap{
		"key":    "profiler_name",
		"value":  "host-profiler",
		"action": "upsert",
	}
	profilerVersionElement := confMap{
		"key":    "profiler_version",
		"value":  version.AgentVersion,
		"action": "upsert",
	}

	attributes = append(attributes, profilerNameElement)
	attributes = append(attributes, profilerVersionElement)
	resourceProcessor["attributes"] = attributes
	if err := Set(resourceProcessor, "attributes", attributes); err != nil {
		return err
	}

	processors, err := Ensure[[]any](conf, "service::pipelines::profiles::processors")
	if err != nil {
		return err
	}
	processors = append(processors, resourceProcessorName)
	if err := Set(conf, "service::pipelines::profiles::processors", processors); err != nil {
		return err
	}

	return nil
}
