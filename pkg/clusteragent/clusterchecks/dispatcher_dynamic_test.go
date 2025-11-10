// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func TestUpdateAdvancedDispatchingMode(t *testing.T) {
	tests := []struct {
		name                        string
		initialAdvancedDispatching  bool
		nodes                       map[string]*nodeStore
		expectedAdvancedDispatching bool
	}{
		{
			name:                       "advanced dispatching disabled initially",
			initialAdvancedDispatching: false,
			nodes: map[string]*nodeStore{
				"node1": {nodetype: cctypes.NodeTypeCLCRunner},
			},
			expectedAdvancedDispatching: false, // Should remain disabled
		},
		{
			name:                       "only CLC runners present",
			initialAdvancedDispatching: true,
			nodes: map[string]*nodeStore{
				"clc1": {nodetype: cctypes.NodeTypeCLCRunner},
				"clc2": {nodetype: cctypes.NodeTypeCLCRunner},
			},
			expectedAdvancedDispatching: true, // Should remain enabled
		},
		{
			name:                       "node agents present",
			initialAdvancedDispatching: true,
			nodes: map[string]*nodeStore{
				"node1": {nodetype: cctypes.NodeTypeNodeAgent},
				"node2": {nodetype: cctypes.NodeTypeNodeAgent},
			},
			expectedAdvancedDispatching: false, // Should be disabled
		},
		{
			name:                       "mixed mode - both CLC runners and node agents",
			initialAdvancedDispatching: true,
			nodes: map[string]*nodeStore{
				"clc1":  {nodetype: cctypes.NodeTypeCLCRunner},
				"node1": {nodetype: cctypes.NodeTypeNodeAgent},
			},
			expectedAdvancedDispatching: false, // Should be disabled in mixed mode
		},
		{
			name:                        "no nodes",
			initialAdvancedDispatching:  true,
			nodes:                       map[string]*nodeStore{},
			expectedAdvancedDispatching: true, // Should remain enabled when no nodes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &dispatcher{
				store: &clusterStore{
					nodes: tt.nodes,
				},
			}
			d.advancedDispatching.Store(tt.initialAdvancedDispatching)

			d.UpdateAdvancedDispatchingMode()

			assert.Equal(t, tt.expectedAdvancedDispatching, d.advancedDispatching.Load())
		})
	}
}

func TestDisableAdvancedDispatching(t *testing.T) {
	d := &dispatcher{}

	// Initially enabled
	d.advancedDispatching.Store(true)
	d.disableAdvancedDispatching()
	assert.False(t, d.advancedDispatching.Load())

	// Already disabled - should not log
	d.disableAdvancedDispatching()
	assert.False(t, d.advancedDispatching.Load())
}

func TestProcessNodeStatusDisablesAdvancedDispatching(t *testing.T) {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.advancedDispatching.Store(true)
	d.store.active = true

	// Process a node agent status
	status := cctypes.NodeStatus{
		NodeType:   cctypes.NodeTypeNodeAgent,
		LastChange: 0, // Same as initial lastConfigChange to be up-to-date
	}

	upToDate := d.processNodeStatus("node1", "", status)

	// Should disable advanced dispatching
	assert.False(t, d.advancedDispatching.Load())
	assert.True(t, upToDate) // Node should be considered up to date

	// Verify node was created with correct type
	node, found := d.store.getNodeStore("node1")
	assert.True(t, found)
	assert.Equal(t, cctypes.NodeTypeNodeAgent, node.nodetype)
}

func TestProcessNodeStatusCLCRunner(t *testing.T) {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.advancedDispatching.Store(true)
	d.store.active = true

	// Process a CLC runner status
	status := cctypes.NodeStatus{
		NodeType:   cctypes.NodeTypeCLCRunner,
		LastChange: 0, // Same as initial lastConfigChange to be up-to-date
	}

	upToDate := d.processNodeStatus("clc1", "10.0.0.1", status)

	// Should keep advanced dispatching enabled
	assert.True(t, d.advancedDispatching.Load())
	assert.True(t, upToDate)

	// Verify node was created with correct type
	node, found := d.store.getNodeStore("clc1")
	assert.True(t, found)
	assert.Equal(t, cctypes.NodeTypeCLCRunner, node.nodetype)
}

func TestAtomicBoolConcurrency(_ *testing.T) {
	d := &dispatcher{}
	d.advancedDispatching.Store(true)

	// Test concurrent reads and writes
	done := make(chan bool)

	// Multiple readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = d.advancedDispatching.Load()
			}
			done <- true
		}()
	}

	// Single writer
	go func() {
		for j := 0; j < 100; j++ {
			d.advancedDispatching.CompareAndSwap(true, false)
			d.advancedDispatching.CompareAndSwap(false, true)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}
}
