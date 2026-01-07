// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
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

// Get retrieves a value of type T from the confMap at the given path.
// Path segments are separated by "::".
// Returns the value and true if found and of correct type, zero value and false otherwise.
// Does not modify the map.
func Get[T any](c confMap, path string) (T, bool) {
	var zero T
	currentMap := c
	pathSlice := strings.Split(path, "::")
	if len(pathSlice) == 0 {
		return zero, false
	}

	target := pathSlice[len(pathSlice)-1]
	for _, key := range pathSlice[:len(pathSlice)-1] {
		childConfMap, exists := currentMap[key]
		if !exists {
			return zero, false
		}

		childMap, isMap := childConfMap.(confMap)
		if !isMap {
			return zero, false
		}

		currentMap = childMap
	}

	obj, exists := currentMap[target]
	if !exists {
		return zero, false
	}

	val, ok := obj.(T)
	return val, ok
}

// Ensure retrieves a value of type T from the confMap at the given path.
// If the path doesn't exist, it creates the path with a zero value of type T.
// Path segments are separated by "::".
// Always returns a value (zero value if created).
func Ensure[T any](c confMap, path string) T {
	if val, ok := Get[T](c, path); ok {
		return val
	}
	var zero T
	// Special handling for map types - create an empty map instead of nil
	switch any(zero).(type) {
	case map[string]any:
		zero = any(make(map[string]any)).(T)
	}
	Set(c, path, zero)
	return zero
}

// Set sets a value of type T in the confMap at the given path.
// Path segments are separated by "::".
// Creates intermediate maps as needed.
// Returns an error if an intermediate path element exists but is not a map.
func Set[T any](c confMap, path string, value T) error {
	currentMap := c
	pathSlice := strings.Split(path, "::")
	if len(pathSlice) == 0 {
		return fmt.Errorf("empty path")
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

// ============================================================================
// converterWithoutAgent
// ============================================================================

type converterWithoutAgent struct{}

func newConverterWithoutAgent(_ confmap.ConverterSettings) confmap.Converter {
	return &converterWithoutAgent{}
}

func (c *converterWithoutAgent) Convert(_ context.Context, conf *confmap.Conf) error {
	confStringMap := conf.ToStringMap()
	if err := removeInfraAttributesProcessor(confStringMap); err != nil {
		return err
	}
	if err := removeDDProfilingExtension(confStringMap); err != nil {
		return err
	}
	if err := removeHpFlareExtension(confStringMap); err != nil {
		return err
	}

	*conf = *confmap.NewFromStringMap(confStringMap)
	return nil
}
