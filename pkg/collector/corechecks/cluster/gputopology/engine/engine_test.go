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

func gpus(n int) GPUCount { return GPUCount{Capacity: n, Allocatable: n} }

// cluster builds 4-GPU nvidia nodes. used[block][rack] lists the allocated
// GPUs per host. Nodes are named <block>-<rack>-h<i>.
func cluster(used map[string]map[string][]int) ([]Node, Allocations) {
	var nodes []Node
	alloc := Allocations{}
	for block, racks := range used {
		for rack, hosts := range racks {
			for i, u := range hosts {
				name := fmt.Sprintf("%s-%s-h%d", block, rack, i)
				nodes = append(nodes, Node{
					Name:   name,
					Labels: map[string]string{blockLabel: block, rackLabel: rack, HostnameLabel: name},
					GPUs:   map[string]GPUCount{nvidia: gpus(4)},
					Ready:  true,
				})
				alloc[name] = map[string]int{nvidia: u}
			}
		}
	}
	return nodes, alloc
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
				Domains:          StateCounts{0, 3, 0},
				FreeGPUs:         StateCounts{0, 27, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0},
				LargestFreeGPUs:  9,
				LargestFreeSlots: []int{9, 3, 0, 0},
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{0, 9, 0},
				FreeGPUs:         StateCounts{0, 27, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0},
				LargestFreeGPUs:  3,
				LargestFreeSlots: []int{3, 1, 0, 0},
			},
		},
		{
			name: "3 racks, after re-packing",
			used: map[string][]int{"rack-1": {4, 4, 1}, "rack-2": {0, 0, 0}, "rack-3": {0, 0, 0}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{2, 1, 0},
				FreeGPUs:         StateCounts{24, 3, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0},
				LargestFreeGPUs:  12,
				LargestFreeSlots: []int{12, 6, 3, 0},
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{6, 1, 2},
				FreeGPUs:         StateCounts{24, 3, 0},
				AllocatedGPUs:    StateCounts{0, 1, 8},
				LargestFreeGPUs:  4,
				LargestFreeSlots: []int{4, 2, 1, 0},
			},
		},
		{
			// Design doc section 4: 2 racks x 3 hosts x 4 GPUs.
			name: "2 racks, before re-packing",
			used: map[string][]int{"rack-1": {2, 1, 2}, "rack-2": {1, 2, 1}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{0, 2, 0},
				FreeGPUs:         StateCounts{0, 15, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0},
				LargestFreeGPUs:  8, // rack-2: 12 - 4
				LargestFreeSlots: []int{8, 3, 0, 0},
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{0, 6, 0},
				FreeGPUs:         StateCounts{0, 15, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0},
				LargestFreeGPUs:  3,
				LargestFreeSlots: []int{3, 1, 0, 0},
			},
		},
		{
			name: "2 racks, after re-packing",
			used: map[string][]int{"rack-1": {4, 4, 1}, "rack-2": {0, 0, 0}},
			rack: LevelResult{
				Label: rackLabel, Depth: 0,
				Domains:          StateCounts{1, 1, 0},
				FreeGPUs:         StateCounts{12, 3, 0},
				AllocatedGPUs:    StateCounts{0, 9, 0},
				LargestFreeGPUs:  12,
				LargestFreeSlots: []int{12, 6, 3, 0},
			},
			host: LevelResult{
				Label: HostnameLabel, Depth: 1, Implicit: true,
				Domains:          StateCounts{3, 1, 2},
				FreeGPUs:         StateCounts{12, 3, 0},
				AllocatedGPUs:    StateCounts{0, 1, 8},
				LargestFreeGPUs:  4,
				LargestFreeSlots: []int{4, 2, 1, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, alloc := cluster(map[string]map[string][]int{"b": tt.used})
			res := Compute(topo, nodes, alloc)

			require.Len(t, res.Resources, 1)
			rr := res.Resources[0]
			assert.Equal(t, nvidia, rr.Resource)
			assert.Equal(t, NodeCoverage{InTopology: len(nodes)}, rr.Nodes)
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
		{Path: []string{"rack-1"}, Hosts: 3, Allocatable: 12, Allocated: 9, Free: 3, FreeSlots: []int{3, 1, 0, 0}, State: DomainPartial},
		{Path: []string{"rack-2"}, Hosts: 3, Allocatable: 12, Allocated: 0, Free: 12, FreeSlots: []int{12, 6, 3, 0}, State: DomainFree},
	}, res.Resources[0].Levels[0].DomainStats)
}

func TestFreeGPUsButNoWholeHosts(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {1, 1, 1, 1}}})
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)
	rack := res.Resources[0].Levels[0]
	assert.Equal(t, 12, rack.LargestFreeGPUs)
	assert.Equal(t, []int{12, 4, 0, 0}, rack.LargestFreeSlots)
}

func TestSlotsMixedHostSizes(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	nodes := []Node{
		{Name: "big-used", Labels: rack, GPUs: map[string]GPUCount{nvidia: gpus(8)}, Ready: true},
		{Name: "big-free", Labels: rack, GPUs: map[string]GPUCount{nvidia: gpus(8)}, Ready: true},
		{Name: "small", Labels: rack, GPUs: map[string]GPUCount{nvidia: gpus(4)}, Ready: true},
	}
	alloc := Allocations{"big-used": {nvidia: 2}, "small": {nvidia: 1}}
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)
	rackLvl, host := res.Resources[0].Levels[0], res.Resources[0].Levels[1]
	assert.Equal(t, 17, rackLvl.LargestFreeGPUs)
	assert.Equal(t, []int{17, 8, 3, 1}, rackLvl.LargestFreeSlots)
	assert.Equal(t, []int{8, 4, 2, 1}, host.LargestFreeSlots)
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
	assert.Equal(t, StateCounts{1, 0, 1}, block.Domains)
	assert.Equal(t, StateCounts{1, 0, 1}, rack.Domains, "rack-1 in two blocks must be two racks")
	assert.Equal(t, [][]string{{"block-a", "rack-1"}, {"block-b", "rack-1"}}, paths(rack))
	assert.Equal(t, 8, rack.LargestFreeGPUs)
	assert.Equal(t, []int{8, 4, 2, 0}, rack.LargestFreeSlots)

	// Declared hostname level: not implicit.
	assert.Equal(t, HostnameLabel, host.Label)
	assert.False(t, host.Implicit)
	assert.Equal(t, StateCounts{2, 0, 2}, host.Domains)
}

func TestNodeStates(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	noSchedule := Taint{Key: "gpu", Value: "true", Effect: EffectNoSchedule}
	tests := []struct {
		name string
		node Node
		want NodeCoverage
	}{
		{"in topology", Node{Labels: rack, Ready: true}, NodeCoverage{InTopology: 1}},
		{"cordoned", Node{Labels: rack, Ready: true, Unschedulable: true}, NodeCoverage{Unavailable: 1}},
		{"not ready", Node{Labels: rack}, NodeCoverage{Unavailable: 1}},
		{"untolerated taint", Node{Labels: rack, Ready: true, Taints: []Taint{{Key: "other", Effect: EffectNoExecute}}}, NodeCoverage{Tainted: 1}},
		{"tolerated taint", Node{Labels: rack, Ready: true, Taints: []Taint{noSchedule}}, NodeCoverage{InTopology: 1}},
		{"prefer no schedule ignored", Node{Labels: rack, Ready: true, Taints: []Taint{{Key: "x", Effect: EffectPreferNoSchedule}}}, NodeCoverage{InTopology: 1}},
		{"missing labels", Node{Labels: map[string]string{}, Ready: true}, NodeCoverage{MissingLabels: 1}},
		{"unsupported", Node{Labels: rack, Ready: true, Unsupported: true}, NodeCoverage{Unsupported: 1}},
		{"no allocatable", Node{Labels: rack, Ready: true, GPUs: map[string]GPUCount{nvidia: {Capacity: 4}}}, NodeCoverage{NoGPUAllocatable: 1}},

		// Precedence: unsupported > missing_labels > no_gpu_allocatable > unavailable > tainted.
		{"unsupported over missing labels", Node{Unsupported: true}, NodeCoverage{Unsupported: 1}},
		{"missing labels over no allocatable", Node{GPUs: map[string]GPUCount{nvidia: {Capacity: 4}}}, NodeCoverage{MissingLabels: 1}},
		{"no allocatable over unavailable", Node{Labels: rack, GPUs: map[string]GPUCount{nvidia: {Capacity: 4}}}, NodeCoverage{NoGPUAllocatable: 1}},
		{"unavailable over tainted", Node{Labels: rack, Taints: []Taint{{Key: "other", Effect: EffectNoSchedule}}}, NodeCoverage{Unavailable: 1}},
	}
	topo := Topology{Levels: []string{rackLabel}, Tolerations: []Toleration{{Key: "gpu", Operator: OpExists}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := tt.node
			n.Name = "n"
			if n.GPUs == nil {
				n.GPUs = map[string]GPUCount{nvidia: gpus(4)}
			}
			res := Compute(topo, []Node{n}, Allocations{"n": {nvidia: 4}})
			require.Len(t, res.Resources, 1)
			rr := res.Resources[0]
			assert.Equal(t, tt.want, rr.Nodes)
			if tt.want.InTopology == 0 {
				assert.Empty(t, rr.Levels[0].DomainStats, "only in_topology nodes contribute")
			}
		})
	}
}

func TestOutOfScopeNodes(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	nodes := []Node{
		{Name: "cpu-only", Labels: rack, Ready: true},
		{Name: "zero-gpu", Labels: rack, GPUs: map[string]GPUCount{nvidia: {}}, Ready: true},
	}
	assert.Empty(t, Compute(Topology{Levels: []string{rackLabel}}, nodes, nil).Resources)
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

func TestUnavailableGPUs(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	nodes := []Node{
		{Name: "degraded", Labels: rack, GPUs: map[string]GPUCount{nvidia: {Capacity: 8, Allocatable: 6}}, Ready: true},
		{Name: "healthy", Labels: rack, GPUs: map[string]GPUCount{nvidia: gpus(8)}, Ready: true},
		{Name: "plugin-down", Labels: rack, GPUs: map[string]GPUCount{nvidia: {Capacity: 8}}, Ready: true},
		{Name: "cordoned", Labels: rack, GPUs: map[string]GPUCount{nvidia: {Capacity: 8, Allocatable: 4}}, Ready: true, Unschedulable: true},
		{Name: "no-labels", GPUs: map[string]GPUCount{nvidia: {Capacity: 8, Allocatable: 4}}, Ready: true},
	}
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, Allocations{"degraded": {nvidia: 6}})
	assert.Equal(t, 2+8, res.Resources[0].UnavailableGPUs, "degraded + plugin-down only")
	rackLvl, host := res.Resources[0].Levels[0], res.Resources[0].Levels[1]
	assert.Equal(t, StateCounts{0, 2, 0}, rackLvl.UnavailableGPUs)
	assert.Equal(t, StateCounts{0, 0, 2}, host.UnavailableGPUs, "the degraded host is full on its allocatable")
	assert.Equal(t, StateCounts{1, 0, 1}, host.Domains)
}

func TestOvercommit(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {6, 1}}})
	alloc["b-rack-1-h1"][nvidia] = -3 // bogus input is clamped too
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)

	assert.Equal(t, []Overcommit{{Node: "b-rack-1-h0", Resource: nvidia, Allocatable: 4, Allocated: 6}}, res.Overcommits)
	rack, host := res.Resources[0].Levels[0], res.Resources[0].Levels[1]
	assert.Equal(t, StateCounts{0, 1, 0}, rack.Domains)
	assert.Equal(t, StateCounts{0, 4, 0}, rack.FreeGPUs)
	assert.Equal(t, StateCounts{0, 4, 0}, rack.AllocatedGPUs, "allocated is clamped at allocatable")
	assert.Equal(t, StateCounts{1, 0, 1}, host.Domains)
}

func TestMixedVendors(t *testing.T) {
	rack := map[string]string{rackLabel: "rack-1"}
	nodes := []Node{
		{Name: "n1", Labels: rack, GPUs: map[string]GPUCount{nvidia: gpus(4), amd: gpus(8)}, Ready: true},
		{Name: "n2", Labels: rack, GPUs: map[string]GPUCount{nvidia: gpus(4)}, Ready: true},
		{Name: "n3", Labels: map[string]string{}, GPUs: map[string]GPUCount{amd: gpus(8)}, Ready: true},
	}
	alloc := Allocations{"n1": {nvidia: 4}, "n2": {amd: 99}} // n2 has no amd GPUs: ignored
	res := Compute(Topology{Levels: []string{rackLabel}}, nodes, alloc)

	require.Len(t, res.Resources, 2)
	a, n := res.Resources[0], res.Resources[1]
	assert.Equal(t, amd, a.Resource)
	assert.Equal(t, nvidia, n.Resource)

	assert.Equal(t, NodeCoverage{InTopology: 1, MissingLabels: 1}, a.Nodes)
	assert.Equal(t, StateCounts{1, 0, 0}, a.Levels[0].Domains, "nvidia usage must not make amd look used")
	assert.Equal(t, 8, a.Levels[0].LargestFreeGPUs)

	assert.Equal(t, NodeCoverage{InTopology: 2}, n.Nodes)
	assert.Equal(t, StateCounts{0, 1, 0}, n.Levels[0].Domains)
	assert.Equal(t, StateCounts{1, 0, 1}, n.Levels[1].Domains)
	assert.Empty(t, res.Overcommits)
}

func TestEmpty(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {1}}})

	res := Compute(Topology{Name: "t"}, nodes, alloc)
	assert.Equal(t, Result{Topology: Topology{Name: "t"}}, res, "no levels: nothing computed")

	res = Compute(Topology{Levels: []string{rackLabel}}, nil, nil)
	assert.Empty(t, res.Resources, "no nodes: nothing computed")
}

func TestSingleLevel(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{"b": {"rack-1": {0, 2, 4}}})
	res := Compute(Topology{Levels: []string{HostnameLabel}}, nodes, alloc)

	levels := res.Resources[0].Levels
	require.Len(t, levels, 1)
	assert.Equal(t, LevelResult{
		Label:            HostnameLabel,
		Domains:          StateCounts{1, 1, 1},
		FreeGPUs:         StateCounts{4, 2, 0},
		AllocatedGPUs:    StateCounts{0, 2, 4},
		LargestFreeGPUs:  4,
		LargestFreeSlots: []int{4, 2, 1, 0},
	}, summary(levels[0]))
}

func TestDeterministicOrder(t *testing.T) {
	nodes, alloc := cluster(map[string]map[string][]int{
		"block-a": {"rack-1": {0, 1}, "rack-2": {4, 4}, "rack-3": {9, 0}},
		"block-b": {"rack-1": {2, 2}, "rack-2": {0, 0}},
	})
	for i := range nodes {
		nodes[i].GPUs[amd] = gpus(2)
	}
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

func TestDomainStateString(t *testing.T) {
	var got []string
	for _, s := range DomainStates {
		got = append(got, s.String())
	}
	assert.Equal(t, []string{"free", "partial", "full"}, got)
}
