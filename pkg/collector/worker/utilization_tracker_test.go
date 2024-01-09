// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"math/rand"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers

//nolint:revive // TODO(AML) Fix revive linter
func newTracker(t *testing.T) (*UtilizationTracker, *clock.Mock) {
	clk := clock.NewMock()
	ut := newUtilizationTrackerWithClock(
		"worker",
		100*time.Millisecond,
		clk,
	)

	return ut, clk
}

// Tests

func TestUtilizationTracker(t *testing.T) {
	ut, clk := newTracker(t)
	defer ut.Stop()

	old := 0.0
	//nolint:revive // TODO(AML) Fix revive linter
	new := 0.0

	// After some time without any checks running, the utilization
	// should be a constant zero value
	clk.Add(300 * time.Millisecond)
	ut.Tick()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	require.Equal(t, old, new)

	clk.Add(300 * time.Millisecond)
	// Ramp up the expected utilization
	ut.CheckStarted()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	require.Equal(t, old, new)

	clk.Add(250 * time.Millisecond)
	ut.Tick()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	require.Greater(t, new, old)

	clk.Add(550 * time.Millisecond)
	ut.Tick()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	require.Greater(t, new, old)

	// Ramp down the expected utilization
	ut.CheckFinished()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	require.Equal(t, old, new) //no time have passed

	clk.Add(250 * time.Millisecond)
	ut.Tick()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	require.Less(t, new, old)

	clk.Add(550 * time.Millisecond)
	ut.Tick()
	require.Less(t, new, old)
}

func TestUtilizationTrackerCheckLifecycle(t *testing.T) {
	ut, clk := newTracker(t)
	defer ut.Stop()

	//nolint:revive // TODO(AML) Fix revive linter
	var old, new float64

	// No tasks should equal no utilization
	clk.Add(250 * time.Millisecond)
	ut.Tick()
	//nolint:revive // TODO(AML) Fix revive linter
	old, new = new, <-ut.Output
	assert.Equal(t, old, new)

	for idx := 0; idx < 3; idx++ {
		// Ramp up utilization
		ut.CheckStarted()
		//nolint:revive // TODO(AML) Fix revive linter
		old, new = new, <-ut.Output
		assert.Equal(t, old, new)

		clk.Add(250 * time.Millisecond)
		ut.Tick()
		//nolint:revive // TODO(AML) Fix revive linter
		old, new = new, <-ut.Output
		assert.Greater(t, new, old)

		clk.Add(250 * time.Millisecond)
		ut.Tick()
		//nolint:revive // TODO(AML) Fix revive linter
		old, new = new, <-ut.Output
		assert.Greater(t, new, old)

		// Ramp down utilization
		ut.CheckFinished()
		//nolint:revive // TODO(AML) Fix revive linter
		old, new = new, <-ut.Output
		assert.Equal(t, new, old)

		clk.Add(250 * time.Millisecond)
		ut.Tick()
		//nolint:revive // TODO(AML) Fix revive linter
		old, new = new, <-ut.Output
		assert.Less(t, new, old)

		clk.Add(250 * time.Millisecond)
		ut.Tick()
		//nolint:revive // TODO(AML) Fix revive linter
		old, new = new, <-ut.Output
		assert.Less(t, new, old)
	}
}

func TestUtilizationTrackerAccuracy(t *testing.T) {
	ut, clk := newTracker(t)

	val := 0.0

	// It would be nice to figure out a way to compute bounds for the
	// smoothed value that would work for any random sequence.
	r := rand.New(rand.NewSource(1))

	for checkIdx := 1; checkIdx <= 100; checkIdx++ {
		// This should provide about 30% utilization
		// Range for the full loop would be between 100-200ms
		totalMs := r.Int31n(100) + 100
		runtimeMs := (totalMs * 30) / 100

		ut.CheckStarted()
		<-ut.Output

		runtimeDuration := time.Duration(runtimeMs) * time.Millisecond
		clk.Add(runtimeDuration)

		ut.CheckFinished()
		val = <-ut.Output

		idleDuration := time.Duration(totalMs-runtimeMs) * time.Millisecond
		clk.Add(idleDuration)

		if checkIdx > 30 {
			require.InDelta(t, 0.3, val, 0.07)
		}
	}

	require.InDelta(t, 0.3, val, 0.07)
}
