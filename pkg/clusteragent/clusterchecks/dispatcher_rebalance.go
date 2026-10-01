// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// moveCheck moves a config by its digest from a node to another
func (d *dispatcher) moveConfig(src, dest, digest string) error {
	if src == dest {
		return nil
	}

	log.Debugf("Moving config %s from %s to %s", digest, src, dest)

	d.store.Lock()
	defer d.store.Unlock()

	destNode, destFound := d.store.getNodeStore(dest)
	sourceNode, srcFound := d.store.getNodeStore(src)
	config, configFound := d.store.digestToConfig[digest]
	var instanceIDs []string
	for checkID, checkDigest := range d.store.idToDigest {
		if checkDigest == digest {
			instanceIDs = append(instanceIDs, string(checkID))
		}
	}

	if !destFound || !srcFound {
		log.Debugf("Nodes not found in store: %s, %s. Config %s will not move", src, dest, digest)
		return fmt.Errorf("node %s not found", src)
	}
	if !configFound {
		return fmt.Errorf("no config registered for digest %s", digest)
	}
	if len(instanceIDs) == 0 {
		return fmt.Errorf("no instances registered for digest %s", digest)
	}

	// Move per-instance runner stats
	var firstErr error
	movedAny := false
	for _, checkID := range instanceIDs {
		stats, err := sourceNode.GetRunnerStats(checkID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		destNode.AddRunnerStats(checkID, stats)
		sourceNode.RemoveRunnerStats(checkID)
		movedAny = true
	}
	if !movedAny {
		log.Debugf("Cannot get runner stats on node %s for config %s; will not move", src, digest)
		return firstErr
	}

	// Reassign the config at the node level.
	d.store.digestToNode[digest] = dest

	sourceNode.Lock()
	sourceNode.removeConfig(digest)
	sourceNode.Unlock()

	destNode.Lock()
	destNode.addConfig(config)
	destNode.Unlock()

	// Re-key configsInfo from src to dest.
	for _, checkID := range instanceIDs {
		configsInfo.Delete(src, config.Name, checkID, le.JoinLeaderValue)
		configsInfo.Set(1.0, dest, config.Name, checkID, le.JoinLeaderValue)
	}

	log.Debugf("Config %s moved from %s to %s", digest, src, dest)
	return nil
}

func (d *dispatcher) rebalance(force bool) []types.RebalanceResponse {
	span := tracer.StartSpan("cluster_checks.dispatcher.rebalance",
		tracer.ResourceName("rebalanceChecks"),
		tracer.SpanType("worker"))
	span.SetTag("force", force)
	defer span.Finish()

	result := d.rebalanceUsingUtilization(force)
	span.SetTag("checks_moved", len(result))
	return result
}

// rebalanceUsingUtilization rebalances the cluster checks deployed in a cluster
// by taking into account the workers utilization of each runner.
//
// When all the workers of a runner are busy running checks (utilization = 1),
// they are not able to accept any new requests to run other checks. If other
// checks need to be run, the agent will be marked as unhealthy and the runner
// pod will be restarted shortly after. What we're trying to achieve is to avoid
// that situation by balancing the checks in a way that all the runners are at
// similar utilization level and none of them approaches a utilization of 1, if
// there's enough capacity in other runners.
//
// The implementation is a classical greedy algorithm. It sorts in descending
// order all the cluster checks by the number of workers that we think that they
// are going to require, and it goes one by one placing them in the runner with
// the lowest utilization. When there are several candidate runners, first, if
// the current node is among the candidates, it leaves the check there to avoid
// unnecessary check schedules and unschedules. If the current runner is not
// among the candidates, it chooses the runner that contains fewer checks.
//
// To apply this algorithm we need the number of workers of each runner and the
// predicted number of workers that each check deployed in the cluster is going
// to require. The number of workers for each runner is fetched from the CLC
// API. The predicted number of workers of a check is calculated as follows:
// avg_execution_time / interval_execution_time. This means that a check that on
// average takes 3 seconds to run and needs to run every 15 seconds, is going to
// require 3/15=0.20 workers approximately. A check that takes longer to run
// than its defined interval, will be running all the time (approx.), so we
// consider that it requires a whole worker.
//
// Limitations and assumptions:
// - This function does not try to find the optimal solution, but in most cases
// it should find one that's good enough for our use case.
// - It assumes that the execution time of a check is more or less stable and
// can be predicted according to the average execution time of the last few
// runs.
// - It assumes that the checks running on the runners that are not cluster
// checks are not very costly. They're ignored by this function.
// - It can't predict the execution time of checks that are running for the
// first time. This could become a problem for checks that take too long.
func (d *dispatcher) rebalanceUsingUtilization(force bool) []types.RebalanceResponse {
	// Collect CLC runners stats and update cache before rebalancing
	d.updateRunnersStats()

	start := time.Now()
	defer func() {
		rebalancingDuration.Set(time.Since(start).Seconds(), le.JoinLeaderValue)
	}()

	currentConfigsDistribution := d.currentDistribution()

	// Partition configs into cohorts by runner group (the group claiming the
	// check, "" for general workers), and rebalance each cohort within its
	// own runners.
	cohorts := make(map[string]*cohort)
	d.store.RLock()
	for digest, config := range currentConfigsDistribution.Configs {
		group := d.checkGroup[config.CheckName]
		c, ok := cohorts[group]
		if !ok {
			c = &cohort{group: group, runners: d.groupNodes(group), configs: make(map[string]*ConfigStatus)}
			cohorts[group] = c
		}
		c.configs[digest] = config
	}
	d.store.RUnlock()

	var allMoves []types.RebalanceResponse
	for _, group := range slices.Sorted(maps.Keys(cohorts)) {
		if c := cohorts[group]; len(c.runners) > 0 { // no live runner: nothing to rebalance onto
			allMoves = append(allMoves, d.rebalanceCohort(force, currentConfigsDistribution, c)...)
		}
	}
	return allMoves
}

// cohort is the set of configs of one runner group, and the group's runners.
type cohort struct {
	group   string // "" for general workers
	runners []string
	configs map[string]*ConfigStatus
}

// rebalanceCohort rebalances one cohort's configs within its eligible runners.
func (d *dispatcher) rebalanceCohort(force bool, current configsDistribution, cohort *cohort) []types.RebalanceResponse {
	// Runners that joined after the current snapshot wait for the next rebalance.
	runners := make(map[string]int, len(cohort.runners))
	for _, r := range cohort.runners {
		if runnerStatus, found := current.Runners[r]; found {
			runners[r] = runnerStatus.Workers
		}
	}

	config := pkgconfigsetup.Datadog()
	stickinessEnabled := config.GetBool("cluster_checks.stickiness_enabled")
	stickinessFactor := config.GetFloat64("cluster_checks.stickiness_factor")
	stickinessUpperLimit := config.GetFloat64("cluster_checks.stickiness_upper_limit")
	stickinessLowerLimit := config.GetFloat64("cluster_checks.stickiness_lower_limit")
	currentCohort := newConfigsDistribution(runners, stickinessEnabled, stickinessFactor, stickinessUpperLimit, stickinessLowerLimit)
	proposedCohort := newConfigsDistribution(runners, stickinessEnabled, stickinessFactor, stickinessUpperLimit, stickinessLowerLimit)

	// Current: each config on its runner. Proposed: pinned configs stay, the
	// rest go greedily to the least busy eligible runner.
	for digest, configInfo := range cohort.configs {
		currentCohort.addConfig(digest, configInfo.CheckName, configInfo.WorkersNeeded, configInfo.Runner, configInfo.Pinned)
		// Pinned configs are placed first so the greedy pass accounts for their load.
		if configInfo.Pinned {
			proposedCohort.addConfig(digest, configInfo.CheckName, configInfo.WorkersNeeded, configInfo.Runner, true)
		}
	}
	for _, digest := range currentCohort.configsSortedByWorkersNeeded() {
		configInfo := currentCohort.Configs[digest]
		if configInfo.Pinned {
			continue
		}
		proposedCohort.addToLeastBusy(digest, configInfo.CheckName, configInfo.WorkersNeeded, configInfo.Runner, "", false)
	}

	minPercImprovement := config.GetInt("cluster_checks.rebalance_min_percentage_improvement")
	currentStdDev, proposedStdDev := currentCohort.utilizationStdDev(), proposedCohort.utilizationStdDev()
	if !force && !rebalanceIsWorthIt(currentCohort, proposedCohort, minPercImprovement) {
		log.Debugf("Cohort rebalance not worth it (current stddev: %.3f, proposed: %.3f)", currentStdDev, proposedStdDev)
		setPredictedUtilization(currentCohort)
		return nil
	}

	moves := d.applyDistribution(proposedCohort, currentCohort)
	prefix := "Cohort rebalance: moved"
	if force {
		prefix = "Forced cohort rebalance: moved"
	}
	log.Infof("%s %d of %d checks in runner group %q on %d runners (stddev %.3f -> %.3f)", prefix, len(moves), len(proposedCohort.Configs), cmp.Or(cohort.group, "general"), len(proposedCohort.Runners), currentStdDev, proposedStdDev)
	setPredictedUtilization(proposedCohort)
	return moves
}

func (d *dispatcher) currentDistribution() configsDistribution {
	currentWorkersPerRunner := map[string]int{}

	d.store.RLock()
	defer d.store.RUnlock()

	for nodeName, nodeInfo := range d.store.nodes {
		currentWorkersPerRunner[nodeName] = nodeInfo.workers
	}

	distribution := newConfigsDistribution(currentWorkersPerRunner, pkgconfigsetup.Datadog().GetBool("cluster_checks.stickiness_enabled"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_factor"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_upper_limit"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_lower_limit"))

	for nodeName, nodeStoreInfo := range d.store.nodes {
		nodeStoreInfo.RLock()
		for checkID, stats := range nodeStoreInfo.clcRunnerStats {
			if !stats.IsClusterCheck {
				continue
			}

			// Group by digest so the algorithm operates on whole configs.
			// Skip checkIDs the dispatcher doesn't own.
			digest, ok := d.store.idToDigest[checkid.ID(checkID)]
			if !ok {
				log.Debugf("No digest registered for check ID %s on node %s; skipping in distribution", checkID, nodeName)
				continue
			}
			conf := d.store.digestToConfig[digest]

			minCollectionInterval := defaults.DefaultCheckInterval
			if len(conf.Instances) > 0 {
				commonOptions := integration.CommonInstanceConfig{}
				err := yaml.Unmarshal(conf.Instances[0], &commonOptions)
				if err != nil {
					log.Errorf("error getting min collection interval for check ID %s: %v", checkID, err)
				} else if commonOptions.MinCollectionInterval != 0 {
					minCollectionInterval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
				}
			}

			workersNeeded := (float64)(stats.AverageExecutionTime) / (float64)(minCollectionInterval.Milliseconds())
			if workersNeeded > 1 {
				workersNeeded = 1
			}

			// Pin if the check is explicitly excluded from rebalancing or if
			// this instance has no usable execution-time signal (AverageExecutionTime == 0).
			_, excluded := d.excludedChecksFromDispatching[conf.Name]
			pinned := excluded || workersNeeded == 0

			distribution.addConfig(digest, conf.Name, workersNeeded, nodeName, pinned)
		}
		nodeStoreInfo.RUnlock()
	}

	return distribution
}

func (d *dispatcher) applyDistribution(proposedDistribution configsDistribution, currentDistribution configsDistribution) []types.RebalanceResponse {
	var checksMoved []types.RebalanceResponse

	for digest, config := range proposedDistribution.Configs {
		currentNode := currentDistribution.runnerForConfig(digest)
		proposedNode := config.Runner

		if proposedNode == currentNode {
			continue
		}

		rebalancingDecisions.Inc(le.JoinLeaderValue)

		err := d.moveConfig(currentNode, proposedNode, digest)
		if err != nil {
			log.Warnf("Cannot move config %s: %v", digest, err)
			continue
		}

		successfulRebalancing.Inc(le.JoinLeaderValue)

		checksMoved = append(
			checksMoved,
			types.RebalanceResponse{
				Digest:         digest,
				CheckName:      config.CheckName,
				SourceNodeName: currentNode,
				DestNodeName:   proposedNode,
			},
		)
	}

	return checksMoved
}

func setPredictedUtilization(distribution configsDistribution) {
	for runnerName, runnerStatus := range distribution.Runners {
		predictedUtilization.Set(runnerStatus.utilization(), runnerName, le.JoinLeaderValue)
	}
}

func rebalanceIsWorthIt(currentDistribution configsDistribution, proposedDistribution configsDistribution, minPercImprovement int) bool {
	// If the current utilization stddev is already good enough, consider that
	// rescheduling checks is not worth it, unless the new distribution has
	// fewer runners with a high utilization or leaves fewer runners empty.
	if currentDistribution.utilizationStdDev() < 0.1 {
		return proposedDistribution.numRunnersWithHighUtilization() < currentDistribution.numRunnersWithHighUtilization() ||
			proposedDistribution.numEmptyRunners() < currentDistribution.numEmptyRunners()
	}

	maxStdDevAccepted := currentDistribution.utilizationStdDev() * ((100 - float64(minPercImprovement)) / 100)
	return proposedDistribution.utilizationStdDev() < maxStdDevAccepted
}
