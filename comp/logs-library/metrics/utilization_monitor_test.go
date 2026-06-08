// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUtilizationMonitorLifecycle(t *testing.T) {
	clock := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("name", "instance", 2*time.Second, clock, nil)

	// Converge on 50% utilization
	for i := 0; i < 100; i++ {
		um.Start()
		clock.Add(1 * time.Second)

		um.Stop()
		clock.Add(1 * time.Second)
	}

	require.InDelta(t, 0.5, um.avg, 0.01)

	// Converge on 100% utilization
	for i := 0; i < 100; i++ {
		um.Start()
		clock.Add(1 * time.Second)

		um.Stop()
		clock.Add(1 * time.Millisecond)
	}

	require.InDelta(t, 0.99, um.avg, 0.01)

	// Converge on 0% utilization
	for i := 0; i < 200; i++ {
		um.Start()
		clock.Add(1 * time.Millisecond)

		um.Stop()
		clock.Add(1 * time.Second)
	}

	require.InDelta(t, 0.0, um.avg, 0.01)

}

// runBusyIterations drives the monitor at ~99% utilization for n samples.
func runBusyIterations(um *TelemetryUtilizationMonitor, clk interface{ Add(time.Duration) }, n int) {
	for i := 0; i < n; i++ {
		um.Start()
		clk.Add(990 * time.Millisecond)
		um.Stop()
		clk.Add(10 * time.Millisecond)
	}
}

// runIdleIterations drives the monitor at ~0.1% utilization for n samples.
func runIdleIterations(um *TelemetryUtilizationMonitor, clk interface{ Add(time.Duration) }, n int) {
	for i := 0; i < n; i++ {
		um.Start()
		clk.Add(1 * time.Millisecond)
		um.Stop()
		clk.Add(999 * time.Millisecond)
	}
}

// TestSample_BlockedComponentObserved checks a component blocked in-use still climbs to saturation via periodic sampling.
func TestSample_BlockedComponentObserved(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk, nil)

	um.Start() // never Stop

	for i := 0; i < 40; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}

	require.GreaterOrEqual(t, um.avg, SaturationThreshold,
		"a component blocked in-use must reach saturation via periodic sampling, not freeze at 0")
	assert.True(t, um.isSaturated, "saturation state must flip while still blocked, before any Stop")
}

// TestSample_IdleComponentStaysLow checks periodic sampling of a never-in-use component keeps the EWMA at 0.
func TestSample_IdleComponentStaysLow(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk, nil)

	for i := 0; i < 40; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}

	assert.InDelta(t, 0.0, um.avg, 0.0001, "an idle component sampled periodically must stay at 0 utilization")
	assert.False(t, um.isSaturated)
}

// TestSaturationStateOnset checks that the state machine flips to saturated at the threshold.
func TestSaturationStateOnset(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk, nil)

	require.False(t, um.isSaturated, "should start healthy")

	runBusyIterations(um, clk, 40)

	require.GreaterOrEqual(t, um.avg, SaturationThreshold, "EWMA must reach saturation threshold")
	assert.True(t, um.isSaturated, "isSaturated must flip true at onset")
	assert.False(t, um.saturatedSince.IsZero(), "saturatedSince must be recorded at onset")
}

// TestRecoveryDebounce_StaysInSaturatedState checks a dip below threshold doesn't recover until recoveryDebounce elapses.
func TestRecoveryDebounce_StaysInSaturatedState(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk, nil)

	runBusyIterations(um, clk, 40)
	require.True(t, um.isSaturated)

	runIdleIterations(um, clk, 5)

	require.Less(t, um.avg, SaturationThreshold, "EWMA must have dropped below threshold")
	assert.True(t, um.isSaturated, "must remain saturated while debounce pending")
	assert.False(t, um.pendingRecoverySince.IsZero(), "debounce timer must start running")
}

// TestRecoveryDebounce_FiresAfterWindow checks recovery fires only after the EWMA stays low for recoveryDebounce.
func TestRecoveryDebounce_FiresAfterWindow(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk, nil)

	runBusyIterations(um, clk, 40)
	require.True(t, um.isSaturated)

	runIdleIterations(um, clk, 20)

	assert.False(t, um.isSaturated, "must have recovered after debounce window")
	assert.True(t, um.pendingRecoverySince.IsZero(), "debounce timer must be cleared after recovery")
}

// TestRecoveryDebounce_ResetByReSaturation checks re-saturating before the debounce fires cancels the pending recovery.
func TestRecoveryDebounce_ResetByReSaturation(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk, nil)

	runBusyIterations(um, clk, 40)
	require.True(t, um.isSaturated)

	runIdleIterations(um, clk, 2)
	require.Less(t, um.avg, SaturationThreshold)
	require.False(t, um.pendingRecoverySince.IsZero(), "debounce must be running")

	runBusyIterations(um, clk, 40)
	require.GreaterOrEqual(t, um.avg, SaturationThreshold)

	assert.True(t, um.pendingRecoverySince.IsZero(), "debounce timer must be cancelled on re-saturation")
}
