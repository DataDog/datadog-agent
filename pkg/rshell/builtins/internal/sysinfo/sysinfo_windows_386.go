// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sysinfo

// ticksToMs reconstructs the full ULONGLONG from a Proc.Call result on
// 32-bit Windows. The Win32 calling convention returns 64-bit integers with
// the low 32 bits in EAX (lo) and the high 32 bits in EDX (hi).
func ticksToMs(lo, hi uintptr) uint64 {
	return uint64(lo) | uint64(hi)<<32
}
