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

// PodSizes are the pod sizes, in GPUs, for which free slots are reported.
var PodSizes = []int{1, 2, 4, 8}

// Topology describes a scheduler topology as seen by one resource flavor.
type Topology struct {
	Scheduler   string // e.g. "kueue"
	Name        string
	Flavor      string
	Levels      []string     // node label keys, coarsest -> finest
	Tolerations []Toleration // flavor tolerations
}

// Node is a node selected by the flavor.
type Node struct {
	Name          string
	Labels        map[string]string
	GPUs          map[string]GPUCount // per GPU resource name
	Ready         bool
	Unschedulable bool
	Taints        []Taint
	// Unsupported is set by the caller for nodes the engine can't model
	// (e.g. MIG nodes).
	Unsupported bool
}

// GPUCount is the GPU capacity and allocatable of a node for one resource.
type GPUCount struct {
	Capacity    int
	Allocatable int
}

// Taint effects and toleration operators, with their Kubernetes values.
const (
	EffectNoSchedule       = "NoSchedule"
	EffectPreferNoSchedule = "PreferNoSchedule"
	EffectNoExecute        = "NoExecute"
	OpExists               = "Exists"
	OpEqual                = "Equal"
)

// Taint is a node taint.
type Taint struct {
	Key    string
	Value  string
	Effect string
}

// Toleration is a pod or flavor toleration.
type Toleration struct {
	Key      string
	Operator string // OpExists or OpEqual (default)
	Value    string
	Effect   string // empty matches all effects
}

// tolerates follows the Kubernetes matching rules.
func (t Toleration) tolerates(taint Taint) bool {
	if t.Effect != "" && t.Effect != taint.Effect {
		return false
	}
	if t.Key == "" {
		return t.Operator == OpExists
	}
	if t.Key != taint.Key {
		return false
	}
	switch t.Operator {
	case OpExists:
		return true
	case "", OpEqual:
		return t.Value == taint.Value
	}
	return false
}

// tolerated reports whether every NoSchedule/NoExecute taint is tolerated.
func tolerated(taints []Taint, tols []Toleration) bool {
	for _, taint := range taints {
		if taint.Effect != EffectNoSchedule && taint.Effect != EffectNoExecute {
			continue
		}
		if !slices.ContainsFunc(tols, func(t Toleration) bool { return t.tolerates(taint) }) {
			return false
		}
	}
	return true
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
)

// DomainStates lists all domain states in reporting order.
var DomainStates = []DomainState{DomainFree, DomainPartial, DomainFull}

func (s DomainState) String() string {
	switch s {
	case DomainFree:
		return "free"
	case DomainPartial:
		return "partial"
	case DomainFull:
		return "full"
	}
	return "unknown"
}

// stateOf checks full first, so a domain with nothing free never looks free.
func stateOf(allocated, free int) DomainState {
	switch {
	case free == 0:
		return DomainFull
	case allocated == 0:
		return DomainFree
	}
	return DomainPartial
}

// StateCounts holds one count per DomainState, indexed by it.
type StateCounts [3]int

// Result is the output of Compute.
type Result struct {
	Topology    Topology
	Resources   []ResourceResult // sorted by resource name
	Overcommits []Overcommit     // sorted by node, then resource
}

// ResourceResult holds the results for one GPU resource name.
type ResourceResult struct {
	Resource string
	Nodes    NodeCoverage
	// UnavailableGPUs is the sum of capacity - allocatable over in_topology
	// and no_gpu_allocatable nodes (e.g. unhealthy GPUs, device plugin down).
	UnavailableGPUs int
	Levels          []LevelResult // coarsest -> finest, host level included
}

// NodeCoverage counts the nodes in scope for a resource (capacity > 0) by
// node state. Each node has exactly one state, see doc.go for precedence.
type NodeCoverage struct {
	InTopology       int
	Unavailable      int // NotReady or cordoned
	Tainted          int // untolerated NoSchedule/NoExecute taint
	NoGPUAllocatable int // capacity > 0 but allocatable = 0
	MissingLabels    int // missing at least one level label
	Unsupported      int // flagged by the caller (e.g. MIG)
}

// LevelResult holds the results for one level.
type LevelResult struct {
	Label    string // node label key (HostnameLabel for the implicit host level)
	Depth    int    // 0 = coarsest
	Implicit bool   // implicit host level, keyed on node name

	Domains       StateCounts // number of domains per state
	FreeGPUs      StateCounts // free GPUs per state of their domain
	AllocatedGPUs StateCounts // allocated GPUs per state of their domain
	// UnavailableGPUs is capacity - allocatable (e.g. unhealthy GPUs), per
	// state of their domain.
	UnavailableGPUs StateCounts

	LargestFreeGPUs int // most free GPUs in one domain
	// LargestFreeSlots is, per pod size (indexed like PodSizes), the most pods
	// of that size that fit in one domain.
	LargestFreeSlots []int

	DomainStats []Domain // sorted by Path
}

// Domain holds the stats of one domain.
type Domain struct {
	Path        []string // label values of levels 0..Depth
	Hosts       int
	Allocatable int
	Allocated   int // clamped at Allocatable
	Free        int
	Unavailable int   // capacity - allocatable
	FreeSlots   []int // sum over hosts of floor(free/size), indexed like PodSizes
	State       DomainState
}

// Overcommit reports a node with more GPUs allocated than allocatable.
type Overcommit struct {
	Node        string
	Resource    string
	Allocatable int
	Allocated   int
}

// host is one in-topology node, for one resource.
type host struct {
	path        []string // label values for every level
	allocatable int
	allocated   int // clamped at allocatable
	unavailable int
}

// Compute groups nodes into domains for every level and every GPU resource.
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

	for _, resource := range resourceNames(nodes) {
		rr := ResourceResult{Resource: resource}
		var hosts []host
		for _, n := range nodes {
			gpus := n.GPUs[resource]
			if gpus.Capacity <= 0 {
				continue
			}
			path, ok := nodePath(n, levels, implicit)
			switch {
			case n.Unsupported:
				rr.Nodes.Unsupported++
				continue
			case !ok:
				rr.Nodes.MissingLabels++
				continue
			case gpus.Allocatable <= 0:
				rr.Nodes.NoGPUAllocatable++
				rr.UnavailableGPUs += gpus.Capacity
				continue
			case !n.Ready || n.Unschedulable:
				rr.Nodes.Unavailable++
				continue
			case !tolerated(n.Taints, t.Tolerations):
				rr.Nodes.Tainted++
				continue
			}
			rr.Nodes.InTopology++
			unavailable := max(gpus.Capacity-gpus.Allocatable, 0)
			rr.UnavailableGPUs += unavailable
			used := max(alloc[n.Name][resource], 0)
			if used > gpus.Allocatable {
				res.Overcommits = append(res.Overcommits, Overcommit{Node: n.Name, Resource: resource, Allocatable: gpus.Allocatable, Allocated: used})
				used = gpus.Allocatable
			}
			hosts = append(hosts, host{
				path:        path,
				allocatable: gpus.Allocatable,
				allocated:   used,
				unavailable: unavailable,
			})
		}
		for depth, label := range levels {
			lvl := computeLevel(hosts, depth)
			lvl.Label = label
			lvl.Implicit = implicit && depth == len(levels)-1
			rr.Levels = append(rr.Levels, lvl)
		}
		res.Resources = append(res.Resources, rr)
	}

	slices.SortFunc(res.Overcommits, func(a, b Overcommit) int {
		return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.Resource, b.Resource))
	})
	return res
}

// computeLevel aggregates hosts into the domains of one level.
func computeLevel(hosts []host, depth int) LevelResult {
	byKey := map[string]*Domain{}
	for _, h := range hosts {
		path := h.path[:depth+1]
		key := strings.Join(path, "\x00")
		d, ok := byKey[key]
		if !ok {
			d = &Domain{Path: slices.Clone(path), FreeSlots: make([]int, len(PodSizes))}
			byKey[key] = d
		}
		free := h.allocatable - h.allocated
		d.Hosts++
		d.Allocatable += h.allocatable
		d.Allocated += h.allocated
		d.Free += free
		d.Unavailable += h.unavailable
		for i, size := range PodSizes {
			d.FreeSlots[i] += free / size
		}
	}

	lvl := LevelResult{Depth: depth, LargestFreeSlots: make([]int, len(PodSizes))}
	for _, d := range byKey {
		if d.Allocatable == 0 {
			continue // can't happen with allocatable > 0 hosts, but never report empty domains
		}
		d.State = stateOf(d.Allocated, d.Free)
		lvl.Domains[d.State]++
		lvl.FreeGPUs[d.State] += d.Free
		lvl.AllocatedGPUs[d.State] += d.Allocated
		lvl.UnavailableGPUs[d.State] += d.Unavailable
		lvl.LargestFreeGPUs = max(lvl.LargestFreeGPUs, d.Free)
		for i := range PodSizes {
			lvl.LargestFreeSlots[i] = max(lvl.LargestFreeSlots[i], d.FreeSlots[i])
		}
		lvl.DomainStats = append(lvl.DomainStats, *d)
	}
	slices.SortFunc(lvl.DomainStats, func(a, b Domain) int { return slices.Compare(a.Path, b.Path) })
	return lvl
}

// nodePath returns the node's label values for every level. The implicit
// host level (last) uses the node name.
func nodePath(n Node, levels []string, implicit bool) ([]string, bool) {
	path := make([]string, len(levels))
	for i, label := range levels {
		if implicit && i == len(levels)-1 {
			path[i] = n.Name
			continue
		}
		v, ok := n.Labels[label]
		if !ok {
			return nil, false
		}
		path[i] = v
	}
	return path, true
}

// resourceNames returns the sorted GPU resource names with capacity on any node.
func resourceNames(nodes []Node) []string {
	var names []string
	for _, n := range nodes {
		for r, c := range n.GPUs {
			if c.Capacity > 0 && !slices.Contains(names, r) {
				names = append(names, r)
			}
		}
	}
	slices.Sort(names)
	return names
}
