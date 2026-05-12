// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the model for the noisy neighbor check
package model

// NoisyNeighborStats is one row of per-cgroup statistics returned by the
// system-probe /check endpoint. The BPF-side counters (latencies, events,
// preemptions, pids) come from the cgroup_agg_stats map; the PMU-side counters
// come from user-space perf-event reads. Any field may be zero when the
// corresponding source had no data for this cgroup in the interval.
type NoisyNeighborStats struct {
	CgroupID        uint64
	SumLatenciesNs  uint64
	EventCount      uint64
	PreemptionCount uint64
	UniquePidCount  uint64 // kernel task_struct->pid (TID) count

	// PMU is the delta-since-last-read for the seven hardware/software perf
	// counters opened per cgroup. EnabledNs and RunningNs let the consumer
	// scale counters when the PMU is multiplexed.
	PMU CgroupPMUStats
}

// CgroupPMUStats is the per-cgroup PMU sample. Counters are summed across
// CPUs; EnabledNs and RunningNs are the matching scaling-time totals (each in
// nanoseconds). When the PMU is multiplexed, RunningNs < EnabledNs and the
// consumer scales counters by EnabledNs/RunningNs to estimate the value at
// 100% allocation.
type CgroupPMUStats struct {
	Cycles          uint64
	Instructions    uint64
	LLCMisses       uint64
	BranchMisses    uint64
	CacheReferences uint64
	ITLBMisses      uint64
	CPUMigrations   uint64
	EnabledNs       uint64
	RunningNs       uint64
}
