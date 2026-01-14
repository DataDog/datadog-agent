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

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.opentelemetry.io/collector/confmap"
)

// NewFactoryWithoutAgent returns a new converterWithoutAgent factory.
func NewFactoryWithoutAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithoutAgent)
}

// NewFactoryWithAgent returns a new converterWithAgent factory.
func NewFactoryWithAgent() confmap.ConverterFactory {
	return confmap.NewConverterFactory(newConverterWithAgent)
}

type confMap = map[string]any

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
	if len(pathSlice) == 0 {
		// should never happen
		log.Debugf("No element to get given")
		return zero, false
	}

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
	if len(pathSlice) == 0 {
		return errors.New("empty path")
	}

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

	currentMap[target] = value
	return nil
}

// ensureKeyStringValue checks if a key exists in the config and converts it to a string if needed.
// Returns true if the key exists (regardless of whether conversion was needed), false otherwise.
func ensureKeyStringValue(config confMap, key string) bool {
	val, ok := config[key]
	if !ok {
		return false
	}

	if _, isString := val.(string); !isString {
		log.Debugf("converting %s value to string", key)
		config[key] = fmt.Sprintf("%v", val)
	}

	return true
}

func ensureOtlpHTTPExporterConfig(conf confMap, exporterNames []any) error {
	// for each otlphttpexporter used, check if necessary api key is present
	hasOtlpHTTP := false
	for _, nameAny := range exporterNames {
		if name, ok := nameAny.(string); ok && isComponentType(name, "otlphttp") {
			hasOtlpHTTP = true

			headers, err := Ensure[confMap](conf, "exporters::"+name+"::headers")
			if err != nil {
				return err
			}

			if !ensureKeyStringValue(headers, "dd-api-key") {
				return fmt.Errorf("%s exporter should contain a datadog API key", name)
			}
		}
	}

	if !hasOtlpHTTP {
		return errors.New("no otlphttp exporter configured in profiles pipeline")
	}

	return nil
}
