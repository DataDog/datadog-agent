// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	// "log"

	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#if defined(__linux__) || defined(_WIN32)
#    include <malloc.h>
#elif defined(__APPLE__) || defined(__FreeBSD__)
#    include <malloc/malloc.h>
#endif

// Used in TrackedCString to get the size of C memory allocated by C.CString
static inline size_t get_alloc_size(void *ptr) {
#if defined(__linux__)
    return malloc_usable_size(ptr);
#elif defined(_WIN32)
    return _msize(ptr);
#elif defined(__APPLE__) || defined(__FreeBSD__)
    return malloc_size(ptr);
#else
    return 0;
#endif
}

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

var (
	tlmAllocations = telemetry.NewCounter("rtloader", "allocations",
		nil, "Allocations count")
	tlmAllocatedBytes = telemetry.NewCounter("rtloader", "allocated_bytes",
		nil, "Allocated bytes amount")
	tlmFrees = telemetry.NewCounter("rtloader", "frees",
		nil, "Frees count")
	tlmFreedBytes = telemetry.NewCounter("rtloader", "freed_bytes",
		nil, "Freed memory amount")
	tlmInuseBytes = telemetry.NewGauge("rtloader", "inuse_bytes",
		nil, "In-use memory")
)

// TrackedCString returns a C string that will be tracked by the memory tracker
var TrackedCString = func(str string) *C.char {
	return C.CString(str)
}

var memoryTrackerInitializer sync.Once

func InitMemoryTracker() {
	memoryTrackerInitializer.Do(func() {
		C.enable_memory_tracker()

		TrackedCString = func(str string) *C.char {
			cstr := C.CString(str)

			sz := C.get_alloc_size(unsafe.Pointer(cstr))
			tlmAllocations.Inc()
			tlmAllocatedBytes.Add(float64(sz))
			tlmInuseBytes.Add(float64(sz))

			return cstr
		}

		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				<-ticker.C
				memoryStats := C.get_and_reset_memory_stats()
				tlmAllocations.Add(float64(memoryStats.allocations))
				tlmAllocatedBytes.Add(float64(memoryStats.allocated_bytes))
				tlmFrees.Add(float64(memoryStats.frees))
				tlmFreedBytes.Add(float64(memoryStats.freed_bytes))
				tlmInuseBytes.Add(float64(memoryStats.inuse_bytes))
			}
		}()
	})
}
