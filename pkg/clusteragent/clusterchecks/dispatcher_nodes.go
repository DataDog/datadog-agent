// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultBusynessValue int = -1
)

// getClusterCheckConfigs returns configurations dispatched to a given node
func (d *dispatcher) getClusterCheckConfigs(nodeName string) ([]integration.Config, int64, error) {
	d.store.RLock()
	defer d.store.RUnlock()

	node, found := d.store.getNodeStore(nodeName)
	if !found {
		return nil, 0, fmt.Errorf("node %s is unknown", nodeName)
	}

	node.RLock()
	defer node.RUnlock()
	return makeConfigArray(node.digestToConfig), node.lastConfigChange, nil
}

// processNodeStatus keeps the node's status in the store, and returns true
// if the last configuration change matches the one sent by the node agent.
func (d *dispatcher) processNodeStatus(nodeName, clientIP string, status types.NodeStatus) bool {
	var warmingUp bool

	d.store.Lock()
	if !d.store.active {
		warmingUp = true
	}
	node := d.store.getOrCreateNodeStore(nodeName, clientIP)
	d.store.Unlock()

	node.Lock()
	defer node.Unlock()
	node.heartbeat = timestampNow()
	// When we receive ExtraHeartbeatLastChangeValue, we only update heartbeat
	if status.LastChange == types.ExtraHeartbeatLastChangeValue {
		return true
	}

	if node.lastConfigChange == status.LastChange {
		// Node-agent is up to date
		return true
	}
	if warmingUp {
		// During the initial warmup phase, we are counting active nodes
		// without dispatching configurations.
		// We tell node-agents they are up to date to keep their cached
		// configurations running while we finish the warmup phase.
		return true
	}

	// Node-agent needs to pull updated configs
	log.Infof("Node %s needs to poll config, cluster config version: %d, node config version: %d", nodeName, node.lastConfigChange, status.LastChange)
	return false
}

// getNodeToScheduleCheck returns the node where a new check should be scheduled

// Advanced dispatching relies on the check stats fetched from the cluster check
// runners API to distribute the checks. The stats are only updated when the
// checks are rebalanced, they are not updated every time a check is scheduled.
// That's why it's not a good idea to pick the least busy node. Rebalance
// happens every few minutes, so all the checks added during that time would get
// scheduled to the same node. It's a better solution to pick a random node and
// rely on rebalancing to distribute when needed.
//
// On the other hand, when advanced dispatching is not used, we can pick the
// node with fewer checks. It's because the number of checks is kept up to date.
func (d *dispatcher) getNodeToScheduleCheck() string {
	if d.advancedDispatching {
		return d.getRandomNode()
	}

	return d.getNodeWithLessChecks()
}

func (d *dispatcher) getRandomNode() string {
	d.store.RLock()
	defer d.store.RUnlock()

	var nodes []string
	for name := range d.store.nodes {
		nodes = append(nodes, name)
	}

	if len(nodes) == 0 {
		return ""
	}

	return nodes[rand.Intn(len(nodes))]
}

func (d *dispatcher) getNodeWithLessChecks() string {
	d.store.RLock()
	defer d.store.RUnlock()

	var selectedNode string
	minNumChecks := 0

	for name, store := range d.store.nodes {
		if selectedNode == "" || len(store.digestToConfig) < minNumChecks {
			selectedNode = name
			minNumChecks = len(store.digestToConfig)
		}
	}

	return selectedNode
}

// expireNodes iterates over nodes and removes the ones that have not
// reported for more than the expiration duration. The configurations
// dispatched to these nodes will be moved to the danglingConfigs map.
func (d *dispatcher) expireNodes() {
	cutoffTimestamp := timestampNow() - d.nodeExpirationSeconds

	d.store.Lock()
	defer d.store.Unlock()

	initialNodeCount := len(d.store.nodes)

	for name, node := range d.store.nodes {
		node.RLock()
		if node.heartbeat < cutoffTimestamp {
			if name != "" {
				// Don't report on the dummy "" host for unscheduled configs
				log.Infof("Expiring out node %s, last status report %d seconds ago", name, timestampNow()-node.heartbeat)
			}
			for digest, config := range node.digestToConfig {
				delete(d.store.digestToNode, digest)
				log.Debugf("Adding %s:%s as a dangling Cluster Check config", config.Name, digest)
				d.store.danglingConfigs[digest] = config
				danglingConfigs.Inc(le.JoinLeaderValue)

				// TODO: Use partial label matching when it becomes available:
				// Replace the loop by a single function call (delete by node name).
				// Requires https://github.com/prometheus/client_golang/pull/1013
				for k, v := range d.store.idToDigest {
					if v == digest {
						configsInfo.Delete(name, config.Name, string(k), le.JoinLeaderValue)
					}
				}
			}
			delete(d.store.nodes, name)

			// Remove metrics linked to this node
			nodeAgents.Dec(le.JoinLeaderValue)
			dispatchedConfigs.Delete(name, le.JoinLeaderValue)
			statsCollectionFails.Delete(name, le.JoinLeaderValue)
			busyness.Delete(name, le.JoinLeaderValue)
		}
		node.RUnlock()
	}

	if initialNodeCount != 0 && len(d.store.nodes) == 0 {
		log.Warn("No nodes reporting, cluster checks will not run")
	}
}

// updateRunnersStats collects stats from the registred
// Cluster Level Check runners and updates the stats cache
func (d *dispatcher) updateRunnersStats() {
	if d.clcRunnersClient == nil {
		log.Debug("Cluster Level Check runner client was not correctly initialised")
		return
	}

	start := time.Now()
	defer func() {
		updateStatsDuration.Set(time.Since(start).Seconds(), le.JoinLeaderValue)
	}()

	d.store.Lock()
	defer d.store.Unlock()
	for name, node := range d.store.nodes {
		node.RLock()
		ip := node.clientIP
		node.RUnlock()

		if config.Datadog.GetBool("cluster_checks.rebalance_with_utilization") {
			workers, err := d.clcRunnersClient.GetRunnerWorkers(ip)
			if err != nil {
				// This can happen in old versions of the runners that do not expose this information.
				log.Debugf("Cannot get number of workers for node %s with IP %s. Assuming default. Error: %v", name, node.clientIP, err)
				node.workers = config.DefaultNumWorkers
			} else {
				node.workers = workers.Count
			}
		}

		stats, err := d.clcRunnersClient.GetRunnerStats(ip)
		if err != nil {
			log.Debugf("Cannot get CLC Runner stats with IP %s on node %s: %v", node.clientIP, name, err)
			statsCollectionFails.Inc(name, le.JoinLeaderValue)
			continue
		}
		node.Lock()
		for id, checkStats := range stats {
			// Stats contain info about all the running checks on a node
			// Node checks must be filtered from Cluster Checks
			// so they can be included in calculating node Agent busyness and excluded from rebalancing decisions.
			if _, found := d.store.idToDigest[checkid.ID(id)]; found {
				// Cluster check detected (exists in the Cluster Agent checks store)
				log.Tracef("Check %s running on node %s is a cluster check", id, node.name)
				checkStats.IsClusterCheck = true
				stats[id] = checkStats
			}
		}
		node.clcRunnerStats = stats
		log.Tracef("Updated CLC Runner stats on node: %s, node IP: %s, stats: %v", name, node.clientIP, stats)
		node.busyness = calculateBusyness(stats)
		log.Debugf("Updated busyness on node: %s, node IP: %s, busyness value: %d", name, node.clientIP, node.busyness)
		busyness.Set(float64(node.busyness), node.name, le.JoinLeaderValue)
		node.Unlock()
	}
}
