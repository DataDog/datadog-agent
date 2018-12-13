// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// getAllConfigs returns all configurations known to the store, for reporting
func (d *dispatcher) getAllConfigs() ([]integration.Config, error) {
	d.store.RLock()
	defer d.store.RUnlock()

	return makeConfigArray(d.store.digestToConfig), nil
}

func (d *dispatcher) addConfig(config integration.Config, targetNodeName string) {
	d.store.Lock()
	defer d.store.Unlock()

	// Register config
	digest := config.Digest()
	d.store.digestToConfig[digest] = config

	// No target node specified: store in danglingConfigs
	if targetNodeName == "" {
		danglingConfigs.Inc()
		d.store.danglingConfigs[digest] = config
		return
	}

	currentNode, foundCurrent := d.store.getNodeStore(d.store.digestToNode[digest])
	targetNode := d.store.getOrCreateNodeStore(targetNodeName)

	// Dispatch to target node
	targetNode.Lock()
	targetNode.addConfig(config)
	targetNode.Unlock()
	d.store.digestToNode[digest] = targetNodeName

	// Remove config from previous node if found
	if foundCurrent {
		currentNode.Lock()
		currentNode.removeConfig(digest)
		currentNode.Unlock()
	}
}

func (d *dispatcher) removeConfig(digest string) {
	d.store.Lock()
	defer d.store.Unlock()

	delete(d.store.digestToConfig, digest)
	delete(d.store.danglingConfigs, digest)

	// Remove from node configs if assigned
	node, found := d.store.getNodeStore(d.store.digestToNode[digest])
	if found {
		node.Lock()
		node.removeConfig(digest)
		node.Unlock()
	}
}

// shouldDispatchDanling returns true if there are dangling configs
// and node registered, available for dispatching.
func (d *dispatcher) shouldDispatchDanling() bool {
	d.store.RLock()
	defer d.store.RUnlock()

	if len(d.store.danglingConfigs) == 0 {
		return false
	}
	if len(d.store.nodes) == 0 {
		return false
	}
	return true
}

// retrieveAndClearDangling extracts dangling configs from the store
func (d *dispatcher) retrieveAndClearDangling() []integration.Config {
	d.store.Lock()
	defer d.store.Unlock()
	configs := makeConfigArray(d.store.danglingConfigs)
	d.store.clearDangling()
	danglingConfigs.Set(0)
	return configs
}
