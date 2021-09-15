// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"expvar"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
)

// Helpers

// getWorkerUtilizationExpvar returns the utilization as presented by expvars
// for a named worker.
func getWorkerUtilizationExpvar(t *testing.T, name string) float64 {
	runnerMapExpvar := expvar.Get("runner")
	require.NotNil(t, runnerMapExpvar)

	workersExpvar := runnerMapExpvar.(*expvar.Map).Get("Workers")
	require.NotNil(t, workersExpvar)

	instancesExpvar := workersExpvar.(*expvar.Map).Get("Instances")
	require.NotNil(t, instancesExpvar)

	workerStatsExpvar := instancesExpvar.(*expvar.Map).Get(name)
	require.NotNil(t, workerStatsExpvar)

	workerStats := workerStatsExpvar.(*expvars.WorkerStats)
	require.NotNil(t, workerStats)

	return workerStats.Utilization
}

func newTracker(t *testing.T) UtilizationTracker {
	ut, err := NewUtilizationTracker("worker", 500*time.Millisecond, 100*time.Millisecond)
	require.Nil(t, err)
	AssertAsyncWorkerCount(t, 0)

	return ut
}

// Tests

func TestUtilizationTracker(t *testing.T) {
	ut := newTracker(t)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	AssertAsyncWorkerCount(t, 1)

	// Initially and after some time without any checks running, the utilization
	// should be a constant zero value
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))

	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))

	// Ramp up the expected utilization
	ut.CheckStarted(false)

	time.Sleep(250 * time.Millisecond)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") > 0)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") < 1)

	time.Sleep(550 * time.Millisecond)
	require.Equal(t, 1.0, getWorkerUtilizationExpvar(t, "worker"))

	// Ramp down the expected utilization
	ut.CheckFinished()

	time.Sleep(250 * time.Millisecond)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") > 0)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") < 1)

	time.Sleep(550 * time.Millisecond)
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))
}

func TestUtilizationTrackerIsRunningLongCheck(t *testing.T) {
	ut := newTracker(t)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	require.False(t, ut.IsRunningLongCheck())

	for idx := 0; idx < 3; idx++ {
		ut.CheckStarted(false)
		assert.False(t, ut.IsRunningLongCheck())
		ut.CheckFinished()

		ut.CheckStarted(true)
		assert.True(t, ut.IsRunningLongCheck())
		ut.CheckFinished()
	}
}

func TestUtilizationTrackerStart(t *testing.T) {
	ut := newTracker(t)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	AssertAsyncWorkerCount(t, 1)
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))

	// Check that on consecutive calls we don't break
	require.Error(t, ut.Start())
	require.Error(t, ut.Start())

	AssertAsyncWorkerCount(t, 1)
}

func TestUtilizationTrackerStop(t *testing.T) {
	ut := newTracker(t)

	// If we haven't started yet, stopping should throw an error
	require.Error(t, ut.Stop())

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	AssertAsyncWorkerCount(t, 1)
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))

	require.NoError(t, ut.Stop())
	AssertAsyncWorkerCount(t, 0)

	// Check that on consecutive calls we don't break
	require.Error(t, ut.Stop())
	require.Error(t, ut.Stop())
	AssertAsyncWorkerCount(t, 0)

	// A stopped tracker should not be able to start again
	require.Error(t, ut.Start())
}

func TestUtilizationTrackerCheckLifecycle(t *testing.T) {
	windowSize := 250 * time.Millisecond
	pollingInterval := 50 * time.Millisecond

	ut, err := NewUtilizationTracker("worker", windowSize, pollingInterval)
	require.Nil(t, err)
	AssertAsyncWorkerCount(t, 0)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	// No tasks should equal no utilization
	time.Sleep(windowSize)
	AssertAsyncWorkerCount(t, 1)
	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0)

	for idx := 0; idx < 3; idx++ {
		// Ramp up utilization
		ut.CheckStarted(false)

		time.Sleep(windowSize / 2)
		AssertAsyncWorkerCount(t, 1)
		assert.True(t, getWorkerUtilizationExpvar(t, "worker") > 0.1)
		assert.True(t, getWorkerUtilizationExpvar(t, "worker") < 0.9)

		time.Sleep(windowSize)
		AssertAsyncWorkerCount(t, 1)
		assert.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 1, 0.05)

		// Ramp down utilization
		ut.CheckFinished()

		time.Sleep(windowSize / 2)
		AssertAsyncWorkerCount(t, 1)
		assert.True(t, getWorkerUtilizationExpvar(t, "worker") > 0.1)
		assert.True(t, getWorkerUtilizationExpvar(t, "worker") < 0.9)

		time.Sleep(windowSize)
		AssertAsyncWorkerCount(t, 1)
		assert.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0.05)
	}
}

func TestUtilizationTrackerAccuracy(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping flaky test on Darwin")
	}

	windowSize := 3000 * time.Millisecond
	pollingInterval := 50 * time.Millisecond

	ut, err := NewUtilizationTracker("worker", windowSize, pollingInterval)
	require.Nil(t, err)
	AssertAsyncWorkerCount(t, 0)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0)

	go func() {
		// This should provide about 30% utilization
		for {
			// Range for the full loop would be between 100-200ms
			totalMs := rand.Int31n(100) + 100
			runtimeMs := (totalMs * 30) / 100

			ut.CheckStarted(false)
			runtimeDuration := time.Duration(runtimeMs) * time.Millisecond
			time.Sleep(runtimeDuration)

			ut.CheckFinished()
			idleDuration := time.Duration(totalMs-runtimeMs) * time.Millisecond
			time.Sleep(idleDuration)
		}
	}()

	for checkIdx := 1; checkIdx <= 10; checkIdx++ {
		// Every cycle, we should be getting closer and closer to 0.3. The
		// function below goes from 0.5 initially to ~0.1 at the end of the
		// iterator.
		delta := 0.5 - (0.40 * float64(checkIdx) / 10.0)

		time.Sleep(windowSize / 5)
		assert.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0.3, delta)
	}

	// Assert after many data points that we're really close to 0.3
	assert.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0.3, 0.07)
}

func TestUtilizationTrackerLongTaskAccuracy(t *testing.T) {
	var previousUtilization, currentUtilization float64

	ut, err := NewUtilizationTracker("worker", 1*time.Second, 25*time.Millisecond)
	require.Nil(t, err)
	AssertAsyncWorkerCount(t, 0)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0)

	go ut.CheckStarted(false)

	for checkIdx := 0; checkIdx < 10; checkIdx++ {
		time.Sleep(100 * time.Millisecond)

		currentUtilization = getWorkerUtilizationExpvar(t, "worker")

		if currentUtilization < 1 {
			require.True(t, currentUtilization > previousUtilization)
		}

		previousUtilization = currentUtilization
	}

	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 1.0, 0.05)
}
