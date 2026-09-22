// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAliveMembers tests lease liveness filtering.
// Test partitions:
// - renewal age: current | exactly expired (boundary: now == renewed+duration) | long expired
// - member set: empty names present | empty input
func TestAliveMembers(t *testing.T) {
	now := time.Now()
	members := []MemberInfo{
		{Name: "dca-0", RenewedAt: now.Add(-1 * time.Second), Duration: 40 * time.Second},
		{Name: "dca-1", RenewedAt: now.Add(-41 * time.Second), Duration: 40 * time.Second},
		{Name: "dca-2", RenewedAt: now.Add(-40 * time.Second), Duration: 40 * time.Second},
		{Name: "", RenewedAt: now, Duration: 40 * time.Second},
	}

	// dca-2 is at the boundary: expired the moment now reaches renewed+duration.
	assert.Equal(t, []string{"dca-0"}, AliveMembers(members, now))
	assert.Empty(t, AliveMembers(nil, now), "no members (boundary)")
}

// TestOwner tests single-node ownership.
// Test partitions:
// - member set: empty | one | several
// - determinism: repeated computation agrees
func TestOwner(t *testing.T) {
	assert.Empty(t, Owner("node-a", nil), "no members: no owner")

	single := []string{"dca-0"}
	assert.Equal(t, "dca-0", Owner("node-a", single), "single member owns everything")

	three := []string{"dca-0", "dca-1", "dca-2"}
	for _, node := range []string{"node-a", "node-b", "node-c", "node-d", "node-e"} {
		owner := Owner(node, three)
		assert.Contains(t, three, owner)
		assert.Equal(t, owner, Owner(node, three), "assignment is deterministic")
		assert.Equal(t, owner, Owner(node, []string{"dca-1", "dca-2", "dca-0"}), "assignment ignores member order")
	}
}

// TestAssignAndOwnedSets tests the assignment and its inversion.
// Test partitions:
// - node coverage: every node assigned | unowned when members empty
// - inversion: OwnedSets matches Assign and is sorted
// - members with no nodes appear with empty lists
func TestAssignAndOwnedSets(t *testing.T) {
	nodes := []string{"node-c", "node-a", "node-b"}
	members := []string{"dca-0", "dca-1"}

	assignment := Assign(nodes, members)
	require.Len(t, assignment, 3)
	for _, node := range nodes {
		assert.Contains(t, members, assignment[node])
	}

	sets := OwnedSets(nodes, members)
	assigned := 0
	for member, owned := range sets {
		assert.Contains(t, members, member)
		assert.IsNonDecreasing(t, owned, "owned sets are sorted")
		assigned += len(owned)
		for _, node := range owned {
			assert.Equal(t, member, assignment[node])
		}
	}
	assert.Equal(t, len(nodes), assigned, "every node appears in exactly one set")

	empty := OwnedSets(nodes, nil)
	assert.Empty(t, empty, "no members: no owned sets")

	// An empty member list in Assign gives unowned nodes, not a crash.
	for _, node := range nodes {
		assert.Empty(t, Assign([]string{node}, nil)[node])
	}
}

// TestRendezvousMovement tests the movement property that justifies rendezvous
// hashing over a naive modulo.
// Test partitions:
// - membership change: member removed | member added
// - removal: only the removed member's nodes change owner (strict invariant)
// - addition: only a minority of nodes change owner (loose bound)
func TestRendezvousMovement(t *testing.T) {
	nodes := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		nodes = append(nodes, "node-"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	members := []string{"dca-0", "dca-1", "dca-2"}

	before := Assign(nodes, members)

	// Remove dca-1: only nodes it owned may change owner.
	afterRemoval := Assign(nodes, []string{"dca-0", "dca-2"})
	changed := 0
	for _, node := range nodes {
		if before[node] != afterRemoval[node] {
			assert.Equal(t, "dca-1", before[node], "removal reassigns only the removed member's nodes")
			changed++
		}
	}
	assert.Positive(t, changed, "dca-1 owned some nodes before removal")

	// Add dca-3: expected movement is nodes/(members+1) = 15/60; assert a
	// loose upper bound to stay robust to the specific hash values.
	afterAddition := Assign(nodes, append(members, "dca-3"))
	moved := 0
	for _, node := range nodes {
		if before[node] != afterAddition[node] {
			moved++
		}
	}
	assert.LessOrEqual(t, moved, len(nodes)/3, "adding a member moves a minority of nodes")
	assert.Positive(t, moved, "the new member takes over some nodes")
}

// TestOwnedNodesAnnotationCodec tests the annotation round trip.
// Test partitions:
// - node set: empty | populated
// - input: empty annotation | malformed value (boundary)
func TestOwnedNodesAnnotationCodec(t *testing.T) {
	encoded, err := MarshalOwnedNodes([]string{"node-b", "node-a"})
	require.NoError(t, err)
	decoded, err := UnmarshalOwnedNodes(encoded)
	require.NoError(t, err)
	assert.Equal(t, []string{"node-a", "node-b"}, decoded)

	empty, err := UnmarshalOwnedNodes("")
	require.NoError(t, err)
	assert.Nil(t, empty, "missing annotation decodes to nil")

	_, err = UnmarshalOwnedNodes("{not json")
	assert.Error(t, err, "malformed annotation value fails loudly")
}
