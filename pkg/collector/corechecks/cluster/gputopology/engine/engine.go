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
	DomainPartiallyOccupied
	DomainFullyOccupied
	DomainUnavailable // no in_topology capacity
)

// DomainStates returns all domain states in reporting order.
func DomainStates() []DomainState {
	return []DomainState{DomainFree, DomainPartiallyOccupied, DomainFullyOccupied, DomainUnavailable}
}

func (s DomainState) String() string {
	switch s {
	case DomainFree:
		return "free"
	case DomainPartiallyOccupied:
		return "partially_occupied"
	case DomainFullyOccupied:
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
		return DomainFullyOccupied
	case d.Allocated == 0:
		return DomainFree
	}
	return DomainPartiallyOccupied
}

// StateCounts holds one count per DomainState, indexed by it.
type StateCounts [4]int

// UnschedulableReason is why GPUs in scope can't be used by the scheduler.
// Its String() values are the unschedulable_reason tag values.
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
	// Nodes has a count for every NodeState and every (GPU resource, GPU
	// type) seen, zeros included, plus the unsupported counts per slicing or
	// sharing mode (non-zero only). Sorted by state, resource, type, then
	// modes.
	Nodes       []NodeCount
	Resources   []ResourceResult // sorted by resource, then GPU type
	Overcommits []Overcommit     // sorted by node, then resource
}

// NodeCount is the number of nodes in a state for a GPU resource and type. A
// node with several GPU resources counts once for each.
type NodeCount struct {
	State       NodeState
	GPUResource string // a real resource key, e.g. nvidia.com/gpu
	GPUType     string // e.g. "h100", or GPUTypeUnknown
	SlicingMode string // SlicingModeMIG on unsupported MIG rows, else ""
	SharingMode string // SharingModeTimeSlicing or SharingModeMPS on unsupported shared rows, else ""
	Count       int
}

// ResourceResult holds the results for one (GPU resource, GPU type): nodes
// of different GPU types never share a domain.
type ResourceResult struct {
	Resource          string
	GPUType           string
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

	// DegradedDomains counts, per state, the domains with usable capacity
	// that also contain unusable nodes or unhealthy GPUs (see
	// Domain.Degraded). The unavailable entry is always 0.
	DegradedDomains StateCounts

	DomainStats []Domain // sorted by Path
}

// Domain holds the stats of one domain.
type Domain struct {
	Path        []string // label values of levels 0..Depth
	ID          string   // Path joined by "/", e.g. "block-a/rack-2"
	Hosts       int      // labelled nodes attributed to the resource, in any state but missing_labels/unsupported
	Allocatable int      // of in_topology nodes
	Allocated   int      // clamped at Allocatable per node
	Free        int
	// Unschedulable is the capacity of the domain's nodes that aren't
	// in_topology, plus capacity - allocatable of its in_topology nodes.
	Unschedulable int
	FreeSlots     []Slots // sum over hosts of floor(free/size), same sizes as the level
	State         DomainState
	// Degraded is set when the domain has usable capacity but also contains
	// a node that isn't in_topology or an unhealthy GPU. The state is
	// measured on usable capacity only, so a rack with one cordoned host and
	// nothing allocated is free and degraded.
	Degraded bool
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
	path          []string // label values for every level
	allocatable   int      // 0 unless in_topology
	allocated     int      // clamped at allocatable
	unschedulable int      // capacity if not in_topology, else capacity - allocatable
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
	var seen []unit
	counts := map[NodeCount]int{}
	for _, n := range nodes {
		c, ok := classify(n, levels, implicit, tols)
		if !ok {
			continue
		}
		all = append(all, c)
		for r, state := range c.states {
			counts[NodeCount{State: state, GPUResource: r, GPUType: c.gpuTypes[r], SlicingMode: c.slicing, SharingMode: c.sharing}]++
			if u := (unit{r, c.gpuTypes[r]}); !slices.Contains(seen, u) {
				seen = append(seen, u)
			}
		}
	}
	for _, state := range NodeStates() {
		for _, u := range seen {
			k := NodeCount{State: state, GPUResource: u.resource, GPUType: u.gpuType}
			k.Count = counts[k]
			res.Nodes = append(res.Nodes, k)
		}
	}
	for k, v := range counts {
		if k.SlicingMode != "" || k.SharingMode != "" {
			k.Count = v
			res.Nodes = append(res.Nodes, k)
		}
	}
	slices.SortFunc(res.Nodes, func(a, b NodeCount) int {
		return cmp.Or(cmp.Compare(a.State, b.State), cmp.Compare(a.GPUResource, b.GPUResource),
			cmp.Compare(a.GPUType, b.GPUType), cmp.Compare(a.SlicingMode, b.SlicingMode), cmp.Compare(a.SharingMode, b.SharingMode))
	})

	for _, u := range units(all) {
		rr, overcommits := computeUnit(u, all, levels, alloc)
		rr.Levels[len(levels)-1].Implicit = implicit
		res.Resources = append(res.Resources, rr)
		res.Overcommits = append(res.Overcommits, overcommits...)
	}
	slices.SortFunc(res.Overcommits, func(a, b Overcommit) int {
		return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.Resource, b.Resource))
	})
	return res
}

// unit is the computation unit: a GPU resource and a GPU type.
type unit struct {
	resource string
	gpuType  string
}

func compareUnits(a, b unit) int {
	return cmp.Or(cmp.Compare(a.resource, b.resource), cmp.Compare(a.gpuType, b.gpuType))
}

func computeUnit(u unit, nodes []classified, levels []string, alloc Allocations) (ResourceResult, []Overcommit) {
	resource := u.resource
	rr := ResourceResult{Resource: resource, GPUType: u.gpuType}
	var overcommits []Overcommit
	var hosts []host
	largestHost := 0 // bounds the pod sizes, independent of node health
	for _, c := range nodes {
		state, ok := c.states[resource]
		if !ok || state == NodeUnsupported || c.gpuTypes[resource] != u.gpuType {
			continue
		}
		capacity := gpuCapacity(c.node, resource)
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
		h := host{path: c.path, unschedulable: capacity}
		if state == NodeInTopology {
			h.allocatable = min(max(c.node.Allocatable[resource], 0), capacity)
			h.unschedulable = capacity - h.allocatable
			rr.UnschedulableGPUs[ReasonUnhealthy] += h.unschedulable
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
			d = &Domain{Path: slices.Clone(path), ID: strings.Join(path, "/"), FreeSlots: newSlots(sizes)}
			byKey[key] = d
		}
		if h.unschedulable > 0 || h.allocatable == 0 {
			d.Degraded = true // reset below for domains without usable capacity
		}
		d.Unschedulable += h.unschedulable
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
		d.Degraded = d.Degraded && d.State != DomainUnavailable
		lvl.Domains[d.State]++
		if d.Degraded {
			lvl.DegradedDomains[d.State]++
		}
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

// units returns the sorted computation units of the classified nodes,
// unsupported nodes excluded.
func units(nodes []classified) []unit {
	var out []unit
	for _, c := range nodes {
		for r, state := range c.states {
			if u := (unit{r, c.gpuTypes[r]}); state != NodeUnsupported && !slices.Contains(out, u) {
				out = append(out, u)
			}
		}
	}
	slices.SortFunc(out, compareUnits)
	return out
}
