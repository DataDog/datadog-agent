// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// getEndpointsConfigs provides configs templates of endpoints checks queried by node name.
// Exposed to node agents by the cluster agent api.
func (d *dispatcher) getEndpointsConfigs(nodeName string) ([]integration.Config, error) {
	result := []integration.Config{}
	d.store.RLock()
	for _, v := range d.store.endpointsConfigs[nodeName] {
		result = append(result, v)
	}
	d.store.RUnlock()
	return result, nil
}

// addEndpointConfig stores a given endpoint configuration by node name
func (d *dispatcher) addEndpointConfig(config integration.Config, nodename string) {
	d.store.Lock()
	defer d.store.Unlock()
	if d.store.endpointsConfigs[nodename] == nil {
		d.store.endpointsConfigs[nodename] = map[string]integration.Config{}
	}
	d.store.endpointsConfigs[nodename][config.Digest()] = config
}

// removeEndpointConfig deletes a given endpoint configuration
func (d *dispatcher) removeEndpointConfig(config integration.Config, nodename string) {
	d.store.Lock()
	defer d.store.Unlock()
	delete(d.store.endpointsConfigs[nodename], config.Digest())
}

// patchEndpointsConfiguration transforms the endpoint configuration from AD into a config
// ready to use by node agents. It does the following changes:
//   - clear the ClusterCheck boolean
//   - inject the extra tags (including `cluster_name` if set) in all instances
func (d *dispatcher) patchEndpointsConfiguration(in integration.Config) (integration.Config, error) {
	out := in
	out.ClusterCheck = false

	// Deep copy the instances to avoid modifying the original
	out.Instances = make([]integration.Data, len(in.Instances))
	copy(out.Instances, in.Instances)

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
