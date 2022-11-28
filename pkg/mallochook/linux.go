// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mallochook

// #cgo CFLAGS: -O3
// #cgo LDFLAGS: -ldl
// #include "mallochook.h"
import "C"

// Stats contains statistics about allocations
type Stats struct {
	// Inuse is the number of bytes currently in use (allocated, but not freed)
	Inuse uint
	// Alloc is the total number of bytes allocated so far
	Alloc uint
}

// GetStats returns a snapshot of memory allocation statistics
func GetStats() Stats {
	var inuse, alloc C.size_t
	C.mallochook_get_stats(&inuse, &alloc)

	return Stats{
		Inuse: uint(inuse),
		Alloc: uint(alloc),
	}
}
