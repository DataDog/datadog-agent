// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package numamonitoring implements capability-driven NUMA monitoring.
//
// It is the system-probe side of the NUMA monitoring check: an eBPF
// sched_switch tracepoint accumulates per-cgroup, per-node scheduler runtime,
// numa_maps supplies resident memory distribution, and resctrl supplies cache
// occupancy and memory bandwidth where the platform exposes them.
package numamonitoring
