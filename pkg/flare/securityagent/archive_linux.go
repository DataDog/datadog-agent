// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securityagent

import (
	"path/filepath"

	"github.com/DataDog/ebpf-manager/tracefs"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/flare/priviledged"
)

// only used in tests when running on linux
var linuxKernelSymbols = getLinuxKernelSymbols

func addSecurityAgentPlatformSpecificEntries(fb flaretypes.FlareBuilder) {
	linuxKernelSymbols(fb)                                 //nolint:errcheck
	getLinuxPid1MountInfo(fb)                              //nolint:errcheck
	fb.AddFileFromFunc("dmesg", priviledged.GetLinuxDmesg) //nolint:errcheck
	getLinuxKprobeEvents(fb)                               //nolint:errcheck
	getLinuxTracingAvailableEvents(fb)                     //nolint:errcheck
	getLinuxTracingAvailableFilterFunctions(fb)            //nolint:errcheck
}

func getLinuxKernelSymbols(fb flaretypes.FlareBuilder) error {
	return fb.CopyFile("/proc/kallsyms")
}

func getLinuxKprobeEvents(fb flaretypes.FlareBuilder) error {
	traceFSPath, err := tracefs.Root()
	if err != nil {
		return err
	}
	return fb.CopyFile(filepath.Join(traceFSPath, "kprobe_events"))
}

func getLinuxPid1MountInfo(fb flaretypes.FlareBuilder) error {
	return fb.CopyFile("/proc/1/mountinfo")
}

func getLinuxTracingAvailableEvents(fb flaretypes.FlareBuilder) error {
	traceFSPath, err := tracefs.Root()
	if err != nil {
		return err
	}
	return fb.CopyFile(filepath.Join(traceFSPath, "available_events"))
}

func getLinuxTracingAvailableFilterFunctions(fb flaretypes.FlareBuilder) error {
	traceFSPath, err := tracefs.Root()
	if err != nil {
		return err
	}
	return fb.CopyFile(filepath.Join(traceFSPath, "available_filter_functions"))
}
