// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package engine

import (
	"fmt"
	"math/rand"
	"strings"
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
		out[i] = Slots{PodGPUs: PodSizes()[i], Count: c}
	}
	return out
}

// summary is a LevelResult without the per-domain stats, to keep tables short.
func summary(l LevelResult) LevelResult {
	l.DomainStats = nil
	return l
}

func nonZero(counts []NodeCount) []NodeCount {
	var out []NodeCount
	for _, c := range counts {
		if c.Count > 0 {
			out = append(out, c)
		}
	}
	return out
}

func withoutType(counts []NodeCount) []NodeCount {
	for i := range counts {
		counts[i].GPUType = ""
	}
	return counts
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

			assert.Equal(t, []NodeCount{{State: NodeInTopology, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: len(nodes)}}, nonZero(res.Nodes))
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
		{Path: []string{"rack-1"}, ID: "rack-1", Hosts: 3, Allocatable: 12, Allocated: 9, Free: 3, FreeSlots: slots(3, 1, 0), State: DomainPartiallyOccupied},
		{Path: []string{"rack-2"}, ID: "rack-2", Hosts: 3, Allocatable: 12, Allocated: 0, Free: 12, FreeSlots: slots(12, 6, 3), State: DomainFree},
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

func TestSlotSizesIndependentOfHealth(t *testing.T) {
	// All nodes cordoned: sizes stay, with 0 slots.
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {0, 0}}})
	for i := range nodes {
		nodes[i].Unschedulable = true
	}
	lvl := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc).Resources[0].Levels[0]
	assert.Equal(t, slots(0, 0, 0), lvl.LargestFreeSlots)

	// 8-GPU hosts with one unhealthy GPU each: the 8-GPU size is still reported.
	rack := map[string]string{rackLabel: "rack-1"}
	var degraded []Node
	for i := range 2 {
		n := gpuNode(fmt.Sprintf("h%d", i), rack, 8, nvidia)
		n.Allocatable[nvidia] = 7
		degraded = append(degraded, n)
	}
	lvl = Compute(Topology{Levels: []string{rackLabel}}, degraded, nil).Resources[0].Levels[0]
	assert.Equal(t, slots(14, 6, 2, 0), lvl.LargestFreeSlots)
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
	shared := func(n Node) Node {
		n.Capacity = map[string]int{GPUResourceShared: 16}
		n.Allocatable = map[string]int{GPUResourceShared: 16}
		n.Labels = with(map[string]string{"nvidia.com/gpu.present": "true"})
		return n
	}
	capacity := func(c int) func(Node) Node {
		return func(n Node) Node { n.Capacity[nvidia], n.Allocatable[nvidia] = c, c; return n }
	}
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
		{"time-slicing renamed to gpu.shared", []func(Node) Node{shared}, NodeUnsupported, GPUResourceShared},
		{"default time-slicing: 8 GPUs x 4 replicas", []func(Node) Node{labels(map[string]string{"nvidia.com/gpu.replicas": "4", "nvidia.com/gpu.sharing-strategy": "time-slicing", labelGPUProduct: "NVIDIA-H100-80GB-HBM3-SHARED"}), capacity(32)}, NodeUnsupported, GPUResourceShared},
		{"replicas > 1 only", []func(Node) Node{labels(map[string]string{"nvidia.com/gpu.replicas": "2"})}, NodeUnsupported, GPUResourceShared},
		{"MPS", []func(Node) Node{labels(map[string]string{"nvidia.com/gpu.sharing-strategy": "mps"})}, NodeUnsupported, GPUResourceShared},
		{"-SHARED product only", []func(Node) Node{labels(map[string]string{labelGPUProduct: "Tesla-T4-SHARED"})}, NodeUnsupported, GPUResourceShared},
		{"sharing-strategy none, replicas 1", []func(Node) Node{labels(map[string]string{"nvidia.com/gpu.sharing-strategy": "none", "nvidia.com/gpu.replicas": "1", labelGPUProduct: "NVIDIA-H100-80GB-HBM3"})}, NodeInTopology, nvidia},
		{"MIG checked before sharing", []func(Node) Node{labels(map[string]string{"nvidia.com/gpu.replicas": "2"}), migMixed}, NodeUnsupported, GPUResourceMIG},
		{"gpu.shared with plugin down", []func(Node) Node{shared, func(n Node) Node { n.Allocatable = map[string]int{}; return n }}, NodeUnsupported, GPUResourceShared},

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
			assert.Equal(t, []NodeCount{{State: tt.want, GPUResource: tt.attr, Count: 1}}, withoutType(nonZero(res.Nodes)))

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
		switch nodes[i].Name {
		case "b-rack-2-h0":
			nodes[i].Unschedulable = true // e.g. GPU operator upgrade
		case "b-rack-2-h1":
			nodes[i].Taints = []Taint{{Key: "maintenance", Effect: EffectNoExecute}}
		case "b-rack-2-h2":
			nodes[i].Allocatable[nvidia] = 0
		}
	}
	alloc["b-rack-2-h0"][nvidia] = 2 // usage on down nodes is ignored
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)
	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 3},
		{State: NodeUnavailable, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 1},
		{State: NodeTainted, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 1},
		{State: NodeNoGPUAllocatable, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 1},
	}, nonZero(res.Nodes))

	rr := res.Resources[0]
	rack, host := rr.Levels[0], rr.Levels[1]
	assert.Equal(t, StateCounts{0, 1, 0, 1}, rack.Domains, "the down rack is unavailable, not free")
	assert.Equal(t, StateCounts{2, 1, 0, 3}, host.Domains)
	assert.Equal(t, 11, rack.LargestFreeGPUs)
	assert.Equal(t, StateCounts{0, 11, 0, 0}, rack.FreeGPUs, "down nodes add nothing")
	assert.Equal(t, StateCounts{0, 1, 0, 0}, rack.AllocatedGPUs)
	assert.Equal(t, Domain{Path: []string{"rack-2"}, ID: "rack-2", Hosts: 3, Unschedulable: 12, FreeSlots: slots(0, 0, 0), State: DomainUnavailable}, rack.DomainStats[1])
	assert.Equal(t, 4, rr.UnschedulableGPUs[ReasonNodeUnavailable])
	assert.Equal(t, 4, rr.UnschedulableGPUs[ReasonTainted])
	assert.Equal(t, 4, rr.UnschedulableGPUs[ReasonNoAllocatable])
}

func TestDegradedDomains(t *testing.T) {
	// Rack of 3 hosts, 1 cordoned, nothing allocated: free and degraded.
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {0, 0, 0}, "rack-2": {0, 0}, "rack-3": {0, 0}}})
	for i := range nodes {
		switch nodes[i].Name {
		case "b-rack-1-h0":
			nodes[i].Unschedulable = true
		case "b-rack-2-h0":
			nodes[i].Allocatable[nvidia] = 3 // one unhealthy GPU
		}
	}
	alloc["b-rack-2-h1"][nvidia] = 1
	res := Compute(Topology{Levels: []string{blockLabel, rackLabel}}, nodes, alloc)
	block, rack, host := res.Resources[0].Levels[0], res.Resources[0].Levels[1], res.Resources[0].Levels[2]

	assert.Equal(t, StateCounts{2, 1, 0, 0}, rack.Domains)
	assert.Equal(t, StateCounts{1, 1, 0, 0}, rack.DegradedDomains, "rack-1 free+degraded, rack-2 partial+degraded")
	assert.Equal(t, []Domain{
		{Path: []string{"b", "rack-1"}, ID: "b/rack-1", Hosts: 3, Allocatable: 8, Free: 8, Unschedulable: 4, FreeSlots: slots(8, 4, 2), State: DomainFree, Degraded: true},
		{Path: []string{"b", "rack-2"}, ID: "b/rack-2", Hosts: 2, Allocatable: 7, Allocated: 1, Free: 6, Unschedulable: 1, FreeSlots: slots(6, 2, 0), State: DomainPartiallyOccupied, Degraded: true},
		{Path: []string{"b", "rack-3"}, ID: "b/rack-3", Hosts: 2, Allocatable: 8, Free: 8, FreeSlots: slots(8, 4, 2), State: DomainFree},
	}, rack.DomainStats)
	assert.Equal(t, StateCounts{0, 1, 0, 0}, block.DegradedDomains)
	assert.Equal(t, 5, block.DomainStats[0].Unschedulable)

	// Hosts: the cordoned one is unavailable (never degraded), the
	// unhealthy one is free and degraded.
	assert.Equal(t, StateCounts{5, 1, 0, 1}, host.Domains)
	assert.Equal(t, StateCounts{1, 0, 0, 0}, host.DegradedDomains)
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
	assert.Equal(t, StateCounts{1, 0, 1, 3}, host.Domains, "degraded host is fully occupied on its allocatable")
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

	assert.True(t, Toleration{Key: "k", Operator: OpExists}.tolerates(Taint{Key: "k", Effect: EffectNoExecute}), "empty effect matches all")
	assert.False(t, Toleration{Key: "k", Operator: "Lt", Value: "v"}.tolerates(taint), "unknown operator")
	two := []Taint{taint, {Key: "other", Effect: EffectNoSchedule}}
	assert.False(t, tolerated(two, []Toleration{{Key: "k", Operator: OpExists}}), "every taint must be tolerated")
	assert.True(t, tolerated(two, []Toleration{{Key: "k", Operator: OpExists}, {Key: "other", Operator: OpExists}}))
}

func TestStateOf(t *testing.T) {
	assert.Equal(t, DomainUnavailable, stateOf(Domain{}))
	assert.Equal(t, DomainFullyOccupied, stateOf(Domain{Allocatable: 4, Allocated: 4}))
	assert.Equal(t, DomainFree, stateOf(Domain{Allocatable: 4, Free: 4}))
	assert.Equal(t, DomainPartiallyOccupied, stateOf(Domain{Allocatable: 4, Allocated: 1, Free: 3}))
}

func TestNodeCountsIncludeZeros(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {0}}})
	mig := gpuNode("mig", map[string]string{rackLabel: "rack-1"}, 7, "nvidia.com/mig-1g.5gb")
	res := Compute(Topology{Levels: []string{rackLabel}}, append(nodes, mig), alloc)
	var want []NodeCount
	for _, s := range NodeStates() {
		for _, r := range []string{GPUResourceMIG, nvidia} {
			want = append(want, NodeCount{State: s, GPUResource: r, GPUType: GPUTypeUnknown})
		}
	}
	want[1].Count = 1                      // in_topology, nvidia
	want[2*int(NodeUnsupported)].Count = 1 // unsupported, mig
	assert.Equal(t, want, res.Nodes)
}

func TestDeclaredHostnameLabelDiffersFromNodeName(t *testing.T) {
	n := gpuNode("ip-10-0-0-1.ec2.internal", map[string]string{rackLabel: "rack-1", HostnameLabel: "ip-10-0-0-1"}, 4, nvidia)
	res := Compute(Topology{Levels: []string{rackLabel, HostnameLabel}}, []Node{n}, nil)
	host := res.Resources[0].Levels[1]
	assert.False(t, host.Implicit)
	assert.Equal(t, [][]string{{"rack-1", "ip-10-0-0-1"}}, paths(host))

	res = Compute(Topology{Levels: []string{rackLabel}}, []Node{n}, nil)
	assert.Equal(t, [][]string{{"rack-1", "ip-10-0-0-1.ec2.internal"}}, paths(res.Resources[0].Levels[1]), "implicit level uses the node name")
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
		{State: NodeInTopology, GPUResource: amd, GPUType: GPUTypeUnknown, Count: 1},
		{State: NodeInTopology, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 2},
		{State: NodeMissingLabels, GPUResource: amd, GPUType: GPUTypeUnknown, Count: 1},
	}, nonZero(res.Nodes))
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

func TestOneVendorPluginDown(t *testing.T) {
	n := gpuNode("n", map[string]string{rackLabel: "rack-1"}, 4, nvidia)
	n.Capacity[amd], n.Allocatable[amd] = 8, 0
	res := Compute(Topology{Levels: []string{rackLabel}}, []Node{n}, nil)
	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 1},
		{State: NodeNoGPUAllocatable, GPUResource: amd, GPUType: GPUTypeUnknown, Count: 1},
	}, nonZero(res.Nodes))
	require.Len(t, res.Resources, 2)
	assert.Equal(t, StateCounts{0, 0, 0, 1}, res.Resources[0].Levels[0].Domains, "amd")
	assert.Equal(t, 8, res.Resources[0].UnschedulableGPUs[ReasonNoAllocatable])
	assert.Equal(t, StateCounts{1, 0, 0, 0}, res.Resources[1].Levels[0].Domains, "nvidia")
}

func TestGPUTypes(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	product := func(p string) map[string]string {
		return map[string]string{rackLabel: "rack-1", labelGPUProduct: p}
	}
	nodes := []Node{
		gpuNode("h100-a", product("NVIDIA-H100-80GB-HBM3"), 8, nvidia),
		gpuNode("h100-b", product("NVIDIA-H100-80GB-HBM3"), 8, nvidia),
		gpuNode("a100", product("NVIDIA-A100-SXM4-80GB"), 8, nvidia),
		gpuNode("no-label", rack, 4, nvidia),
		gpuNode("garbage", product("something-else"), 4, nvidia),
	}
	alloc := Allocations{"h100-a": {nvidia: 8}, "a100": {nvidia: 2}}
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)

	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: nvidia, GPUType: "a100", Count: 1},
		{State: NodeInTopology, GPUResource: nvidia, GPUType: "h100", Count: 2},
		{State: NodeInTopology, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 2},
	}, nonZero(res.Nodes))

	require.Len(t, res.Resources, 3)
	var types []string
	for _, rr := range res.Resources {
		assert.Equal(t, nvidia, rr.Resource)
		types = append(types, rr.GPUType)
	}
	assert.Equal(t, []string{"a100", "h100", GPUTypeUnknown}, types)

	// The mixed rack is one rack per type.
	a100, h100, unknown := res.Resources[0].Levels[0], res.Resources[1].Levels[0], res.Resources[2].Levels[0]
	assert.Equal(t, []Domain{{Path: []string{"rack-1"}, ID: "rack-1", Hosts: 1, Allocatable: 8, Allocated: 2, Free: 6, FreeSlots: slots(6, 3, 1, 0), State: DomainPartiallyOccupied}}, a100.DomainStats)
	assert.Equal(t, []Domain{{Path: []string{"rack-1"}, ID: "rack-1", Hosts: 2, Allocatable: 16, Allocated: 8, Free: 8, FreeSlots: slots(8, 4, 2, 1), State: DomainPartiallyOccupied}}, h100.DomainStats)
	assert.Equal(t, StateCounts{1, 0, 0, 0}, unknown.Domains)
	assert.Equal(t, slots(8, 4, 2), unknown.LargestFreeSlots, "pod sizes are bounded per type")
	assert.Equal(t, StateCounts{1, 0, 1, 0}, res.Resources[1].Levels[1].Domains, "h100 hosts")

	// Unschedulable GPUs and pod sizes are per type too.
	cordoned := gpuNode("a100-cordoned", product("NVIDIA-A100-SXM4-80GB"), 8, nvidia)
	cordoned.Unschedulable = true
	small := gpuNode("t4", product("Tesla-T4"), 1, nvidia)
	res = Compute(Topology{Levels: []string{rackLabel}}, append(nodes, cordoned, small), alloc)
	require.Len(t, res.Resources, 4)
	assert.Equal(t, "a100", res.Resources[0].GPUType)
	assert.Equal(t, 8, res.Resources[0].UnschedulableGPUs[ReasonNodeUnavailable])
	assert.Equal(t, 0, res.Resources[1].UnschedulableGPUs[ReasonNodeUnavailable], "h100")
	assert.Equal(t, "t4", res.Resources[2].GPUType)
	assert.Equal(t, slots(1), res.Resources[2].Levels[0].LargestFreeSlots, "t4 hosts bound their own sizes")
}

func TestGPUTypeSources(t *testing.T) {
	const (
		product     = labelGPUProduct
		gke         = "cloud.google.com/gke-accelerator"
		karpenter   = "karpenter.k8s.aws/instance-gpu-name"
		manufactur  = "karpenter.k8s.aws/instance-gpu-manufacturer"
		amdProduct  = "amd.com/gpu.product-name"
		intelXe     = "gpu.intel.com/xe"
		amdPresence = "feature.node.kubernetes.io/amd-gpu"
	)
	l := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	tests := []struct {
		name     string
		resource string
		labels   map[string]string
		want     string
	}{
		// NVIDIA GPU Feature Discovery.
		{"GFD H100", nvidia, l(product, "NVIDIA-H100-80GB-HBM3"), "h100"},
		{"GFD A100", nvidia, l(product, "NVIDIA-A100-SXM4-80GB"), "a100"},
		{"GFD A10G variant", nvidia, l(product, "NVIDIA-A10G"), "a10"},
		{"GFD T4", nvidia, l(product, "Tesla-T4"), "t4"},
		{"GFD RTX 3090", nvidia, l(product, "NVIDIA-GeForce-RTX-3090"), "rtx_3090"},
		{"GFD -SHARED gives the base type", nvidia, l(product, "NVIDIA-H100-80GB-HBM3-SHARED"), "h100"},
		{"GFD on a MIG node", GPUResourceMIG, l(product, "NVIDIA-A100-SXM4-40GB-MIG-1g.5gb"), "a100"},
		// GKE.
		{"GKE nvidia-tesla-a100", nvidia, l(gke, "nvidia-tesla-a100"), "a100"},
		{"GKE nvidia-tesla-t4", nvidia, l(gke, "nvidia-tesla-t4"), "t4"},
		{"GKE nvidia-h100-80gb", nvidia, l(gke, "nvidia-h100-80gb"), "h100"},
		{"GKE nvidia-h100-mega-80gb", nvidia, l(gke, "nvidia-h100-mega-80gb"), "h100"},
		{"GKE nvidia-a100-80gb", nvidia, l(gke, "nvidia-a100-80gb"), "a100"},
		{"GKE nvidia-l4", nvidia, l(gke, "nvidia-l4"), "l4"},
		{"GKE l40s joins the node check's l40", nvidia, l(gke, "nvidia-l40s"), "l40"},
		// EKS Karpenter, NVIDIA manufacturer only.
		{"Karpenter h100", nvidia, l(karpenter, "h100", manufactur, "nvidia"), "h100"},
		{"Karpenter t4", nvidia, l(karpenter, "t4", manufactur, "nvidia"), "t4"},
		{"Karpenter a10g joins the node check's a10", nvidia, l(karpenter, "a10g", manufactur, "nvidia"), "a10"},
		{"Karpenter l40s joins the node check's l40", nvidia, l(karpenter, "l40s", manufactur, "nvidia"), "l40"},
		{"Karpenter without manufacturer ignored", nvidia, l(karpenter, "h100"), GPUTypeUnknown},
		{"Karpenter non-NVIDIA ignored", nvidia, l(karpenter, "radeon-pro-v520", manufactur, "amd"), GPUTypeUnknown},
		// AMD node labeller.
		{"AMD MI300X", amd, l(amdProduct, "AMD_Instinct_MI300X"), "mi300x"},
		{"AMD OAM", amd, l(amdProduct, "AMD_Instinct_MI300X_OAM"), "mi300x"},
		{"AMD without vendor prefix", amd, l(amdProduct, "Instinct_MI210"), "mi210"},
		{"AMD node without product-name label", amd, l(amdPresence, "true"), GPUTypeUnknown},
		// Sources are picked by the resource's vendor.
		{"NVIDIA label ignored for amd.com/gpu", amd, l(product, "NVIDIA-H100-80GB-HBM3"), GPUTypeUnknown},
		{"AMD label ignored for nvidia.com/gpu", nvidia, l(amdProduct, "AMD_Instinct_MI300X"), GPUTypeUnknown},
		{"Intel is always unknown", intelXe, l(product, "NVIDIA-H100-80GB-HBM3"), GPUTypeUnknown},
		// Order and fallbacks.
		{"GFD wins over GKE", nvidia, l(product, "NVIDIA-A100-SXM4-80GB", gke, "nvidia-h100-80gb"), "a100"},
		{"unparseable falls through", nvidia, l(product, "garbage", karpenter, "l4", manufactur, "nvidia"), "l4"},
		{"none", nvidia, l(), GPUTypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, gpuType(Node{Labels: tt.labels}, tt.resource))
		})
	}
}

func TestMixedVendorNodeGPUTypes(t *testing.T) {
	n := gpuNode("n", map[string]string{rackLabel: "rack-1", labelGPUProduct: "NVIDIA-H100-80GB-HBM3", "amd.com/gpu.product-name": "AMD_Instinct_MI300X"}, 8, nvidia, amd)
	res := Compute(Topology{Levels: []string{rackLabel}}, []Node{n}, nil)
	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: amd, GPUType: "mi300x", Count: 1},
		{State: NodeInTopology, GPUResource: nvidia, GPUType: "h100", Count: 1},
	}, nonZero(res.Nodes))
}

func TestUnsupportedRowsCarryGPUType(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	mig := gpuNode("mig", map[string]string{rackLabel: "rack-1", labelGPUProduct: "NVIDIA-A100-SXM4-40GB-MIG-1g.5gb"}, 7, nvidia)
	shared := gpuNode("shared", map[string]string{rackLabel: "rack-1", labelGPUProduct: "Tesla-T4-SHARED"}, 16, nvidia)
	plain := gpuNode("plain", rack, 4, nvidia)
	res := Compute(Topology{Levels: []string{rackLabel}}, []Node{mig, shared, plain}, nil)
	assert.Equal(t, []NodeCount{
		{State: NodeInTopology, GPUResource: nvidia, GPUType: GPUTypeUnknown, Count: 1},
		{State: NodeUnsupported, GPUResource: GPUResourceMIG, GPUType: "a100", Count: 1},
		{State: NodeUnsupported, GPUResource: GPUResourceShared, GPUType: "t4", Count: 1},
	}, nonZero(res.Nodes))
}

func TestCPUOnlyNodesNeverCreateUnknown(t *testing.T) {
	cpu := Node{Name: "cpu", Labels: map[string]string{rackLabel: "rack-1"}, Ready: true, Capacity: map[string]int{"cpu": 64}}
	h100 := gpuNode("h100", map[string]string{rackLabel: "rack-1", labelGPUProduct: "NVIDIA-H100-80GB-HBM3"}, 8, nvidia)
	res := Compute(Topology{Levels: []string{rackLabel}}, []Node{cpu, h100}, nil)
	require.Len(t, res.Resources, 1)
	assert.Equal(t, "h100", res.Resources[0].GPUType)
	for _, c := range res.Nodes {
		assert.Equal(t, "h100", c.GPUType)
	}

	res = Compute(Topology{Levels: []string{rackLabel}}, []Node{cpu}, nil)
	assert.Empty(t, res.Nodes, "CPU-only flavor: the check emits gpu_resource:none")
	assert.Empty(t, res.Resources)
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
		if strings.HasSuffix(nodes[i].Name, "-h1") {
			nodes[i].Labels[labelGPUProduct] = "NVIDIA-H100-80GB-HBM3" // two GPU types
			nodes[i].Labels["amd.com/gpu.product-name"] = "AMD_Instinct_MI300X"
		}
		if nodes[i].Name == "block-a-rack-1-h0" {
			nodes[i].Unschedulable = true
		}
	}
	topo := Topology{Levels: []string{blockLabel, rackLabel}}
	want := Compute(topo, nodes, alloc)
	var units []string
	for _, rr := range want.Resources {
		units = append(units, rr.Resource+"/"+rr.GPUType)
	}
	assert.Equal(t, []string{amd + "/mi300x", amd + "/unknown", nvidia + "/h100", nvidia + "/unknown"}, units)

	rng := rand.New(rand.NewSource(1))
	for range 20 {
		rng.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
		require.Equal(t, want, Compute(topo, nodes, alloc))
	}
	assert.Equal(t, [][]string{
		{"block-a", "rack-1"}, {"block-a", "rack-2"}, {"block-a", "rack-3"},
		{"block-b", "rack-1"}, {"block-b", "rack-2"},
	}, paths(want.Resources[3].Levels[1]))
}

func TestStateStrings(t *testing.T) {
	var got []string
	for _, s := range DomainStates() {
		got = append(got, s.String())
	}
	assert.Equal(t, []string{"free", "partially_occupied", "fully_occupied", "unavailable"}, got)

	got = nil
	for _, s := range NodeStates() {
		got = append(got, s.String())
	}
	assert.Equal(t, []string{"in_topology", "unavailable", "tainted", "no_gpu_allocatable", "missing_labels", "unsupported"}, got)

	got = nil
	for _, r := range UnschedulableReasons() {
		got = append(got, r.String())
	}
	assert.Equal(t, []string{"unhealthy", "node_unavailable", "tainted", "no_allocatable", "missing_labels"}, got)
}
