// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import (
	"cmp"
	"reflect"
	"runtime"
)

// Min returns the smaller of two items, for any ordered type.
func Min[T cmp.Ordered](a T, b T) T {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of two items, for any ordered type.
func Max[T cmp.Ordered](a T, b T) T {
	if a > b {
		return a
	}
	return b
}

// Sizeof estimates the total memory size (in bytes) of any Go value.
// It traverses structs, pointers, slices, maps, and interfaces recursively.
func Sizeof(val any) int {
	seen := make(map[uintptr]bool) // Track visited pointers to avoid cycles
	return estimate(reflect.ValueOf(val), seen)
}

func estimate(val reflect.Value, seen map[uintptr]bool) int {
	if !val.IsValid() {
		return 0
	}

	// Handle pointers and avoid duplicate visits (cycle-safe)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return 0
		}
		ptr := val.Pointer()
		if seen[ptr] {
			return 0 // already visited
		}
		seen[ptr] = true
		return int(val.Type().Size()) + estimate(val.Elem(), seen)
	}

	switch val.Kind() {
	case reflect.Struct:
		size := int(val.Type().Size()) // includes padding
		for i := 0; i < val.NumField(); i++ {
			if val.Type().Field(i).IsExported() {
				size += estimate(val.Field(i), seen)
			}
		}
		return size

	case reflect.Slice:
		if val.IsNil() {
			return 0
		}
		size := int(val.Type().Size()) // slice header
		for i := 0; i < val.Len(); i++ {
			size += estimate(val.Index(i), seen)
		}
		return size

	case reflect.Array:
		size := 0
		for i := 0; i < val.Len(); i++ {
			size += estimate(val.Index(i), seen)
		}
		return size

	case reflect.String:
		return int(val.Type().Size()) + val.Len()

	case reflect.Map:
		if val.IsNil() {
			return 0
		}
		size := int(val.Type().Size()) // map header
		for _, key := range val.MapKeys() {
			size += estimate(key, seen)
			size += estimate(val.MapIndex(key), seen)
		}
		return size

	case reflect.Interface:
		if val.IsNil() {
			return 0
		}
		return int(val.Type().Size()) + estimate(val.Elem(), seen)

	default:
		return int(val.Type().Size()) // basic value (int, bool, float, etc.)
	}
}

// MemUsage returns the current memory usage of the Go runtime in bytes.
func MemUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}
