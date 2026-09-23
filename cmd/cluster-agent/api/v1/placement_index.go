// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"iter"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// placementIndex counts, for every workload, its pods bound to each node.
//
// It holds one entry per pod in the cluster, so nodes and workloads are
// interned: a pod's placement is two small IDs rather than a node name and a
// workload target. IDs are reference counted by the placements using them and
// reused once released, so the tables stay bounded by the live nodes and
// workloads whatever the churn.
//
// Not safe for concurrent use.
type placementIndex struct {
	nodes     internTable[string]
	workloads internTable[kubernetes.WorkloadTarget]
	pods      map[string]placement         // pod UID -> placement
	counts    map[uint32]map[uint32]uint32 // workload ID -> node ID -> pods
}

// placement is where a pod runs and which workload owns it, as interned IDs.
type placement struct {
	node     uint32
	workload uint32
}

// hostingChange is a workload that started or stopped having pods on a node.
type hostingChange struct {
	node   string
	target kubernetes.WorkloadTarget
}

func newPlacementIndex() *placementIndex {
	return &placementIndex{
		nodes:     newInternTable[string](),
		workloads: newInternTable[kubernetes.WorkloadTarget](),
		pods:      make(map[string]placement),
		counts:    make(map[uint32]map[uint32]uint32),
	}
}

// set records that a pod runs on node and is owned by target. A pod already
// recorded elsewhere, or under another workload, is moved. It appends to
// changes the workloads that started or stopped having pods on a node, and
// returns it.
func (idx *placementIndex) set(podUID, node string, target kubernetes.WorkloadTarget, changes []hostingChange) []hostingChange {
	if previous, known := idx.pods[podUID]; known {
		if idx.nodes.value(previous.node) == node && idx.workloads.value(previous.workload) == target {
			return changes
		}
		changes = idx.remove(podUID, previous, changes)
	}

	p := placement{node: idx.nodes.acquire(node), workload: idx.workloads.acquire(target)}
	idx.pods[podUID] = p
	nodes := idx.counts[p.workload]
	if nodes == nil {
		nodes = make(map[uint32]uint32)
		idx.counts[p.workload] = nodes
	}
	nodes[p.node]++
	if nodes[p.node] == 1 {
		changes = append(changes, hostingChange{node: node, target: target})
	}
	return changes
}

// delete forgets a pod. It appends to changes the workload that stopped having
// pods on a node, if any, and returns it.
func (idx *placementIndex) delete(podUID string, changes []hostingChange) []hostingChange {
	previous, known := idx.pods[podUID]
	if !known {
		return changes
	}
	return idx.remove(podUID, previous, changes)
}

func (idx *placementIndex) remove(podUID string, p placement, changes []hostingChange) []hostingChange {
	delete(idx.pods, podUID)
	nodes := idx.counts[p.workload]
	nodes[p.node]--
	if nodes[p.node] == 0 {
		changes = append(changes, hostingChange{node: idx.nodes.value(p.node), target: idx.workloads.value(p.workload)})
		delete(nodes, p.node)
		if len(nodes) == 0 {
			delete(idx.counts, p.workload)
		}
	}
	idx.nodes.release(p.node)
	idx.workloads.release(p.workload)
	return changes
}

// hosts reports whether target has at least one pod on node.
func (idx *placementIndex) hosts(target kubernetes.WorkloadTarget, node string) bool {
	workload, ok := idx.workloads.lookup(target)
	if !ok {
		return false
	}
	nodeID, ok := idx.nodes.lookup(node)
	if !ok {
		return false
	}
	return idx.counts[workload][nodeID] > 0
}

// nodesOf yields the nodes where target has at least one pod.
func (idx *placementIndex) nodesOf(target kubernetes.WorkloadTarget) iter.Seq[string] {
	return func(yield func(string) bool) {
		workload, ok := idx.workloads.lookup(target)
		if !ok {
			return
		}
		for node := range idx.counts[workload] {
			if !yield(idx.nodes.value(node)) {
				return
			}
		}
	}
}

// internTable maps values to small dense IDs and back. Each ID is reference
// counted; once released by its last user it is freed and reused.
type internTable[T comparable] struct {
	ids    map[T]uint32
	values []T      // ID -> value
	refs   []uint32 // ID -> references
	free   []uint32 // released IDs, reused first
}

func newInternTable[T comparable]() internTable[T] {
	return internTable[T]{ids: make(map[T]uint32)}
}

// acquire returns the ID of v, allocating one if needed, and takes a reference
// on it.
func (t *internTable[T]) acquire(v T) uint32 {
	if id, ok := t.ids[v]; ok {
		t.refs[id]++
		return id
	}

	var id uint32
	if n := len(t.free); n > 0 {
		id = t.free[n-1]
		t.free = t.free[:n-1]
		t.values[id] = v
		t.refs[id] = 1
	} else {
		id = uint32(len(t.values))
		t.values = append(t.values, v)
		t.refs = append(t.refs, 1)
	}
	t.ids[v] = id
	return id
}

// release drops a reference on id, and frees it when it was the last one.
func (t *internTable[T]) release(id uint32) {
	t.refs[id]--
	if t.refs[id] > 0 {
		return
	}
	delete(t.ids, t.values[id])
	var zero T
	t.values[id] = zero // do not keep the value's strings alive
	t.free = append(t.free, id)
}

// lookup returns the ID of v without allocating one.
func (t *internTable[T]) lookup(v T) (uint32, bool) {
	id, ok := t.ids[v]
	return id, ok
}

func (t *internTable[T]) value(id uint32) T {
	return t.values[id]
}

// len returns the number of live IDs.
func (t *internTable[T]) len() int {
	return len(t.ids)
}
