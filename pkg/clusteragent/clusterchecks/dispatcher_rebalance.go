// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
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
		return -1, fmt.Errorf("zero nodes reporting")
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

// pickCheckToMove select the most appropriate check to move from a node to another.
// A check Xi running on a node N is chosen to move to another node if it satisfies the following
// Weight(Xi) >  Weight(Xj) (for each j != i, 0 <= j < len(weights))
// where Weight(X) is the busyness value caused by running the check X.
func (d *dispatcher) pickCheckToMove(nodeName string) (string, int, error) {
	d.store.RLock()
	node, found := d.store.getNodeStore(nodeName)
	d.store.RUnlock()

	if !found {
		log.Debugf("Node %s not found in store. Won't consider moving check", nodeName)
		return "", -1, fmt.Errorf("node %s not found in store", nodeName)
	}

	return node.GetMostWeightedClusterCheck(busynessFunc)
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

// moveCheck moves a check by its ID from a node to another
func (d *dispatcher) moveCheck(src, dest, checkID string) error {
	log.Debugf("Moving %s from %s to %s", checkID, src, dest)

	d.store.RLock()
	destNode, destFound := d.store.getNodeStore(dest)
	sourceNode, srcFound := d.store.getNodeStore(src)
	d.store.RUnlock()

	if !destFound || !srcFound {
		log.Debugf("Nodes not found in store: %s, %s. Check %s will not move", src, dest, checkID)
		return fmt.Errorf("node %s not found", src)
	}

	runnerStats, err := sourceNode.GetRunnerStats(checkID)
	if err != nil {
		log.Debugf("Cannot get runner stats on node %s, check %s will not move", src, checkID)
		return err
	}

	destNode.AddRunnerStats(checkID, runnerStats)
	sourceNode.RemoveRunnerStats(checkID)

	config, digest := d.getConfigAndDigest(checkID)
	log.Tracef("Moving check %s with digest %s and config %s from %s to %s", checkID, digest, config.String(), src, dest)

	d.removeConfig(digest)
	d.addConfig(config, dest)

	log.Debugf("Check %s moved from %s to %s", checkID, src, dest)

	return nil
}

func (d *dispatcher) rebalance(force bool) []types.RebalanceResponse {
	if config.Datadog.GetBool("cluster_checks.rebalance_with_utilization") {
		return d.rebalanceUsingUtilization(force)
	}

	return d.rebalanceUsingBusyness()
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
			// try to move checks from a node only of the node busyness is above the average
			sourceNodeName := nodeWeight.nodeName
			checkID, checkWeight, err := d.pickCheckToMove(sourceNodeName)
			if err != nil {
				log.Debugf("Cannot pick a check to move from node %s: %v", sourceNodeName, err)
				break
			}

			destNodeName := pickNode(diffMap, sourceNodeName)
			sourceDiff := diffMap[sourceNodeName]
			destDiff := diffMap[destNodeName]

			// move a check to a new node only if it keeps the
			// busyness of the new node lower than the original
			// node's busyness multiplied by the tolerationMargin
			// value the toleration margin is used to lean towards
			// stability over perfectly optimal balance
			if destDiff+checkWeight < int(float64(sourceDiff)*tolerationMargin) {
				rebalancingDecisions.Inc(le.JoinLeaderValue)
				err = d.moveCheck(sourceNodeName, destNodeName, checkID)
				if err != nil {
					log.Debugf("Cannot move check %s: %v", checkID, err)
					continue
				}

				successfulRebalancing.Inc(le.JoinLeaderValue)
				log.Tracef("Check %s with weight %d moved, total avg: %d, source diff: %d, dest diff: %d",
					checkID, checkWeight, totalAvg, sourceDiff, destDiff)
				// diffMap needs to be updated on every check moved
				diffMap = d.updateDiff(totalAvg)
				checksMoved = append(checksMoved, types.RebalanceResponse{
					CheckID:        checkID,
					CheckWeight:    checkWeight,
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

	currentChecksDistribution := d.currentDistribution()

	proposedDistribution := newChecksDistribution(currentChecksDistribution.runnerWorkers())
	for _, checkID := range currentChecksDistribution.checksSortedByWorkersNeeded() {
		proposedDistribution.addToLeastBusy(
			checkID,
			currentChecksDistribution.workersNeededForCheck(checkID),
			currentChecksDistribution.runnerForCheck(checkID),
		)
	}

	// We don't calculate the optimal distribution, so it might be worse than
	// the current one or not good enough so that it's worth it to schedule and
	// unschedule checks. When that's the case, return without moving any
	// checks.
	currentUtilizationStdDev := currentChecksDistribution.utilizationStdDev()
	proposedUtilizationStdDev := proposedDistribution.utilizationStdDev()
	minPercImprovement := config.Datadog.GetInt("cluster_checks.rebalance_min_percentage_improvement")

	if force || rebalanceIsWorthIt(currentChecksDistribution, proposedDistribution, minPercImprovement) {

		jsonDistribution, _ := json.Marshal(proposedDistribution)

		if force {
			log.Infof("Forcing rebalance proposed distribution for the cluster checks. Utilization stdDev of proposed distribution: %.3f. StdDev of current distribution: %.3f. Proposed distribution: %s",
				proposedUtilizationStdDev, currentUtilizationStdDev, jsonDistribution)
		} else {
			log.Infof("Found a better distribution for the cluster checks. Utilization stdDev of proposed distribution: %.3f. StdDev of current distribution: %.3f. Proposed distribution: %s",
				proposedUtilizationStdDev, currentUtilizationStdDev, jsonDistribution)
		}

		setPredictedUtilization(proposedDistribution)

		return d.applyDistribution(proposedDistribution, currentChecksDistribution)
	}

	log.Debugf("Didn't find a distribution better enough so that rescheduling checks is worth it (current utilization stddev: %.3f, found utilization stddev: %.3f)",
		currentUtilizationStdDev, proposedUtilizationStdDev)
	setPredictedUtilization(currentChecksDistribution)
	return nil
}

func (d *dispatcher) currentDistribution() checksDistribution {
	currentWorkersPerRunner := map[string]int{}

	d.store.Lock()
	defer d.store.Unlock()

	for nodeName, nodeInfo := range d.store.nodes {
		currentWorkersPerRunner[nodeName] = nodeInfo.workers
	}

	distribution := newChecksDistribution(currentWorkersPerRunner)

	for nodeName, nodeStoreInfo := range d.store.nodes {
		for checkID, stats := range nodeStoreInfo.clcRunnerStats {
			digest, found := d.store.idToDigest[checkid.ID(checkID)]
			if !found { // Not a cluster check
				continue
			}

			minCollectionInterval := defaults.DefaultCheckInterval

			conf := d.store.digestToConfig[digest]

			if len(conf.Instances) > 0 {
				commonOptions := integration.CommonInstanceConfig{}
				err := yaml.Unmarshal(conf.Instances[0], &commonOptions)
				if err != nil {
					// Assume default
					log.Errorf("error getting min collection interval for check ID %s: %v", checkID, err)
				} else if commonOptions.MinCollectionInterval != 0 {
					minCollectionInterval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
				}
			}

			workersNeeded := (float64)(stats.AverageExecutionTime) / (float64)(minCollectionInterval.Milliseconds())
			if workersNeeded > 1 {
				workersNeeded = 1
			}

			distribution.addCheck(checkID, workersNeeded, nodeName)
		}
	}

	return distribution
}

func (d *dispatcher) applyDistribution(proposedDistribution checksDistribution, currentDistribution checksDistribution) []types.RebalanceResponse {
	var checksMoved []types.RebalanceResponse

	for checkID, checkStatus := range proposedDistribution.Checks {
		currentNode := currentDistribution.runnerForCheck(checkID)
		proposedNode := checkStatus.Runner

		if proposedNode == currentNode {
			continue
		}

		rebalancingDecisions.Inc(le.JoinLeaderValue)

		err := d.moveCheck(currentNode, proposedNode, checkID)
		if err != nil {
			log.Warnf("Cannot move check %s: %v", checkID, err)
			continue
		}

		successfulRebalancing.Inc(le.JoinLeaderValue)

		checksMoved = append(
			checksMoved,
			types.RebalanceResponse{
				CheckID:        checkID,
				SourceNodeName: currentNode,
				DestNodeName:   proposedNode,
			},
		)
	}

	return checksMoved
}

func setPredictedUtilization(distribution checksDistribution) {
	for runnerName, runnerStatus := range distribution.Runners {
		predictedUtilization.Set(runnerStatus.utilization(), runnerName, le.JoinLeaderValue)
	}
}

func rebalanceIsWorthIt(currentDistribution checksDistribution, proposedDistribution checksDistribution, minPercImprovement int) bool {
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
