// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNvmlStateTracker_CheckUnavailable(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	tracker := NewNvmlStateTracker(telemetryComp)

	// First check - should increment error counter but not set unavailable gauge
	tracker.Check()
	assert.Equal(t, 1, tracker.errorCount)
	assert.False(t, tracker.unavailabilityMarked)
	assert.False(t, tracker.firstCheckTime.IsZero())

	// Simulate time passing beyond threshold (threshold is 5 minutes)
	tracker.mu.Lock()
	tracker.firstCheckTime = time.Now().Add(-6 * time.Minute)
	tracker.mu.Unlock()

	// Second check - should now mark as unavailable
	tracker.Check()
	assert.Equal(t, 2, tracker.errorCount)
	assert.True(t, tracker.unavailabilityMarked)
}

func TestNvmlStateTracker_CheckMultipleErrors(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	tracker := NewNvmlStateTracker(telemetryComp)

	// Multiple checks before threshold
	for i := 0; i < 5; i++ {
		tracker.Check()
	}

	assert.Equal(t, 5, tracker.errorCount)
	assert.False(t, tracker.unavailabilityMarked)
}

func TestNvmlStateTracker_CheckRecovery(t *testing.T) {
	// Skip this test if NVML is not available, as we can't test recovery
	// without being able to initialize the library successfully
	t.Skip("Skipping recovery test - requires NVML library to be available")
}

func TestNewNvmlStateTracker(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	tracker := NewNvmlStateTracker(telemetryComp)

	require.NotNil(t, tracker)
	require.NotNil(t, tracker.errorCounter)
	require.NotNil(t, tracker.unavailableGauge)
	assert.Equal(t, 0, tracker.errorCount)
	assert.True(t, tracker.firstCheckTime.IsZero())
	assert.False(t, tracker.unavailabilityMarked)
	assert.Equal(t, defaultCheckInterval, tracker.checkInterval)
	require.NotNil(t, tracker.done)
}

func TestNvmlStateTracker_StartStop(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	// Use a short interval for testing
	tracker := NewNvmlStateTrackerWithInterval(telemetryComp, 100*time.Millisecond)

	// Start the tracker
	tracker.Start()

	// Wait for a few checks to occur
	time.Sleep(350 * time.Millisecond)

	// Verify some checks occurred (error count should be > 0 since NVML is not available in test)
	tracker.mu.Lock()
	errorCount := tracker.errorCount
	tracker.mu.Unlock()

	assert.Greater(t, errorCount, 0, "Expected at least one check to have occurred")
	assert.LessOrEqual(t, errorCount, 5, "Expected no more than 5 checks in 350ms with 100ms interval")

	// Stop should cleanly shut down
	tracker.Stop()

	// After stop, no more checks should occur
	tracker.mu.Lock()
	errorCountAfterStop := tracker.errorCount
	tracker.mu.Unlock()

	time.Sleep(200 * time.Millisecond)

	tracker.mu.Lock()
	errorCountAfterWait := tracker.errorCount
	tracker.mu.Unlock()

	assert.Equal(t, errorCountAfterStop, errorCountAfterWait, "No checks should occur after Stop()")
}

func TestNvmlStateTracker_StartPerformsImmediateCheck(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	// Use a long interval so we can verify the immediate check
	tracker := NewNvmlStateTrackerWithInterval(telemetryComp, 10*time.Second)

	// Start the tracker
	tracker.Start()
	defer tracker.Stop()

	// Give it a moment to perform the initial check
	time.Sleep(50 * time.Millisecond)

	// Verify the immediate check occurred
	tracker.mu.Lock()
	errorCount := tracker.errorCount
	tracker.mu.Unlock()

	assert.Equal(t, 1, errorCount, "Expected exactly one check immediately after Start()")
}
