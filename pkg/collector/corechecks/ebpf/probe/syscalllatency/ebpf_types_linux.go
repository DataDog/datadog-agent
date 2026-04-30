// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syscalllatency

// ebpfSyscallStats mirrors syscall_stats_t from syscall-latency-kern-user.h.
// Field order and sizes must match exactly; ebpf_types_linux_test.go verifies this.
type ebpfSyscallStats struct {
	Total_time_ns uint64
	Count         uint64
	Max_time_ns   uint64
	Slow_count    uint64
}

// ebpfCgroupStatsKey mirrors cgroup_stats_key_t from syscall-latency-kern-user.h.
// Field order, sizes, and padding must match exactly.
type ebpfCgroupStatsKey struct {
	Cgroup_name [128]byte
	Slot        uint8
	Pad         [7]byte
}
