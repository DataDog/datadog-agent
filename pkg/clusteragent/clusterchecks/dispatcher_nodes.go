// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"time"

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
	d.updateCheckCompatibility(nodeName, node, status.CheckCompatibility)
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
func (d *dispatcher) getNodeToScheduleCheck(checkName string) (node string, anyNode bool) {
	d.store.RLock()
	defer d.store.RUnlock()

	if d.advancedDispatching.Load() {
		node = d.getRandomNode(checkName)
	} else {
		node = d.getNodeWithLessChecks(checkName)
	}
	return node, len(d.store.nodes) > 0
}

// updateCheckCompatibility records the worker's declared compatibility
// Note: NodeAgents reuse nodeName, so updating particularly important if new exclusion needed.
func (d *dispatcher) updateCheckCompatibility(nodeName string, node *nodeStore, compat *types.CheckCompatibility) {
	signature := declarationSignature(compat)
	if signature == node.signature {
		return
	}
	log.Infof("Node %s check compatibility changed: %s -> %s", nodeName, node.signature, signature)
	node.checkCompat = compat
	node.signature = signature

	node.Lock()
	defer node.Unlock()
	for digest, config := range node.digestToConfig {
		if !compat.Accepts(config.Name) {
			log.Infof("Node %s no longer accepts %s:%s, will re-dispatch it", nodeName, config.Name, digest)
			node.removeConfig(digest)
			d.moveToDangling(nodeName, digest, config)
		}
	}
}

// declarationSignature returns the canonical form of a worker's compat
// declaration, following Accepts' precedence (include first, then exclude)
func declarationSignature(compat *types.CheckCompatibility) string {
	switch {
	case compat == nil:
		return "unrestricted"
	case len(compat.Include) > 0:
		return "include=" + canonicalList(compat.Include)
	case len(compat.Exclude) > 0:
		return "exclude=" + canonicalList(compat.Exclude)
	default:
		return "unrestricted"
	}
}

func canonicalList(checks []string) string {
	return strings.Join(slices.Compact(slices.Sorted(slices.Values(checks))), ",")
}

// cohortKey returns the sorted, distinct signatures of the given nodes. The
// eligible nodes of a check are whole groups of same-signature workers, so
// this key identifies the eligible set exactly, while staying bounded by the
// number of declarations and stable across pod restarts. The store must be
// read-locked.
func (d *dispatcher) cohortKey(nodes []string) string {
	signatures := make([]string, 0, len(nodes))
	for _, name := range nodes {
		signatures = append(signatures, d.store.nodes[name].signature)
	}
	slices.Sort(signatures)
	return strings.Join(slices.Compact(signatures), " | ")
}

// eligibleNodes returns the sorted nodes accepting a check. The store must be read-locked.
func (d *dispatcher) eligibleNodes(checkName string) []string {
	var nodes []string
	for name, node := range d.store.nodes {
		if node.checkCompat.Accepts(checkName) {
			nodes = append(nodes, name)
		}
	}
	sort.Strings(nodes)
	return nodes
}

// getRandomNode must be called with the store read-locked.
func (d *dispatcher) getRandomNode(checkName string) string {
	nodes := d.eligibleNodes(checkName)
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
		if !store.checkCompat.Accepts(checkName) {
			continue
		}
		if selectedNode == "" || len(store.digestToConfig) < minNumChecks {
			selectedNode = name
			minNumChecks = len(store.digestToConfig)
		}
	}

	return selectedNode
}

// anyCompatDeclared returns whether any live worker declared a compatibility; false = legacy behavior.
func (d *dispatcher) anyCompatDeclared() bool {
	d.store.RLock()
	defer d.store.RUnlock()

	for _, node := range d.store.nodes {
		if node.checkCompat != nil {
			return true
		}
	}
	return false
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
				d.moveToDangling(name, digest, config)
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

// moveToDangling unassigns a config dispatched to nodeName and stores it as a
// dangling config, to be re-dispatched. It doesn't touch the node's own
// config map. The store must be locked.
func (d *dispatcher) moveToDangling(nodeName, digest string, config integration.Config) {
	delete(d.store.digestToNode, digest)
	log.Debugf("Adding %s:%s as a dangling Cluster Check config", config.Name, digest)
	d.store.danglingConfigs[digest] = createDanglingConfig(config)
	danglingConfigs.Inc(le.JoinLeaderValue)

	// TODO: Use partial label matching when it becomes available:
	// Replace the loop by a single function call (delete by node name).
	// Requires https://github.com/prometheus/client_golang/pull/1013
	for k, v := range d.store.idToDigest {
		if v == digest {
			configsInfo.Delete(nodeName, config.Name, string(k), le.JoinLeaderValue)
		}
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

	// Worker counts feed the utilization algorithm, also when compat forces it.
	// Computed before locking the store: it takes the store read lock itself.
	fetchWorkers := d.useUtilizationRebalance()

	d.store.Lock()
	defer d.store.Unlock()
	for name, node := range d.store.nodes {
		node.RLock()
		ip := node.clientIP
		node.RUnlock()

		if fetchWorkers {
			workers, err := d.clcRunnersClient.GetRunnerWorkers(ip)
			if err != nil {
				// This can happen in old versions of the runners that do not expose this information.
				log.Debugf("Cannot get number of workers for node %s with IP %s. Assuming default. Error: %v", name, node.clientIP, err)
				node.workers = constants.DefaultNumWorkers
			} else {
				node.workers = workers.Count
			}
		}

		stats, err := d.clcRunnersClient.GetRunnerStats(ip)
		if err != nil {
			log.Debugf("Cannot get CLC Runner stats with IP %s on node %s: %v", node.clientIP, name, err)
			statsCollectionFails.Inc(name, le.JoinLeaderValue)
			continue
		}
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
