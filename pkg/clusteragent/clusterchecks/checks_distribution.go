// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"math"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigStatus is one config's entry in a distribution.
// WorkersNeeded is summed across the config's instances.
type ConfigStatus struct {
	WorkersNeeded float64
	Runner        string
	CheckName     string
}

// RunnerStatus represents the status of a check runner
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

// configsDistribution represents the placement of cluster check configs
// across the runners of a cluster. Each entry is keyed by config digest.
type configsDistribution struct {
	Configs map[string]*ConfigStatus
	Runners map[string]*RunnerStatus
}

func newConfigsDistribution(workersPerRunner map[string]int) configsDistribution {
	runners := map[string]*RunnerStatus{}
	for runnerName, runnerWorkers := range workersPerRunner {
		runners[runnerName] = &RunnerStatus{
			Workers:     runnerWorkers,
			WorkersUsed: 0.0,
			NumChecks:   0,
		}
	}

	return configsDistribution{
		Configs: map[string]*ConfigStatus{},
		Runners: runners,
	}
}

// leastBusyRunner returns the runner with the lowest utilization. If there are
// several options, it gives preference to preferredRunner. If preferredRunner
// is not among the runners with the lowest utilization, it gives precedence to
// the runner with the lowest number of configs deployed. excludeRunner can be
// set to avoid assigning a config to a specific runner.
func (distribution *configsDistribution) leastBusyRunner(preferredRunner string, excludeRunner string) string {
	leastBusyRunner := ""
	minUtilization := 0.0
	numChecksLeastBusyRunner := 0

	for runnerName, runnerStatus := range distribution.Runners {
		if runnerName == excludeRunner {
			continue
		}

		runnerUtilization := runnerStatus.utilization()
		runnerNumChecks := runnerStatus.NumChecks

		selectRunner := (leastBusyRunner == "") ||
			(runnerUtilization < minUtilization) ||
			(runnerUtilization == minUtilization && runnerName == preferredRunner) ||
			(runnerUtilization == minUtilization && leastBusyRunner != preferredRunner && runnerNumChecks < numChecksLeastBusyRunner)

		if selectRunner {
			leastBusyRunner = runnerName
			minUtilization = runnerUtilization
			numChecksLeastBusyRunner = runnerNumChecks
		}
	}

	return leastBusyRunner
}

func (distribution *configsDistribution) addToLeastBusy(digest, checkName string, workersNeeded float64, preferredRunner string, excludeRunner string) {
	leastBusy := distribution.leastBusyRunner(preferredRunner, excludeRunner)
	if leastBusy == "" {
		return
	}

	distribution.addConfig(digest, checkName, workersNeeded, leastBusy)
}

func (distribution *configsDistribution) addConfig(digest, checkName string, workersNeeded float64, runner string) {
	// Initialize the runner and attribute work
	runnerInfo, runnerExists := distribution.Runners[runner]
	if !runnerExists {
		runnerInfo = &RunnerStatus{}
		distribution.Runners[runner] = runnerInfo
	}
	runnerInfo.WorkersUsed += workersNeeded
	runnerInfo.NumChecks++

	// Initialize the config and attribute work
	configInfo, configExists := distribution.Configs[digest]
	if !configExists {
		configInfo = &ConfigStatus{
			Runner:    runner,
			CheckName: checkName,
		}
		distribution.Configs[digest] = configInfo
	}
	configInfo.WorkersNeeded += workersNeeded

	// Expect condition to never be true
	if configInfo.Runner != runner {
		log.Warnf("configsDistribution.addConfig: digest %s already placed on runner %q, but received conflicting assignment to %q; workers credited to %q",
			digest, configInfo.Runner, runner, runner)
	}
}

func (distribution *configsDistribution) runnerWorkers() map[string]int {
	res := map[string]int{}

	for runnerName, runnerStatus := range distribution.Runners {
		res[runnerName] = runnerStatus.Workers
	}

	return res
}

func (distribution *configsDistribution) runnerForConfig(digest string) string {
	if info, found := distribution.Configs[digest]; found {
		return info.Runner
	}
	return ""
}

func (distribution *configsDistribution) workersNeededForConfig(digest string) float64 {
	if info, found := distribution.Configs[digest]; found {
		return info.WorkersNeeded
	}
	return 0
}

// configsSortedByWorkersNeeded returns config digests by descending
// workersNeeded, with ties broken alphabetically to keep placement stable.
func (distribution *configsDistribution) configsSortedByWorkersNeeded() []string {
	var configs []struct {
		digest        string
		workersNeeded float64
	}

	for digest, info := range distribution.Configs {
		configs = append(configs, struct {
			digest        string
			workersNeeded float64
		}{
			digest:        digest,
			workersNeeded: info.WorkersNeeded,
		})
	}

	sort.Slice(configs, func(i, j int) bool {
		if configs[i].workersNeeded == configs[j].workersNeeded {
			return configs[i].digest < configs[j].digest
		}
		return configs[i].workersNeeded > configs[j].workersNeeded
	})

	res := make([]string, 0, len(configs))
	for _, c := range configs {
		res = append(res, c.digest)
	}
	return res
}

func (distribution *configsDistribution) numEmptyRunners() int {
	empty := 0
	for _, runnerStatus := range distribution.Runners {
		if runnerStatus.NumChecks == 0 {
			empty++
		}
	}
	return empty
}

func (distribution *configsDistribution) numRunnersWithHighUtilization() int {
	withHighUtilization := 0
	for _, runnerStatus := range distribution.Runners {
		if runnerStatus.utilization() > 0.8 {
			withHighUtilization++
		}
	}
	return withHighUtilization
}

func (distribution *configsDistribution) utilizationStdDev() float64 {
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
