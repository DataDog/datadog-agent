// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

import (
	"expvar"
	"fmt"
	"sync"
)

const (
	// Top-level key for this expvar
	workersExpvarKey = "Workers"

	countExpvarKey     = "Count"
	instancesExpvarKey = "Instances"
)

var (
	workerInstancesStats *expvar.Map
	workersStats         *expvar.Map
	workersStatsLock     sync.Mutex
)

// WorkerStats is the update object that will be used to populate
// the individual `worker.Worker` instance expvar stats
type WorkerStats struct {
	Utilization float64
}

// String is used by expvar package to print the variables
func (ws *WorkerStats) String() string {
	return fmt.Sprintf("{\"Utilization\": %.2f}", ws.Utilization)
}

func newWorkersExpvar(parent *expvar.Map) {
	workersStatsLock.Lock()
	defer workersStatsLock.Unlock()

	workerInstancesStats = &expvar.Map{}

	workersStats = &expvar.Map{}
	workersStats.Add(countExpvarKey, 0)
	workersStats.Set(instancesExpvarKey, workerInstancesStats)

	parent.Set(workersExpvarKey, workersStats)
}

func resetWorkersExpvar(parent *expvar.Map) {
	newWorkersExpvar(parent)
}

// SetWorkerStats is used to add individual worker stats or update them
// if they were already present
func SetWorkerStats(name string, ws *WorkerStats) {
	workersStatsLock.Lock()

	if workerInstancesStats.Get(name) == nil {
		workersStats.Add(countExpvarKey, int64(1))
	}

	workerInstancesStats.Set(name, ws)

	workersStatsLock.Unlock()
}

// DeleteWorkerStats is used to remove instance information about a worker and update
// the count of the curretly running workers
func DeleteWorkerStats(name string) {
	workersStatsLock.Lock()

	if workerInstancesStats.Get(name) != nil {
		workersStats.Add(countExpvarKey, -1)
	}

	workerInstancesStats.Delete(name)

	workersStatsLock.Unlock()
}

// GetWorkerCount is used to get the value of 'Workers'->'Count' expvar
func GetWorkerCount() int {
	count := workersStats.Get(countExpvarKey)
	if count == nil {
		return 0
	}

	return int(count.(*expvar.Int).Value())
}
