// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"expvar"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

/*
#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

void TestMemoryTracker(void *, size_t, rtloader_mem_ops_t);
void initTestMemoryTracker(void) {
	set_memory_tracker_cb(TestMemoryTracker);
}

*/
import "C"

// InitMemoryTracker initializes RTLoader memory tracking
func InitMemoryTracker() {
	C.initTestMemoryTracker()
}

// TrackedCString retruns an allocation-tracked pointer to a string
func TrackedCString(str string) unsafe.Pointer {
	cstr := C.CString(str)
	Allocations.Add(1)

	return unsafe.Pointer(cstr)
}

// ResetMemoryStats resets allocations and frees counters to zero
func ResetMemoryStats() {
	Allocations.Set(0)
	Frees.Set(0)
}

// AssertMemoryUsage makes sure the allocations and frees match
func AssertMemoryUsage(t *testing.T) {
	assert.Equal(t, Allocations.Value(), Frees.Value(),
		"Number of allocations doesn't match number of frees")
}

// AssertMemoryExpectation makes sure the allocations match the
// provided value
func AssertMemoryExpectation(t *testing.T, counter expvar.Int, expected int64) {
	assert.Equal(t, expected, counter.Value(),
		"Memory statistic doesn't match the expected value")
}
