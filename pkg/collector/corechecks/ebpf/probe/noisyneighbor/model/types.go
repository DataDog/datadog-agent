// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the model for the noisy neighbor check
package model

// NoisyNeighborStats contains the statistics from the noisy neighbor check
type NoisyNeighborStats struct {
	// Primary fields - populated from aggregated eBPF stats
	CgroupID        uint64
	CgroupName      string
	SumLatenciesNs  uint64
	EventCount      uint64
	PreemptionCount uint64
	UniquePidCount  uint64 // Note: "PID" here refers to kernel task_struct->pid, which is actually a Thread ID (TID) in userspace
}
