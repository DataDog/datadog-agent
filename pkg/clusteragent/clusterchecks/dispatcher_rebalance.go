// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// tolerationMargin is used to lean towards stability when rebalancing cluster level checks
// by moving a check from a node to another if destNodeBusyness + checkWeight < srcNodeBusyness*tolerationMargin
// the 0.9 value is tentative and could be changed
const tolerationMargin float64 = 0.9

// Weight holds a node name and the corresponding busyness score
type Weight struct {
	nodeName string
	busyness int
}

// Weights is an array of node weights
type Weights []Weight

func (w Weights) Len() int           { return len(w) }
func (w Weights) Less(i, j int) bool { return w[i].busyness > w[j].busyness }
func (w Weights) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }

func (d *dispatcher) calculateAvg() (int, error) {
	busyness := 0
	length := 0

	d.store.RLock()
	defer d.store.RUnlock()

	for _, node := range d.store.nodes {
		busyness += node.GetBusyness(busynessFunc)
		length++
	}

	if length == 0 {
		return -1, errors.New("zero nodes reporting")
	}

	return busyness / length, nil
}

// getDiffAndWeights creates a map that contains the difference between
// the busyness on each node and the total average busyness, and a Weights
// struct containing nodes and their busyness values
func (d *dispatcher) getDiffAndWeights(avg int) (map[string]int, Weights) {
	diffMap := make(map[string]int)
	weights := Weights{}

	d.store.RLock()
	defer d.store.RUnlock()

	for nodeName, node := range d.store.nodes {
		busyness := node.GetBusyness(busynessFunc)
		diffMap[nodeName] = busyness - avg
		weights = append(weights, Weight{
			nodeName: nodeName,
			busyness: busyness,
		})
	}
	return diffMap, weights
}

// updateDiff creates a map that contains the difference between
// the busyness on each node and the total average busyness.
func (d *dispatcher) updateDiff(avg int) map[string]int {
	diffMap := make(map[string]int)

	d.store.RLock()
	defer d.store.RUnlock()

	for nodeName, node := range d.store.nodes {
		busyness := node.GetBusyness(busynessFunc)
		diffMap[nodeName] = busyness - avg
	}

	return diffMap
}

// pickConfigToMove returns the digest of the heaviest config on the node,
// with weight summed across the config's instances.
func (d *dispatcher) pickConfigToMove(nodeName string) (string, int, error) {
	d.store.RLock()
	node, found := d.store.getNodeStore(nodeName)
	if !found {
		d.store.RUnlock()
		log.Debugf("Node %s not found in store. Won't consider moving config", nodeName)
		return "", -1, fmt.Errorf("node %s not found in store", nodeName)
	}

	node.RLock()
	weightsByDigest := make(map[string]int, len(node.clcRunnerStats))
	for checkID, stats := range node.clcRunnerStats {
		if !stats.IsClusterCheck {
			continue
		}
		digest, ok := d.store.idToDigest[checkid.ID(checkID)]
		if !ok {
			continue
		}
		weightsByDigest[digest] += busynessFunc(stats)
	}
	node.RUnlock()
	d.store.RUnlock()

	if len(weightsByDigest) == 0 {
		log.Debugf("Node %s has no cluster check stats", nodeName)
		return "", -1, fmt.Errorf("no cluster checks found on node %s", nodeName)
	}

	bestDigest := ""
	bestWeight := 0
	for digest, weight := range weightsByDigest {
		if weight >= bestWeight {
			bestDigest = digest
			bestWeight = weight
		}
	}
	return bestDigest, bestWeight, nil
}

// pickNode select the most appropriate node to receive a specific check.
// A node Ni is most appropriate to receive a check with a weight W
// if it satisfies the following
// Diff(Ni) < Diff(Nj) (for each j != i, 0 <= j < len(nodes))
// where Diff(N) is the difference between the busyness on N and the total average busyness.
func pickNode(diffMap map[string]int, sourceNode string) string {
	firstItr := true
	minDiff := 0
	pickedNode := ""
	for _, node := range orderedKeys(diffMap) {
		if node == sourceNode {
			continue
		}
		if diffMap[node] < minDiff || firstItr {
			minDiff = diffMap[node]
			pickedNode = node
			firstItr = false
		}
	}
	return pickedNode
}

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

	var result []types.RebalanceResponse
	if pkgconfigsetup.Datadog().GetBool("cluster_checks.rebalance_with_utilization") {
		result = d.rebalanceUsingUtilization(force)
		span.SetTag("algorithm", "utilization")
	} else {
		result = d.rebalanceUsingBusyness()
		span.SetTag("algorithm", "busyness")
	}
	span.SetTag("checks_moved", len(result))
	return result
}

// rebalanceUsingBusyness tries to optimize the checks repartition on cluster
// level check runners with less possible check moves based on the runner stats.
func (d *dispatcher) rebalanceUsingBusyness() []types.RebalanceResponse {
	// Collect CLC runners stats and update cache before rebalancing
	d.updateRunnersStats()

	start := time.Now()
	defer func() {
		rebalancingDuration.Set(time.Since(start).Seconds(), le.JoinLeaderValue)
	}()

	log.Trace("Trying to rebalance cluster checks distribution if needed")
	totalAvg, err := d.calculateAvg()
	if err != nil {
		log.Debugf("Cannot rebalance checks: %v", err)
		return nil
	}

	checksMoved := []types.RebalanceResponse{}
	diffMap, weights := d.getDiffAndWeights(totalAvg)
	sort.Sort(weights)

	for _, nodeWeight := range weights {
		for diffMap[nodeWeight.nodeName] > 0 {
			// try to move configs from a node only if the node busyness is above the average
			sourceNodeName := nodeWeight.nodeName
			digest, configWeight, err := d.pickConfigToMove(sourceNodeName)
			if err != nil {
				log.Debugf("Cannot pick a config to move from node %s: %v", sourceNodeName, err)
				break
			}

			destNodeName := pickNode(diffMap, sourceNodeName)
			sourceDiff := diffMap[sourceNodeName]
			destDiff := diffMap[destNodeName]

			// move a config to a new node only if it keeps the
			// busyness of the new node lower than the original
			// node's busyness multiplied by the tolerationMargin
			// value the toleration margin is used to lean towards
			// stability over perfectly optimal balance
			if destDiff+configWeight < int(float64(sourceDiff)*tolerationMargin) {
				rebalancingDecisions.Inc(le.JoinLeaderValue)
				err = d.moveConfig(sourceNodeName, destNodeName, digest)
				if err != nil {
					log.Debugf("Cannot move config %s: %v", digest, err)
					break
				}

				successfulRebalancing.Inc(le.JoinLeaderValue)
				log.Tracef("Config %s with weight %d moved, total avg: %d, source diff: %d, dest diff: %d",
					digest, configWeight, totalAvg, sourceDiff, destDiff)
				// diffMap needs to be updated on every move
				diffMap = d.updateDiff(totalAvg)
				checksMoved = append(checksMoved, types.RebalanceResponse{
					Digest:         digest,
					CheckWeight:    configWeight,
					SourceNodeName: sourceNodeName,
					SourceDiff:     sourceDiff,
					DestNodeName:   destNodeName,
					DestDiff:       destDiff,
				})
			} else {
				break
			}
		}
	}

	return checksMoved
}

// rebalanceUsingUtilization rebalances the cluster checks deployed in a cluster
// by taking into account the workers utilization of each runner instead of
// using the busyness function as the rebalanceUsingBusyness function.
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
// The algorithm used by rebalanceUsingBusyness has 2 limitations that this one
// does not have:
// - It does not try to move checks from runners where the busyness is below
// average.
// - It tries to move checks from a runner trying the ones with the highest
// busyness first. It stops when it cannot move one, even if ones with a lower
// busyness could be moved.
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
	proposedDistribution := newConfigsDistribution(currentConfigsDistribution.runnerWorkers(), pkgconfigsetup.Datadog().GetBool("cluster_checks.stickiness_enabled"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_factor"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_upper_limit"), pkgconfigsetup.Datadog().GetFloat64("cluster_checks.stickiness_lower_limit"))

	// Place configs in proposed: pinned ones stay on their current runner,
	for digest, config := range currentConfigsDistribution.Configs {
		if config.Pinned {
			proposedDistribution.addConfig(digest, config.CheckName, config.WorkersNeeded, config.Runner, true)
		}
	}
	// the rest go greedily on the least busy runner (descending workersNeeded).
	for _, digest := range currentConfigsDistribution.configsSortedByWorkersNeeded() {
		config := currentConfigsDistribution.Configs[digest]
		if config.Pinned {
			continue
		}
		proposedDistribution.addToLeastBusy(
			digest,
			config.CheckName,
			config.WorkersNeeded,
			config.Runner,
			"",
			false,
		)
	}

	// We don't calculate the optimal distribution, so it might be worse than
	// the current one or not good enough so that it's worth it to schedule and
	// unschedule checks. When that's the case, return without moving any
	// checks.
	currentUtilizationStdDev := currentConfigsDistribution.utilizationStdDev()
	proposedUtilizationStdDev := proposedDistribution.utilizationStdDev()
	minPercImprovement := pkgconfigsetup.Datadog().GetInt("cluster_checks.rebalance_min_percentage_improvement")

	if force || rebalanceIsWorthIt(currentConfigsDistribution, proposedDistribution, minPercImprovement) {

		jsonDistribution, _ := json.Marshal(proposedDistribution)

		calculatedMoves := d.applyDistribution(proposedDistribution, currentConfigsDistribution)
		numOfMoves, numOfConfigs, numOfRunners := len(calculatedMoves), len(proposedDistribution.Configs), len(proposedDistribution.Runners)

		prefixMessage := "Found a better distribution for the cluster checks. "
		if force {
			prefixMessage = "Forcing rebalance proposed distribution for the cluster checks. "
		}

		log.Infof("%s Moving %d checks out of %d configs on %d runners. Utilization stdDev of proposed distribution: %.3f. StdDev of current distribution: %.3f. Proposed distribution: %s",
			prefixMessage, numOfMoves, numOfConfigs, numOfRunners, proposedUtilizationStdDev, currentUtilizationStdDev, jsonDistribution)

		setPredictedUtilization(proposedDistribution)
		return calculatedMoves
	}

	log.Debugf("Didn't find a distribution better enough so that rescheduling checks is worth it (current utilization stddev: %.3f, found utilization stddev: %.3f)",
		currentUtilizationStdDev, proposedUtilizationStdDev)
	setPredictedUtilization(currentConfigsDistribution)
	return nil
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
