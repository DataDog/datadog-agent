// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

// Package prebuilt implements prebuilt specific eBPF functionality
package prebuilt

// IsDeprecated returns true if prebuilt ebpf is deprecated
// on this host
func IsDeprecated() bool {
	return false
}
