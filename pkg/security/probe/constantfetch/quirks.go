// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package constantfetch holds constantfetch related files
package constantfetch

import "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"

// GetRHEL93MMapDelta returns the potential offset in `sys_enter_mmap` fields when reading from the tracepoint
// format
func GetRHEL93MMapDelta(kv *kernel.Version) uint64 {
	switch {
	// rh 9.3 is completely buggy.. the tracepoint format of `sys_enter_mmap` is not the actual format..
	// bpftrace is as confused as us on this
	// this check is to fix this manually
	case kv.IsInRangeCloseOpen(kernel.Kernel5_14, kernel.Kernel5_15) && kv.IsRH9_3Kernel():
		return 8
	default:
		return 0
	}
}
