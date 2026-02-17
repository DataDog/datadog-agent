// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// addProfilerMetadataTags always creates a dedicated resource/profiler-metadata processor
// without searching for existing resource processors.
func addProfilerMetadataTags(conf confMap, profilesProcessors []any) ([]any, error) {
	const resourceProcessorName = "resource/dd-profiler-internal-metadata"

	// Check if the processor is already defined in root processors
	globalProcessors, _ := Get[confMap](conf, "processors")
	if _, exists := globalProcessors[resourceProcessorName]; exists {
		return nil, fmt.Errorf("%s is a reserved resource processor name. Please change it in your configuration file", resourceProcessorName)
	}

	for _, proc := range profilesProcessors {
		if procName := proc.(string); procName == resourceProcessorName {
			return nil, fmt.Errorf("%s is a reserved resource processor name. Please remove it from the profiles pipeline", resourceProcessorName)
		}
	}

	resourceProcessor, err := Ensure[confMap](conf, "processors::"+resourceProcessorName)
	if err != nil {
		return nil, err
	}

	attributes, err := Ensure[[]any](resourceProcessor, "attributes")
	if err != nil {
		return nil, err
	}

	profilerNameElement := confMap{
		"key":    "profiler_name",
		"value":  version.ProfilerName,
		"action": "upsert",
	}
	profilerVersionElement := confMap{
		"key":    "profiler_version",
		"value":  version.ProfilerVersion,
		"action": "upsert",
	}

	attributes = append(attributes, profilerNameElement)
	attributes = append(attributes, profilerVersionElement)
	if err := Set(resourceProcessor, "attributes", attributes); err != nil {
		return nil, err
	}

	return append(profilesProcessors, resourceProcessorName), nil
}
