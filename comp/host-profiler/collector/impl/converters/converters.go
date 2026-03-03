// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements OTEL collector configuration converters for the host profiler.
//
// Converters normalize user-provided OTEL collector configs by adding required Datadog-specific
// components while preserving explicit user configuration values wherever possible.
package converters

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/xconfmap"
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
			slog.Debug("non-existent intermediate map", slog.String("key", key), slog.String("path", path))
			return zero, false
		}

		childMap, isMap := childConfMap.(confMap)
		if !isMap {
			slog.Debug("intermediate node is not a map", slog.String("key", key), slog.String("path", path))
			return zero, false
		}

		currentMap = childMap
	}

	obj, exists := currentMap[target]
	if !exists {
		slog.Debug("leaf element doesn't exist", slog.String("path", path))
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

// ensurePath walks the confMap along the given "::" separated path, creating intermediate maps as needed.
// Returns the final map and the target key name.
// Returns an error if an intermediate path element exists but is not a map.
func ensurePath(c confMap, path string) (confMap, string, error) {
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
			return nil, "", fmt.Errorf("path element %q is not a map", key)
		}

		currentMap = childMap
	}

	return currentMap, target, nil
}

// Set sets a value of type T in the confMap at the given path.
// Path segments are separated by "::".
// Creates intermediate maps as needed.
// Returns an error if an intermediate path element exists but is not a map.
func Set[T any](c confMap, path string, value T) error {
	currentMap, target, err := ensurePath(c, path)
	if err != nil {
		return err
	}

	if existingValue, exists := currentMap[target]; exists {
		slog.Debug("overwriting config", slog.String("path", path), slog.Any("old", existingValue), slog.Any("new", value))
	}
	currentMap[target] = value
	return nil
}

// SetDefault sets a default value if the key does not exist or already holds the same value.
// If the key exists with a different value, the existing value is preserved (user override wins).
// Path segments are separated by "::".
// Creates intermediate maps as needed.
// Returns true if the default is active (set or already matching), false if a user override was preserved.
// Returns an error only if path traversal fails (intermediate element is not a map).
func SetDefault[T any](c confMap, path string, value T) (bool, error) {
	currentMap, target, err := ensurePath(c, path)
	if err != nil {
		return false, err
	}

	if existingValue, exists := currentMap[target]; exists {
		return reflect.DeepEqual(existingValue, value), nil
	}
	currentMap[target] = value
	return true, nil
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
		slog.Debug("converting value to string", slog.String("key", key), slog.String("type", fmt.Sprintf("%T", val)))
		config[key] = fmt.Sprintf("%v", v)
		return true
	case xconfmap.ExpandedValue:
		// ExpandedValues should not be altered at conversion stage
		return true
	default:
		slog.Warn("API key has unexpected type, cannot convert", slog.String("key", key), slog.String("type", fmt.Sprintf("%T", val)))
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
