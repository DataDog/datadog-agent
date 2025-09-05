// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
)

// OverviewData - a summary of the workers in the system
type OverviewData struct {
	TotalWorkers         int
	Threshold            float64
	WorkersOverThreshold []string
	AverageUtilization   float64
}

// UtilizationMonitor - manages the checking of the worker utilization
type UtilizationMonitor struct {
	Threshold float64
}

// NewUtilizationMonitor - creates a new UtilizationMonitor
func NewUtilizationMonitor(threshold float64) *UtilizationMonitor {
	return &UtilizationMonitor{
		Threshold: threshold,
	}
}

// GetWorkerUtilization - returns the current utilization for a specific worker
func (m *UtilizationMonitor) GetWorkerUtilization(workerName string) (float64, error) {
	// Retrieve the worker instance data from expvars (needs several expvar lookups)
	// Race conditions are possible here since expvar is global state

	// Runner map
	runnerExpvar := expvar.Get(expvars.RunnerExpvarKey)
	if runnerExpvar == nil {
		return 0.0, fmt.Errorf("runner not found in expvars")
	}
	runnerMap, ok := runnerExpvar.(*expvar.Map)
	if !ok {
		return 0.0, fmt.Errorf("runner expvar is not a map")
	}

	// Workers map
	workersExpvar := runnerMap.Get(expvars.WorkersExpvarKey)
	if workersExpvar == nil {
		return 0.0, fmt.Errorf("runner.Workers not found in expvars")
	}
	workersMap, ok := workersExpvar.(*expvar.Map)
	if !ok {
		return 0.0, fmt.Errorf("runner.Workers expvar is not a map")
	}

	// Instances map
	instancesExpvar := workersMap.Get(expvars.InstancesExpvarKey)
	if instancesExpvar == nil {
		return 0.0, fmt.Errorf("runner.Workers.Instances not found in expvars")
	}
	instancesMap, ok := instancesExpvar.(*expvar.Map)
	if !ok {
		return 0.0, fmt.Errorf("runner.Workers.Instances expvar is not a map")
	}

	// Look for the specific worker
	worker := instancesMap.Get(workerName)
	if worker == nil {
		return 0.0, fmt.Errorf("worker %s not found", workerName)
	}
	workerStats, ok := worker.(*expvars.WorkerStats)
	if !ok {
		return 0.0, fmt.Errorf("unable to retrieve utilization for worker %s", workerName)
	}

	return workerStats.Utilization, nil
}

// GetAllWorkerUtilizations - returns utilization data for all workers
func (m *UtilizationMonitor) GetAllWorkerUtilizations() (map[string]float64, error) {
	// Retrieve the worker instance data from expvars (needs several expvar lookups)
	// Race conditions are possible here since expvar is global state

	// Runner map
	runnerExpvar := expvar.Get(expvars.RunnerExpvarKey)
	if runnerExpvar == nil {
		return nil, fmt.Errorf("runner not found in expvars")
	}
	runnerMap, ok := runnerExpvar.(*expvar.Map)
	if !ok {
		return nil, fmt.Errorf("runner expvar is not a map")
	}

	// Workers map
	workersExpvar := runnerMap.Get(expvars.WorkersExpvarKey)
	if workersExpvar == nil {
		return nil, fmt.Errorf("runner.Workers not found in expvars")
	}
	workersMap, ok := workersExpvar.(*expvar.Map)
	if !ok {
		return nil, fmt.Errorf("runner.Workers expvar is not a map")
	}

	// Instances map
	instancesExpvar := workersMap.Get(expvars.InstancesExpvarKey)
	if instancesExpvar == nil {
		return nil, fmt.Errorf("runner.Workers.Instances not found in expvars")
	}

	instancesMap, ok := instancesExpvar.(*expvar.Map)
	if !ok {
		return nil, fmt.Errorf("runner.Workers.Instances expvar is not a map")
	}

	// Add all data to the return map
	utilizations := make(map[string]float64)
	instancesMap.Do(func(kv expvar.KeyValue) {
		if workerStats, ok := kv.Value.(*expvars.WorkerStats); ok {
			utilizations[kv.Key] = workerStats.Utilization
		}
	})

	return utilizations, nil
}

// GetWorkerOverview - returns detailed status information about workers
func (m *UtilizationMonitor) GetWorkerOverview() (OverviewData, error) {
	overview := OverviewData{
		TotalWorkers:         0,
		Threshold:            m.Threshold,
		WorkersOverThreshold: make([]string, 0),
		AverageUtilization:   0.0,
	}

	utilizations, err := m.GetAllWorkerUtilizations()
	if err != nil {
		return overview, err
	}

	overview.TotalWorkers = len(utilizations)
	totalUtilization := 0.0

	for workerName, utilization := range utilizations {
		totalUtilization += utilization
		if utilization >= m.Threshold {
			overview.WorkersOverThreshold = append(overview.WorkersOverThreshold, workerName)
		}
	}

	if len(utilizations) > 0 {
		overview.AverageUtilization = totalUtilization / float64(len(utilizations))
	}

	return overview, nil
}
