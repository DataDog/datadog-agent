// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package flare

func zipLinuxKernelSymbols(tempDir, hostname string) error {
	return nil
}

func zipLinuxKprobeEvents(tempDir, hostname string) error {
	return nil
}

func zipLinuxDmesg(tempDir, hostname string) error {
	return nil
}

func zipLinuxPid1MountInfo(tempDir, hostname string) error {
	return nil
}

func zipLinuxTracingAvailableEvents(tempDir, hostname string) error {
	return nil
}

func zipLinuxTracingAvailableFilterFunctions(tempDir, hostname string) error {
	return nil
}
