// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package syscalllatency is the system-probe side of the syscall latency check.
// It attaches eBPF raw tracepoints to sys_enter/sys_exit to measure per-syscall
// call latency for a curated set of high-signal syscalls.
package syscalllatency
