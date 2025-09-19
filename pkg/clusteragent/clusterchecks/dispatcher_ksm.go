// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	// Check if this config was already sharded (to avoid re-sharding on every Schedule call)
	if d.isAlreadySharded(config) {
		log.Debugf("KSM config %s already sharded, skipping re-sharding", config.Digest())
		return false // Let normal scheduling handle the already-sharded configs
	}

	// Get list of namespaces (if available)
	// If workloadmeta is not ready yet, we'll fall back to normal scheduling
	namespaces, err := d.getNamespaces()
	if err != nil {
		log.Infof("Workloadmeta not ready for KSM sharding: %v, falling back to normal scheduling", err)
		return false
	}

	// If we have no namespaces yet, fall back to normal scheduling
	if len(namespaces) == 0 {
		log.Debugf("No namespaces found for KSM sharding, falling back to normal scheduling")
		return false
	}

	// Check if we have any cluster check runners available
	runners := d.getAvailableRunners()
	if len(runners) == 0 {
		log.Warnf("No cluster check runners available for KSM sharding, falling back to normal scheduling")
		return false
	}

	// Create sharded configs
	// If namespaces is empty, this will create fixed bucket configs
	log.Infof("Creating sharded KSM configs for %d namespaces across %d CLC runners",
		len(namespaces), len(runners))
	shardedConfigs, clusterConfig, err := d.ksmSharding.CreateShardedKSMConfigs(config, namespaces)
	if err != nil {
		log.Warnf("Failed to create sharded KSM configs: %v, falling back to normal scheduling", err)
		return false
	}

	log.Infof("Created %d sharded configs from bucketing", len(shardedConfigs))

	// Schedule cluster-wide config if present
	if clusterConfig.Name != "" {
		log.Infof("Scheduling cluster-wide KSM config for cluster-scoped collectors")
		d.add(clusterConfig)
	}

	// Schedule namespace-sharded configs using dispatcher's logic
	// Advanced dispatching will distribute based on its algorithm
	totalSharded := 0
	assignmentMap := make(map[string]int) // Track which runner gets how many configs

	for i, cfg := range shardedConfigs {
		// Get the target node before adding
		targetNode := d.getNodeToScheduleCheck()
		log.Infof("Assigning sharded KSM config %d/%d (digest: %s) to node: %s",
			i+1, len(shardedConfigs), cfg.Digest(), targetNode)

		// Use d.add() which respects advanced dispatching settings
		if d.add(cfg) {
			totalSharded++
			if targetNode != "" {
				assignmentMap[targetNode]++
			}
		}
	}

	if totalSharded > 0 {
		log.Infof("Successfully sharded KSM check into %d namespace-scoped checks", totalSharded)

		// Log distribution details
		log.Infof("Distribution across %d runners:", len(assignmentMap))
		for runner, count := range assignmentMap {
			log.Infof("  Runner %s: %d configs", runner, count)
		}

		if d.advancedDispatching {
			log.Infof("Using advanced dispatching (will rebalance periodically)")
		}
	} else {
		log.Warnf("KSM sharding enabled but no checks were distributed - check runner availability")
	}

	// Store that we've sharded this config to avoid re-sharding
	d.markAsSharded(config)

	return true
}

// isAlreadySharded checks if a KSM config was already sharded
func (d *dispatcher) isAlreadySharded(config integration.Config) bool {
	if d.ksmShardedConfig.Name != "" && d.ksmShardedConfig.Digest() == config.Digest() {
		return true
	}
	return false
}

// markAsSharded marks a KSM config as having been sharded
func (d *dispatcher) markAsSharded(config integration.Config) {
	d.ksmShardedConfig = config
}

// getNamespaces retrieves all namespaces from workloadmeta
func (d *dispatcher) getNamespaces() ([]string, error) {
	if d.wmeta == nil {
		return nil, fmt.Errorf("workloadmeta is not available")
	}

	// Create a filter for namespace resources
	namespaceFilter := func(metadata *workloadmeta.KubernetesMetadata) bool {
		return metadata.GVR != nil && metadata.GVR.Group == "" && metadata.GVR.Resource == "namespaces"
	}

	// List all Kubernetes namespaces from workloadmeta
	namespaceMetadata := d.wmeta.ListKubernetesMetadata(namespaceFilter)

	namespaces := make([]string, 0, len(namespaceMetadata))
	for _, metadata := range namespaceMetadata {
		// The namespace name is in the Name field of the metadata
		// For namespace resources, the EntityID.ID is in format: /namespaces//{namespaceName}
		namespaces = append(namespaces, metadata.Name)
	}

	if len(namespaces) == 0 {
		log.Warnf("No namespaces found in workloadmeta, KSM sharding may not work correctly")
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
