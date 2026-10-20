// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && !386

package sysinfo

// ticksToMs returns the ULONGLONG from a Proc.Call result on 64-bit Windows
// (amd64, arm64 — the only windows/GOARCH targets Go supports beyond 386).
// lo already holds the full 64-bit value; hi is the XMM0 floating-point
// register (unspecified for integer-returning functions) and must not be used.
func ticksToMs(lo, hi uintptr) uint64 {
	return uint64(lo)
}
