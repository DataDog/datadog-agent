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
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("name", "instance", 2*time.Second, clock)

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

// runBusyIterations drives the utilization monitor at ~99% utilization for n
// samples (990ms busy, 10ms idle per sample at 1s sampleRate).
func runBusyIterations(um *TelemetryUtilizationMonitor, clk interface{ Add(time.Duration) }, n int) {
	for i := 0; i < n; i++ {
		um.Start()
		clk.Add(990 * time.Millisecond)
		um.Stop()
		clk.Add(10 * time.Millisecond)
	}
}

// runIdleIterations drives the utilization monitor at ~0.1% utilization for n
// samples (1ms busy, 999ms idle per sample at 1s sampleRate).
func runIdleIterations(um *TelemetryUtilizationMonitor, clk interface{ Add(time.Duration) }, n int) {
	for i := 0; i < n; i++ {
		um.Start()
		clk.Add(1 * time.Millisecond)
		um.Stop()
		clk.Add(999 * time.Millisecond)
	}
}

// TestSample_BlockedComponentObserved is the core regression for the event-driven
// sampling blind spot. A component enters in-use (Start) and never returns (Stop) —
// modelling a goroutine blocked on a full output channel, which is the backpressure
// event we most want to observe. With only Start/Stop sampling, no sample would ever be
// taken and the EWMA would freeze at 0. The periodic sample() must credit the in-progress
// in-use interval so the EWMA climbs to saturation while the component is still blocked.
func TestSample_BlockedComponentObserved(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk)

	um.Start() // enter in-use; deliberately never call Stop

	for i := 0; i < 40; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}

	require.GreaterOrEqual(t, um.avg, SaturationThreshold,
		"a component blocked in-use must reach saturation via periodic sampling, not freeze at 0")
	assert.True(t, um.isSaturated, "saturation state must flip while still blocked, before any Stop")
}

// TestSample_IdleComponentStaysLow verifies the symmetric case: periodic sampling of a
// component that is not in-use credits idle time, so the EWMA stays at 0 rather than being
// pulled up. Guards against settleLocked mis-attributing idle ticks as in-use.
func TestSample_IdleComponentStaysLow(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk)

	// Never Start: the open interval is idle the whole time.
	for i := 0; i < 40; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}

	assert.InDelta(t, 0.0, um.avg, 0.0001, "an idle component sampled periodically must stay at 0 utilization")
	assert.False(t, um.isSaturated)
}

// TestSaturationStateOnset verifies that the saturation state machine marks a
// component as saturated once the EWMA reaches SaturationThreshold.
func TestSaturationStateOnset(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk)

	require.False(t, um.isSaturated, "should start healthy")

	runBusyIterations(um, clk, 40)

	require.GreaterOrEqual(t, um.avg, SaturationThreshold, "EWMA must reach saturation threshold")
	assert.True(t, um.isSaturated, "isSaturated must flip true at onset")
	assert.False(t, um.saturatedSince.IsZero(), "saturatedSince must be recorded at onset")
}

// TestRecoveryDebounce_StaysInSaturatedState verifies that a single dip below
// SaturationThreshold does not immediately trigger recovery — the EWMA must
// stay below threshold for at least recoveryDebounce before recovery is logged.
func TestRecoveryDebounce_StaysInSaturatedState(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk)

	runBusyIterations(um, clk, 40)
	require.True(t, um.isSaturated)

	// The first idle Start() fires with the prior busy period's rawRatio; need a second
	// idle iteration for a genuinely low rawRatio to update the EWMA below threshold.
	runIdleIterations(um, clk, 5)

	require.Less(t, um.avg, SaturationThreshold, "EWMA must have dropped below threshold")
	assert.True(t, um.isSaturated, "must remain saturated while debounce pending")
	assert.False(t, um.pendingRecoverySince.IsZero(), "debounce timer must start running")
}

// TestRecoveryDebounce_FiresAfterWindow verifies that the recovery state
// transition (isSaturated → false) fires only after the EWMA has stayed below
// SaturationThreshold for at least recoveryDebounce (10s).
func TestRecoveryDebounce_FiresAfterWindow(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk)

	runBusyIterations(um, clk, 40)
	require.True(t, um.isSaturated)

	// 20 idle iterations: the 2nd fires the first low-EWMA sample (sets pendingRecoverySince),
	// and by the 12th the debounce window (10s) has elapsed, triggering recovery.
	runIdleIterations(um, clk, 20)

	assert.False(t, um.isSaturated, "must have recovered after debounce window")
	assert.True(t, um.pendingRecoverySince.IsZero(), "debounce timer must be cleared after recovery")
}

// TestRecoveryDebounce_ResetByReSaturation verifies that if the EWMA rises
// back above SaturationThreshold before the recovery debounce fires, the
// pending recovery is cancelled.
func TestRecoveryDebounce_ResetByReSaturation(t *testing.T) {
	clk := clock.NewMock()
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("comp", "0", time.Second, clk)

	runBusyIterations(um, clk, 40)
	require.True(t, um.isSaturated)

	// 2 idle iterations: iter 1 fires with leftover busy rawRatio; iter 2 fires the
	// genuinely low sample that drops EWMA below threshold and starts the debounce.
	runIdleIterations(um, clk, 2)
	require.Less(t, um.avg, SaturationThreshold)
	require.False(t, um.pendingRecoverySince.IsZero(), "debounce must be running")

	// Re-saturate: 40 busy samples bring EWMA back above threshold.
	runBusyIterations(um, clk, 40)
	require.GreaterOrEqual(t, um.avg, SaturationThreshold)

	assert.True(t, um.pendingRecoverySince.IsZero(), "debounce timer must be cancelled on re-saturation")
}
