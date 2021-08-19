// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"expvar"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
)

// Helpers

// getWorkerUtilizationExpvar returns the utilization as presented by expvars
// for a particular named worker. Since we use this in `worker_test` too, the
// method is public
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
	ut, err := NewUtilizationTracker("worker", 50*time.Millisecond, 10*time.Millisecond)
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

	time.Sleep(60 * time.Millisecond)
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))

	// Ramp up the expected utilization
	ut.CheckStarted()

	time.Sleep(25 * time.Millisecond)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") > 0)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") < 1)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1.0, getWorkerUtilizationExpvar(t, "worker"))

	// Ramp down the expected utilization
	ut.CheckFinished()

	time.Sleep(25 * time.Millisecond)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") > 0)
	require.True(t, getWorkerUtilizationExpvar(t, "worker") < 1)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0.0, getWorkerUtilizationExpvar(t, "worker"))
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
	ut, err := NewUtilizationTracker("worker", 50*time.Millisecond, 10*time.Millisecond)
	require.Nil(t, err)
	AssertAsyncWorkerCount(t, 0)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	// No tasks should equal no utilization
	time.Sleep(50 * time.Millisecond)
	AssertAsyncWorkerCount(t, 1)
	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0)

	for idx := 0; idx < 10; idx++ {
		// Ramp up utilization
		ut.CheckStarted()

		time.Sleep(25 * time.Millisecond)
		AssertAsyncWorkerCount(t, 1)
		require.True(t, getWorkerUtilizationExpvar(t, "worker") > 0.1)
		require.True(t, getWorkerUtilizationExpvar(t, "worker") < 0.9)

		time.Sleep(50 * time.Millisecond)
		AssertAsyncWorkerCount(t, 1)
		require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 1, 0.05)

		// Ramp down utilization
		ut.CheckFinished()

		time.Sleep(25 * time.Millisecond)
		AssertAsyncWorkerCount(t, 1)
		require.True(t, getWorkerUtilizationExpvar(t, "worker") > 0.1)
		require.True(t, getWorkerUtilizationExpvar(t, "worker") < 0.9)

		time.Sleep(50 * time.Millisecond)
		AssertAsyncWorkerCount(t, 1)
		require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0.05)
	}
}

func TestUtilizationTrackerAccuracy(t *testing.T) {
	ut, err := NewUtilizationTracker("worker", 500*time.Millisecond, 20*time.Millisecond)
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
			totalMs := rand.Int31n(100)
			runtimeMs := (totalMs * 30) / 100

			ut.CheckStarted()
			runtimeDuration := time.Duration(runtimeMs) * time.Millisecond
			time.Sleep(runtimeDuration)

			ut.CheckFinished()
			idleDuration := time.Duration(totalMs-runtimeMs) * time.Millisecond
			time.Sleep(idleDuration)
		}
	}()

	for checkIdx := 0; checkIdx < 10; checkIdx++ {
		time.Sleep(100 * time.Millisecond)
		require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0.3, 0.2)
	}

	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0.3, 0.03)
}

func TestUtilizationTrackerLongTaskAccuracy(t *testing.T) {
	var previousUtilization, currentUtilization float64

	ut, err := NewUtilizationTracker("worker", 1*time.Second, 50*time.Millisecond)
	require.Nil(t, err)
	AssertAsyncWorkerCount(t, 0)

	require.NoError(t, ut.Start())
	defer func() {
		ut.Stop()
		AssertAsyncWorkerCount(t, 0)
	}()

	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 0, 0)

	go ut.CheckStarted()

	for checkIdx := 0; checkIdx < 10; checkIdx++ {
		time.Sleep(100 * time.Millisecond)

		currentUtilization = getWorkerUtilizationExpvar(t, "worker")
		require.True(t, getWorkerUtilizationExpvar(t, "worker") > previousUtilization)
		previousUtilization = currentUtilization
	}

	require.InDelta(t, getWorkerUtilizationExpvar(t, "worker"), 1.0, 0.05)
}
