// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

// Package ebpf implements tracing network events with eBPF
package ebpf

// IsPrecompiledEbpfDeprecated returns true if precompiled ebpf is deprecated
// on this host
func IsPrecompiledEbpfDeprecated() bool {
	return false
}
