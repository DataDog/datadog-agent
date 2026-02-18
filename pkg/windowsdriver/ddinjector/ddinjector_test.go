// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjector

import (
	"errors"
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

var currentTest *testing.T
var mockCounters DDInjectorCountersV1
var mockQueryCountersError error

// //////////////////////////////////////////////////////////
// Mock telemetry gauge implementation

type mockSimpleGauge struct {
	value atomic.Uint64 // stored as bits of float64
}

func (g *mockSimpleGauge) Inc() {
	g.Add(1.0)
}

func (g *mockSimpleGauge) Dec() {
	g.Sub(1.0)
}

func (g *mockSimpleGauge) Add(v float64) {
	oldBits := g.value.Load()
	oldVal := float64frombits(oldBits)
	newVal := oldVal + v
	newBits := float64bits(newVal)
	g.value.Store(newBits)
}

func (g *mockSimpleGauge) Sub(v float64) {
	g.Add(-v)
}

func (g *mockSimpleGauge) Set(v float64) {
	g.value.Store(float64bits(v))
}

func (g *mockSimpleGauge) Get() float64 {
	return float64frombits(g.value.Load())
}

// float64bits and float64frombits convert between float64 and uint64 representations.
func float64bits(f float64) uint64 {
	return *(*uint64)(unsafe.Pointer(&f))
}

func float64frombits(b uint64) float64 {
	return *(*float64)(unsafe.Pointer(&b))
}

// createCounters creates a new InjectorCounters with mock telemetry gauges for testing.
func createCounters() *InjectorCounters {
	return &InjectorCounters{
		ProcessesAddedToInjectionTracker:     &mockSimpleGauge{},
		ProcessesRemovedFromInjectionTracker: &mockSimpleGauge{},
		ProcessesSkippedSubsystem:            &mockSimpleGauge{},
		ProcessesSkippedContainer:            &mockSimpleGauge{},
		ProcessesSkippedProtected:            &mockSimpleGauge{},
		ProcessesSkippedSystem:               &mockSimpleGauge{},
		ProcessesSkippedExcluded:             &mockSimpleGauge{},
		InjectionAttempts:                    &mockSimpleGauge{},
		InjectionAttemptFailures:             &mockSimpleGauge{},
		InjectionMaxTimeUs:                   &mockSimpleGauge{},
		InjectionSuccesses:                   &mockSimpleGauge{},
		InjectionFailures:                    &mockSimpleGauge{},
		PeCachingFailures:                    &mockSimpleGauge{},
		ImportDirectoryRestorationFailures:   &mockSimpleGauge{},
		PeMemoryAllocationFailures:           &mockSimpleGauge{},
		PeInjectionContextAllocated:          &mockSimpleGauge{},
		PeInjectionContextCleanedup:          &mockSimpleGauge{},
	}
}

////////////////////////////////////////////////////////////

// testOpenDriverHandle is a mock implementation to avoid opening a real driver.
func testOpenDriverHandle() (windows.Handle, error) {
	return windows.InvalidHandle, nil
}

// testQueryDriverCounters is a mock implementation of queryDriverCounters for testing.
func testQueryDriverCounters(_ windows.Handle, request *DDInjectorCounterRequest, counters *DDInjectorCountersV1) error {
	assert.NotNil(currentTest, request)
	assert.NotNil(currentTest, counters)
	assert.Equal(currentTest, int(request.RequestedVersion), countersVersion)
	*counters = mockCounters
	return mockQueryCountersError
}

// overrideDriverCallbacks swaps out driver specific API calls to facilitate mocking.
func overrideDriverCallbacks() {
	OverrideOpenDriverHandle(testOpenDriverHandle)
	OverrideQueryDriverCounters(testQueryDriverCounters)
}

// restoreDriverCallbacks restores the original driver specific API calls.
func restoreDriverCallbacks() {
	OverrideOpenDriverHandle(openDriverHandle)
	OverrideQueryDriverCounters(queryDriverCounters)
}

func TestQueryCounters(t *testing.T) {
	// needed for assertions in callbacks
	currentTest = t

	overrideDriverCallbacks()
	defer restoreDriverCallbacks()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	// setup fake values
	mockCounters.ProcessesAddedToInjectionTracker = 1
	mockCounters.ProcessesRemovedFromInjectionTracker = 2
	mockCounters.ProcessesSkippedSubsystem = 3
	mockCounters.ProcessesSkippedContainer = 4
	mockCounters.ProcessesSkippedProtected = 5
	mockCounters.ProcessesSkippedSystem = 6
	mockCounters.ProcessesSkippedExcluded = 7
	mockCounters.InjectionAttempts = 8
	mockCounters.InjectionAttemptFailures = 9
	mockCounters.InjectionMaxTimeUs = 10
	mockCounters.InjectionSuccesses = 11
	mockCounters.InjectionFailures = 12
	mockCounters.PeCachingFailures = 13
	mockCounters.ImportDirectoryRestorationFailures = 14
	mockCounters.PeMemoryAllocationFailures = 15
	mockCounters.PeInjectionContextAllocated = 16
	mockCounters.PeInjectionContextCleanedup = 17

	// setup no error
	mockQueryCountersError = nil

	// Create counters with mock telemetry gauges
	counters := createCounters()

	// Call GetCounters with new API (pass pointer)
	err = inj.GetCounters(counters)
	assert.NoError(t, err)
	assertCountersEqual(t, &mockCounters, counters)

	inj.Close()
}

func TestQueryCountersError(t *testing.T) {
	// needed for assertions in callbacks
	currentTest = t

	overrideDriverCallbacks()
	defer restoreDriverCallbacks()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	// setup fake error
	mockQueryCountersError = errors.New("mock error")

	// Create counters with mock telemetry gauges
	counters := createCounters()

	// Call GetCounters with new API (pass pointer) - should return error
	err = inj.GetCounters(counters)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock error")

	inj.Close()
}

func assertCountersEqual(t *testing.T, expected *DDInjectorCountersV1, actual *InjectorCounters) {
	assert.Equal(t, float64(expected.ProcessesAddedToInjectionTracker), actual.ProcessesAddedToInjectionTracker.Get())
	assert.Equal(t, float64(expected.ProcessesRemovedFromInjectionTracker), actual.ProcessesRemovedFromInjectionTracker.Get())
	assert.Equal(t, float64(expected.ProcessesSkippedSubsystem), actual.ProcessesSkippedSubsystem.Get())
	assert.Equal(t, float64(expected.ProcessesSkippedContainer), actual.ProcessesSkippedContainer.Get())
	assert.Equal(t, float64(expected.ProcessesSkippedProtected), actual.ProcessesSkippedProtected.Get())
	assert.Equal(t, float64(expected.ProcessesSkippedSystem), actual.ProcessesSkippedSystem.Get())
	assert.Equal(t, float64(expected.ProcessesSkippedExcluded), actual.ProcessesSkippedExcluded.Get())
	assert.Equal(t, float64(expected.InjectionAttempts), actual.InjectionAttempts.Get())
	assert.Equal(t, float64(expected.InjectionAttemptFailures), actual.InjectionAttemptFailures.Get())
	assert.Equal(t, float64(expected.InjectionMaxTimeUs), actual.InjectionMaxTimeUs.Get())
	assert.Equal(t, float64(expected.InjectionSuccesses), actual.InjectionSuccesses.Get())
	assert.Equal(t, float64(expected.InjectionFailures), actual.InjectionFailures.Get())
	assert.Equal(t, float64(expected.PeCachingFailures), actual.PeCachingFailures.Get())
	assert.Equal(t, float64(expected.ImportDirectoryRestorationFailures), actual.ImportDirectoryRestorationFailures.Get())
	assert.Equal(t, float64(expected.PeMemoryAllocationFailures), actual.PeMemoryAllocationFailures.Get())
	assert.Equal(t, float64(expected.PeInjectionContextAllocated), actual.PeInjectionContextAllocated.Get())
	assert.Equal(t, float64(expected.PeInjectionContextCleanedup), actual.PeInjectionContextCleanedup.Get())
}
