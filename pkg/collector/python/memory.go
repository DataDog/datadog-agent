// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"expvar"
	// "log"
	"runtime/debug"
	"sync"
	"unsafe"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import (
	"C"
)

var (
	pointerCache = sync.Map{}

	// TODO(remy): if they're not exposed in the status page we may
	// remove all these expvars
	rtLoaderExpvars = expvar.NewMap("rtloader")
	inuseBytes      = expvar.Int{}
	allocatedBytes  = expvar.Int{}
	freedBytes      = expvar.Int{}
	allocations     = expvar.Int{}
	frees           = expvar.Int{}
	untrackedFrees  = expvar.Int{}

	tlmFreedBytes = telemetry.NewCounter("rtloader", "freed_bytes",
		nil, "Freed memory amount")
	tlmInuseBytes = telemetry.NewGauge("rtloader", "inuse_bytes",
		nil, "In-use memory")
	tlmAllocatedBytes = telemetry.NewCounter("rtloader", "allocated_bytes",
		nil, "Allocated bytes amount")
	tlmAllocations = telemetry.NewCounter("rtloader", "allocations",
		nil, "Allocations count")
	tlmFrees = telemetry.NewCounter("rtloader", "frees",
		nil, "Frees count")
	tlmUntrackedFrees = telemetry.NewCounter("rtloader", "untracked_frees",
		nil, "Untracked frees count")
)

func init() {
	rtLoaderExpvars.Set("InuseBytes", &inuseBytes)
	rtLoaderExpvars.Set("AllocatedBytes", &allocatedBytes)
	rtLoaderExpvars.Set("FreedBytes", &freedBytes)
	rtLoaderExpvars.Set("Allocations", &allocations)
	rtLoaderExpvars.Set("Frees", &frees)
	rtLoaderExpvars.Set("UntrackedFrees", &untrackedFrees)
}

// MemoryTracker is the method exposed to the RTLoader for memory tracking
//
//export MemoryTracker
func MemoryTracker(ptr unsafe.Pointer, sz C.size_t, op C.rtloader_mem_ops_t) {
	// run sync for reliability reasons
	log.Tracef("Memory Tracker - ptr: %v, sz: %v, op: %v", ptr, sz, op)
	switch op {
	case C.DATADOG_AGENT_RTLOADER_ALLOCATION:
		pointerCache.Store(ptr, sz)
		allocations.Add(1)
		tlmAllocations.Inc()
		allocatedBytes.Add(int64(sz))
		tlmAllocatedBytes.Add(float64(sz))
		inuseBytes.Add(int64(sz))
		tlmInuseBytes.Set(float64(inuseBytes.Value()))

	case C.DATADOG_AGENT_RTLOADER_FREE:
		bytes, ok := pointerCache.Load(ptr)
		if !ok {
			log.Debugf("untracked memory was attempted to be freed - set trace level for details")
			lvl, err := log.GetLogLevel()
			if err == nil && lvl == seelog.TraceLvl {
				stack := string(debug.Stack())
				log.Tracef("Memory Tracker - stacktrace: \n%s", stack)
			}
			untrackedFrees.Add(1)
			tlmUntrackedFrees.Inc()
			return
		}
		defer pointerCache.Delete(ptr)

		frees.Add(1)
		tlmFrees.Inc()
		freedBytes.Add(int64(bytes.(C.size_t)))
		tlmFreedBytes.Add(float64(bytes.(C.size_t)))
		inuseBytes.Add(-1 * int64(bytes.(C.size_t)))
		tlmInuseBytes.Set(float64(inuseBytes.Value()))
	}
}

func TrackedCString(str string) *C.char {
	cstr := C.CString(str)

	if config.Datadog.GetBool("memtrack_enabled") {
		MemoryTracker(unsafe.Pointer(cstr), C.size_t(len(str)+1), C.DATADOG_AGENT_RTLOADER_ALLOCATION)
	}

	return cstr
}
