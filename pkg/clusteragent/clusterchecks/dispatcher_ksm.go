// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package clusterchecks

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// scheduleKSMCheck handles the special case of KSM checks with sharding
func (d *dispatcher) scheduleKSMCheck(config integration.Config) bool {
	if !d.ksmSharding.IsEnabled() || !d.ksmSharding.IsKSMCheck(config) {
		// Not a KSM check or sharding disabled, use normal scheduling
		return false
	}

	// KSM sharding requires advanced dispatching for deterministic runner assignment
	if !d.advancedDispatching {
		log.Warnf("KSM sharding is enabled but advanced dispatching is not. Advanced dispatching is required for KSM sharding to work correctly. Falling back to normal scheduling.")
		return false
	}

	// Check if this KSM check should be sharded
	if !d.ksmSharding.ShouldShardKSMCheck(config) {
		// KSM check but not suitable for sharding (e.g., only cluster-scoped collectors)
		log.Infof("KSM check %s not suitable for sharding (no namespace-scoped collectors), using normal scheduling", config.Digest())
		return false
	}

	// Get list of namespaces
	namespaces, err := d.getNamespaces()
	if err != nil {
		log.Warnf("Failed to get namespaces for KSM sharding: %v, falling back to normal scheduling", err)
		return false
	}

	// Check if we have any cluster check runners available
	runners := d.getAvailableRunners()
	if len(runners) == 0 {
		log.Warnf("No cluster check runners available for KSM sharding, falling back to normal scheduling")
		return false
	}

	// Create sharded configs
	shardedConfigs, clusterConfig, err := d.ksmSharding.CreateShardedKSMConfigs(config, namespaces)
	if err != nil {
		log.Warnf("Failed to create sharded KSM configs: %v, falling back to normal scheduling", err)
		return false
	}

	// Schedule cluster-wide config if present
	if clusterConfig.Name != "" {
		log.Infof("Scheduling cluster-wide KSM config for cluster-scoped collectors")
		d.add(clusterConfig)
	}

	// Schedule namespace-sharded configs using dispatcher's logic
	// Advanced dispatching will automatically distribute to least busy nodes
	totalSharded := 0
	for _, cfg := range shardedConfigs {
		// Use d.add() which uses getNodeToScheduleCheck() internally
		// This respects advanced dispatching if enabled
		if d.add(cfg) {
			totalSharded++
		}
	}

	if totalSharded > 0 {
		log.Infof("Successfully sharded KSM check into %d namespace-scoped checks distributed across runners",
			totalSharded)
		if d.advancedDispatching {
			log.Infof("Using advanced dispatching for optimal load distribution")
		}
	} else {
		log.Warnf("KSM sharding enabled but no checks were distributed - check runner availability")
	}

	return true
}

// getNamespaces retrieves all namespaces from the Kubernetes API
func (d *dispatcher) getNamespaces() ([]string, error) {
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, err
	}

	namespaceList, err := apiClient.Cl.CoreV1().Namespaces().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	namespaces := make([]string, 0, len(namespaceList.Items))
	for _, ns := range namespaceList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	return namespaces, nil
}

// getAvailableRunners returns a list of available CLC runner node names
func (d *dispatcher) getAvailableRunners() []string {
	d.store.RLock()
	defer d.store.RUnlock()

	runners := make([]string, 0, len(d.store.nodes))
	for nodeName, node := range d.store.nodes {
		// Skip regular node agents (only include CLC runners or unknown types for backward compatibility)
		// NodeType == 0 means older version that doesn't report type (likely a CLC runner)
		// NodeTypeNodeAgent (2) are explicitly node agents and should be excluded
		if node.nodetype == types.NodeTypeNodeAgent {
			continue
		}
		// All nodes in the store are healthy since expireNodes() removes expired ones
		runners = append(runners, nodeName)
	}

	return runners
}
