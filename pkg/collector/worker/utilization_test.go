// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
)

const Epsilon = 0.001 // Used for floating point comparisons

func TestNewUtilizationMonitor(t *testing.T) {
	monitor := NewUtilizationMonitor(0.8)

	assert.NotNil(t, monitor)
	assert.InEpsilon(t, 0.8, monitor.Threshold, Epsilon)
}

func TestExpvarUtilizationMonitor_GetWorkerUtilization(t *testing.T) {
	// Reset expvars before test
	expvars.Reset()

	monitor := NewUtilizationMonitor(0.8)

	// Test when no workers exist
	_, err := monitor.GetWorkerUtilization("worker-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker worker-1 not found")

	// Add some worker stats
	workerStats1 := &expvars.WorkerStats{Utilization: 0.85}
	workerStats2 := &expvars.WorkerStats{Utilization: 0.45}

	expvars.SetWorkerStats("worker-1", workerStats1)
	expvars.SetWorkerStats("worker-2", workerStats2)

	// Test getting existing worker
	utilization, err := monitor.GetWorkerUtilization("worker-1")
	require.NoError(t, err)
	assert.InEpsilon(t, 0.85, utilization, Epsilon)

	utilization, err = monitor.GetWorkerUtilization("worker-2")
	require.NoError(t, err)
	assert.InEpsilon(t, 0.45, utilization, Epsilon)

	// Test getting non-existent worker
	_, err = monitor.GetWorkerUtilization("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker non-existent not found")
}

func TestExpvarUtilizationMonitor_GetAllWorkerUtilizations(t *testing.T) {
	// Reset expvars before test
	expvars.Reset()

	monitor := NewUtilizationMonitor(0.8)

	// Test when no workers exist
	// This is a legal case and should return an empty map, not an error
	utilizations, err := monitor.GetAllWorkerUtilizations()
	require.NoError(t, err)
	assert.Empty(t, utilizations)

	// Add some worker stats
	workerStats1 := &expvars.WorkerStats{Utilization: 0.65}
	workerStats2 := &expvars.WorkerStats{Utilization: 0.85}
	workerStats3 := &expvars.WorkerStats{Utilization: 0.25}

	expvars.SetWorkerStats("worker-1", workerStats1)
	expvars.SetWorkerStats("worker-2", workerStats2)
	expvars.SetWorkerStats("worker-3", workerStats3)

	// Test getting all workers
	utilizations, err = monitor.GetAllWorkerUtilizations()
	require.NoError(t, err)

	expected := map[string]float64{
		"worker-1": 0.65,
		"worker-2": 0.85,
		"worker-3": 0.25,
	}

	for workerName, utilization := range utilizations {
		assert.InEpsilon(t, expected[workerName], utilization, Epsilon)
	}
}

func TestExpvarUtilizationMonitor_GetWorkerOverview(t *testing.T) {
	// Reset expvars before test
	expvars.Reset()

	monitor := NewUtilizationMonitor(0.8)

	// Test when no workers exist
	// This is a legal case and should return an empty overview, not an error
	overview, err := monitor.GetWorkerOverview()
	require.NoError(t, err)
	assert.Equal(t, 0, overview.TotalWorkers)
	assert.Equal(t, 0.8, overview.Threshold)
	assert.Equal(t, 0.0, overview.AverageUtilization)
	assert.Empty(t, overview.WorkersOverThreshold)

	// Add some workers and their stats
	workerStats1 := &expvars.WorkerStats{Utilization: 0.5}
	workerStats2 := &expvars.WorkerStats{Utilization: 0.8}
	workerStats3 := &expvars.WorkerStats{Utilization: 0.6}
	workerStats4 := &expvars.WorkerStats{Utilization: 0.9}

	expvars.SetWorkerStats("worker-1", workerStats1)
	expvars.SetWorkerStats("worker-2", workerStats2)
	expvars.SetWorkerStats("worker-3", workerStats3)
	expvars.SetWorkerStats("worker-4", workerStats4)

	// Test getting overview
	overview, err = monitor.GetWorkerOverview()
	require.NoError(t, err)

	assert.Equal(t, 4, overview.TotalWorkers)
	assert.InEpsilon(t, 0.8, overview.Threshold, Epsilon)
	assert.InEpsilon(t, 0.7, overview.AverageUtilization, Epsilon) // (0.5 + 0.8 + 0.6 + 0.9) = 2.8 / 4 = 0.7
	assert.Len(t, overview.WorkersOverThreshold, 2)
	assert.Contains(t, overview.WorkersOverThreshold, "worker-2")
	assert.Contains(t, overview.WorkersOverThreshold, "worker-4")
}

func TestExpvarUtilizationMonitor_EdgeCases(t *testing.T) {
	// Reset expvars before test
	expvars.Reset()

	monitor := NewUtilizationMonitor(1.0)

	// Add workers and then test edge cases
	workerStats := &expvars.WorkerStats{Utilization: 1.0} // 100% utilization
	expvars.SetWorkerStats("busy-worker", workerStats)

	// Test with 100% utilization
	utilization, err := monitor.GetWorkerUtilization("busy-worker")
	require.NoError(t, err)
	assert.Equal(t, 1.0, utilization)

	// Test overview with single worker exactly at threshold
	overview, err := monitor.GetWorkerOverview()
	require.NoError(t, err)
	assert.Equal(t, 1, overview.TotalWorkers)
	assert.InEpsilon(t, 1.0, overview.AverageUtilization, Epsilon)
	assert.Len(t, overview.WorkersOverThreshold, 1)
	assert.Contains(t, overview.WorkersOverThreshold, "busy-worker")
}
