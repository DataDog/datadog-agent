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

// mock driver state, configured per test.
var (
	mockCapabilitiesVersion   uint32
	mockCapabilitiesError     error
	mockCountersV1            DDInjectorCountersV1
	mockCountersV2            DDInjectorCountersV2
	mockQueryCountersError    error
	mockBytesReturnedOverride uint32 // if non-zero, returned instead of the full struct size
	mockRequestedVersion      uint32 // records the version the caller requested
)

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
		CrashesDuringInjection:               &mockSimpleGauge{},
		CrashesPostInjection:                 &mockSimpleGauge{},
		BootRecoveryCrashBootsDetected:       &mockSimpleGauge{},
		BootRecoveryDriverSelfDisabled:       &mockSimpleGauge{},
		BootRecoveryStabilityTimerFired:      &mockSimpleGauge{},
	}
}

////////////////////////////////////////////////////////////

// testOpenDriverHandle is a mock implementation to avoid opening a real driver.
func testOpenDriverHandle() (windows.Handle, error) {
	return windows.InvalidHandle, nil
}

// testQueryDriverCapabilities is a mock implementation of queryDriverCapabilities for testing.
func testQueryDriverCapabilities(_ windows.Handle) (uint32, error) {
	return mockCapabilitiesVersion, mockCapabilitiesError
}

// testQueryDriverCounters is a mock implementation of queryDriverCounters for testing.
// It copies the mock counters matching the requested version into the buffer and
// returns a configurable number of bytes.
func testQueryDriverCounters(_ windows.Handle, request *DDInjectorCounterRequest, buffer unsafe.Pointer, bufferSize uint32) (uint32, error) {
	assert.NotNil(currentTest, request)
	assert.NotNil(currentTest, buffer)

	mockRequestedVersion = uint32(request.RequestedVersion)

	if mockQueryCountersError != nil {
		return 0, mockQueryCountersError
	}

	var bytesReturned uint32
	if mockRequestedVersion >= CountersVersion2 {
		assert.GreaterOrEqual(currentTest, bufferSize, uint32(DDInjectorCountersV2Size))
		*(*DDInjectorCountersV2)(buffer) = mockCountersV2
		bytesReturned = uint32(DDInjectorCountersV2Size)
	} else {
		assert.GreaterOrEqual(currentTest, bufferSize, uint32(DDInjectorCountersV1Size))
		*(*DDInjectorCountersV1)(buffer) = mockCountersV1
		bytesReturned = uint32(DDInjectorCountersV1Size)
	}

	if mockBytesReturnedOverride != 0 {
		bytesReturned = mockBytesReturnedOverride
	}

	return bytesReturned, nil
}

// overrideDriverCallbacks swaps out driver specific API calls to facilitate mocking.
func overrideDriverCallbacks() {
	OverrideOpenDriverHandle(testOpenDriverHandle)
	OverrideQueryDriverCapabilities(testQueryDriverCapabilities)
	OverrideQueryDriverCounters(testQueryDriverCounters)
}

// restoreDriverCallbacks restores the original driver specific API calls.
func restoreDriverCallbacks() {
	OverrideOpenDriverHandle(openDriverHandle)
	OverrideQueryDriverCapabilities(queryDriverCapabilities)
	OverrideQueryDriverCounters(queryDriverCounters)
}

// resetMockState clears mock configuration between tests so they do not leak
// into each other.
func resetMockState() {
	mockCapabilitiesVersion = 0
	mockCapabilitiesError = nil
	mockCountersV1 = DDInjectorCountersV1{}
	mockCountersV2 = DDInjectorCountersV2{}
	mockQueryCountersError = nil
	mockBytesReturnedOverride = 0
	mockRequestedVersion = 0
}

// fillMockCountersV1 populates mockCountersV1 with deterministic test values.
func fillMockCountersV1() {
	mockCountersV1 = DDInjectorCountersV1{}
	mockCountersV1.ProcessesAddedToInjectionTracker = 1
	mockCountersV1.ProcessesRemovedFromInjectionTracker = 2
	mockCountersV1.ProcessesSkippedSubsystem = 3
	mockCountersV1.ProcessesSkippedContainer = 4
	mockCountersV1.ProcessesSkippedProtected = 5
	mockCountersV1.ProcessesSkippedSystem = 6
	mockCountersV1.ProcessesSkippedExcluded = 7
	mockCountersV1.InjectionAttempts = 8
	mockCountersV1.InjectionAttemptFailures = 9
	mockCountersV1.InjectionMaxTimeUs = 10
	mockCountersV1.InjectionSuccesses = 11
	mockCountersV1.InjectionFailures = 12
	mockCountersV1.PeCachingFailures = 13
	mockCountersV1.ImportDirectoryRestorationFailures = 14
	mockCountersV1.PeMemoryAllocationFailures = 15
	mockCountersV1.PeInjectionContextAllocated = 16
	mockCountersV1.PeInjectionContextCleanedup = 17
}

// fillMockCountersV2 populates mockCountersV2 (V1 head + V2 tail) with deterministic values.
func fillMockCountersV2() {
	fillMockCountersV1()
	mockCountersV2 = DDInjectorCountersV2{}
	// The V1 counters live at the head of the V2 struct; copy them via a pointer reinterpret.
	*(*DDInjectorCountersV1)(unsafe.Pointer(&mockCountersV2)) = mockCountersV1
	mockCountersV2.CrashesDuringInjection = 100
	mockCountersV2.CrashesPostInjection = 101
	mockCountersV2.BootRecoveryCrashBootsDetected = 102
	mockCountersV2.BootRecoveryDriverSelfDisabled = 103
	mockCountersV2.BootRecoveryStabilityTimerFired = 104
}

// TestQueryCountersV2 covers a driver that supports V2 counters: both V1 and V2
// gauges are populated and the negotiated request version is 2.
func TestQueryCountersV2(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion2
	fillMockCountersV2()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	assert.Equal(t, CountersVersion2, mockRequestedVersion)
	assertV1CountersEqual(t, &mockCountersV1, counters)
	assertV2CountersEqual(t, &mockCountersV2, counters)

	inj.Close()
}

// TestQueryCountersV1Only covers a driver that only supports V1: V1 gauges are
// populated, V2 gauges stay at 0, and the negotiated request version is 1.
func TestQueryCountersV1Only(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion1
	fillMockCountersV1()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	assert.Equal(t, CountersVersion1, mockRequestedVersion)
	assertV1CountersEqual(t, &mockCountersV1, counters)
	assertV2CountersZero(t, counters)

	inj.Close()
}

// TestQueryCountersCapabilitiesUnsupported covers an older driver that does not
// implement the capabilities IOCTL: we fall back to V1 without surfacing an error.
func TestQueryCountersCapabilitiesUnsupported(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesError = errors.New("capabilities IOCTL not supported")
	fillMockCountersV1()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	assert.Equal(t, CountersVersion1, mockRequestedVersion)
	assertV1CountersEqual(t, &mockCountersV1, counters)
	assertV2CountersZero(t, counters)

	inj.Close()
}

// TestQueryCountersFallbackToV1ClearsV2Counters covers a long-lived system-probe
// process observing a V2 driver first, then falling back to the V1 contract.
func TestQueryCountersFallbackToV1ClearsV2Counters(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion2
	fillMockCountersV2()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)
	assertV2CountersEqual(t, &mockCountersV2, counters)

	mockCapabilitiesVersion = 0
	mockCapabilitiesError = errors.New("capabilities IOCTL not supported")
	fillMockCountersV1()

	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	assert.Equal(t, CountersVersion1, mockRequestedVersion)
	assertV1CountersEqual(t, &mockCountersV1, counters)
	assertV2CountersZero(t, counters)

	inj.Close()
}

// TestQueryCountersDriverNewerThanAgent covers a driver advertising a version
// beyond what the agent understands: the negotiated version is clamped to
// maxKnownVersion (V2).
func TestQueryCountersDriverNewerThanAgent(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion2 + 1 // driver claims V3
	fillMockCountersV2()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	assert.Equal(t, maxKnownVersion, mockRequestedVersion)
	assertV1CountersEqual(t, &mockCountersV1, counters)
	assertV2CountersEqual(t, &mockCountersV2, counters)

	inj.Close()
}

// TestQueryCountersDriverBelowV1 covers a driver that answers the capabilities
// query but reports a version below the V1 baseline (e.g. 0): the agent
// negotiates "no version", skips the counters query, and leaves every gauge at
// zero.
func TestQueryCountersDriverBelowV1(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = 0 // driver reports no supported counter version, with no error
	fillMockCountersV1()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	// No counters query should have been issued and no gauges populated.
	assert.Zero(t, mockRequestedVersion)
	assertV1CountersZero(t, counters)
	assertV2CountersZero(t, counters)

	inj.Close()
}

// TestQueryCountersDriverBelowV1ClearsCounters covers a long-lived system-probe
// process that first observes a V2 driver, then a driver reporting no supported
// version: all previously collected gauges are cleared and no query is issued.
func TestQueryCountersDriverBelowV1ClearsCounters(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion2
	fillMockCountersV2()

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.NoError(t, err)
	assertV1CountersEqual(t, &mockCountersV1, counters)
	assertV2CountersEqual(t, &mockCountersV2, counters)

	// Driver now reports no supported version.
	mockCapabilitiesVersion = 0
	mockRequestedVersion = 99 // sentinel; a skipped query leaves this untouched

	err = inj.GetCounters(counters)
	assert.NoError(t, err)

	assert.Equal(t, uint32(99), mockRequestedVersion)
	assertV1CountersZero(t, counters)
	assertV2CountersZero(t, counters)

	inj.Close()
}

// TestQueryCountersShortRead covers a size skew where the driver returns fewer
// bytes than the negotiated struct: GetCounters returns an error and no garbage
// values are set on the gauges.
func TestQueryCountersShortRead(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion2
	fillMockCountersV2()
	// Driver returns only a V1-sized payload for a negotiated V2 read.
	mockBytesReturnedOverride = uint32(DDInjectorCountersV1Size)

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected at least")

	// No gauges should have been populated.
	assertV1CountersZero(t, counters)
	assertV2CountersZero(t, counters)

	inj.Close()
}

// TestQueryCountersError covers the DeviceIoControl call itself failing.
func TestQueryCountersError(t *testing.T) {
	currentTest = t
	overrideDriverCallbacks()
	defer restoreDriverCallbacks()
	resetMockState()

	inj, err := NewInjector()
	assert.NoError(t, err)
	assert.NotNil(t, inj)

	mockCapabilitiesVersion = CountersVersion2
	mockQueryCountersError = errors.New("mock error")

	counters := createCounters()
	err = inj.GetCounters(counters)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock error")

	inj.Close()
}

func assertV1CountersEqual(t *testing.T, expected *DDInjectorCountersV1, actual *InjectorCounters) {
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

func assertV2CountersEqual(t *testing.T, expected *DDInjectorCountersV2, actual *InjectorCounters) {
	assert.Equal(t, float64(expected.CrashesDuringInjection), actual.CrashesDuringInjection.Get())
	assert.Equal(t, float64(expected.CrashesPostInjection), actual.CrashesPostInjection.Get())
	assert.Equal(t, float64(expected.BootRecoveryCrashBootsDetected), actual.BootRecoveryCrashBootsDetected.Get())
	assert.Equal(t, float64(expected.BootRecoveryDriverSelfDisabled), actual.BootRecoveryDriverSelfDisabled.Get())
	assert.Equal(t, float64(expected.BootRecoveryStabilityTimerFired), actual.BootRecoveryStabilityTimerFired.Get())
}

func assertV1CountersZero(t *testing.T, actual *InjectorCounters) {
	assert.Zero(t, actual.ProcessesAddedToInjectionTracker.Get())
	assert.Zero(t, actual.ProcessesRemovedFromInjectionTracker.Get())
	assert.Zero(t, actual.ProcessesSkippedSubsystem.Get())
	assert.Zero(t, actual.ProcessesSkippedContainer.Get())
	assert.Zero(t, actual.ProcessesSkippedProtected.Get())
	assert.Zero(t, actual.ProcessesSkippedSystem.Get())
	assert.Zero(t, actual.ProcessesSkippedExcluded.Get())
	assert.Zero(t, actual.InjectionAttempts.Get())
	assert.Zero(t, actual.InjectionAttemptFailures.Get())
	assert.Zero(t, actual.InjectionMaxTimeUs.Get())
	assert.Zero(t, actual.InjectionSuccesses.Get())
	assert.Zero(t, actual.InjectionFailures.Get())
	assert.Zero(t, actual.PeCachingFailures.Get())
	assert.Zero(t, actual.ImportDirectoryRestorationFailures.Get())
	assert.Zero(t, actual.PeMemoryAllocationFailures.Get())
	assert.Zero(t, actual.PeInjectionContextAllocated.Get())
	assert.Zero(t, actual.PeInjectionContextCleanedup.Get())
}

func assertV2CountersZero(t *testing.T, actual *InjectorCounters) {
	assert.Zero(t, actual.CrashesDuringInjection.Get())
	assert.Zero(t, actual.CrashesPostInjection.Get())
	assert.Zero(t, actual.BootRecoveryCrashBootsDetected.Get())
	assert.Zero(t, actual.BootRecoveryDriverSelfDisabled.Get())
	assert.Zero(t, actual.BootRecoveryStabilityTimerFired.Get())
}
