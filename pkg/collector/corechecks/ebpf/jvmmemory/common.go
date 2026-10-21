// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package jvmmemory contains the jvm_memory check, which reports the memory
// pools and garbage collection pauses of HotSpot JVMs, as observed by the
// jvm_memory system-probe module through the HotSpot USDT probes.
package jvmmemory

const (
	// CheckName is the name of the check
	CheckName = "jvm_memory"
)
