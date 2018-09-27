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

	if targetNodeName == "" {
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

	// Remove from node configs if assigned
	node, found := d.store.getNodeStore(d.store.digestToNode[digest])
	if found {
		node.Lock()
		node.removeConfig(digest)
		node.Unlock()
	}
}
