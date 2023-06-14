// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

// getAllConfigs returns all configurations known to the store, for reporting
func (d *dispatcher) getAllConfigs() ([]integration.Config, error) {
	d.store.RLock()
	defer d.store.RUnlock()

	return makeConfigArray(d.store.digestToConfig), nil
}

func (d *dispatcher) getState() (types.StateResponse, error) {
	d.store.RLock()
	defer d.store.RUnlock()

	response := types.StateResponse{
		Warmup:   !d.store.active,
		Dangling: makeConfigArray(d.store.danglingConfigs),
	}
	for _, node := range d.store.nodes {
		n := types.StateNodeResponse{
			Name:    node.name,
			Configs: makeConfigArray(node.digestToConfig),
		}
		response.Nodes = append(response.Nodes, n)
	}

	return response, nil
}

func (d *dispatcher) addConfig(config integration.Config, targetNodeName string) {
	d.store.Lock()
	defer d.store.Unlock()

	// Register config
	digest := config.Digest()
	fastDigest := config.FastDigest()
	d.store.digestToConfig[digest] = config
	for _, instance := range config.Instances {
		checkID := check.BuildID(config.Name, fastDigest, instance, config.InitConfig)
		d.store.idToDigest[checkID] = digest
		if targetNodeName != "" {
			configsInfo.Set(1.0, targetNodeName, string(checkID), le.JoinLeaderValue)
		}
	}

	// No target node specified: store in danglingConfigs
	if targetNodeName == "" {
		danglingConfigs.Inc(le.JoinLeaderValue)
		d.store.danglingConfigs[digest] = config
		return
	}

	currentNode, foundCurrent := d.store.getNodeStore(d.store.digestToNode[digest])
	targetNode := d.store.getOrCreateNodeStore(targetNodeName, "")

	// Dispatch to target node
	targetNode.Lock()
	targetNode.addConfig(config)
	targetNode.Unlock()
	d.store.digestToNode[digest] = targetNodeName

	// Remove config from previous node if found
	// We double-check the config actually changed nodes, to
	// prevent de-scheduling the check we just scheduled.
	// See https://github.com/DataDog/datadog-agent/pull/3023
	if foundCurrent && currentNode != targetNode {
		currentNode.Lock()
		currentNode.removeConfig(digest)
		currentNode.Unlock()
	}
}

func (d *dispatcher) removeConfig(digest string) {
	d.store.Lock()
	defer d.store.Unlock()

	node, found := d.store.getNodeStore(d.store.digestToNode[digest])
	delete(d.store.digestToNode, digest)
	delete(d.store.digestToConfig, digest)
	delete(d.store.danglingConfigs, digest)

	for k, v := range d.store.idToDigest {
		if v == digest {
			configsInfo.Delete(node.name, string(k), le.JoinLeaderValue)
			delete(d.store.idToDigest, k)
		}
	}

	// Remove from node configs if assigned
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
	danglingConfigs.Set(0, le.JoinLeaderValue)
	return configs
}

// patchConfiguration transforms the configuration from AD into a config
// ready to use by node agents. It does the following changes:
//   - empty the ADIdentifiers array, to avoid node-agents detecting them as templates
//   - clear the ClusterCheck boolean
//   - add the empty_default_hostname option to all instances
//   - inject the extra tags (including `cluster_name` if set) in all instances
func (d *dispatcher) patchConfiguration(in integration.Config) (integration.Config, error) {
	out := in
	out.ADIdentifiers = nil
	out.ClusterCheck = false

	// Deep copy the instances to avoid modifying the original
	out.Instances = make([]integration.Data, len(in.Instances))
	copy(out.Instances, in.Instances)

	for i := range out.Instances {
		err := out.Instances[i].SetField("empty_default_hostname", true)
		if err != nil {
			return in, err
		}

		// Inject extra tags if not empty
		if len(d.extraTags) == 0 {
			continue
		}
		err = out.Instances[i].MergeAdditionalTags(d.extraTags)
		if err != nil {
			return in, err
		}
	}

	return out, nil
}

// getConfigAndDigest returns config and digest of a check by checkID
func (d *dispatcher) getConfigAndDigest(checkID string) (integration.Config, string) {
	d.store.RLock()
	defer d.store.RUnlock()

	digest := d.store.idToDigest[check.ID(checkID)]
	return d.store.digestToConfig[digest], digest
}
