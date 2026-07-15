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
// Pinned configs are kept on their current runner during rebalancing.
type ConfigStatus struct {
	WorkersNeeded float64
	Runner        string
	CheckName     string
	Pinned        bool
}

// RunnerStatus represents the status of a check runner
type RunnerStatus struct {
	Workers     int
	WorkersUsed float64
	// Note: this is different from the number of configs
	// because a config can contain multiple check instances
	NumChecks int
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
	Configs              map[string]*ConfigStatus
	Runners              map[string]*RunnerStatus
	stickinessEnabled    bool
	stickinessFactor     float64
	stickinessUpperLimit float64
	stickinessLowerLimit float64
}

func newConfigsDistribution(workersPerRunner map[string]int, stickinessEnabled bool, stickinessFactor float64, stickinessUpperLimit float64, stickinessLowerLimit float64) configsDistribution {
	runners := map[string]*RunnerStatus{}
	for runnerName, runnerWorkers := range workersPerRunner {
		runners[runnerName] = &RunnerStatus{
			Workers:     runnerWorkers,
			WorkersUsed: 0.0,
			NumChecks:   0,
		}
	}

	return configsDistribution{
		Configs:              map[string]*ConfigStatus{},
		Runners:              runners,
		stickinessEnabled:    stickinessEnabled,
		stickinessFactor:     stickinessFactor,
		stickinessUpperLimit: stickinessUpperLimit,
		stickinessLowerLimit: stickinessLowerLimit,
	}
}

// leastBusyRunner returns the runner with the lowest utilization. If there are
// several options, it gives preference to preferredRunner. If preferredRunner
// is not among the runners with the lowest utilization, it gives precedence to
// the runner with the lowest number of configs deployed. excludeRunner can be
// set to avoid assigning a config to a specific runner.
func (distribution *configsDistribution) leastBusyRunner(preferredRunner string, excludeRunner string, workersNeeded float64) string {
	leastBusyRunner := ""
	minUtilization := 0.0
	numChecksLeastBusyRunner := 0

	for runnerName, runnerStatus := range distribution.Runners {
		if runnerName == excludeRunner {
			continue
		}

		runnerUtilization := runnerStatus.utilization()
		runnerNumChecks := runnerStatus.NumChecks

		if distribution.stickinessEnabled && runnerName == preferredRunner {
			bias := max(min(workersNeeded*distribution.stickinessFactor, distribution.stickinessUpperLimit), distribution.stickinessLowerLimit)
			runnerUtilization -= bias
		}

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

func (distribution *configsDistribution) addToLeastBusy(digest, checkName string, workersNeeded float64, preferredRunner string, excludeRunner string, pinned bool) {
	leastBusy := distribution.leastBusyRunner(preferredRunner, excludeRunner, workersNeeded)
	if leastBusy == "" {
		return
	}

	distribution.addConfig(digest, checkName, workersNeeded, leastBusy, pinned)
}

// addConfig records a config instance in the distribution.
func (distribution *configsDistribution) addConfig(digest, checkName string, workersNeeded float64, runner string, pinned bool) {
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

	// Prioritize the new assigned runner over the existing one
	// Note: this edge case should never happen in practice
	if configInfo.Runner != runner {
		log.Warnf("digest %s already placed on runner %q, but received conflicting assignment to %q",
			digest, configInfo.Runner, runner)
		distribution.Runners[configInfo.Runner].WorkersUsed -= configInfo.WorkersNeeded
		runnerInfo.WorkersUsed += configInfo.WorkersNeeded
		configInfo.Runner = runner
	}

	configInfo.WorkersNeeded += workersNeeded

	// Cumulate Pinned: Pin the entire config if any of its instances are pinned
	configInfo.Pinned = configInfo.Pinned || pinned
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
