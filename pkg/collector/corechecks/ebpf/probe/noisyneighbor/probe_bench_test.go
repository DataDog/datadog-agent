// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Benchmark GetAndFlush with varying numbers of cgroups
func BenchmarkGetAndFlush(b *testing.B) {
	if kv < minimumKernelVersion {
		b.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
	}

	cfg := ebpf.NewConfig()
	probe, err := NewProbe(cfg)
	if err != nil {
		b.Fatalf("Failed to create probe: %v", err)
	}
	defer probe.Close()

	// Let some events accumulate
	// Note: In real workload, we'd have many cgroups actively scheduling
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats := probe.GetAndFlush()
		_ = stats // Prevent compiler optimization
	}
}

// Benchmark just the PID counting operation
func BenchmarkCountUniquePidsForCgroup(b *testing.B) {
	if kv < minimumKernelVersion {
		b.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
	}

	cfg := ebpf.NewConfig()
	probe, err := NewProbe(cfg)
	if err != nil {
		b.Fatalf("Failed to create probe: %v", err)
	}
	defer probe.Close()

	// Get a cgroup ID from the first stats entry
	stats := probe.GetAndFlush()
	if len(stats) == 0 {
		b.Skip("No cgroups found, skipping benchmark")
	}
	cgroupID := stats[0].CgroupID

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		count := probe.countUniquePidsForCgroup(cgroupID)
		_ = count
	}
}

// Benchmark the PID deletion operation
func BenchmarkDeletePidsForCgroup(b *testing.B) {
	if kv < minimumKernelVersion {
		b.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
	}

	cfg := ebpf.NewConfig()
	probe, err := NewProbe(cfg)
	if err != nil {
		b.Fatalf("Failed to create probe: %v", err)
	}
	defer probe.Close()

	// Get a cgroup ID from the first stats entry
	stats := probe.GetAndFlush()
	if len(stats) == 0 {
		b.Skip("No cgroups found, skipping benchmark")
	}
	cgroupID := stats[0].CgroupID

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		probe.deletePidsForCgroup(cgroupID)
		// Note: This will delete the PIDs on first iteration,
		// subsequent iterations will be faster but less representative
	}
}

// Benchmark map iteration overhead
func BenchmarkMapIteration(b *testing.B) {
	if kv < minimumKernelVersion {
		b.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
	}

	cfg := ebpf.NewConfig()
	probe, err := NewProbe(cfg)
	if err != nil {
		b.Fatalf("Failed to create probe: %v", err)
	}
	defer probe.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Benchmark just the map iteration part without aggregation
		aggMap, found, err := probe.mgr.GetMap("cgroup_agg_stats")
		if err != nil || !found {
			b.Fatalf("Failed to get map: %v", err)
		}

		iter := aggMap.Iterate()
		var cgroupID uint64
		var perCPUStats []ebpfCgroupAggStats
		count := 0

		for iter.Next(&cgroupID, &perCPUStats) {
			count++
		}

		if err := iter.Err(); err != nil {
			b.Fatalf("Iterator error: %v", err)
		}
	}
}

// Benchmark with memory allocation tracking
func BenchmarkGetAndFlushAllocs(b *testing.B) {
	if kv < minimumKernelVersion {
		b.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
	}

	cfg := ebpf.NewConfig()
	probe, err := NewProbe(cfg)
	if err != nil {
		b.Fatalf("Failed to create probe: %v", err)
	}
	defer probe.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := probe.GetAndFlush()
		_ = stats
	}
}

