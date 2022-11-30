// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package mallochook

// #cgo CFLAGS: -O3
// #cgo LDFLAGS: -ldl
// #include "mallochook.h"
import "C"

// Supported returns true when mallochook can provide stats on the current platform
func Supported() bool {
	return true
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
