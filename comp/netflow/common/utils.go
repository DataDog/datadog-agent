// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import (
	"cmp"
	"reflect"
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
// If followPointers is true, it includes the size of data that pointers point to.
// If followPointers is false, it only counts the pointer size itself.
func Sizeof(val any, followPointers bool) uint64 {
	seen := make(map[uintptr]bool) // Track visited pointers to avoid cycles
	return estimate(reflect.ValueOf(val), seen, followPointers)
}

func estimate(val reflect.Value, seen map[uintptr]bool, followPointers bool) uint64 {
	if !val.IsValid() {
		return 0
	}

	// Handle pointers and avoid duplicate visits (cycle-safe)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return 0
		}
		ptrSize := uint64(val.Type().Size())
		if !followPointers {
			return ptrSize
		}
		ptr := val.Pointer()
		if seen[ptr] {
			return 0 // already visited
		}
		seen[ptr] = true
		return ptrSize + estimate(val.Elem(), seen, followPointers)
	}

	switch val.Kind() {
	case reflect.Struct:
		size := uint64(val.Type().Size()) // includes padding for the struct itself

		// Special handling for time.Time - skip the location pointer
		if val.Type().String() == "time.Time" {
			// For time.Time, only count additional memory from first two fields (wall, ext)
			// Skip the loc pointer field (index 2) to avoid including shared timezone data
			for i := 0; i < val.NumField() && i < 2; i++ {
				field := val.Field(i)
				additionalSize := estimateAdditional(field, seen, followPointers)
				size += additionalSize
			}
			return size
		}

		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			// Only count additional memory beyond the struct (e.g., heap allocations)
			// For basic types, this returns 0 since they're already counted in struct size
			additionalSize := estimateAdditional(field, seen, followPointers)
			size += additionalSize
		}
		return size

	case reflect.Slice:
		if val.IsNil() {
			return 0
		}
		size := uint64(val.Type().Size()) // slice header
		for i := 0; i < val.Len(); i++ {
			size += estimate(val.Index(i), seen, followPointers)
		}
		return size

	case reflect.Array:
		size := uint64(0)
		for i := 0; i < val.Len(); i++ {
			size += estimate(val.Index(i), seen, followPointers)
		}
		return size

	case reflect.String:
		return uint64(val.Type().Size()) + uint64(val.Len())

	case reflect.Map:
		if val.IsNil() {
			return 0
		}
		size := uint64(val.Type().Size()) // map header
		for _, key := range val.MapKeys() {
			size += estimate(key, seen, followPointers)
			size += estimate(val.MapIndex(key), seen, followPointers)
		}
		return size

	case reflect.Interface:
		if val.IsNil() {
			return 0
		}
		return uint64(val.Type().Size()) + estimate(val.Elem(), seen, followPointers)

	default:
		return uint64(val.Type().Size()) // basic value (int, bool, float, etc.)
	}
}

// estimateAdditional estimates only the additional memory beyond what's already counted
// in the parent struct's size (e.g., heap allocations like slices, maps, strings, pointed-to data)
func estimateAdditional(val reflect.Value, seen map[uintptr]bool, followPointers bool) uint64 {
	if !val.IsValid() {
		return 0
	}

	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return 0
		}
		ptr := val.Pointer()
		if seen[ptr] {
			return 0 // already visited
		}
		seen[ptr] = true
		// For pointers, only count what they point to (pointer itself is in struct)
		if !followPointers {
			return 0
		}
		return estimate(val.Elem(), seen, followPointers)

	case reflect.Slice:
		if val.IsNil() {
			return 0
		}
		// Slice header is in struct, count the underlying array
		size := uint64(0)
		for i := 0; i < val.Len(); i++ {
			size += estimate(val.Index(i), seen, followPointers)
		}
		return size

	case reflect.String:
		// String header is in struct, count the underlying byte array
		return uint64(val.Len())

	case reflect.Map:
		if val.IsNil() {
			return 0
		}
		// Map header is in struct, count the key-value pairs
		size := uint64(0)
		for _, key := range val.MapKeys() {
			size += estimate(key, seen, followPointers)
			size += estimate(val.MapIndex(key), seen, followPointers)
		}
		return size

	case reflect.Interface:
		if val.IsNil() {
			return 0
		}
		// Interface header is in struct, count what it contains
		return estimate(val.Elem(), seen, followPointers)

	case reflect.Struct:
		// Special handling for time.Time - skip the location pointer
		if val.Type().String() == "time.Time" {
			// For time.Time, only count the first two fields (wall, ext) and skip loc pointer
			size := uint64(0)
			for i := 0; i < val.NumField() && i < 2; i++ {
				field := val.Field(i)
				size += estimateAdditional(field, seen, followPointers)
			}
			return size
		}

		// Nested struct: count additional memory beyond its base size
		size := uint64(0)
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			size += estimateAdditional(field, seen, followPointers)
		}
		return size

	case reflect.Array:
		// Arrays are stored inline in structs, but may contain additional data
		size := uint64(0)
		for i := 0; i < val.Len(); i++ {
			size += estimateAdditional(val.Index(i), seen, followPointers)
		}
		return size

	default:
		// Basic types (int, bool, float, etc.) are already counted in struct size
		return 0
	}
}
