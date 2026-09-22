// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

// RingState is this replica's current view of the ring.
type RingState struct {
	// Members is the alive member IDs.
	Members []string
	// Owned maps each alive member to the nodes it owns.
	Owned map[string][]string
	// MyNodes is the node set owned by this replica.
	MyNodes []string
	// NodeSynced reports, per owned node, whether the pod watch for that node has synced.
	NodeSynced map[string]bool
}

// Ready reports whether every owned node's pod watch has synced.
func (s RingState) Ready() bool {
	for _, node := range s.MyNodes {
		if !s.NodeSynced[node] {
			return false
		}
	}
	return true
}

// RingController reconciles the ring on a ticker: it reads the member
// leases, computes the assignment, publishes this replica's owned-node set
// on its own Lease, and exposes the resulting state to the LocalStore.
type RingController struct {
	manager *LeaseManager
	// nodeSource is a function that returns the set of nodes in the cluster
	nodeSource func(ctx context.Context) ([]string, error)
	selfID     string
	interval   time.Duration

	mu    sync.RWMutex
	state RingState

	onOwnedNodesChanged func(prev, next []string)
}

// NewRingController returns a controller for this replica.
func NewRingController(manager *LeaseManager, selfID string, interval time.Duration, nodeSource func(ctx context.Context) ([]string, error)) *RingController {
	return &RingController{
		manager:    manager,
		nodeSource: nodeSource,
		selfID:     selfID,
		interval:   interval,
	}
}

// OnOwnedNodesChanged registers a callback fired when a rebalance changes this replica's owned nodes.
func (c *RingController) OnOwnedNodesChanged(fn func(prev, next []string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onOwnedNodesChanged = fn
}

// Reconsile is responsible for ensuring our lease is present,
// reading the ring, computing the assignment, publishing our owned set, and renewing our lease.
// Ensure runs on every pass: it is idempotent and heals a lease deleted
// out from under this replica.
func (c *RingController) Reconcile(ctx context.Context) error {
	if err := c.manager.Ensure(ctx); err != nil {
		return err
	}

	// read the ring leases and filter out expired ones
	members, err := c.manager.Members(ctx)
	if err != nil {
		return err
	}
	alive := AliveMembers(members, time.Now())

	// get the node set owned by this replica
	nodes, err := c.nodeSource(ctx)
	if err != nil {
		return err
	}
	owned := OwnedSets(nodes, alive)
	myNodes := owned[c.selfID]

	c.mu.RLock()
	prev := c.state.MyNodes
	prevSynced := c.state.NodeSynced
	callback := c.onOwnedNodesChanged
	c.mu.RUnlock()

	// publish our owned set on our own Lease
	c.manager.SetOwnedNodes(myNodes)
	if err := c.manager.Renew(ctx); err != nil {
		return err
	}

	nodeSynced := make(map[string]bool)
	for _, node := range myNodes {
		nodeSynced[node] = prevSynced[node]
	}

	c.mu.Lock()
	c.state = RingState{
		Members:    alive,
		Owned:      owned,
		MyNodes:    myNodes,
		NodeSynced: nodeSynced,
	}
	c.mu.Unlock()

	if callback != nil && !sameNodes(prev, myNodes) {
		callback(prev, myNodes)
	}
	return nil
}

// Run reconciles on the ticker until ctx is done.
func (c *RingController) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		if err := c.Reconcile(ctx); err != nil {
			log.Warnf("cluster metadata ring reconcile failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// State returns a copy of the current ring view.
func (c *RingController) State() RingState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state := RingState{
		Members:    append([]string(nil), c.state.Members...),
		Owned:      make(map[string][]string, len(c.state.Owned)),
		MyNodes:    append([]string(nil), c.state.MyNodes...),
		NodeSynced: make(map[string]bool, len(c.state.NodeSynced)),
	}
	for member, nodes := range c.state.Owned {
		state.Owned[member] = append([]string(nil), nodes...)
	}
	for node, synced := range c.state.NodeSynced {
		state.NodeSynced[node] = synced
	}
	return state
}

// SetNodeSynced records the sync state of one owned node's pod watch.
func (c *RingController) SetNodeSynced(node string, synced bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.NodeSynced == nil {
		c.state.NodeSynced = map[string]bool{}
	}
	c.state.NodeSynced[node] = synced
}

// sameNodes compares two node sets for equality, order-insensitive.
func sameNodes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, n := range a {
		counts[n]++
	}
	for _, n := range b {
		counts[n]--
		if counts[n] < 0 {
			return false
		}
	}
	return true
}
