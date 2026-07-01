// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package confmaputils provides shared utilities for manipulating OpenTelemetry
// collector confmaps (map[string]any) and building standard Datadog component configs.
package confmaputils

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"go.opentelemetry.io/collector/confmap/xconfmap"
)

// AutoConfiguredSuffix is the OTEL component name suffix used for all components
// that are automatically injected by Datadog tooling (as opposed to components
// explicitly declared by the user).
const AutoConfiguredSuffix = "dd-autoconfigured"

// ConfMap is an alias for the map type used throughout OTel confmap manipulation.
type ConfMap = map[string]any

// ComponentName returns the type portion of a full OTEL component name.
// OTEL components follow the convention "type" or "type/id".
// Examples: "otlp_http/prod" → "otlp_http", "prometheus" → "prometheus".
func ComponentName(fullName string) string {
	parts := strings.SplitN(fullName, "/", 2)
	return parts[0]
}

// IsComponentType reports whether name belongs to the given componentType.
// It matches both the bare type ("prometheus") and any named instance ("prometheus/foo").
func IsComponentType(name, componentType string) bool {
	return name == componentType || strings.HasPrefix(name, componentType+"/")
}

// Get retrieves a value of type T from the ConfMap at the given "::" separated path.
// Returns the value and true if found and of the correct type, zero value and false otherwise.
// Does not modify the map.
func Get[T any](c ConfMap, path string) (T, bool) {
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

		childMap, isMap := childConfMap.(ConfMap)
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

	return GetValue[T](obj)
}

// ensurePath walks the ConfMap along the given "::" separated path, creating
// intermediate maps as needed. Returns the final map and the target key name.
// Returns an error if an intermediate path element exists but is not a map.
func ensurePath(c ConfMap, path string) (ConfMap, string, error) {
	currentMap := c
	pathSlice := strings.Split(path, "::")

	target := pathSlice[len(pathSlice)-1]
	for _, key := range pathSlice[:len(pathSlice)-1] {
		childConfMap, exists := currentMap[key]
		if !exists {
			currentMap[key] = make(ConfMap)
			childConfMap = currentMap[key]
		}

		childMap, isMap := childConfMap.(ConfMap)
		if !isMap {
			if childConfMap != nil {
				return nil, "", fmt.Errorf("path element %q is not a map", key)
			}
			// nil means the YAML section was declared but left empty — treat as an empty map.
			childMap = make(ConfMap)
			currentMap[key] = childMap
		}

		currentMap = childMap
	}

	return currentMap, target, nil
}

// Set sets a value of type T in the ConfMap at the given "::" separated path.
// Creates intermediate maps as needed.
// Returns an error if an intermediate path element exists but is not a map.
func Set[T any](c ConfMap, path string, value T) error {
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

// Ensure retrieves a value of type T from the ConfMap at the given path.
// If the path does not exist, it creates it with the zero value of T.
// For map types, creates an empty map rather than nil.
// Returns an error if an intermediate path element exists but is not a map.
func Ensure[T any](c ConfMap, path string) (T, error) {
	if val, ok := Get[T](c, path); ok {
		return val, nil
	}
	var zero T
	switch any(zero).(type) {
	case ConfMap:
		zero = any(make(ConfMap)).(T)
	}
	if err := Set(c, path, zero); err != nil {
		return zero, fmt.Errorf("failed to ensure path %q: %w", path, err)
	}
	return zero, nil
}

// SetDefault sets a default value if the key does not exist at the given path.
// If the key exists with a different value, the existing value is preserved (user override wins).
// Returns (true, nil) if the default is active (newly set or already matching), (false, nil) if a
// user override was preserved, or an error if path traversal fails.
func SetDefault[T any](c ConfMap, path string, value T) (bool, error) {
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

// GetValue resolves the underlying value of a leaf confMap element.
// This allows for safe retrieval of objects that might be coming from an xconfmap.ExpandedValue which value we still
// need to check without overwriting it in the map
func GetValue[T any](obj any) (T, bool) {
	switch t := obj.(type) {
	case T:
		return t, true
	case xconfmap.ExpandedValue:
		// For string requests, prefer Original: OTel stores the substituted text there,
		// while Value may have been parsed into a non-string scalar (int, bool, etc.)
		// when the env var content looks like a YAML literal (e.g. DD_API_KEY=12345).
		var zero T
		if _, isString := any(zero).(string); isString {
			if s, ok := any(t.Original).(T); ok {
				return s, true
			}
		}
		val, ok := t.Value.(T)
		return val, ok
	default:
		var zero T
		return zero, false
	}
}

// FilterProcessorConfig returns the configuration for a filter processor that
// drops internal Prometheus scrape metrics from being exported.
func FilterProcessorConfig() map[string]any {
	return ConfMap{
		"metrics": ConfMap{
			"exclude": ConfMap{
				"match_type": "regexp",
				"metric_names": []any{
					"^scrape_.*$",
					"^up$",
					"^promhttp_metric_handler_errors_total$",
				},
			},
		},
	}
}

// PrometheusReceiverConfig returns a Prometheus receiver configuration that
// scrapes the given target with the given job name.
// Both the otelcol converter ("datadog-agent", "0.0.0.0:8888") and the host
// profiler ("host-profiler-internal", "127.0.0.1:8889") use this same structure.
func PrometheusReceiverConfig(jobName, target string) map[string]any {
	return ConfMap{
		"config": ConfMap{
			"scrape_configs": []any{
				ConfMap{
					"job_name":                      jobName,
					"metric_name_validation_scheme": "legacy",
					"metric_name_escaping_scheme":   "underscores",
					"scrape_interval":               "60s",
					"scrape_protocols":              []any{"PrometheusText0.0.4"},
					"fallback_scrape_protocol":      "PrometheusText0.0.4",
					"static_configs": []any{
						ConfMap{
							"targets": []any{target},
						},
					},
				},
			},
		},
	}
}
