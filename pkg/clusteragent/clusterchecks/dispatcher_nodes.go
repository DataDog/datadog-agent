// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultBusynessValue int = -1

// getNodeConfigs returns configurations dispatched to a given node
func (d *dispatcher) getNodeConfigs(nodeName string) ([]integration.Config, int64, error) {
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
func (d *dispatcher) processNodeStatus(nodeName, clientIP string, status types.NodeStatus) (bool, error) {
	var warmingUp bool

	d.store.Lock()
	if !d.store.active {
		warmingUp = true
	}
	node := d.store.getOrCreateNodeStore(nodeName, clientIP)
	d.store.Unlock()

	node.Lock()
	defer node.Unlock()
	node.lastStatus = status
	node.heartbeat = timestampNow()

	if node.lastConfigChange == status.LastChange {
		// Node-agent is up to date
		return true, nil
	}
	if warmingUp {
		// During the initial warmup phase, we are counting active nodes
		// without dispatching configurations.
		// We tell node-agents they are up to date to keep their cached
		// configurations running while we finish the warmup phase.
		return true, nil
	}

	// Node-agent needs to pull updated configs
	return false, nil
}

// getLeastBusyNode returns the name of the node that is assigned
// the lowest number of checks. In case of equality, one is chosen
// randomly, based on map iterations being randomized.
func (d *dispatcher) getLeastBusyNode() string {
	var leastBusyNode string
	minCheckCount := int(-1)
	minBusyness := int(-1)

	d.store.RLock()
	defer d.store.RUnlock()

	for name, store := range d.store.nodes {
		if name == "" {
			continue
		}
		if d.advancedDispatching && store.busyness > defaultBusynessValue {
			// dispatching based on clc runners stats
			// only when advancedDispatching is true and
			// started collecting busyness values
			if minBusyness == -1 || store.busyness < minBusyness {
				leastBusyNode = name
				minBusyness = store.busyness
			}
		} else {
			// count-based round robin dispatching
			if minCheckCount == -1 || len(store.digestToConfig) < minCheckCount {
				leastBusyNode = name
				minCheckCount = len(store.digestToConfig)
			}
		}
	}
	return leastBusyNode
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
				danglingConfigs.Inc()
			}
			delete(d.store.nodes, name)

			// Remove metrics linked to this node
			nodeAgents.Dec()
			dispatchedConfigs.Delete(name)
			statsCollectionFails.Delete(name)
			busyness.Delete(name)
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
		updateStatsDuration.Set(time.Since(start).Seconds())
	}()

	d.store.Lock()
	defer d.store.Unlock()
	for name, node := range d.store.nodes {
		node.RLock()
		ip := node.clientIP
		node.RUnlock()

		stats, err := d.clcRunnersClient.GetRunnerStats(ip)
		if err != nil {
			log.Debugf("Cannot get CLC Runner stats with IP %s on node %s: %v", node.clientIP, name, err)
			statsCollectionFails.Inc(name)
			continue
		}
		node.Lock()
		for id, checkStats := range stats {
			// Stats contain info about all the running checks on a node
			// Node checks must be filtered from Cluster Checks
			// so they can be included in calculating node Agent busyness and excluded from rebalancing decisions.
			if _, found := d.store.idToDigest[check.ID(id)]; found {
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
		busyness.Set(float64(node.busyness), node.name)
		node.Unlock()
	}
}
