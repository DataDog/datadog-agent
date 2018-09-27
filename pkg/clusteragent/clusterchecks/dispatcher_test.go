// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

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

	dispatcher.Unschedule([]integration.Config{config1, config2})
	stored, err = dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 0)
	requireNotLocked(t, dispatcher.store)
}

func TestScheduleReschedule(t *testing.T) {
	dispatcher := newDispatcher()

	config := integration.Config{
		Name:         "cluster-check",
		ClusterCheck: true,
	}

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

	status1 := types.NodeStatus{LastChange: 0}
	//status2 := types.NodeStatus{LastChange: 1000}

	// Initial node register
	upToDate, err := dispatcher.processNodeStatus("node1", status1)
	assert.NoError(t, err)
	assert.True(t, upToDate)
	node1, found := dispatcher.store.getNodeStore("node1")
	assert.True(t, found)
	assert.Equal(t, status1, node1.lastStatus)
	assert.True(t, timestampNow() >= node1.lastPing)
	assert.True(t, timestampNow() <= node1.lastPing+1)

	// Give changes
	node1.lastConfigChange = timestampNow()
	node1.lastPing = node1.lastPing - 50
	status2 := types.NodeStatus{LastChange: node1.lastConfigChange - 2}
	upToDate, err = dispatcher.processNodeStatus("node1", status2)
	assert.NoError(t, err)
	assert.False(t, upToDate)
	assert.True(t, timestampNow() >= node1.lastPing)
	assert.True(t, timestampNow() <= node1.lastPing+1)

	// No change
	status3 := types.NodeStatus{LastChange: node1.lastConfigChange}
	upToDate, err = dispatcher.processNodeStatus("node1", status3)
	assert.NoError(t, err)
	assert.True(t, upToDate)

	requireNotLocked(t, dispatcher.store)
}
