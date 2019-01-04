// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func generateIntegration(name string) integration.Config {
	return integration.Config{
		Name:         name,
		ClusterCheck: true,
	}
}

func extractCheckNames(configs []integration.Config) []string {
	var names []string
	for _, c := range configs {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}

func TestScheduleUnschedule(t *testing.T) {
	dispatcher := newDispatcher()
	stored, err := dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 0)

	config1 := integration.Config{
		Name:         "non-cluster-check",
		ClusterCheck: false,
	}
	config2 := integration.Config{
		Name:         "cluster-check",
		ClusterCheck: true,
	}

	dispatcher.Schedule([]integration.Config{config1, config2})
	stored, err = dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 1)
	assert.Contains(t, stored, config2)
	assert.Equal(t, 1, len(dispatcher.store.danglingConfigs))

	dispatcher.Unschedule([]integration.Config{config1, config2})
	stored, err = dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 0)
	assert.Equal(t, 0, len(dispatcher.store.danglingConfigs))

	requireNotLocked(t, dispatcher.store)
}

func TestScheduleReschedule(t *testing.T) {
	dispatcher := newDispatcher()
	config := generateIntegration("cluster-check")

	// Register to node1
	dispatcher.addConfig(config, "node1")
	configs1, _, err := dispatcher.getNodeConfigs("node1")
	assert.NoError(t, err)
	assert.Len(t, configs1, 1)
	assert.Contains(t, configs1, config)

	// Move to node2
	dispatcher.addConfig(config, "node2")
	configs2, _, err := dispatcher.getNodeConfigs("node2")
	assert.NoError(t, err)
	assert.Len(t, configs2, 1)
	assert.Contains(t, configs2, config)

	// De-registered from previous node
	configs1, _, err = dispatcher.getNodeConfigs("node1")
	assert.NoError(t, err)
	assert.Len(t, configs1, 0)

	// Only registered once in global list
	stored, err := dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 1)
	assert.Contains(t, stored, config)

	requireNotLocked(t, dispatcher.store)
}

func TestProcessNodeStatus(t *testing.T) {
	dispatcher := newDispatcher()
	status1 := types.NodeStatus{LastChange: 10}

	// Warmup phase, upToDate is unconditionally true
	upToDate, err := dispatcher.processNodeStatus("node1", status1)
	assert.NoError(t, err)
	assert.True(t, upToDate)
	node1, found := dispatcher.store.getNodeStore("node1")
	assert.True(t, found)
	assert.Equal(t, status1, node1.lastStatus)
	assert.True(t, timestampNow() >= node1.heartbeat)
	assert.True(t, timestampNow() <= node1.heartbeat+1)

	// Warmup is finished, timestamps differ
	dispatcher.store.active = true
	upToDate, err = dispatcher.processNodeStatus("node1", status1)
	assert.NoError(t, err)
	assert.False(t, upToDate)

	// Give changes
	node1.lastConfigChange = timestampNow()
	node1.heartbeat = node1.heartbeat - 50
	status2 := types.NodeStatus{LastChange: node1.lastConfigChange - 2}
	upToDate, err = dispatcher.processNodeStatus("node1", status2)
	assert.NoError(t, err)
	assert.False(t, upToDate)
	assert.True(t, timestampNow() >= node1.heartbeat)
	assert.True(t, timestampNow() <= node1.heartbeat+1)

	// No change
	status3 := types.NodeStatus{LastChange: node1.lastConfigChange}
	upToDate, err = dispatcher.processNodeStatus("node1", status3)
	assert.NoError(t, err)
	assert.True(t, upToDate)

	requireNotLocked(t, dispatcher.store)
}

func TestGetLeastBusyNode(t *testing.T) {
	dispatcher := newDispatcher()

	// No node registered -> empty string
	assert.Equal(t, "", dispatcher.getLeastBusyNode())

	// 1 config on node1, 2 on node2
	dispatcher.addConfig(generateIntegration("A"), "node1")
	dispatcher.addConfig(generateIntegration("B"), "node2")
	dispatcher.addConfig(generateIntegration("C"), "node2")
	assert.Equal(t, "node1", dispatcher.getLeastBusyNode())

	// 3 configs on node1, 2 on node2
	dispatcher.addConfig(generateIntegration("D"), "node1")
	dispatcher.addConfig(generateIntegration("E"), "node1")
	assert.Equal(t, "node2", dispatcher.getLeastBusyNode())

	// Add an empty node3
	dispatcher.processNodeStatus("node3", types.NodeStatus{})
	assert.Equal(t, "node3", dispatcher.getLeastBusyNode())

	requireNotLocked(t, dispatcher.store)
}

func TestExpireNodes(t *testing.T) {
	dispatcher := newDispatcher()

	// Node with no status (bug ?), handled by expiration
	dispatcher.addConfig(generateIntegration("one"), "node1")
	assert.Equal(t, 1, len(dispatcher.store.nodes))
	dispatcher.expireNodes()
	assert.Equal(t, 0, len(dispatcher.store.nodes))
	assert.Equal(t, 1, len(dispatcher.store.danglingConfigs))

	// Nodes with valid statuses
	dispatcher.store.clearDangling()
	dispatcher.addConfig(generateIntegration("A"), "nodeA")
	dispatcher.addConfig(generateIntegration("B1"), "nodeB")
	dispatcher.addConfig(generateIntegration("B2"), "nodeB")
	dispatcher.processNodeStatus("nodeA", types.NodeStatus{})
	dispatcher.processNodeStatus("nodeB", types.NodeStatus{})
	assert.Equal(t, 2, len(dispatcher.store.nodes))

	// Fake the status report timestamps, nodeB should expire
	dispatcher.store.nodes["nodeA"].heartbeat = timestampNow() - 25
	dispatcher.store.nodes["nodeB"].heartbeat = timestampNow() - 35

	assert.Equal(t, 0, len(dispatcher.store.danglingConfigs))
	dispatcher.expireNodes()
	assert.Equal(t, 1, len(dispatcher.store.nodes))
	assert.Equal(t, 2, len(dispatcher.store.danglingConfigs))

	requireNotLocked(t, dispatcher.store)
}

func TestDispatchFourConfigsTwoNodes(t *testing.T) {
	dispatcher := newDispatcher()

	// Register two nodes
	dispatcher.processNodeStatus("nodeA", types.NodeStatus{})
	dispatcher.processNodeStatus("nodeB", types.NodeStatus{})
	assert.Equal(t, 2, len(dispatcher.store.nodes))

	dispatcher.Schedule([]integration.Config{
		generateIntegration("A"),
		generateIntegration("B"),
		generateIntegration("C"),
		generateIntegration("D"),
	})

	allConfigs, err := dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(allConfigs))
	assert.Equal(t, []string{"A", "B", "C", "D"}, extractCheckNames(allConfigs))

	configsA, _, err := dispatcher.getNodeConfigs("nodeA")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(configsA))

	configsB, _, err := dispatcher.getNodeConfigs("nodeB")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(configsB))

	// Make sure all checks are on a node
	names := extractCheckNames(configsA)
	names = append(names, extractCheckNames(configsB)...)
	sort.Strings(names)
	assert.Equal(t, []string{"A", "B", "C", "D"}, names)

	requireNotLocked(t, dispatcher.store)
}

func TestDanglingConfig(t *testing.T) {
	dispatcher := newDispatcher()
	config := integration.Config{
		Name:         "cluster-check",
		ClusterCheck: true,
	}

	assert.False(t, dispatcher.shouldDispatchDanling())

	// No node is available, config will be dispatched to the dummy "" node
	dispatcher.Schedule([]integration.Config{config})
	assert.Equal(t, 0, len(dispatcher.store.digestToNode))
	assert.Equal(t, 1, len(dispatcher.store.danglingConfigs))

	// shouldDispatchDanling is still false because no node is available
	assert.False(t, dispatcher.shouldDispatchDanling())

	// register a node, shouldDispatchDanling will become true
	dispatcher.processNodeStatus("nodeA", types.NodeStatus{})
	assert.True(t, dispatcher.shouldDispatchDanling())

	// get the danglings and make sure they are removed from the store
	configs := dispatcher.retrieveAndClearDangling()
	assert.Equal(t, []integration.Config{config}, configs)
	assert.Equal(t, 0, len(dispatcher.store.danglingConfigs))
}

func TestReset(t *testing.T) {
	dispatcher := newDispatcher()
	config := generateIntegration("cluster-check")

	// Register to node1
	dispatcher.addConfig(config, "node1")
	configs1, _, err := dispatcher.getNodeConfigs("node1")
	assert.NoError(t, err)
	assert.Len(t, configs1, 1)
	assert.Contains(t, configs1, config)

	// Reset
	dispatcher.reset()
	stored, err := dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 0)
	_, _, err = dispatcher.getNodeConfigs("node1")
	assert.EqualError(t, err, "node node1 is unknown")

	requireNotLocked(t, dispatcher.store)
}
