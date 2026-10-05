// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUtilization(t *testing.T) {
	tests := []struct {
		name                string
		runnerStatus        RunnerStatus
		expectedUtilization float64
	}{
		{
			name: "standard case",
			runnerStatus: RunnerStatus{
				Workers:     4,
				WorkersUsed: 1,
				NumChecks:   1,
			},
			expectedUtilization: 0.25,
		},
		{
			name: "0 workers used",
			runnerStatus: RunnerStatus{
				Workers:     4,
				WorkersUsed: 0,
				NumChecks:   0,
			},
			expectedUtilization: 0,
		},
		{
			name: "0 workers",
			runnerStatus: RunnerStatus{
				Workers:     0,
				WorkersUsed: 0,
				NumChecks:   0,
			},
			expectedUtilization: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.InDelta(t, test.expectedUtilization, test.runnerStatus.utilization(), 0.05)
		})
	}
}

func TestAddToLeastBusy(t *testing.T) {
	tests := []struct {
		name              string
		existingRunners   map[string]int
		existingChecks    map[string]ConfigStatus
		preferredRunner   string
		expectedPlacement string
	}{
		{
			name: "standard case",
			existingRunners: map[string]int{
				"runner1": 4,
				"runner2": 4,
				"runner3": 4,
			},
			existingChecks: map[string]ConfigStatus{
				"check1": {WorkersNeeded: 3, Runner: "runner1"},
				"check2": {WorkersNeeded: 1, Runner: "runner2"},
				"check3": {WorkersNeeded: 2, Runner: "runner3"},
			},
			preferredRunner:   "",
			expectedPlacement: "runner2",
		},
		{
			name: "2 least busy runners. Add to preferred",
			existingRunners: map[string]int{
				"runner1": 4,
				"runner2": 4,
				"runner3": 4,
			},
			existingChecks: map[string]ConfigStatus{
				"check1": {WorkersNeeded: 3, Runner: "runner1"},
				"check2": {WorkersNeeded: 1, Runner: "runner2"},
				"check3": {WorkersNeeded: 1, Runner: "runner3"},
			},
			preferredRunner:   "runner2",
			expectedPlacement: "runner2",
		},
		{
			name: "2 least busy runners. Add to the one with less checks",
			existingRunners: map[string]int{
				"runner1": 4,
				"runner2": 4,
				"runner3": 4,
			},
			existingChecks: map[string]ConfigStatus{
				"check1": {WorkersNeeded: 3, Runner: "runner1"},
				"check2": {WorkersNeeded: 2, Runner: "runner2"},
				"check3": {WorkersNeeded: 1, Runner: "runner3"},
				"check4": {WorkersNeeded: 1, Runner: "runner3"},
			},
			preferredRunner:   "",
			expectedPlacement: "runner2",
		},
		{
			name: "2 least busy runners. Preferred wins over fewer checks",
			existingRunners: map[string]int{
				"runner1": 4,
				"runner2": 4,
				"runner3": 4,
			},
			existingChecks: map[string]ConfigStatus{
				"check1": {WorkersNeeded: 3, Runner: "runner1"}, // runner1: util 0.75, 1 check
				"check2": {WorkersNeeded: 1, Runner: "runner2"}, // runner2: util 0.5,  2 checks
				"check3": {WorkersNeeded: 1, Runner: "runner2"},
				"check4": {WorkersNeeded: 2, Runner: "runner3"}, // runner3: util 0.5,  1 check
			},
			preferredRunner:   "runner2",
			expectedPlacement: "runner2",
		},
		{
			name: "only one runner",
			existingRunners: map[string]int{
				"runner1": 4,
			},
			existingChecks: map[string]ConfigStatus{
				"check1": {WorkersNeeded: 3, Runner: "runner1"},
			},
			preferredRunner:   "",
			expectedPlacement: "runner1",
		},
		{
			name:              "no runners",
			existingRunners:   map[string]int{},
			existingChecks:    map[string]ConfigStatus{},
			preferredRunner:   "",
			expectedPlacement: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			distribution := newConfigsDistribution(test.existingRunners, false, 4.0, 1.0, 0.05)

			for checkID, checkStatus := range test.existingChecks {
				distribution.addConfig(checkID, checkStatus.CheckName, checkStatus.WorkersNeeded, checkStatus.Runner, false)
			}

			distribution.addToLeastBusy("newCheck", "newCheck", 10, test.preferredRunner, "", false)

			assert.Equal(t, test.expectedPlacement, distribution.runnerForConfig("newCheck"))
		})
	}
}

func TestAddCheck(t *testing.T) {
	distribution := newConfigsDistribution(map[string]int{
		"runner1": 4,
	}, false, 4.0, 1.0, 0.05)

	distribution.addConfig("check1", "check1", 3, "runner1", false)
	assert.Equal(t, "runner1", distribution.runnerForConfig("check1"))
	assert.Equal(t, 3.0, distribution.workersNeededForConfig("check1"))
}

func TestChecksSortedByWorkersNeeded(t *testing.T) {
	// Standard case
	distribution := newConfigsDistribution(map[string]int{
		"runner1": 4,
		"runner2": 4,
		"runner3": 4,
	}, false, 4.0, 1.0, 0.05)

	distribution.addConfig("check1", "check1", 3, "runner1", false)
	distribution.addConfig("check2", "check2", 1, "runner1", false)
	distribution.addConfig("check3", "check3", 4, "runner2", false)
	distribution.addConfig("check4", "check4", 2, "runner3", false)

	assert.Equal(t, []string{"check3", "check1", "check4", "check2"}, distribution.configsSortedByWorkersNeeded())

	// Sorted alphabetically when the number of workers is the same
	distribution = newConfigsDistribution(map[string]int{
		"runner1": 4,
		"runner2": 4,
	}, false, 4.0, 1.0, 0.05)

	distribution.addConfig("check_B", "check_B", 1, "runner1", false)
	distribution.addConfig("check_A", "check_A", 1, "runner2", false)
	distribution.addConfig("check_C", "check_C", 1, "runner1", false)
	distribution.addConfig("check_Z", "check_Z", 2, "runner2", false)

	assert.Equal(t, []string{"check_Z", "check_A", "check_B", "check_C"}, distribution.configsSortedByWorkersNeeded())
}

func TestNumEmptyRunners(t *testing.T) {
	distribution := newConfigsDistribution(map[string]int{
		"runner1": 4,
		"runner2": 2,
	}, false, 4.0, 1.0, 0.05)
	assert.Equal(t, 2, distribution.numEmptyRunners())

	distribution.addConfig("check1", "check1", 1, "runner1", false)
	assert.Equal(t, 1, distribution.numEmptyRunners())

	distribution.addConfig("check2", "check2", 1, "runner2", false)
	assert.Equal(t, 0, distribution.numEmptyRunners())
}

func TestNumRunnersWithHighUtilization(t *testing.T) {
	distribution := newConfigsDistribution(map[string]int{
		"runner1": 4,
		"runner2": 2,
	}, false, 4.0, 1.0, 0.05)
	assert.Equal(t, 0, distribution.numRunnersWithHighUtilization())

	distribution.addConfig("check1", "check1", 1, "runner1", false) // runner 1 at 25%
	assert.Equal(t, 0, distribution.numRunnersWithHighUtilization())

	distribution.addConfig("check2", "check2", 2.5, "runner1", false) // runner 1 at 3.5/4=0.875, above threshold
	assert.Equal(t, 1, distribution.numRunnersWithHighUtilization())

	distribution.addConfig("check3", "check3", 2, "runner2", false) // runner 2 at 100%
	assert.Equal(t, 2, distribution.numRunnersWithHighUtilization())
}

func TestAddConfigConflictTransfersAccumulatedWeight(t *testing.T) {
	distribution := newConfigsDistribution(map[string]int{
		"runner1": 4,
		"runner2": 4,
	}, false, 4.0, 1.0, 0.05)

	// Two instances of digestA are processed on runner1 first.
	distribution.addConfig("digestA", "configA", 2.0, "runner1", false)
	distribution.addConfig("digestA", "configA", 1.5, "runner1", false)
	assert.InDelta(t, 3.5, distribution.Runners["runner1"].WorkersUsed, 0.001)
	assert.InDelta(t, 0.0, distribution.Runners["runner2"].WorkersUsed, 0.001)

	// Conflicting assignment of digestA to runner2
	distribution.addConfig("digestA", "configA", 3.0, "runner2", false)

	// All accumulated weight for digestA (3.5) must transfer from runner1 to runner2,
	// plus the new instance weight (3.0).
	assert.InDelta(t, 0.0, distribution.Runners["runner1"].WorkersUsed, 0.001)
	assert.InDelta(t, 6.5, distribution.Runners["runner2"].WorkersUsed, 0.001)
	assert.InDelta(t, 6.5, distribution.Configs["digestA"].WorkersNeeded, 0.001)
	assert.Equal(t, "runner2", distribution.Configs["digestA"].Runner)
}

func TestUtilizationStdDev(t *testing.T) {
	// Define runner1 with 3 workers needed, runner2 with 5, runner3 with 8, and runner4 with 0
	distribution := newConfigsDistribution(map[string]int{
		"runner1": 4,
		"runner2": 4,
		"runner3": 4,
	}, false, 4.0, 1.0, 0.05)
	distribution.addConfig("check1", "check1", 1, "runner1", false)
	distribution.addConfig("check2", "check2", 2, "runner1", false)
	distribution.addConfig("check3", "check3", 2, "runner2", false)
	distribution.addConfig("check4", "check4", 4, "runner3", false)

	// The avg utilization is (0.75 + 0.5 + 1)/3 = 0.75
	// The variance is ((0.75-0.75)^2 + (0.5-0.75)^2 + (1-0.75)^2)/3 = 0.125/3
	// The stddev is sqrt(0.125/3) = 0.204
	assert.InDelta(t, 0.204, distribution.utilizationStdDev(), 0.05)
}
