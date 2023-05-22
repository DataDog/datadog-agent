// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
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
		busyness = node.GetBusyness(busynessFunc)
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

// rebalance tries to optimize the checks repartition on cluster level check
// runners with less possible check moves based on the runner stats.
func (d *dispatcher) rebalance() []types.RebalanceResponse {
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
