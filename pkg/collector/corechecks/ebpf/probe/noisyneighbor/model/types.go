// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the platform-neutral payload types shared between
// the noisy_neighbor system-probe probe and the agent-side core check.
package model

// Stats holds the aggregated per-cgroup scheduling and PMU counters emitted
// by the noisy_neighbor probe for one collection window.
type Stats struct {
	CgroupID        uint64
	SumLatenciesNs  uint64
	EventCount      uint64
	PreemptionCount uint64
	// UniquePidCount is the number of distinct kernel task_struct->pid values
	// observed (Linux TIDs, not POSIX PIDs).
	UniquePidCount     uint64
	SumCycles          uint64
	SumInstructions    uint64
	SumLLCMisses       uint64
	SumITLBMisses      uint64
	SumSoftirqNs       uint64
	BlockIORequests    uint64
	SumBranchMisses    uint64
	SumCPUMigrations   uint64
	WakeupCount        uint64
	SumCacheReferences uint64
}
