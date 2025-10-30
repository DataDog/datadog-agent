// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package maps provides utility functions for dealing with maps
package maps

import "golang.org/x/exp/constraints"

// Map transforms keys and values from the provided into map into a copy using the given mapping functions.
func Map[K1, K2 comparable, V1, V2 any](m map[K1]V1, kmap func(x K1) K2, vmap func(x V1) V2) map[K2]V2 {
	if m == nil {
		return nil
	}
	dst := make(map[K2]V2, len(m))
	for k, v := range m {
		dst[kmap(k)] = vmap(v)
	}
	return dst
}

// MapKeys transforms keys from the provided map into a copy using the given mapping function.
func MapKeys[K1, K2 comparable, V any](m map[K1]V, kmap func(x K1) K2) map[K2]V {
	if m == nil {
		return nil
	}
	dst := make(map[K2]V, len(m))
	for k, v := range m {
		dst[kmap(k)] = v
	}
	return dst
}

// CastIntegerKeys transforms keys from the provided map into a copy using integer casting.
func CastIntegerKeys[K1, K2 constraints.Integer, V any](m map[K1]V) map[K2]V {
	if m == nil {
		return nil
	}
	dst := make(map[K2]V, len(m))
	for k, v := range m {
		dst[K2(k)] = v
	}
	return dst
}

// MapValues transforms values from the provided into map into a copy using the given mapping function.
func MapValues[K comparable, V1, V2 any](m map[K]V1, vmap func(x V1) V2) map[K]V2 {
	if m == nil {
		return nil
	}
	dst := make(map[K]V2, len(m))
	for k, v := range m {
		dst[k] = vmap(v)
	}
	return dst
}
