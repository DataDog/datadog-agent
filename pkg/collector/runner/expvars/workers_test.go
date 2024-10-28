// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

import (
	"expvar"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper methods

func getWorkersStatsExpvarMap(t *testing.T) *expvar.Map {
	runnerMap := getRunnerExpvarMap(t)
	mapExpvar := runnerMap.Get(workersExpvarKey)
	if !assert.NotNil(t, mapExpvar) {
		assert.FailNow(t, fmt.Sprintf("Variable '%s' not found in expvars!", workersExpvarKey))
	}

	return mapExpvar.(*expvar.Map)
}

func getWorkersCountExpvar(t *testing.T) int {
	workersStats := getWorkersStatsExpvarMap(t)
	countExpvar := workersStats.Get(countExpvarKey)
	if !assert.NotNil(t, countExpvar) {
		assert.FailNow(t, fmt.Sprintf("Variable '%s' not found in worker expvars!", countExpvarKey))
	}

	return int(countExpvar.(*expvar.Int).Value())
}

func getWorkerInstancesStats(t *testing.T) string {
	workersStats := getWorkersStatsExpvarMap(t)
	instancesExpvar := workersStats.Get(instancesExpvarKey)
	if !assert.NotNil(t, instancesExpvar) {
		assert.FailNow(t, fmt.Sprintf("Variable '%s' not found in worker expvars!", countExpvarKey))
	}

	return instancesExpvar.String()
}

func assertExpectedInitialState(t *testing.T) {
	workersStats := getWorkersStatsExpvarMap(t)
	keys := getExpvarMapKeys(workersStats)
	assert.Equal(t, []string{"Count", "Instances"}, keys)

	instances := workersStats.Get("Instances")
	require.NotNil(t, instances)
	assert.Equal(t, "{}", instances.String())

	count := workersStats.Get("Count")
	require.NotNil(t, count)
	assert.Equal(t, "0", count.String())

	assert.Equal(t, 0, GetWorkerCount())
}

func assertWorkerInstanceStats(t *testing.T, stats map[string]*WorkerStats) {
	workersStats := getWorkersStatsExpvarMap(t)
	keys := getExpvarMapKeys(workersStats)
	assert.Equal(t, []string{"Count", "Instances"}, keys)

	instances := workersStats.Get("Instances")
	require.NotNil(t, instances)

	instancesMap := instances.(*expvar.Map)

	keyCount := 0
	instancesMap.Do(func(kv expvar.KeyValue) {
		require.Equal(t, stats[kv.Key].String(), kv.Value.String())

		keyCount++
	})

	require.Equal(t, len(stats), keyCount)
}

// Tests

func TestWorkersInitialState(t *testing.T) {
	setUp()
	assertExpectedInitialState(t)
}

func TestWorkersStatsReset(t *testing.T) {
	setUp()

	expectedWorkers := 20

	require.Equal(t, 0, GetWorkerCount())
	require.Equal(t, 0, getWorkersCountExpvar(t))

	// Test addition of new stats
	for idx := 0; idx < expectedWorkers; idx++ {
		stats := &WorkerStats{}
		SetWorkerStats(fmt.Sprintf("stats %d", idx), stats)

		require.Equal(t, idx+1, GetWorkerCount())
		require.Equal(t, idx+1, getWorkersCountExpvar(t))
	}

	Reset()

	assertExpectedInitialState(t)
}

func TestWorkersInstances(t *testing.T) {
	setUp()

	stats1Update1 := &WorkerStats{Utilization: 1.01}
	stats1Update2 := &WorkerStats{Utilization: 1.02}
	stats1Update3 := &WorkerStats{Utilization: 1.03}

	stats2Update1 := &WorkerStats{Utilization: 2.01}

	SetWorkerStats("stats1", stats1Update1)
	SetWorkerStats("stats1", stats1Update2)
	SetWorkerStats("stats2", stats2Update1)
	SetWorkerStats("stats1", stats1Update3)

	// Sanity check to ensure that the output is exactly what we expect
	require.Equal(
		t,
		"{\"stats1\": {\"Utilization\": 1.03}, \"stats2\": {\"Utilization\": 2.01}}",
		getWorkerInstancesStats(t),
	)

	// Ensure that the instances are exactly the ones we passed in
	assertWorkerInstanceStats(
		t,
		map[string]*WorkerStats{
			"stats1": stats1Update3,
			"stats2": stats2Update1,
		},
	)
}

func TestWorkersInstancesAsync(t *testing.T) {
	setUp()

	var wg sync.WaitGroup
	var expectedStatsLock sync.Mutex

	maxTestWorkers := 500

	start := make(chan struct{})
	expectedStats := make(map[string]*WorkerStats)

	require.Equal(t, 0, GetWorkerCount())
	require.Equal(t, 0, getWorkersCountExpvar(t))

	canary1Stats := &WorkerStats{Utilization: -100}
	SetWorkerStats("canary1", canary1Stats)
	expectedStats["canary1"] = canary1Stats

	for idx := 0; idx < maxTestWorkers; idx++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			stats := &WorkerStats{Utilization: -1.0}
			updatedStats := &WorkerStats{Utilization: float64(id)}

			<-start

			name := fmt.Sprintf("stats %d", id)

			SetWorkerStats(name, stats)
			SetWorkerStats(name, updatedStats)

			// Keep every even stat and delete every odd worker stat
			if id%2 == 0 {
				expectedStatsLock.Lock()
				expectedStats[name] = updatedStats
				expectedStatsLock.Unlock()
			} else {
				DeleteWorkerStats(name)
			}
		}(idx)
	}

	close(start)

	wg.Wait()

	require.Equal(t, maxTestWorkers/2+1, GetWorkerCount())
	assertWorkerInstanceStats(t, expectedStats)
}

func TestWorkersCount(t *testing.T) {
	setUp()

	expectedWorkers := 20

	require.Equal(t, 0, GetWorkerCount())
	require.Equal(t, 0, getWorkersCountExpvar(t))

	// Test addition of new stats
	for idx := 0; idx < expectedWorkers; idx++ {
		stats := &WorkerStats{}
		SetWorkerStats(fmt.Sprintf("stats %d", idx), stats)

		require.Equal(t, idx+1, GetWorkerCount())
		require.Equal(t, idx+1, getWorkersCountExpvar(t))
	}

	// Test updates of stats do not increase the count
	for idx := 0; idx < expectedWorkers; idx += 2 {
		stats := &WorkerStats{}
		SetWorkerStats(fmt.Sprintf("stats %d", idx), stats)

		require.Equal(t, expectedWorkers, GetWorkerCount())
		require.Equal(t, expectedWorkers, getWorkersCountExpvar(t))
	}

	// Test removals of stats
	numRemoved := 0
	for idx := 0; idx < expectedWorkers; idx += 2 {
		DeleteWorkerStats(fmt.Sprintf("stats %d", idx))
		numRemoved++

		require.Equal(t, expectedWorkers-numRemoved, GetWorkerCount())
		require.Equal(t, expectedWorkers-numRemoved, getWorkersCountExpvar(t))
	}

	// Ensure that double-removal does not change the count
	for idx := 0; idx < expectedWorkers; idx += 2 {
		DeleteWorkerStats(fmt.Sprintf("stats %d", idx))
		require.Equal(t, expectedWorkers-numRemoved, GetWorkerCount())
		require.Equal(t, expectedWorkers-numRemoved, getWorkersCountExpvar(t))
	}
}

func TestWorkersCountAsync(t *testing.T) {
	setUp()

	var wg sync.WaitGroup
	maxTestWorkers := 500
	start := make(chan struct{})

	require.Equal(t, 0, GetWorkerCount())
	require.Equal(t, 0, getWorkersCountExpvar(t))

	SetWorkerStats("canary1", &WorkerStats{})

	for idx := 0; idx < maxTestWorkers; idx++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			stats := &WorkerStats{}

			<-start

			SetWorkerStats(fmt.Sprintf("stats %d", id), stats)
			SetWorkerStats(fmt.Sprintf("stats %d", id), stats)
			DeleteWorkerStats(fmt.Sprintf("stats %d", id))

			SetWorkerStats(fmt.Sprintf("stats %d", id), stats)
			SetWorkerStats(fmt.Sprintf("stats %d", id), stats)
			DeleteWorkerStats(fmt.Sprintf("stats %d", id))
		}(idx)
	}

	SetWorkerStats("canary2", &WorkerStats{})

	close(start)

	wg.Wait()

	require.Equal(t, 2, GetWorkerCount())
}
