// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package maps provides utility functions for dealing with maps
package maps

// Filter returns a copy of the map with only the key/value pairs the provided function returns true for.
func Filter[K comparable, V any](m map[K]V, fn func(key K, value V) bool) map[K]V {
	result := make(map[K]V)
	for k, v := range m {
		if fn(k, v) {
			result[k] = v
		}
	}
	return result
}
