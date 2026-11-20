// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package engine

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	blockLabel = "cloud.provider.com/topology-block"
	rackLabel  = "cloud.provider.com/topology-rack"
	nvidia     = "nvidia.com/gpu"
	amd        = "amd.com/gpu"
)

// gpuNode returns a Ready node with n whole GPUs of each given resource.
func gpuNode(name string, labels map[string]string, n int, resources ...string) Node {
	node := Node{Name: name, Labels: labels, Capacity: map[string]int{}, Allocatable: map[string]int{}, Ready: true}
	for _, r := range resources {
		node.Capacity[r] = n
		node.Allocatable[r] = n
	}
	return node
}

// cluster builds 4-GPU nvidia nodes. used[block][rack] lists the allocated
// GPUs per host. Nodes are named <block>-<rack>-h<i>.
func cluster(used map[string]map[string][]int) ([]Node, Allocations) {
	var nodes []Node
	alloc := Allocations{}
	for block, racks := range used {
		for rack, hosts := range racks {
			for i, u := range hosts {
				name := fmt.Sprintf("%s-%s-h%d", block, rack, i)
				nodes = append(nodes, gpuNode(name, map[string]string{blockLabel: block, rackLabel: rack, HostnameLabel: name}, 4, nvidia))
				alloc[name] = map[string]int{nvidia: u}
			}
		}
	}
	return nodes, alloc
}

// slots builds Slots for the first len(counts) PodSizes.
func slots(counts ...int) []Slots {
	out := make([]Slots, len(counts))
	for i, c := range counts {
		out[i] = Slots{PodGPUs: PodSizes[i], Count: c}
	}
	return out
}

// summary is a LevelResult without the per-domain stats, to keep tables short.
func summary(l LevelResult) LevelResult {
	l.DomainStats = nil
	return l
}

func paths(l LevelResult) [][]string {
	var out [][]string
	for _, d := range l.DomainStats {
		out = append(out, d.Path)
	}
	return out
}

func TestPodRequest(t *testing.T) {
	regular := func(req int) InitContainer { return InitContainer{Request: req} }
	sidecar := func(req int) InitContainer { return InitContainer{Request: req, Sidecar: true} }
	tests := []struct {
		name       string
		containers []int
		inits      []InitContainer
		want       int
	}{
		{"empty", nil, nil, 0},
		{"containers summed", []int{1, 2}, nil, 3},
		{"init below containers", []int{1, 2}, []InitContainer{regular(2), regular(1)}, 3},
		{"init above containers", []int{1, 0}, []InitContainer{regular(4), regular(1)}, 4},
		{"sidecar adds to containers", []int{2}, []InitContainer{sidecar(1)}, 3},
		{"init runs with sidecars started before it", []int{1}, []InitContainer{sidecar(1), regular(3), sidecar(1)}, 4},
		{"init before sidecar runs alone", []int{1}, []InitContainer{regular(3), sidecar(1)}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PodRequest(tt.containers, tt.inits))
		})
	}
}

func TestWorkedExamples(t *testing.T) {
	topo := Topology{Scheduler: "kueue", Name: "default", Flavor: "tas-flavor", Levels: []string{rackLabel}}

	tests := []struct {
		name string
		used map[string][]int
		rack LevelResult
		host LevelResult
	}{
		{
			// Guillermo's example: 3 racks x 3 hosts x 4 GPUs, 1 GPU used on every host.
			name: "3 racks, before re-packing",
			used: map[string][]int{"rack-1": {1, 1, 1}, "rack-2": {1, 1, 1}, "rack-3": {1, 1, 1}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{0, 3, 0, 0},
				FreeGPUs:         StateCounts{0, 27, 0, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0, 0},
				LargestFreeGPUs:  9,
				LargestFreeSlots: slots(9, 3, 0), // 8-GPU pods skipped: no 8-GPU host
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{0, 9, 0, 0},
				FreeGPUs:         StateCounts{0, 27, 0, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0, 0},
				LargestFreeGPUs:  3,
				LargestFreeSlots: slots(3, 1, 0),
			},
		},
		{
			name: "3 racks, after re-packing",
			used: map[string][]int{"rack-1": {4, 4, 1}, "rack-2": {0, 0, 0}, "rack-3": {0, 0, 0}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{2, 1, 0, 0},
				FreeGPUs:         StateCounts{24, 3, 0, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0, 0},
				LargestFreeGPUs:  12,
				LargestFreeSlots: slots(12, 6, 3),
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{6, 1, 2, 0},
				FreeGPUs:         StateCounts{24, 3, 0, 0},
				AllocatedGPUs:    StateCounts{0, 1, 8, 0},
				LargestFreeGPUs:  4,
				LargestFreeSlots: slots(4, 2, 1),
			},
		},
		{
			// Design doc example C: 2 racks x 3 hosts x 4 GPUs.
			name: "2 racks, before re-packing",
			used: map[string][]int{"rack-1": {2, 1, 2}, "rack-2": {1, 2, 1}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{0, 2, 0, 0},
				FreeGPUs:         StateCounts{0, 15, 0, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0, 0},
				LargestFreeGPUs:  8, // rack-2: 12 - 4
				LargestFreeSlots: slots(8, 3, 0),
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{0, 6, 0, 0},
				FreeGPUs:         StateCounts{0, 15, 0, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0, 0},
				LargestFreeGPUs:  3,
				LargestFreeSlots: slots(3, 1, 0),
			},
		},
		{
			name: "2 racks, after re-packing",
			used: map[string][]int{"rack-1": {4, 4, 1}, "rack-2": {0, 0, 0}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{1, 1, 0, 0},
				FreeGPUs:         StateCounts{12, 3, 0, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0, 0},
				LargestFreeGPUs:  12,
				LargestFreeSlots: slots(12, 6, 3),
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{3, 1, 2, 0},
				FreeGPUs:         StateCounts{12, 3, 0, 0},
				AllocatedGPUs:    StateCounts{0, 1, 8, 0},
				LargestFreeGPUs:  4,
				LargestFreeSlots: slots(4, 2, 1),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, alloc := cluster(map[string]map[string][]int{"b": tt.used})
			res := Compute(topo, nodes, alloc)

			assert.Equal(t, []NodeCount{{State: NodeInTopology, GPUResource: nvidia, Count: len(nodes)}}, res.Nodes)
			require.Len(t, res.Resources, 1)
			rr := res.Resources[0]
			assert.Equal(t, nvidia, rr.Resource)
			assert.Equal(t, ReasonCounts{}, rr.UnschedulableGPUs)
			require.Len(t, rr.Levels, 2)
			assert.Equal(t, tt.rack, summary(rr.Levels[0]))
			assert.Equal(t, tt.host, summary(rr.Levels[1]))
			assert.Empty(t, res.Overcommits)
		})
	}
}

func TestWorkedExampleDomainStats(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {4, 4, 1}, "rack-2": {0, 0, 0}}})
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)
	assert.Equal(t, []Domain{
		{Path: []string{"rack-1"}, Hosts: 3, Allocatable: 12, Allocated: 9, Free: 3, FreeSlots: slots(3, 1, 0), State: DomainPartial},
		{Path: []string{"rack-2"}, Hosts: 3, Allocatable: 12, Allocated: 0, Free: 12, FreeSlots: slots(12, 6, 3), State: DomainFree},
	}, res.Resources[0].Levels[0].DomainStats)
}

func TestSlots(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	tests := []struct {
		name     string
		hostGPUs []int // allocatable per host
		used     []int
		rack     []Slots
		largest  int
	}{
		{"free [3,3,3,3]: GPUs but no 4-GPU slot", []int{4, 4, 4, 4}, []int{1, 1, 1, 1}, slots(12, 4, 0), 12},
		{"8-GPU hosts half used: three 4-GPU slots", []int{8, 8, 8}, []int{4, 4, 4}, slots(12, 6, 3, 0), 12},
		{"mixed host sizes", []int{8, 8, 4}, []int{2, 0, 1}, slots(17, 8, 3, 1), 17},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodes []Node
			alloc := Allocations{}
			for i, g := range tt.hostGPUs {
				name := fmt.Sprintf("h%d", i)
				nodes = append(nodes, gpuNode(name, rack, g, nvidia))
				alloc[name] = map[string]int{nvidia: tt.used[i]}
			}
			lvl := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc).Resources[0].Levels[0]
			assert.Equal(t, tt.rack, lvl.LargestFreeSlots)
			assert.Equal(t, tt.largest, lvl.LargestFreeGPUs)
		})
	}
}

func TestRepeatedRackNamesAcrossBlocks(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{
		"block-a": {"rack-1": {0, 0}},
		"block-b": {"rack-1": {4, 4}},
	})
	res := Compute(Topology{Levels: []string{blockLabel, rackLabel, HostnameLabel}}, nodes, alloc)
	levels := res.Resources[0].Levels
	require.Len(t, levels, 3)

	block, rack, host := levels[0], levels[1], levels[2]
	assert.Equal(t, StateCounts{1, 0, 1, 0}, block.Domains)
	assert.Equal(t, StateCounts{1, 0, 1, 0}, rack.Domains, "rack-1 in two blocks must be two racks")
	assert.Equal(t, [][]string{{"block-a", "rack-1"}, {"block-b", "rack-1"}}, paths(rack))
	assert.Equal(t, 8, rack.LargestFreeGPUs)
	assert.Equal(t, slots(8, 4, 2), rack.LargestFreeSlots)

	// Declared hostname level: not implicit.
	assert.Equal(t, HostnameLabel, host.Label)
	assert.False(t, host.Implicit)
	assert.Equal(t, StateCounts{2, 0, 2, 0}, host.Domains)
}

func TestNodeStates(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	with := func(extra map[string]string) map[string]string {
		l := map[string]string{rackLabel: "rack-1"}
		for k, v := range extra {
			l[k] = v
		}
		return l
	}
	// pluginDown: the resource is still in capacity, allocatable is 0.
	pluginDown := func(n Node) Node { n.Allocatable = map[string]int{nvidia: 0}; return n }
	// pluginGone: > 5 min after the plugin died, kubelet removed the resource
	// from capacity and allocatable. Only the presence label is left.
	pluginGone := func(n Node) Node {
		n.Capacity, n.Allocatable = map[string]int{}, map[string]int{}
		n.Labels = with(map[string]string{"nvidia.com/gpu.present": "true"})
		return n
	}
	cordon := func(n Node) Node { n.Unschedulable = true; return n }
	notReady := func(n Node) Node { n.Ready = false; return n }
	taint := func(key string, effect string) func(Node) Node {
		return func(n Node) Node {
			n.Taints = append(n.Taints, Taint{Key: key, Value: "present", Effect: effect})
			return n
		}
	}
	noLabels := func(n Node) Node { n.Labels = map[string]string{}; return n }
	migMixed := func(n Node) Node { n.Capacity["nvidia.com/mig-1g.5gb"] = 7; return n }
	labels := func(extra map[string]string) func(Node) Node {
		return func(n Node) Node { n.Labels = with(extra); return n }
	}

	tests := []struct {
		name   string
		modify []func(Node) Node
		want   NodeState
		attr   string
	}{
		{"ready and schedulable", nil, NodeInTopology, nvidia},
		{"cordoned", []func(Node) Node{cordon}, NodeUnavailable, nvidia},
		{"not ready", []func(Node) Node{notReady}, NodeUnavailable, nvidia},
		{"untolerated taint", []func(Node) Node{taint("other", EffectNoExecute)}, NodeTainted, nvidia},
		{"flavor toleration", []func(Node) Node{taint("flavor-taint", EffectNoSchedule)}, NodeInTopology, nvidia},
		{"assumed toleration", []func(Node) Node{taint("assumed-taint", EffectNoSchedule)}, NodeInTopology, nvidia},
		{"GKE nvidia.com/gpu=present taint", []func(Node) Node{taint(nvidia, EffectNoSchedule)}, NodeInTopology, nvidia},
		{"PreferNoSchedule ignored", []func(Node) Node{taint("other", EffectPreferNoSchedule)}, NodeInTopology, nvidia},
		{"missing labels", []func(Node) Node{noLabels}, NodeMissingLabels, nvidia},
		{"device plugin down", []func(Node) Node{pluginDown}, NodeNoGPUAllocatable, nvidia},
		{"device plugin removed > 5 min", []func(Node) Node{pluginGone}, NodeNoGPUAllocatable, nvidia},

		{"MIG strategy none", []func(Node) Node{labels(map[string]string{"nvidia.com/mig.strategy": "none", labelGPUProduct: "NVIDIA-A100-SXM4-80GB"})}, NodeInTopology, nvidia},
		{"MIG single, MIG off", []func(Node) Node{labels(map[string]string{"nvidia.com/mig.strategy": "single", labelGPUProduct: "NVIDIA-A100-SXM4-80GB"})}, NodeInTopology, nvidia},
		{"MIG single, MIG on", []func(Node) Node{labels(map[string]string{"nvidia.com/mig.strategy": "single", labelGPUProduct: "NVIDIA-A100-SXM4-40GB-MIG-1g.5gb"})}, NodeUnsupported, GPUResourceMIG},
		{"MIG mixed", []func(Node) Node{labels(map[string]string{"nvidia.com/mig.strategy": "mixed"}), migMixed}, NodeUnsupported, GPUResourceMIG},

		// Precedence: unsupported > missing_labels > unavailable > no_gpu_allocatable > tainted.
		{"unsupported over missing labels", []func(Node) Node{migMixed, noLabels}, NodeUnsupported, GPUResourceMIG},
		{"missing labels over unavailable", []func(Node) Node{noLabels, cordon}, NodeMissingLabels, nvidia},
		{"unavailable over no allocatable", []func(Node) Node{pluginDown, cordon}, NodeUnavailable, nvidia},
		{"no allocatable over tainted", []func(Node) Node{pluginDown, taint("other", EffectNoSchedule)}, NodeNoGPUAllocatable, nvidia},
	}
	topo := Topology{
		Levels:             []string{rackLabel},
		Tolerations:        []Toleration{{Key: "flavor-taint", Operator: OpExists}},
		AssumedTolerations: []Toleration{{Key: "assumed-taint", Operator: OpExists, Effect: EffectNoSchedule}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := gpuNode("n", rack, 4, nvidia)
			for _, m := range tt.modify {
				n = m(n)
			}
			res := Compute(topo, []Node{n}, Allocations{"n": {nvidia: 1}})
			assert.Equal(t, []NodeCount{{State: tt.want, GPUResource: tt.attr, Count: 1}}, res.Nodes)

			if tt.want == NodeUnsupported {
				assert.Empty(t, res.Resources, "unsupported nodes are only counted")
				return
			}
			require.Len(t, res.Resources, 1)
			rack := res.Resources[0].Levels[0]
			switch tt.want {
			case NodeInTopology:
				assert.Equal(t, StateCounts{0, 1, 0, 0}, rack.Domains)
			case NodeMissingLabels:
				assert.Empty(t, rack.DomainStats, "missing_labels nodes have no domain")
			default:
				assert.Equal(t, StateCounts{0, 0, 0, 1}, rack.Domains, "labelled nodes without capacity make unavailable domains")
			}
		})
	}
}

func TestNoGPUEvidenceIgnored(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	nodes := []Node{
		{Name: "cpu-only", Labels: rack, Ready: true, Capacity: map[string]int{"example.com/foo": 1}},
		{Name: "gpu-present-false", Labels: map[string]string{rackLabel: "rack-1", "nvidia.com/gpu.present": "false"}, Ready: true},
	}
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, nil)
	assert.Empty(t, res.Nodes)
	assert.Empty(t, res.Resources)
}

func TestFullyDownRack(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {1, 0, 0}, "rack-2": {0, 0, 0}}})
	for i := range nodes {
		if nodes[i].Labels[rackLabel] == "rack-2" {
			nodes[i].Unschedulable = true // e.g. GPU operator upgrade
		}
	}
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)
	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: nvidia, Count: 3},
		{State: NodeUnavailable, GPUResource: nvidia, Count: 3},
	}, res.Nodes)

	rr := res.Resources[0]
	rack, host := rr.Levels[0], rr.Levels[1]
	assert.Equal(t, StateCounts{0, 1, 0, 1}, rack.Domains, "the down rack is unavailable, not free")
	assert.Equal(t, StateCounts{2, 1, 0, 3}, host.Domains)
	assert.Equal(t, 11, rack.LargestFreeGPUs)
	assert.Equal(t, Domain{Path: []string{"rack-2"}, Hosts: 3, FreeSlots: slots(0, 0, 0), State: DomainUnavailable}, rack.DomainStats[1])
	assert.Equal(t, 12, rr.UnschedulableGPUs[ReasonNodeUnavailable])
}

func TestUnschedulableGPUs(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	degraded := gpuNode("degraded", rack, 8, nvidia)
	degraded.Allocatable[nvidia] = 6
	cordoned := gpuNode("cordoned", rack, 8, nvidia)
	cordoned.Unschedulable = true
	tainted := gpuNode("tainted", rack, 4, nvidia)
	tainted.Taints = []Taint{{Key: "x", Effect: EffectNoSchedule}}
	pluginDown := gpuNode("plugin-down", rack, 8, nvidia)
	pluginDown.Allocatable[nvidia] = 0
	mig := gpuNode("mig", rack, 7, nvidia)
	mig.Labels = map[string]string{rackLabel: "rack-1", labelGPUProduct: "NVIDIA-A100-SXM4-40GB-MIG-1g.5gb"}
	nodes := []Node{
		degraded,
		gpuNode("healthy", rack, 8, nvidia),
		cordoned,
		tainted,
		pluginDown,
		gpuNode("no-labels", map[string]string{}, 2, nvidia),
		mig,
	}
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, Allocations{"degraded": {nvidia: 6}})
	rr := res.Resources[0]
	want := ReasonCounts{}
	want[ReasonUnhealthy] = 2
	want[ReasonNodeUnavailable] = 8
	want[ReasonTainted] = 4
	want[ReasonNoAllocatable] = 8
	want[ReasonMissingLabels] = 2
	assert.Equal(t, want, rr.UnschedulableGPUs, "MIG GPUs are not counted")

	host := rr.Levels[1]
	assert.Equal(t, StateCounts{1, 0, 1, 3}, host.Domains, "degraded host is full on its allocatable")
	assert.Equal(t, 14, rr.Levels[0].DomainStats[0].Allocatable)
}

func TestTolerations(t *testing.T) {
	taint := Taint{Key: "k", Value: "v", Effect: EffectNoSchedule}
	tests := []struct {
		tol  Toleration
		want bool
	}{
		{Toleration{Operator: OpExists}, true},
		{Toleration{}, false},
		{Toleration{Key: "k", Operator: OpExists}, true},
		{Toleration{Key: "k", Value: "v"}, true},
		{Toleration{Key: "k", Operator: OpEqual, Value: "v", Effect: EffectNoSchedule}, true},
		{Toleration{Key: "k", Operator: OpEqual, Value: "other"}, false},
		{Toleration{Key: "other", Operator: OpExists}, false},
		{Toleration{Key: "k", Operator: OpExists, Effect: EffectNoExecute}, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.tol.tolerates(taint), "%+v", tt.tol)
	}
}

func TestOvercommit(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {6, 1}}})
	alloc["b-rack-1-h1"][nvidia] = -3 // bogus input is clamped too
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)

	assert.Equal(t, []Overcommit{{Node: "b-rack-1-h0", Resource: nvidia, Allocatable: 4, Allocated: 6}}, res.Overcommits)
	rack, host := res.Resources[0].Levels[0], res.Resources[0].Levels[1]
	assert.Equal(t, StateCounts{0, 1, 0, 0}, rack.Domains)
	assert.Equal(t, StateCounts{0, 4, 0, 0}, rack.FreeGPUs)
	assert.Equal(t, StateCounts{0, 4, 0, 0}, rack.AllocatedGPUs, "allocated is clamped at allocatable")
	assert.Equal(t, StateCounts{1, 0, 1, 0}, host.Domains)
}

func TestMixedVendors(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	both := gpuNode("n1", rack, 4, nvidia)
	both.Capacity[amd], both.Allocatable[amd] = 8, 8
	nodes := []Node{
		both,
		gpuNode("n2", rack, 4, nvidia),
		gpuNode("n3", map[string]string{}, 8, amd),
	}
	alloc := Allocations{"n1": {nvidia: 4}, "n2": {amd: 99}} // n2 has no amd GPUs: ignored
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)

	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: amd, Count: 1},
		{State: NodeInTopology, GPUResource: nvidia, Count: 2},
		{State: NodeMissingLabels, GPUResource: amd, Count: 1},
	}, res.Nodes)
	require.Len(t, res.Resources, 2)
	a, n := res.Resources[0], res.Resources[1]
	assert.Equal(t, amd, a.Resource)
	assert.Equal(t, nvidia, n.Resource)

	assert.Equal(t, StateCounts{1, 0, 0, 0}, a.Levels[0].Domains, "nvidia usage must not make amd look used")
	assert.Equal(t, 8, a.Levels[0].LargestFreeGPUs)
	assert.Equal(t, slots(8, 4, 2, 1), a.Levels[0].LargestFreeSlots)
	assert.Equal(t, 8, a.UnschedulableGPUs[ReasonMissingLabels])

	assert.Equal(t, StateCounts{0, 1, 0, 0}, n.Levels[0].Domains)
	assert.Equal(t, StateCounts{1, 0, 1, 0}, n.Levels[1].Domains)
	assert.Equal(t, slots(4, 2, 1), n.Levels[0].LargestFreeSlots)
	assert.Empty(t, res.Overcommits)
}

func TestEmpty(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {1}}})

	res := Compute(Topology{Name: "t"}, nodes, alloc)
	assert.Equal(t, Result{Topology: Topology{Name: "t"}}, res, "no levels: nothing computed")

	res = Compute(Topology{Levels: []string{rackLabel}}, nil, nil)
	assert.Empty(t, res.Nodes, "no nodes: nothing computed")
	assert.Empty(t, res.Resources)
}

func TestSingleLevel(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {0, 2, 4}}})
	res := Compute(Topology{Levels: []string{HostnameLabel}}, nodes, alloc)

	levels := res.Resources[0].Levels
	require.Len(t, levels, 1)
	assert.Equal(t, LevelResult{
		Label:            HostnameLabel,
		Domains:          StateCounts{1, 1, 1, 0},
		FreeGPUs:         StateCounts{4, 2, 0, 0},
		AllocatedGPUs:    StateCounts{0, 2, 4, 0},
		LargestFreeGPUs:  4,
		LargestFreeSlots: slots(4, 2, 1),
	}, summary(levels[0]))
}

func TestDeterministicOrder(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{
		"block-a": {"rack-1": {0, 1}, "rack-2": {4, 4}, "rack-3": {9, 0}},
		"block-b": {"rack-1": {2, 2}, "rack-2": {0, 0}},
	})
	for i := range nodes {
		nodes[i].Capacity[amd], nodes[i].Allocatable[amd] = 2, 2
	}
	nodes[0].Unschedulable = true
	topo := Topology{Levels: []string{blockLabel, rackLabel}}
	want := Compute(topo, nodes, alloc)

	rng := rand.New(rand.NewSource(1))
	for range 20 {
		rng.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
		require.Equal(t, want, Compute(topo, nodes, alloc))
	}
	assert.Equal(t, [][]string{
		{"block-a", "rack-1"}, {"block-a", "rack-2"}, {"block-a", "rack-3"},
		{"block-b", "rack-1"}, {"block-b", "rack-2"},
	}, paths(want.Resources[0].Levels[1]))
}

func TestStateStrings(t *testing.T) {
	var got []string
	for _, s := range DomainStates {
		got = append(got, s.String())
	}
	assert.Equal(t, []string{"free", "partial", "full", "unavailable"}, got)

	got = nil
	for _, s := range NodeStates {
		got = append(got, s.String())
	}
	assert.Equal(t, []string{"in_topology", "unavailable", "tainted", "no_gpu_allocatable", "missing_labels", "unsupported"}, got)

	got = nil
	for _, r := range UnschedulableReasons {
		got = append(got, r.String())
	}
	assert.Equal(t, []string{"unhealthy", "node_unavailable", "tainted", "no_allocatable", "missing_labels"}, got)
}
