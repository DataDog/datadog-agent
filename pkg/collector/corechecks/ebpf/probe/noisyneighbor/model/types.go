// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the model for the noisy neighbor check
package model

// NoisyNeighborStats contains per-cgroup scheduling statistics from Layer 2
type NoisyNeighborStats struct {
	CgroupID                uint64
	SumLatenciesNs          uint64
	EventCount              uint64
	ForeignPreemptionCount  uint64
	SelfPreemptionCount     uint64
	TaskCount               uint64 // total tasks in cgroup from pids_cgroup->counter
	LatencyBucketLt100us    uint64
	LatencyBucket100us1ms   uint64
	LatencyBucket1ms10ms    uint64
	LatencyBucketGt10ms     uint64
	CpuMigrations           uint64
}

// PreemptorStats identifies which foreign cgroup is preempting a victim cgroup
type PreemptorStats struct {
	VictimCgroupID    uint64
	PreemptorCgroupID uint64
	Count             uint64
}

// CheckResponse is the combined response from the system-probe noisy neighbor module
type CheckResponse struct {
	CgroupStats    []NoisyNeighborStats `json:"cgroup_stats"`
	PreemptorStats []PreemptorStats     `json:"preemptor_stats"`
}

// WatchlistRequest is sent by the agent to update the eBPF watchlist
type WatchlistRequest struct {
	CgroupIDs []uint64 `json:"cgroup_ids"`
}
