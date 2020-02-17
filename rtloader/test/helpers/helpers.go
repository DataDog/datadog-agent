package helpers

import (
	"expvar"
	"unsafe"
)

/*
#cgo CFLAGS: -I../../include -I../../common
#cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"

*/
import "C"

var (
	Allocations = expvar.Int{}
	Frees       = expvar.Int{}
)

// TestMemoryTracker is the method exposed to the RTLoader for memory tracking
//export TestMemoryTracker
func TestMemoryTracker(ptr unsafe.Pointer, sz C.size_t, op C.rtloader_mem_ops_t) {
	switch op {
	case C.DATADOG_AGENT_RTLOADER_ALLOCATION:
		Allocations.Add(1)
	case C.DATADOG_AGENT_RTLOADER_FREE:
		Frees.Add(1)
	}
}
