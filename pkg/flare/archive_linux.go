// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package flare

import (
	"path/filepath"
)

func zipLinuxKernelSymbols(tempDir, hostname string) error {
	return zipFile("/proc/kallsyms", filepath.Join(tempDir, hostname, "kallsyms"))
}

func zipLinuxKrobeEvents(tempDir, hostname string) error {
	return zipFile("/sys/kernel/debug/tracing/kprobe_events", filepath.Join(tempDir, hostname, "kprobe_events"))
}

func zipLinuxPid1MountInfo(tempDir, hostname string) error {
	return zipFile("/proc/1/mountinfo", filepath.Join(tempDir, hostname, "mountinfo"))
}

func zipLinuxTracingAvailableEvents(tempDir, hostname string) error {
	return zipFile("/sys/kernel/debug/tracing/available_events", filepath.Join(tempDir, hostname, "available_events"))
}

func zipLinuxTracingAvailableFilterFunctions(tempDir, hostname string) error {
	return zipFile("/sys/kernel/debug/tracing/available_filter_functions", filepath.Join(tempDir, hostname, "available_filter_functions"))
}
