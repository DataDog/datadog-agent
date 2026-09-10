// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// TestBeginNVMLUseCounting pins the gate's counting semantics: Begin is not a
// held lock, so nested users — a workloadmeta pull that triggers a device-cache
// refresh — cannot deadlock against each other or against a pending release.
func TestBeginNVMLUseCounting(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithSymbolsMock(allSymbols))
	WithMockNVML(t, mockNvml)
	t.Cleanup(func() { SetNVMLReleased(false) }) // don't leak the flag into other tests

	require.NoError(t, BeginNVMLUse())
	require.NoError(t, BeginNVMLUse(), "a nested user (pull → cache refresh) must not deadlock")

	EndNVMLUse()
	require.NoError(t, BeginNVMLUse(), "one user is still active")
	EndNVMLUse()
	EndNVMLUse()
}

// TestReleaseNVMLWaitsForInFlightUsers pins the drain semantics: arming the
// release rejects all new users immediately, and the shutdown waits for the
// in-flight users to finish before tearing the library down.
func TestReleaseNVMLWaitsForInFlightUsers(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithSymbolsMock(allSymbols))
	WithMockNVML(t, mockNvml)
	t.Cleanup(func() { SetNVMLReleased(false) }) // don't leak the flag into other tests
	mockNvml.ShutdownFunc = func() nvml.Return { return nvml.SUCCESS }

	require.NoError(t, BeginNVMLUse()) // in-flight user

	done := make(chan error, 1)
	go func() { done <- ReleaseNVML() }()

	// Wait for the arming to be observable instead of assuming it happened
	// within a fixed delay: if the Begin below were admitted, the count would
	// never drain to zero, the release would spin to its 30s timeout, and the
	// extra acquisition would leak into later tests.
	require.Eventually(t, nvmlReleased.Load, 5*time.Second, time.Millisecond,
		"the release must arm the flag before new acquisitions are refused")

	// the release is armed while the user is in flight: new users are refused
	err := BeginNVMLUse()
	assert.ErrorIs(t, err, ErrNVMLReleased, "new acquisitions must be refused while arming")

	// the in-flight user finishes → the release drains and completes
	EndNVMLUse()
	require.NoError(t, <-done)
	assert.True(t, nvmlReleased.Load())
}

// TestReleaseNVMLShutdownErrorDoesNotLatch pins the no-latch semantics: a
// failed shutdown must NOT leave the released flag armed over an initialized
// (still reset-blocking) library — the next cycle retries instead.
func TestReleaseNVMLShutdownErrorDoesNotLatch(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithSymbolsMock(allSymbols))
	WithMockNVML(t, mockNvml)
	t.Cleanup(func() { SetNVMLReleased(false) }) // don't leak the flag into other tests

	failShutdown := true
	mockNvml.ShutdownFunc = func() nvml.Return {
		if failShutdown {
			return nvml.ERROR_UNKNOWN
		}
		return nvml.SUCCESS
	}

	require.NoError(t, BeginNVMLUse())
	EndNVMLUse()

	require.Error(t, ReleaseNVML(), "the shutdown failed")
	assert.False(t, nvmlReleased.Load(), "a failed shutdown must not latch the released state")

	// once the shutdown succeeds, the release completes
	failShutdown = false
	require.NoError(t, ReleaseNVML())
	assert.True(t, nvmlReleased.Load())
}

// TestReacquireAfterRelease covers the re-acquire path: clearing the released
// flag re-allows initialization, and the new library works.
func TestReacquireAfterRelease(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithSymbolsMock(allSymbols))
	WithMockNVML(t, mockNvml)
	t.Cleanup(func() { SetNVMLReleased(false) }) // don't leak the flag into other tests
	mockNvml.ShutdownFunc = func() nvml.Return { return nvml.SUCCESS }

	require.NoError(t, BeginNVMLUse())
	EndNVMLUse()
	require.NoError(t, ReleaseNVML())
	require.ErrorIs(t, BeginNVMLUse(), ErrNVMLReleased)

	SetNVMLReleased(false)
	// the re-acquire re-initializes via the (still installed) mock — on a
	// real node this is the driver dlopen
	WithMockNvmlNewFunc(t, func(_ ...nvml.LibraryOption) nvml.Interface {
		return mockNvml
	})
	// the re-init through the mock needs its Init handler
	mockNvml.InitFunc = func() nvml.Return { return nvml.SUCCESS }
	require.NoError(t, BeginNVMLUse(), "the re-acquire must re-initialize the library")
	EndNVMLUse()
}
