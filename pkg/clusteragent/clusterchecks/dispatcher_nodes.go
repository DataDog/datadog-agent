// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"bytes"
	"cmp"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultBusynessValue int = -1
)

// getClusterCheckConfigs returns configurations dispatched to a given node
func (d *dispatcher) getClusterCheckConfigs(nodeName string) ([]integration.Config, int64, error) {
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

// processNodeStatus keeps the node's status in the store, and returns true
// if the last configuration change matches the one sent by the node agent.
func (d *dispatcher) processNodeStatus(nodeName, clientIP string, status types.NodeStatus) bool {
	var warmingUp bool

	d.store.Lock()
	if !d.store.active {
		warmingUp = true
	}
	node := d.store.getOrCreateNodeStore(nodeName, clientIP)
	d.updateGroup(nodeName, node, status.Group)
	d.store.Unlock()

	node.Lock()
	defer node.Unlock()
	node.heartbeat = timestampNow()
	node.nodetype = status.NodeType

	// Check if we need to disable advanced dispatching when node agents join
	if d.advancedDispatching.Load() && status.NodeType == types.NodeTypeNodeAgent {
		d.disableAdvancedDispatching()
	}

	// When we receive ExtraHeartbeatLastChangeValue, we only update heartbeat
	if status.LastChange == types.ExtraHeartbeatLastChangeValue {
		return true
	}

	if node.lastConfigChange == status.LastChange {
		// Node-agent is up to date
		return true
	}
	if warmingUp {
		// During the initial warmup phase, we are counting active nodes
		// without dispatching configurations.
		// We tell node-agents they are up to date to keep their cached
		// configurations running while we finish the warmup phase.
		return true
	}

	// Node-agent needs to pull updated configs
	log.Infof("Node %s needs to poll config, cluster config version: %d, node config version: %d", nodeName, node.lastConfigChange, status.LastChange)
	return false
}

// getNodeToScheduleCheck returns the node where a new check should be scheduled, and whether any node is registered.
//
// Advanced dispatching relies on the check stats fetched from the cluster check
// runners API to distribute the checks. The stats are only updated when the
// checks are rebalanced, they are not updated every time a check is scheduled.
// That's why it's not a good idea to pick the least busy node. Rebalance
// happens every few minutes, so all the checks added during that time would get
// scheduled to the same node. It's a better solution to pick a random node and
// rely on rebalancing to distribute when needed.
//
// On the other hand, when advanced dispatching is not used, we can pick the
// node with fewer checks. It's because the number of checks is kept up to date.
//
// The store must be read-locked.
func (d *dispatcher) getNodeToScheduleCheck(checkName string) (node string, anyNode bool) {
	if d.advancedDispatching.Load() {
		node = d.getRandomNode(checkName)
	} else {
		node = d.getNodeWithLessChecks(checkName)
	}
	return node, len(d.store.nodes) > 0
}

// updateGroup records the worker's runner group. The store must be locked.
func (d *dispatcher) updateGroup(nodeName string, node *nodeStore, group string) {
	if group == node.group {
		return
	}
	if _, known := d.runnerGroups[group]; group != "" && !known {
		log.Warnf("Node %s declares unknown cluster checks runner group %q: it won't receive any cluster check", nodeName, group)
	} else {
		log.Infof("Node %s is in cluster checks runner group %q", nodeName, cmp.Or(group, "general"))
	}
	node.group = group
}

// accepts reports whether a worker may run a check: a check claimed by a
// runner group only runs in that group, any other check only on general
// workers (no group). A worker in a group the Cluster Agent doesn't know runs
// nothing.
func (d *dispatcher) accepts(node *nodeStore, checkName string) bool {
	return node.group == d.checkGroup[checkName]
}

// groupNodes returns the sorted nodes of a runner group ("" for general
// workers). The store must be read-locked.
func (d *dispatcher) groupNodes(group string) []string {
	var nodes []string
	for name, node := range d.store.nodes {
		if node.group == group {
			nodes = append(nodes, name)
		}
	}
	sort.Strings(nodes)
	return nodes
}

// getRandomNode must be called with the store read-locked.
func (d *dispatcher) getRandomNode(checkName string) string {
	nodes := d.groupNodes(d.checkGroup[checkName])
	if len(nodes) == 0 {
		return ""
	}

	return nodes[rand.Intn(len(nodes))]
}

// getNodeWithLessChecks must be called with the store read-locked.
func (d *dispatcher) getNodeWithLessChecks(checkName string) string {
	var selectedNode string
	minNumChecks := 0

	for name, store := range d.store.nodes {
		if !d.accepts(store, checkName) {
			continue
		}
		if selectedNode == "" || len(store.digestToConfig) < minNumChecks {
			selectedNode = name
			minNumChecks = len(store.digestToConfig)
		}
	}

	return selectedNode
}

// expireNodes iterates over nodes and removes the ones that have not
// reported for more than the expiration duration. The configurations
// dispatched to these nodes will be moved to the danglingConfigs map.
func (d *dispatcher) expireNodes() {
	cutoffTimestamp := timestampNow() - d.nodeExpirationSeconds

	d.store.Lock()
	defer d.store.Unlock()

	initialNodeCount := len(d.store.nodes)

	for name, node := range d.store.nodes {
		node.RLock()
		if node.heartbeat < cutoffTimestamp {
			if name != "" {
				// Don't report on the dummy "" host for unscheduled configs
				log.Infof("Expiring out node %s, last status report %d seconds ago", name, timestampNow()-node.heartbeat)
			}
			for digest, config := range node.digestToConfig {
				delete(d.store.digestToNode, digest)
				log.Debugf("Adding %s:%s as a dangling Cluster Check config", config.Name, digest)
				d.store.danglingConfigs[digest] = createDanglingConfig(config)
				danglingConfigs.Inc(le.JoinLeaderValue)

				// TODO: Use partial label matching when it becomes available:
				// Replace the loop by a single function call (delete by node name).
				// Requires https://github.com/prometheus/client_golang/pull/1013
				for k, v := range d.store.idToDigest {
					if v == digest {
						configsInfo.Delete(name, config.Name, string(k), le.JoinLeaderValue)
					}
				}
			}
			delete(d.store.nodes, name)

			// Remove metrics linked to this node
			nodeAgents.Dec(le.JoinLeaderValue)
			dispatchedConfigs.Delete(name, le.JoinLeaderValue)
			statsCollectionFails.Delete(name, le.JoinLeaderValue)
			busyness.Delete(name, le.JoinLeaderValue)
		}
		node.RUnlock()
	}

	if initialNodeCount != 0 && len(d.store.nodes) == 0 {
		log.Warn("No nodes reporting, cluster checks will not run")
	}
}

// updateRunnersStats collects stats from the registred
// Cluster Level Check runners and updates the stats cache
func (d *dispatcher) updateRunnersStats() {
	if d.clcRunnersClient == nil {
		log.Debug("Cluster Level Check runner client was not correctly initialised")
		return
	}

	start := time.Now()
	defer func() {
		updateStatsDuration.Set(time.Since(start).Seconds(), le.JoinLeaderValue)
	}()

	type runnerResult struct {
		node     *nodeStore
		ip       string
		workers  int
		stats    types.CLCRunnersStats
		statsErr error
	}

	// Each runner call can take up to the client timeout, so the runners are
	// queried without holding the store lock.
	d.store.RLock()
	results := make(map[string]*runnerResult, len(d.store.nodes))
	for name, node := range d.store.nodes {
		node.RLock()
		results[name] = &runnerResult{node: node, ip: node.clientIP}
		node.RUnlock()
	}
	d.store.RUnlock()

	for name, r := range results {
		workers, err := d.clcRunnersClient.GetRunnerWorkers(r.ip)
		if err != nil {
			// This can happen in old versions of the runners that do not expose this information.
			log.Debugf("Cannot get number of workers for node %s with IP %s. Assuming default. Error: %v", name, r.ip, err)
			r.workers = constants.DefaultNumWorkers
		} else {
			r.workers = workers.Count
		}
		r.stats, r.statsErr = d.clcRunnersClient.GetRunnerStats(r.ip)
	}

	d.store.Lock()
	defer d.store.Unlock()
	for name, r := range results {
		node := r.node
		if d.store.nodes[name] != node {
			// Expired while we were querying it: don't bring its metrics back.
			continue
		}
		if node.clientIP != r.ip {
			// Re-registered from another IP: these results describe the old one.
			continue
		}
		node.workers = r.workers

		if r.statsErr != nil {
			log.Debugf("Cannot get CLC Runner stats with IP %s on node %s: %v", r.ip, name, r.statsErr)
			statsCollectionFails.Inc(name, le.JoinLeaderValue)
			continue
		}
		stats := r.stats
		node.Lock()
		for idStr, checkStats := range stats {
			id := checkid.ID(idStr)

			// Stats contain info about all the running checks on a node
			// Node checks must be filtered from Cluster Checks
			// so they can be included in calculating node Agent busyness and excluded from rebalancing decisions.
			if _, found := d.store.idToDigest[id]; found {
				// Cluster check detected (exists in the Cluster Agent checks store)
				log.Tracef("Check %s running on node %s is a cluster check", id, node.name)
				checkStats.IsClusterCheck = true
				stats[idStr] = checkStats
			}
		}
		node.clcRunnerStats = stats
		log.Tracef("Updated CLC Runner stats on node: %s, node IP: %s, stats: %v", name, node.clientIP, stats)
		node.busyness = calculateBusyness(stats)
		log.Debugf("Updated busyness on node: %s, node IP: %s, busyness value: %d", name, node.clientIP, node.busyness)
		busyness.Set(float64(node.busyness), node.name, le.JoinLeaderValue)
		node.Unlock()
	}
}

const (
	// helmReleasesGroup is the colocation group of the Helm check and of the
	// orchestrator instance that collects Helm releases. Both watch the Helm release
	// ConfigMaps, so on the same runner they share that informer.
	helmReleasesGroup = "helm-releases"

	helmCheckName         = "helm"
	orchestratorCheckName = "orchestrator"

	// The Helm releases ConfigMap collector of the orchestrator check is
	// enabled with this setting, and runs in the instances that collect ConfigMaps.
	helmReleasesConfigMapsSetting = "orchestrator_explorer.helm_releases.configmaps.enabled"
	configMapsCollector           = "configmaps"
)

type orchestratorInstance struct {
	Collectors []string `yaml:"collectors"`
}

// colocationGroup returns the colocation group of a config, or "" if it has
// none. Configs of the same group are dispatched to the same runner.
func (d *dispatcher) colocationGroup(config integration.Config) string {
	if !d.helmReleasesColocation {
		return ""
	}
	switch config.Name {
	case helmCheckName:
		return helmReleasesGroup
	case orchestratorCheckName:
		if collectsConfigMaps(config) {
			return helmReleasesGroup
		}
	}
	return ""
}

// collectsConfigMaps reports whether an orchestrator config has an instance
// that runs the ConfigMap collector, and so the Helm releases collector.
func collectsConfigMaps(config integration.Config) bool {
	for _, instance := range config.Instances {
		// Avoid parsing the YAML of the instances that can't match.
		if !bytes.Contains(instance, []byte(configMapsCollector)) {
			continue
		}

		var parsed orchestratorInstance
		if err := yaml.Unmarshal(instance, &parsed); err != nil {
			log.Debugf("Cannot read the collectors of %s: %v", config.Name, err)
			continue
		}
		for _, collector := range parsed.Collectors {
			// Collectors are named "<resource>" or "<group>/<version>/<resource>",
			// split like the orchestrator check's SplitCollectorName.
			groupVersion, resource := "", collector
			if idx := strings.LastIndex(collector, "/"); idx != -1 {
				groupVersion, resource = collector[:idx], collector[idx+1:]
			}
			if resource == configMapsCollector && (groupVersion == "" || groupVersion == "v1") {
				return true
			}
		}
	}
	return false
}

// colocatedNode returns a node already running a config of the same
// colocation group as config that may run config too, or "" when there is none.
//
// The store must be read-locked.
func (d *dispatcher) colocatedNode(config integration.Config) string {
	group := d.colocationGroup(config)
	if group == "" {
		return ""
	}

	digest := config.Digest()
	for otherDigest, nodeName := range d.store.digestToNode {
		if otherDigest == digest {
			continue
		}
		other, found := d.store.digestToConfig[otherDigest]
		if !found || d.colocationGroup(other) != group {
			continue
		}

		node, found := d.store.nodes[nodeName]
		if !found {
			continue
		}
		if !d.accepts(node, config.Name) {
			log.Debugf("Cannot colocate %s with %s on node %s for group %q: the node doesn't accept it", config.Name, other.Name, nodeName, group)
			continue
		}
		return nodeName
	}
	return ""
}

// hasColocatedPeer reports whether another config of the colocation group of
// config runs on nodeName, so config must stay there. The store must be read-locked.
func (d *dispatcher) hasColocatedPeer(digest string, config integration.Config, nodeName string) bool {
	group := d.colocationGroup(config)
	if group == "" {
		return false
	}
	for otherDigest, otherNode := range d.store.digestToNode {
		if otherDigest == digest || otherNode != nodeName {
			continue
		}
		if other, found := d.store.digestToConfig[otherDigest]; found && d.colocationGroup(other) == group {
			return true
		}
	}
	return false
}

// getNodeToSchedule returns the node where a new config should be scheduled:
// the node of its colocation group when it has one, otherwise the node chosen
// for its check. The store must be read-locked.
func (d *dispatcher) getNodeToSchedule(config integration.Config) (node string, anyNode bool) {
	if node = d.colocatedNode(config); node != "" {
		return node, true
	}
	return d.getNodeToScheduleCheck(config.Name)
}
