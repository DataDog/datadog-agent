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
	node.lastPing = timestampNow()

	return (node.lastConfigChange == status.LastChange), nil
}
