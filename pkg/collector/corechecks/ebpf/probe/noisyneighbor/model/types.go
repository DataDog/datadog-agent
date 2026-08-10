// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the model for the noisy neighbor check
package model

// PMU event bits shared by the Agent check and system-probe.
const (
	EventCycles uint64 = 1 << iota
	EventInstructions
	EventCacheMisses
	EventCacheReferences
	EventITLBMisses
	EventBranchMisses
	EventCPUMigrations
)

// HardwareEventMask contains events backed by per-CPU perf events.
const HardwareEventMask = EventCycles | EventInstructions | EventCacheMisses | EventCacheReferences | EventITLBMisses | EventBranchMisses

// NoisyNeighborStats contains the statistics from the noisy neighbor check
type NoisyNeighborStats struct {
	CgroupID         uint64
	SumLatenciesNs   uint64
	EventCount       uint64
	PreemptionCount  uint64
	UniquePidCount   uint64 // kernel task_struct->pid (TID) count
	Cycles           uint64
	Instructions     uint64
	CacheMisses      uint64
	CacheReferences  uint64
	ITLBMisses       uint64
	BranchMisses     uint64
	CPUMigrations    uint64
	SampledEventMask uint64
}

// WatchlistRequest atomically replaces the cgroups sampled by the PMU probes.
type WatchlistRequest struct {
	CgroupIDs       []uint64
	EligibleCgroups int
}

// WatchlistResponse reports the installed watchlist generation.
type WatchlistResponse struct {
	Generation uint64
	Size       int
}
