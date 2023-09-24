// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"math"
	"sort"
)

type CheckStatus struct {
	WorkersNeeded float64
	Runner        string
}

type RunnerStatus struct {
	Workers     int
	WorkersUsed float64
	NumChecks   int
}

func (ns RunnerStatus) utilization() float64 {
	if ns.Workers == 0 {
		return 0
	}

	return ns.WorkersUsed / (float64)(ns.Workers)
}

// checksDistribution represents the placement of cluster checks across the
// different runners of a cluster
type checksDistribution struct {
	Checks  map[string]*CheckStatus
	Runners map[string]*RunnerStatus
}

func newChecksDistribution(workersPerRunner map[string]int) checksDistribution {
	runners := map[string]*RunnerStatus{}
	for runnerName, runnerWorkers := range workersPerRunner {
		runners[runnerName] = &RunnerStatus{
			Workers:     runnerWorkers,
			WorkersUsed: 0.0,
			NumChecks:   0,
		}
	}

	return checksDistribution{
		Checks:  map[string]*CheckStatus{},
		Runners: runners,
	}
}

// leastBusyRunner returns the runner with the lowest utilization. If there are
// several options, it gives preference to preferredRunner. If preferredRunner
// is not among the runners with the lowest utilization, it gives precedence to
// the runner with the lowest number of checks deployed.
func (distribution *checksDistribution) leastBusyRunner(preferredRunner string) string {
	leastBusyRunner := ""
	minUtilization := 0.0
	numChecksLeastBusyRunner := 0

	for runnerName, runnerStatus := range distribution.Runners {
		runnerUtilization := runnerStatus.utilization()
		runnerNumChecks := runnerStatus.NumChecks

		selectRunner := leastBusyRunner == "" ||
			runnerUtilization < minUtilization ||
			runnerUtilization == minUtilization && runnerName == preferredRunner ||
			runnerUtilization == minUtilization && runnerNumChecks < numChecksLeastBusyRunner

		if selectRunner {
			leastBusyRunner = runnerName
			minUtilization = runnerUtilization
			numChecksLeastBusyRunner = runnerNumChecks
		}
	}

	return leastBusyRunner
}

func (distribution *checksDistribution) addToLeastBusy(checkID string, workersNeeded float64, preferredRunner string) {
	leastBusy := distribution.leastBusyRunner(preferredRunner)
	if leastBusy == "" {
		return
	}

	distribution.addCheck(checkID, workersNeeded, leastBusy)
}

func (distribution *checksDistribution) addCheck(checkID string, workersNeeded float64, runner string) {
	distribution.Checks[checkID] = &CheckStatus{
		WorkersNeeded: workersNeeded,
		Runner:        runner,
	}

	runnerInfo, runnerExists := distribution.Runners[runner]
	if runnerExists {
		runnerInfo.WorkersUsed += workersNeeded
		runnerInfo.NumChecks++
	} else {
		distribution.Runners[runner] = &RunnerStatus{
			WorkersUsed: workersNeeded,
			NumChecks:   1,
		}
	}
}

func (distribution *checksDistribution) runnerWorkers() map[string]int {
	res := map[string]int{}

	for runnerName, runnerStatus := range distribution.Runners {
		res[runnerName] = runnerStatus.Workers
	}

	return res
}

func (distribution *checksDistribution) runnerForCheck(checkID string) string {
	if checkInfo, found := distribution.Checks[checkID]; found {
		return checkInfo.Runner
	}

	return ""
}

func (distribution *checksDistribution) workersNeededForCheck(checkID string) float64 {
	if checkInfo, found := distribution.Checks[checkID]; found {
		return checkInfo.WorkersNeeded
	}

	return 0
}

// Note: if there are several checks with the same number of workers needed,
// they are returned in alphabetical order.
// When distributing the checks, having the same order will help in minimizing
// the number of re-schedules.
func (distribution *checksDistribution) checksSortedByWorkersNeeded() []string {
	var checks []struct {
		checkID       string
		workersNeeded float64
	}

	for checkID, checkStatus := range distribution.Checks {
		checks = append(checks, struct {
			checkID       string
			workersNeeded float64
		}{
			checkID:       checkID,
			workersNeeded: checkStatus.WorkersNeeded,
		})
	}

	sort.Slice(checks, func(i, j int) bool {
		if checks[i].workersNeeded == checks[j].workersNeeded {
			return checks[i].checkID < checks[j].checkID
		}

		return checks[i].workersNeeded > checks[j].workersNeeded
	})

	var res []string
	for _, check := range checks {
		res = append(res, check.checkID)
	}
	return res
}

func (distribution *checksDistribution) numEmptyRunners() int {
	empty := 0

	for _, runnerStatus := range distribution.Runners {
		if runnerStatus.NumChecks == 0 {
			empty++
		}
	}

	return empty
}

func (distribution *checksDistribution) numRunnersWithHighUtilization() int {
	withHighUtilization := 0

	for _, runnerStatus := range distribution.Runners {
		if runnerStatus.utilization() > 0.8 {
			withHighUtilization++
		}
	}

	return withHighUtilization
}

func (distribution *checksDistribution) utilizationStdDev() float64 {
	totalUtilization := 0.0
	for _, runnerStatus := range distribution.Runners {
		totalUtilization += runnerStatus.utilization()
	}

	avgUtilization := totalUtilization / float64(len(distribution.Runners))

	sumSquaredDeviations := 0.0
	for _, runnerStatus := range distribution.Runners {
		sumSquaredDeviations += math.Pow(runnerStatus.utilization()-avgUtilization, 2)
	}

	variance := sumSquaredDeviations / float64(len(distribution.Runners))

	return math.Sqrt(variance)
}
