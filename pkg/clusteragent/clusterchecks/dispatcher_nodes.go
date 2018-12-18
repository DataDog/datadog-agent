// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

// processNodeStatus returns configurations dispatched to a given node.
// returns true if the last configuration change matches the one sent by the node agent.
func (d *dispatcher) processNodeStatus(nodeName string, status types.NodeStatus) (bool, error) {
	d.store.Lock()
	node := d.store.getOrCreateNodeStore(nodeName)
	d.store.Unlock()

	node.Lock()
	defer node.Unlock()
	node.lastStatus = status
	node.heartbeat = timestampNow()

	return (node.lastConfigChange == status.LastChange), nil
}

// getLeastBusyNode returns the name of the node that is assigned
// the lowest number of checks. In case of equality, one is chosen
// randomly, based on map iterations being randomized.
func (d *dispatcher) getLeastBusyNode() string {
	var leastBusyNode string
	minCheckCount := int(-1)

	d.store.RLock()
	defer d.store.RUnlock()

	for name, store := range d.store.nodes {
		if name == "" {
			continue
		}
		if minCheckCount == -1 || len(store.digestToConfig) < minCheckCount {
			leastBusyNode = name
			minCheckCount = len(store.digestToConfig)
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

	for name, node := range d.store.nodes {
		node.RLock()
		if node.heartbeat < cutoffTimestamp {
			if name != "" {
				// Don't report on the dummy "" host for unscheduled configs
				log.Infof("Expiring out node %s, last status report %d seconds ago", name, timestampNow()-node.heartbeat)
			}
			for digest, config := range node.digestToConfig {
				d.store.danglingConfigs[digest] = config
			}
			delete(d.store.nodes, name)
		}
		node.RUnlock()
	}
}
