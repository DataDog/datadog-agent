// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"maps"
	"slices"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getAllConfigs returns all configurations known to the store, for reporting
func (d *dispatcher) getAllConfigs() ([]integration.Config, error) {
	d.store.RLock()
	defer d.store.RUnlock()

	return makeConfigArray(d.store.digestToConfig), nil
}

// nodeConfigsSnapshot holds a node's configs and precomputed instance IDs,
// copied out of the store so scrubbing can happen without holding d.store's lock.
type nodeConfigsSnapshot struct {
	name        string
	rawConfigs  []integration.Config
	instanceIDs [][]string
	stats       types.CLCRunnersStats
	nodeType    types.NodeType
	group       string
	workers     int
	heartbeat   int64
}

func (d *dispatcher) getState(scrub bool) (types.StateResponse, error) {
	advanced := d.advancedDispatching.Load()
	var utilization map[string]float64
	if advanced {
		utilization = d.runnerUtilization()
	}

	// Copy data out while holding the lock, then scrub after releasing it.
	// Scrubbing is an O(instances) operation and would otherwise starve
	// writers like expireNodes and updateLeaderIP.
	d.store.RLock()
	danglingConf := makeConfigArrayFromDangling(d.store.danglingConfigs)
	warmup := !d.store.active
	dispatching := d.dispatchingState(advanced)
	var unassignedReasons map[string]string
	for _, config := range danglingConf {
		if unassignedReasons == nil {
			unassignedReasons = make(map[string]string)
		}
		unassignedReasons[config.Name] = d.unassignedReason(config.Name)
	}

	nodeSnapshots := make([]nodeConfigsSnapshot, 0, len(d.store.nodes))
	for _, node := range d.store.nodes {
		rawConfigs := makeConfigArray(node.digestToConfig)

		// Copy runner stats for this node. Stats are populated by the periodic
		// rebalance cycle when advanced dispatching is enabled. Without advanced
		// dispatching or when the runner is unreachable, stats will be empty.
		// Take node read lock to avoid concurrent map read/write with updateRunnersStats.
		var stats types.CLCRunnersStats
		node.RLock()
		if len(node.clcRunnerStats) > 0 {
			stats = make(types.CLCRunnersStats, len(node.clcRunnerStats))
			for k, v := range node.clcRunnerStats {
				stats[k] = v
			}
		}
		nodeType, workers, heartbeat := node.nodetype, node.workers, node.heartbeat
		node.RUnlock()

		nodeSnapshots = append(nodeSnapshots, nodeConfigsSnapshot{
			name:       node.name,
			rawConfigs: rawConfigs,
			stats:      stats,
			nodeType:   nodeType,
			group:      node.group,
			workers:    workers,
			heartbeat:  heartbeat,
		})
	}
	d.store.RUnlock()

	// IDs must be computed before scrubbing, since scrubbing changes the
	// digest. BuildID unmarshals YAML, so do this after releasing d.store.
	for i, snap := range nodeSnapshots {
		instanceIDs := make([][]string, len(snap.rawConfigs))
		for j, config := range snap.rawConfigs {
			digest := config.FastDigest()
			ids := make([]string, 0, len(config.Instances))
			for _, inst := range config.Instances {
				ids = append(ids, string(checkid.BuildID(config.Name, digest, inst, config.InitConfig)))
			}
			instanceIDs[j] = ids
		}
		nodeSnapshots[i].instanceIDs = instanceIDs
	}

	if scrub {
		scrubbedConf := make([]integration.Config, 0, len(danglingConf))
		for _, config := range danglingConf {
			scrubbedConf = append(scrubbedConf, integration.ScrubCheckConfig(config, log.NewWrapper(1)))
		}
		danglingConf = scrubbedConf
	}

	response := types.StateResponse{
		Warmup:            warmup,
		Dangling:          danglingConf,
		Dispatching:       dispatching,
		UnassignedReasons: unassignedReasons,
	}

	for _, snap := range nodeSnapshots {
		configsWithIDs := make([]types.ConfigWithInstanceIDs, 0, len(snap.rawConfigs))
		for i, config := range snap.rawConfigs {
			if scrub {
				config = integration.ScrubCheckConfig(config, log.NewWrapper(1))
			}

			configsWithIDs = append(configsWithIDs, types.ConfigWithInstanceIDs{
				InstanceIDs: snap.instanceIDs[i],
				Config:      config,
			})
		}

		nodeResponse := types.StateNodeResponse{
			Name:          snap.name,
			Configs:       configsWithIDs,
			Stats:         snap.stats,
			NodeType:      snap.nodeType,
			Group:         snap.group,
			Workers:       snap.workers,
			LastHeartbeat: snap.heartbeat,
		}
		if u, found := utilization[snap.name]; found {
			nodeResponse.Utilization = &u
		}
		response.Nodes = append(response.Nodes, nodeResponse)
	}

	return response, nil
}

// runnerUtilization returns the share of workers used by cluster checks on
// each runner that reports its workers.
func (d *dispatcher) runnerUtilization() map[string]float64 {
	utilization := make(map[string]float64)
	for name, runner := range d.currentDistribution().Runners {
		if runner.Workers > 0 {
			utilization[name] = runner.utilization()
		}
	}
	return utilization
}

// dispatchingState describes the dispatching mode and settings. The store
// must be read-locked.
func (d *dispatcher) dispatchingState(advanced bool) *types.DispatchingState {
	state := &types.DispatchingState{
		AdvancedDispatching:           advanced,
		NodeExpirationSeconds:         d.nodeExpirationSeconds,
		ExcludedChecks:                slices.Sorted(maps.Keys(d.excludedChecks)),
		ExcludedChecksFromDispatching: slices.Sorted(maps.Keys(d.excludedChecksFromDispatching)),
	}
	if advanced {
		state.RebalancePeriod = d.rebalancingPeriod.String()
	} else {
		state.AdvancedDispatchingReason = d.advancedDispatchingDisabledReason()
	}
	for _, group := range slices.Sorted(maps.Keys(d.runnerGroups)) {
		groupState := types.RunnerGroupState{Name: group, Checks: []string{}}
		for check, owner := range d.checkGroup {
			if owner == group {
				groupState.Checks = append(groupState.Checks, check)
			}
		}
		slices.Sort(groupState.Checks)
		state.RunnerGroups = append(state.RunnerGroups, groupState)
	}
	return state
}

// advancedDispatchingDisabledReason explains why advanced dispatching is
// disabled. The store must be read-locked.
func (d *dispatcher) advancedDispatchingDisabledReason() string {
	if !pkgconfigsetup.Datadog().GetBool("cluster_checks.advanced_dispatching_enabled") {
		return "disabled by cluster_checks.advanced_dispatching_enabled"
	}
	for _, node := range d.store.nodes {
		node.RLock()
		nodeType := node.nodetype
		node.RUnlock()
		if nodeType == types.NodeTypeNodeAgent {
			return "node agents are reporting"
		}
	}
	return "the cluster check runners client is unavailable"
}

// unassignedReason explains why a check's dangling configs aren't dispatched.
// The store must be read-locked.
func (d *dispatcher) unassignedReason(checkName string) string {
	if len(d.store.nodes) == 0 {
		return "no agent is reporting"
	}
	group := d.checkGroup[checkName]
	if len(d.groupNodes(group)) > 0 {
		return "waiting for the next dispatch attempt"
	}
	if group == "" {
		return "no general worker is reporting: every worker is in a runner group"
	}
	return fmt.Sprintf("runner group %q has no live runner", group)
}

func (d *dispatcher) addConfig(config integration.Config, targetNodeName string) bool {
	d.store.Lock()
	defer d.store.Unlock()

	// Register config
	digest := config.Digest()
	fastDigest := config.FastDigest()
	d.store.digestToConfig[digest] = config
	for _, instance := range config.Instances {
		checkID := checkid.BuildID(config.Name, fastDigest, instance, config.InitConfig)
		d.store.idToDigest[checkID] = digest
		if targetNodeName != "" {
			configsInfo.Set(1.0, targetNodeName, config.Name, string(checkID), le.JoinLeaderValue)
		}
	}

	// No target node specified: store in danglingConfigs
	if targetNodeName == "" {
		// Only update if it's a new dangling config
		if _, found := d.store.danglingConfigs[digest]; !found {
			danglingConfigs.Inc(le.JoinLeaderValue)
			d.store.danglingConfigs[digest] = createDanglingConfig(config)
		}
		return false
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

	return true
}

func (d *dispatcher) removeConfig(digest string) {
	d.store.Lock()
	defer d.store.Unlock()

	node, found := d.store.getNodeStore(d.store.digestToNode[digest])

	checkName := d.store.digestToConfig[digest].Name

	delete(d.store.digestToNode, digest)
	delete(d.store.digestToConfig, digest)
	d.deleteDangling([]string{digest})

	// This is a list because each instance in a config has its own check ID and
	// all of them need to be deleted.
	var checkIDsToRemove []checkid.ID
	for checkID, checkDigest := range d.store.idToDigest {
		if checkDigest == digest {
			checkIDsToRemove = append(checkIDsToRemove, checkID)
			delete(d.store.idToDigest, checkID)
		}
	}

	if !found { // Dangling config. Not assigned to any node.
		danglingConfigs.Dec(le.JoinLeaderValue)
		return
	}

	// Remove from node configs if assigned
	node.Lock()
	nodeName := node.name
	node.removeConfig(digest)
	node.Unlock()

	for _, checkID := range checkIDsToRemove {
		configsInfo.Delete(nodeName, checkName, string(checkID), le.JoinLeaderValue)
	}
}

// shouldDispatchDangling returns true if there are dangling configs
// and node registered, available for dispatching.
func (d *dispatcher) shouldDispatchDangling() bool {
	d.store.RLock()
	defer d.store.RUnlock()

	return len(d.store.danglingConfigs) > 0 && len(d.store.nodes) > 0
}

// retrieveDangling extracts dangling configs from the store
func (d *dispatcher) retrieveDangling() []integration.Config {
	d.store.RLock()
	defer d.store.RUnlock()

	configs := makeConfigArrayFromDangling(d.store.danglingConfigs)
	return configs
}

// deleteDangling clears the dangling configs from the store
func (d *dispatcher) deleteDangling(ids []string) {
	for _, id := range ids {
		if c, found := d.store.danglingConfigs[id]; found {
			delete(d.store.danglingConfigs, id)
			danglingConfigs.Dec(le.JoinLeaderValue)
			if c.unscheduledCheck {
				unscheduledCheck.Dec(le.JoinLeaderValue, c.config.Name, c.config.Source)
			}
		}
	}
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
