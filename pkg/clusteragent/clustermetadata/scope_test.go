// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRingScope tests the ring adapter for the pod-watch interfaces.
// Test partitions:
// - Nodes: reflects the controller's MyNodes
// - Subscribe: immediate delivery | change delivery | unsubscribe
// - NodeSynced: forwarded to the controller's sync state
func TestRingScope(t *testing.T) {
	ctx := context.Background()
	controller, _ := newRingFixture(t, []string{"node-a", "node-b"})
	require.NoError(t, controller.Reconcile(ctx))

	scope := NewRingScope(controller)

	assert.Equal(t, controller.State().MyNodes, scope.Nodes())

	var mu sync.Mutex
	delivered := 0
	unsubscribe := scope.Subscribe(func() {
		mu.Lock()
		delivered++
		mu.Unlock()
	})
	// Subscribe delivers the current state immediately.
	mu.Lock()
	assert.Equal(t, 1, delivered)
	mu.Unlock()

	// A sync report flows into the controller's readiness gate.
	scope.NodeSynced(controller.State().MyNodes[0], true)
	assert.True(t, controller.State().NodeSynced[controller.State().MyNodes[0]],
		"NodeSynced forwards to the controller sync state")

	// Unsubscribe stops delivery.
	unsubscribe()
	controller.SetNodeSynced("nonexistent", true) // no-op, but touches state
	scope.notify()
	mu.Lock()
	assert.Equal(t, 1, delivered, "no delivery after unsubscribe")
	mu.Unlock()
}
