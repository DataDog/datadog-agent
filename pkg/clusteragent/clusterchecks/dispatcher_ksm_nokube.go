// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// scheduleKSMCheck stub when kubeapiserver is not available
func (d *dispatcher) scheduleKSMCheck(config integration.Config) bool {
	log.Warn("KSM sharding requires kubeapiserver support to be compiled in")
	return false
}

// getNamespaces stub when kubeapiserver is not available
func (d *dispatcher) getNamespaces() ([]string, error) {
	return nil, nil
}

// getAvailableRunners stub when kubeapiserver is not available
func (d *dispatcher) getAvailableRunners() []string {
	d.store.RLock()
	defer d.store.RUnlock()

	// Still return CLC runners even without kube API access
	// This maintains consistency with the kube version
	runners := make([]string, 0, len(d.store.nodes))
	for nodeName, node := range d.store.nodes {
		if node.nodetype == types.NodeTypeNodeAgent {
			continue
		}
		runners = append(runners, nodeName)
	}

	return runners
}
