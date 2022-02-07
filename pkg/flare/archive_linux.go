// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package flare

import (
	"path/filepath"
)

func zipLinuxFile(source, tempDir, hostname, filename string) error {
	return zipFile(source, filepath.Join(tempDir, hostname), filename)
}

func zipLinuxKernelSymbols(tempDir, hostname string) error {
	return zipLinuxFile("/proc", tempDir, hostname, "kallsyms")
}

func zipLinuxKrobeEvents(tempDir, hostname string) error {
	return zipLinuxFile("/sys/kernel/debug/tracing", tempDir, hostname, "kprobe_events")
}

func zipLinuxPid1MountInfo(tempDir, hostname string) error {
	return zipLinuxFile("/proc/1", tempDir, hostname, "mountinfo")
}

func zipLinuxTracingAvailableEvents(tempDir, hostname string) error {
	return zipLinuxFile("/sys/kernel/debug/tracing", tempDir, hostname, "available_events")
}

func zipLinuxTracingAvailableFilterFunctions(tempDir, hostname string) error {
	return zipLinuxFile("/sys/kernel/debug/tracing", tempDir, hostname, "available_filter_functions")
}
