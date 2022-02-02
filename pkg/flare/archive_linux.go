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

func zipLinuxFunctions(source, tempDir, hostname, filename string) error {
	return zipFile(source, filepath.Join(tempDir, hostname), filename)
}

func zipLinuxKernelSymbols(tempDir, hostname string) error {
	return zipLinuxFunctions("/proc", filepath.Join(tempDir, hostname), "kallsyms")
}

func zipLinuxKrobeEvents(tempDir, hostname string) error {
	return zipLinuxFunctions("/sys/kernel/debug/tracing", filepath.Join(tempDir, hostname), "kprobe_events")
}

func zipLinuxPid1MountInfo(tempDir, hostname string) error {
	return zipLinuxFunctions("/proc/1", filepath.Join(tempDir, hostname), "mountinfo")
}

func zipLinuxTracingAvailableEvents(tempDir, hostname string) error {
	return zipLinuxFunctions("/sys/kernel/debug/tracing", filepath.Join(tempDir, hostname), "available_events")
}

func zipLinuxTracingAvailableFilterFunctions(tempDir, hostname string) error {
	return zipLinuxFunctions("/sys/kernel/debug/tracing", filepath.Join(tempDir, hostname), "available_filter_functions")
}
