// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package model contains the model for the syscall latency check
package model

// SyscallLatencyStats holds per-syscall latency statistics for one flush interval.
type SyscallLatencyStats struct {
	Syscall     string `json:"syscall"`
	CgroupName  string `json:"cgroup_name"`  // raw cgroup leaf name from eBPF
	ContainerID string `json:"container_id"` // extracted by ContainerFilter; empty = host-level
	TotalTimeNs uint64 `json:"total_time_ns"`
	Count       uint64 `json:"count"`
	MaxTimeNs   uint64 `json:"max_time_ns"`
	SlowCount   uint64 `json:"slow_count"`
}
