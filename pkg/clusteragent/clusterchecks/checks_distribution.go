// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"math"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigStatus is one config's entry in a distribution.
// WorkersNeeded is summed across the config's instances.
// Pinned configs are kept on their current runner during rebalancing.
// EligibleRunners is the sorted set of runners the config may be placed on per
// the workers' advertised check compatibility (see placementCandidates); it is
// empty for configs built without compatibility info (legacy callers), which
// Cohort == "" denotes.
type ConfigStatus struct {
	WorkersNeeded   float64
	Runner          string
	CheckName       string
	Pinned          bool
	EligibleRunners []string
	// Cohort is the eligibility cohort key this config belongs to: the sorted,
	// comma-joined EligibleRunners, or "" when no compatibility info is
	// available (legacy/global cohort covering all runners of the
	// distribution).
	Cohort string
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

// leastBusyRunnerIn returns the runner with the lowest utilization among the
// eligible runners. If eligible is nil, every runner is considered. If there are
// several options, it gives preference to preferredRunner. If preferredRunner
// is not among the runners with the lowest utilization, it gives precedence to
// the runner with the lowest number of configs deployed. excludeRunner can be
// set to avoid assigning a config to a specific runner.
func (distribution *configsDistribution) leastBusyRunnerIn(eligible map[string]struct{}, preferredRunner string, excludeRunner string, workersNeeded float64) string {
	leastBusyRunner := ""
	minUtilization := 0.0
	numChecksLeastBusyRunner := 0

	for runnerName, runnerStatus := range distribution.Runners {
		if runnerName == excludeRunner {
			continue
		}
		if eligible != nil {
			if _, ok := eligible[runnerName]; !ok {
				continue
			}
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
	distribution.addToLeastBusyIn(nil, digest, checkName, workersNeeded, preferredRunner, excludeRunner, pinned)
}

// addToLeastBusyIn is addToLeastBusy restricted to the given eligible runners
// (nil eligible means no restriction), so compatibility-declared runner
// groups only receive the checks they declared. If no eligible runner is
// available, the config is not placed and the distribution is left unchanged
// for it.
func (distribution *configsDistribution) addToLeastBusyIn(eligibleRunners []string, digest, checkName string, workersNeeded float64, preferredRunner string, excludeRunner string, pinned bool) {
	var eligible map[string]struct{}
	if eligibleRunners != nil {
		eligible = make(map[string]struct{}, len(eligibleRunners))
		for _, r := range eligibleRunners {
			eligible[r] = struct{}{}
		}
	}
	leastBusy := distribution.leastBusyRunnerIn(eligible, preferredRunner, excludeRunner, workersNeeded)
	if leastBusy == "" {
		return
	}

	distribution.addConfigWithEligibility(eligibleRunners, digest, checkName, workersNeeded, leastBusy, pinned)
}

// addConfig records a config instance in the distribution.
func (distribution *configsDistribution) addConfig(digest, checkName string, workersNeeded float64, runner string, pinned bool) {
	distribution.addConfigWithEligibility(nil, digest, checkName, workersNeeded, runner, pinned)
}

// addConfigWithEligibility records a config instance in the distribution along
// with its eligibility cohort. eligibleRunners is the sorted candidate set for
// this config; nil means no compatibility info (the config belongs to the
// global cohort spanning every runner of the distribution), while an empty
// non-nil slice means no eligible worker at all (the config is expected to be
// pinned in place; its cohort is its current runner alone).
func (distribution *configsDistribution) addConfigWithEligibility(eligibleRunners []string, digest, checkName string, workersNeeded float64, runner string, pinned bool) {
	// Initialize the runner and attribute work
	runnerInfo, runnerExists := distribution.Runners[runner]
	if !runnerExists {
		runnerInfo = &RunnerStatus{}
		distribution.Runners[runner] = runnerInfo
	}
	runnerInfo.WorkersUsed += workersNeeded
	runnerInfo.NumChecks++

	// Cohort key for the compatibility-aware stddev: the sorted candidate
	// runner set, or "" for legacy configs with no eligibility info. Configs
	// with no eligible worker at all are pinned in place and form a
	// single-runner cohort of their own.
	cohort := ""
	if eligibleRunners != nil {
		if len(eligibleRunners) == 0 {
			cohort = "pinned:" + runner
		} else {
			cohort = strings.Join(eligibleRunners, ",")
		}
	}

	// Initialize the config and attribute work
	configInfo, configExists := distribution.Configs[digest]
	if !configExists {
		configInfo = &ConfigStatus{
			Runner:    runner,
			CheckName: checkName,
		}
		distribution.Configs[digest] = configInfo
	}
	configInfo.EligibleRunners = eligibleRunners
	configInfo.Cohort = cohort

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

// utilizationStdDevWeighted is the compatibility-aware variant of
// utilizationStdDev: configs are partitioned into eligibility cohorts (by
// their candidate runner set), each cohort's stddev is computed over its
// candidate runners, and the result is the config-count-weighted average.
// A single global stddev would fight isolation (a deliberately skewed group
// reads as a fixable imbalance). Cohort "" spans all runners, so with no
// compatibility declared the result degrades to the plain utilizationStdDev.
func (distribution *configsDistribution) utilizationStdDevWeighted() float64 {
	cohortRunners := map[string]map[string]struct{}{} // cohort key -> runner names
	cohortConfigs := map[string]int{}

	for _, configInfo := range distribution.Configs {
		cohort := configInfo.Cohort
		runners, ok := cohortRunners[cohort]
		if !ok {
			runners = map[string]struct{}{}
			cohortRunners[cohort] = runners
		}
		switch {
		case cohort == "":
			// Legacy cohort: all runners of the distribution.
			for runnerName := range distribution.Runners {
				runners[runnerName] = struct{}{}
			}
		case strings.HasPrefix(cohort, "pinned:"):
			// No eligible worker: the config is pinned to its current runner.
			runners[configInfo.Runner] = struct{}{}
		default:
			for _, runnerName := range configInfo.EligibleRunners {
				runners[runnerName] = struct{}{}
			}
		}
		cohortConfigs[cohort]++
	}

	if len(cohortRunners) == 0 {
		return 0
	}

	weightedStdDev := 0.0
	totalConfigs := 0
	for cohort, runners := range cohortRunners {
		if len(runners) == 0 {
			continue
		}
		totalUtilization := 0.0
		for runnerName := range runners {
			if runnerStatus, ok := distribution.Runners[runnerName]; ok {
				totalUtilization += runnerStatus.utilization()
			}
		}
		avgUtilization := totalUtilization / float64(len(runners))

		sumSquaredDeviations := 0.0
		for runnerName := range runners {
			if runnerStatus, ok := distribution.Runners[runnerName]; ok {
				sumSquaredDeviations += math.Pow(runnerStatus.utilization()-avgUtilization, 2)
			}
		}
		variance := sumSquaredDeviations / float64(len(runners))

		weightedStdDev += math.Sqrt(variance) * float64(cohortConfigs[cohort])
		totalConfigs += cohortConfigs[cohort]
	}

	if totalConfigs == 0 {
		return 0
	}

	return weightedStdDev / float64(totalConfigs)
}
