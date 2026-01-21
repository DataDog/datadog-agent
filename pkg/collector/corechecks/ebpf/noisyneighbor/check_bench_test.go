// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package noisyneighbor

import (
	"testing"

	"github.com/stretchr/testify/mock"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
)

// Benchmark buildTags with container
func BenchmarkBuildTags(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	check := &NoisyNeighborCheck{
		tagger: fakeTagger,
	}

	// Create a realistic stat with container
	stat := model.NoisyNeighborStats{
		CgroupID:        12345,
		CgroupName:      "/kubepods/besteffort/pod123/container456",
		SumLatenciesNs:  1000000,
		EventCount:      100,
		PreemptionCount: 10,
		UniquePidCount:  5,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tags := check.buildTags(stat)
		_ = tags
	}
}

// Benchmark buildTags with host cgroup (no container)
func BenchmarkBuildTagsHost(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	check := &NoisyNeighborCheck{
		tagger: fakeTagger,
	}

	// Create a host-level stat
	stat := model.NoisyNeighborStats{
		CgroupID:        1,
		CgroupName:      "/system.slice/sshd.service",
		SumLatenciesNs:  500000,
		EventCount:      50,
		PreemptionCount: 5,
		UniquePidCount:  2,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tags := check.buildTags(stat)
		_ = tags
	}
}

// Benchmark getContainerID extraction
func BenchmarkGetContainerID(b *testing.B) {
	cgroupNames := []string{
		"/kubepods/besteffort/pod123/container456",
		"/system.slice/docker-abcdef123456.scope",
		"/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/cri-containerd-456.scope",
		"/system.slice/sshd.service",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cgroupName := cgroupNames[i%len(cgroupNames)]
		containerID := getContainerID(cgroupName)
		_ = containerID
	}
}

// Benchmark metric submission methods
func BenchmarkSubmitPrimaryMetrics(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	check := &NoisyNeighborCheck{
		tagger:      fakeTagger,
		tagCache:    make(map[string][]string),
		tagCacheMax: 1000,
	}
	sender := mocksender.NewMockSender("test")
	// Set up mock expectations for all metric calls
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	stat := model.NoisyNeighborStats{
		CgroupID:        12345,
		CgroupName:      "/kubepods/besteffort/pod123/container456",
		SumLatenciesNs:  1000000,
		EventCount:      100,
		PreemptionCount: 10,
		UniquePidCount:  5,
	}
	tags := []string{"container_id:abc123", "pod_name:test-pod"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		check.submitPrimaryMetrics(sender, stat, tags)
	}
}

// Benchmark the full metric submission pipeline
func BenchmarkSubmitAllMetrics(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	check := &NoisyNeighborCheck{
		tagger:      fakeTagger,
		tagCache:    make(map[string][]string),
		tagCacheMax: 1000,
	}
	sender := mocksender.NewMockSender("test")

	stat := model.NoisyNeighborStats{
		CgroupID:        12345,
		CgroupName:      "/kubepods/besteffort/pod123/container456",
		SumLatenciesNs:  1000000,
		EventCount:      100,
		PreemptionCount: 10,
		UniquePidCount:  5,
	}
	tags := []string{"container_id:abc123", "pod_name:test-pod"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		check.submitPrimaryMetrics(sender, stat, tags)
		check.submitRawCounters(sender, stat, tags)
	}
}

// Benchmark processing multiple stats (simulating a real check run)
func BenchmarkProcessMultipleStats(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	check := &NoisyNeighborCheck{
		tagger: fakeTagger,
	}
	sender := mocksender.NewMockSender("test")

	// Create a realistic set of stats (10 containers)
	stats := make([]model.NoisyNeighborStats, 10)
	for i := 0; i < 10; i++ {
		stats[i] = model.NoisyNeighborStats{
			CgroupID:        uint64(10000 + i),
			CgroupName:      "/kubepods/besteffort/pod123/container" + string(rune('a'+i)),
			SumLatenciesNs:  uint64(1000000 * (i + 1)),
			EventCount:      uint64(100 * (i + 1)),
			PreemptionCount: uint64(10 * (i + 1)),
			UniquePidCount:  uint64(5 * (i + 1)),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, stat := range stats {
			tags := check.buildTags(stat)
			check.submitPrimaryMetrics(sender, stat, tags)
			check.submitRawCounters(sender, stat, tags)
		}
	}
}

