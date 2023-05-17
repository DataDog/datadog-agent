// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

// getEndpointsConfigs provides configs templates of endpoints checks queried by node name.
// Exposed to node agents by the cluster agent api.
func (d *dispatcher) getEndpointsConfigs(nodeName string) ([]integration.Config, error) {
	nodeConfigs := []integration.Config{}
	d.store.RLock()
	for _, v := range d.store.endpointsConfigs[nodeName] {
		nodeConfigs = append(nodeConfigs, v)
	}
	d.store.RUnlock()
	return nodeConfigs, nil
}

// getAllEndpointsCheckConfigs provides all config templates of endpoints checks
func (d *dispatcher) getAllEndpointsCheckConfigs() ([]integration.Config, error) {
	configs := []integration.Config{}
	d.store.RLock()
	defer d.store.RUnlock()
	for _, configMap := range d.store.endpointsConfigs {
		for _, config := range configMap {
			configs = append(configs, config)
		}
	}
	return configs, nil
}

// addEndpointConfig stores a given endpoint configuration by node name
func (d *dispatcher) addEndpointConfig(config integration.Config, nodename string) {
	d.store.Lock()
	defer d.store.Unlock()
	if d.store.endpointsConfigs[nodename] == nil {
		d.store.endpointsConfigs[nodename] = map[string]integration.Config{}
	}
	d.store.endpointsConfigs[nodename][config.Digest()] = config
	dispatchedEndpoints.Inc(nodename, le.JoinLeaderValue)
}

// removeEndpointConfig deletes a given endpoint configuration
func (d *dispatcher) removeEndpointConfig(config integration.Config, nodename string) {
	d.store.Lock()
	defer d.store.Unlock()

	digest := config.Digest()
	if _, found := d.store.endpointsConfigs[nodename][digest]; !found {
		return
	}

	delete(d.store.endpointsConfigs[nodename], digest)
	dispatchedEndpoints.Dec(nodename, le.JoinLeaderValue)
}

// patchEndpointsConfiguration transforms the endpoint configuration from AD into a config
// ready to use by node agents. It does the following changes:
//   - clear the ClusterCheck boolean
//   - inject the extra tags (including `cluster_name` if set) in all instances
func (d *dispatcher) patchEndpointsConfiguration(in integration.Config) (integration.Config, error) {
	out := in
	out.ClusterCheck = false

	if out.Provider == names.CloudFoundryBBS {
		// Remove ADIdentifiers if the config comes from the cloudfoundry provider, so that they are ready
		// to be scheduled on the node agent directly (config is already resolved by the DCA)
		out.ADIdentifiers = nil
	}

	// Deep copy the instances to avoid modifying the original
	out.Instances = make([]integration.Data, len(in.Instances))
	copy(out.Instances, in.Instances)
	out.NodeName = in.NodeName

	for i := range out.Instances {
		// Inject extra tags if not empty
		if len(d.extraTags) == 0 {
			continue
		}
		err := out.Instances[i].MergeAdditionalTags(d.extraTags)
		if err != nil {
			return in, err
		}
	}

	return out, nil
}
