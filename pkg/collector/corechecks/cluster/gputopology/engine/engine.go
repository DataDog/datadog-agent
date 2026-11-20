// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package engine

import (
	"cmp"
	"slices"
	"strings"
)

// HostnameLabel is the node label key used for the host level. The implicit
// host level is reported with this key too.
const HostnameLabel = "kubernetes.io/hostname"

// PodSizes returns the pod sizes, in GPUs, for which free slots are reported.
func PodSizes() []int { return []int{1, 2, 4, 8} }

// Topology describes a scheduler topology as seen by one resource flavor.
type Topology struct {
	Scheduler   string // e.g. "kueue"
	Name        string
	Flavor      string
	Levels      []string     // node label keys, coarsest -> finest
	Tolerations []Toleration // flavor tolerations
	// AssumedTolerations are tolerated on top of the flavor's (from config),
	// for taints every workload is known to tolerate.
	AssumedTolerations []Toleration
}

// Node is a node selected by the flavor.
type Node struct {
	Name   string
	Labels map[string]string
	// Capacity and Allocatable are the node's extended resources
	// (status.capacity, status.allocatable). Non-GPU resources are ignored.
	Capacity      map[string]int
	Allocatable   map[string]int
	Ready         bool
	Unschedulable bool
	Taints        []Taint
}

// Allocations holds the allocated GPUs per node name, then per resource name.
type Allocations map[string]map[string]int

// InitContainer is the GPU request of an init container. Sidecar is true for
// restartPolicy: Always (native sidecars).
type InitContainer struct {
	Request int
	Sidecar bool
}

// PodRequest returns the effective GPU request of a pod for one resource,
// following Kubernetes: sidecars run alongside the containers, and each
// regular init container runs alongside the sidecars started before it.
//
//	max(sum(containers) + sum(sidecars), max over regular init containers of (request + sidecars before it))
func PodRequest(containers []int, initContainers []InitContainer) int {
	sidecars, initPeak := 0, 0
	for _, c := range initContainers {
		if c.Sidecar {
			sidecars += c.Request
			initPeak = max(initPeak, sidecars)
			continue
		}
		initPeak = max(initPeak, sidecars+c.Request)
	}
	req := sidecars
	for _, c := range containers {
		req += c
	}
	return max(req, initPeak)
}

// DomainState is the occupancy state of a domain.
type DomainState int

// Domain states. They index StateCounts.
const (
	DomainFree DomainState = iota
	DomainPartial
	DomainFull
	DomainUnavailable // no in_topology capacity
)

// DomainStates returns all domain states in reporting order.
func DomainStates() []DomainState {
	return []DomainState{DomainFree, DomainPartial, DomainFull, DomainUnavailable}
}

func (s DomainState) String() string {
	switch s {
	case DomainFree:
		return "free"
	case DomainPartial:
		return "partially_occupied"
	case DomainFull:
		return "fully_occupied"
	case DomainUnavailable:
		return "unavailable"
	}
	return "unknown"
}

func stateOf(d Domain) DomainState {
	switch {
	case d.Allocatable == 0:
		return DomainUnavailable
	case d.Free == 0:
		return DomainFull
	case d.Allocated == 0:
		return DomainFree
	}
	return DomainPartial
}

// StateCounts holds one count per DomainState, indexed by it.
type StateCounts [4]int

// UnschedulableReason is why GPUs in scope can't be used by the scheduler.
type UnschedulableReason int

// Unschedulable reasons. They index ReasonCounts.
const (
	ReasonUnhealthy       UnschedulableReason = iota // capacity - allocatable on in_topology nodes
	ReasonNodeUnavailable                            // capacity of unavailable nodes
	ReasonTainted                                    // capacity of tainted nodes
	ReasonNoAllocatable                              // capacity of no_gpu_allocatable nodes
	ReasonMissingLabels                              // capacity of missing_labels nodes
)

// UnschedulableReasons returns all reasons in reporting order.
func UnschedulableReasons() []UnschedulableReason {
	return []UnschedulableReason{ReasonUnhealthy, ReasonNodeUnavailable, ReasonTainted, ReasonNoAllocatable, ReasonMissingLabels}
}

func (r UnschedulableReason) String() string {
	switch r {
	case ReasonUnhealthy:
		return "unhealthy"
	case ReasonNodeUnavailable:
		return "node_unavailable"
	case ReasonTainted:
		return "tainted"
	case ReasonNoAllocatable:
		return "no_allocatable"
	case ReasonMissingLabels:
		return "missing_labels"
	}
	return "unknown"
}

// ReasonCounts holds one GPU count per UnschedulableReason, indexed by it.
type ReasonCounts [5]int

// Result is the output of Compute.
type Result struct {
	Topology Topology
	// Nodes has a count for every NodeState and every GPU resource
	// attribution seen, zeros included, sorted by state then resource.
	Nodes       []NodeCount
	Resources   []ResourceResult // sorted by resource name
	Overcommits []Overcommit     // sorted by node, then resource
}

// NodeCount is the number of nodes in a state for a GPU resource. A node
// with several GPU resources counts once for each.
type NodeCount struct {
	State       NodeState
	GPUResource string // resource name, or GPUResourceMIG
	Count       int
}

// ResourceResult holds the results for one GPU resource name.
type ResourceResult struct {
	Resource          string
	UnschedulableGPUs ReasonCounts
	Levels            []LevelResult // coarsest -> finest, host level included
}

// Slots is the number of pods of PodGPUs GPUs that fit.
type Slots struct {
	PodGPUs int
	Count   int
}

// LevelResult holds the results for one level.
type LevelResult struct {
	Label    string // node label key (HostnameLabel for the implicit host level)
	Depth    int    // 0 = coarsest
	Implicit bool   // implicit host level, keyed on node name

	Domains       StateCounts // number of domains per state
	FreeGPUs      StateCounts // free GPUs per state of their domain
	AllocatedGPUs StateCounts // allocated GPUs per state of their domain

	LargestFreeGPUs int // most free GPUs in one domain
	// LargestFreeSlots is, per pod size, the most pods of that size that fit
	// in one domain. Pod sizes larger than the largest host capacity of the
	// resource (any node state) are skipped, so the sizes don't change with
	// node health.
	LargestFreeSlots []Slots

	DomainStats []Domain // sorted by Path
}

// Domain holds the stats of one domain.
type Domain struct {
	Path        []string // label values of levels 0..Depth
	Hosts       int      // labelled nodes attributed to the resource, in any state but missing_labels/unsupported
	Allocatable int      // of in_topology nodes
	Allocated   int      // clamped at Allocatable per node
	Free        int
	FreeSlots   []Slots // sum over hosts of floor(free/size), same sizes as the level
	State       DomainState
}

// Overcommit reports a node with more GPUs allocated than allocatable.
type Overcommit struct {
	Node        string
	Resource    string
	Allocatable int
	Allocated   int
}

// host is one labelled node, for one resource.
type host struct {
	path        []string // label values for every level
	allocatable int      // 0 unless in_topology
	allocated   int      // clamped at allocatable
}

// Compute classifies the nodes, then groups them into domains for every
// level and every GPU resource.
func Compute(t Topology, nodes []Node, alloc Allocations) Result {
	res := Result{Topology: t}
	if len(t.Levels) == 0 {
		return res
	}

	levels := slices.Clone(t.Levels)
	implicit := !slices.Contains(levels, HostnameLabel)
	if implicit {
		levels = append(levels, HostnameLabel)
	}
	tols := slices.Concat(t.Tolerations, t.AssumedTolerations)

	var all []classified
	var attributions []string
	counts := map[NodeCount]int{}
	for _, n := range nodes {
		c, ok := classify(n, levels, implicit, tols)
		if !ok {
			continue
		}
		all = append(all, c)
		for r, state := range c.states {
			counts[NodeCount{State: state, GPUResource: r}]++
			if !slices.Contains(attributions, r) {
				attributions = append(attributions, r)
			}
		}
	}
	slices.Sort(attributions)
	for _, state := range NodeStates() {
		for _, r := range attributions {
			k := NodeCount{State: state, GPUResource: r}
			k.Count = counts[k]
			res.Nodes = append(res.Nodes, k)
		}
	}

	for _, resource := range resourceNames(all) {
		rr, overcommits := computeResource(resource, all, levels, alloc)
		rr.Levels[len(levels)-1].Implicit = implicit
		res.Resources = append(res.Resources, rr)
		res.Overcommits = append(res.Overcommits, overcommits...)
	}
	slices.SortFunc(res.Overcommits, func(a, b Overcommit) int {
		return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.Resource, b.Resource))
	})
	return res
}

func computeResource(resource string, nodes []classified, levels []string, alloc Allocations) (ResourceResult, []Overcommit) {
	rr := ResourceResult{Resource: resource}
	var overcommits []Overcommit
	var hosts []host
	largestHost := 0 // bounds the pod sizes, independent of node health
	for _, c := range nodes {
		state, ok := c.states[resource]
		if !ok {
			continue
		}
		capacity := max(c.node.Capacity[resource], 0)
		largestHost = max(largestHost, capacity)
		switch state {
		case NodeMissingLabels:
			rr.UnschedulableGPUs[ReasonMissingLabels] += capacity
			continue
		case NodeUnavailable:
			rr.UnschedulableGPUs[ReasonNodeUnavailable] += capacity
		case NodeNoGPUAllocatable:
			rr.UnschedulableGPUs[ReasonNoAllocatable] += capacity
		case NodeTainted:
			rr.UnschedulableGPUs[ReasonTainted] += capacity
		}
		h := host{path: c.path}
		if state == NodeInTopology {
			h.allocatable = min(max(c.node.Allocatable[resource], 0), capacity)
			rr.UnschedulableGPUs[ReasonUnhealthy] += capacity - h.allocatable
			h.allocated = max(alloc[c.node.Name][resource], 0)
			if h.allocated > h.allocatable {
				overcommits = append(overcommits, Overcommit{Node: c.node.Name, Resource: resource, Allocatable: h.allocatable, Allocated: h.allocated})
				h.allocated = h.allocatable
			}
		}
		hosts = append(hosts, h)
	}

	var sizes []int
	for _, s := range PodSizes() {
		if s <= largestHost {
			sizes = append(sizes, s)
		}
	}
	for depth, label := range levels {
		lvl := computeLevel(hosts, depth, sizes)
		lvl.Label = label
		rr.Levels = append(rr.Levels, lvl)
	}
	return rr, overcommits
}

// computeLevel aggregates hosts into the domains of one level.
func computeLevel(hosts []host, depth int, sizes []int) LevelResult {
	byKey := map[string]*Domain{}
	for _, h := range hosts {
		path := h.path[:depth+1]
		key := strings.Join(path, "\x00")
		d, ok := byKey[key]
		if !ok {
			d = &Domain{Path: slices.Clone(path), FreeSlots: newSlots(sizes)}
			byKey[key] = d
		}
		free := h.allocatable - h.allocated
		d.Hosts++
		d.Allocatable += h.allocatable
		d.Allocated += h.allocated
		d.Free += free
		for i := range d.FreeSlots {
			d.FreeSlots[i].Count += free / d.FreeSlots[i].PodGPUs
		}
	}

	lvl := LevelResult{Depth: depth, LargestFreeSlots: newSlots(sizes)}
	for _, d := range byKey {
		d.State = stateOf(*d)
		lvl.Domains[d.State]++
		lvl.FreeGPUs[d.State] += d.Free
		lvl.AllocatedGPUs[d.State] += d.Allocated
		lvl.LargestFreeGPUs = max(lvl.LargestFreeGPUs, d.Free)
		for i, s := range d.FreeSlots {
			lvl.LargestFreeSlots[i].Count = max(lvl.LargestFreeSlots[i].Count, s.Count)
		}
		lvl.DomainStats = append(lvl.DomainStats, *d)
	}
	slices.SortFunc(lvl.DomainStats, func(a, b Domain) int { return slices.Compare(a.Path, b.Path) })
	return lvl
}

func newSlots(sizes []int) []Slots {
	slots := make([]Slots, len(sizes))
	for i, s := range sizes {
		slots[i].PodGPUs = s
	}
	return slots
}

// resourceNames returns the sorted GPU resources attributed to the
// classified nodes, unsupported nodes excluded.
func resourceNames(nodes []classified) []string {
	var names []string
	for _, c := range nodes {
		for r, state := range c.states {
			if state != NodeUnsupported && !slices.Contains(names, r) {
				names = append(names, r)
			}
		}
	}
	slices.Sort(names)
	return names
}
