// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"expvar"
	"unsafe"
)

/*
#include "datadog_agent_rtloader.h"

*/
import "C"

var (
	// Allocations tracks number of memory allocations
	Allocations = expvar.Int{}
	// Frees tracks number of memory frees
	Frees = expvar.Int{}
)

// TestMemoryTracker is the method exposed to the RTLoader for memory tracking
//
//export TestMemoryTracker
func TestMemoryTracker(ptr unsafe.Pointer, sz C.size_t, op C.rtloader_mem_ops_t) {
	switch op {
	case C.DATADOG_AGENT_RTLOADER_ALLOCATION:
		Allocations.Add(1)
	case C.DATADOG_AGENT_RTLOADER_FREE:
		Frees.Add(1)
	}
}
