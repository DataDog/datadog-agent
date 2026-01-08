// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

/*
#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

// InitMemoryTracker initializes RTLoader memory tracking
func InitMemoryTracker() {
	C.enable_memory_tracker()
}

// TrackedCString returns an allocation-tracked pointer to a string
func TrackedCString(str string) unsafe.Pointer {
	Allocations.Add(1)
	return unsafe.Pointer(C.CString(str))
}

// ResetMemoryStats resets allocations and frees counters to zero
func ResetMemoryStats() {
	C.get_and_reset_memory_stats()
	Allocations.Store(0)
	Frees.Store(0)
}

// AssertMemoryUsage makes sure the allocations and frees match
func AssertMemoryUsage(t *testing.T) {
	t.Helper()
	stats := C.get_and_reset_memory_stats()
	assert.Equal(t, uint64(stats.allocations)+Allocations.Load(), uint64(stats.frees)+Frees.Load(),
		"Number of allocations doesn't match number of frees")
}

// AssertMemoryExpectation makes sure the allocations match the
// provided value
func AssertMemoryExpectation(t *testing.T, counter atomic.Uint64, expected uint64) {
	t.Helper()
	assert.Equal(t, expected, counter.Load(),
		"Memory statistic doesn't match the expected value")
}
