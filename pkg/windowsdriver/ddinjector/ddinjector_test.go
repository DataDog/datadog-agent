// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjector

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

var currentTest *testing.T
var mockCounters DDInjectorCountersV1
var mockQueryCountersError error

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

	counters, err := inj.GetCounters()
	assert.NoError(t, err)
	assert.NotNil(t, counters)
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

	counters, err := inj.GetCounters()
	assert.NotNil(t, err)
	assert.Nil(t, counters)

	inj.Close()
}

func assertCountersEqual(t *testing.T, expected *DDInjectorCountersV1, actual *InjectorCounters) {
	assert.Equal(t, int64(expected.ProcessesAddedToInjectionTracker), int64(actual.ProcessesAddedToInjectionTracker))
	assert.Equal(t, int64(expected.ProcessesRemovedFromInjectionTracker), int64(actual.ProcessesRemovedFromInjectionTracker))
	assert.Equal(t, int64(expected.ProcessesSkippedSubsystem), int64(actual.ProcessesSkippedSubsystem))
	assert.Equal(t, int64(expected.ProcessesSkippedContainer), int64(actual.ProcessesSkippedContainer))
	assert.Equal(t, int64(expected.ProcessesSkippedProtected), int64(actual.ProcessesSkippedProtected))
	assert.Equal(t, int64(expected.ProcessesSkippedSystem), int64(actual.ProcessesSkippedSystem))
	assert.Equal(t, int64(expected.ProcessesSkippedExcluded), int64(actual.ProcessesSkippedExcluded))
	assert.Equal(t, int64(expected.InjectionAttempts), int64(actual.InjectionAttempts))
	assert.Equal(t, int64(expected.InjectionAttemptFailures), int64(actual.InjectionAttemptFailures))
	assert.Equal(t, int64(expected.InjectionMaxTimeUs), int64(actual.InjectionMaxTimeUs))
	assert.Equal(t, int64(expected.InjectionSuccesses), int64(actual.InjectionSuccesses))
	assert.Equal(t, int64(expected.InjectionFailures), int64(actual.InjectionFailures))
	assert.Equal(t, int64(expected.PeCachingFailures), int64(actual.PeCachingFailures))
	assert.Equal(t, int64(expected.ImportDirectoryRestorationFailures), int64(actual.ImportDirectoryRestorationFailures))
	assert.Equal(t, int64(expected.PeMemoryAllocationFailures), int64(actual.PeMemoryAllocationFailures))
	assert.Equal(t, int64(expected.PeInjectionContextAllocated), int64(actual.PeInjectionContextAllocated))
	assert.Equal(t, int64(expected.PeInjectionContextCleanedup), int64(actual.PeInjectionContextCleanedup))
}
