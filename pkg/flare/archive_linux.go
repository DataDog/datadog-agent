// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package flare

import (
	"path/filepath"

	"github.com/DataDog/ebpf-manager/tracefs"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func addSystemProbePlatformSpecificEntries(fb flaretypes.FlareBuilder) {
	systemProbeConfigBPFDir := pkgconfigsetup.SystemProbe().GetString("system_probe_config.bpf_dir")
	if systemProbeConfigBPFDir != "" {
		fb.RegisterDirPerm(systemProbeConfigBPFDir)
	}

	sysprobeSocketLocation := getSystemProbeSocketPath()
	if sysprobeSocketLocation != "" {
		fb.RegisterDirPerm(filepath.Dir(sysprobeSocketLocation))
	}

	if pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enabled") {
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_cached.log"), getSystemProbeConntrackCached)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_host.log"), getSystemProbeConntrackHost)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "ebpf_btf_loader.log"), getSystemProbeBTFLoaderInfo)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "dmesg.log"), getLinuxDmesg)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_sestatus.log"), getSystemProbeSelinuxSestatus)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_semodule_list.log"), getSystemProbeSelinuxSemoduleList)
	}
}

// only used in tests when running on linux
var linuxKernelSymbols = getLinuxKernelSymbols

func addSecurityAgentPlatformSpecificEntries(fb flaretypes.FlareBuilder) {
	linuxKernelSymbols(fb)                      //nolint:errcheck
	getLinuxPid1MountInfo(fb)                   //nolint:errcheck
	fb.AddFileFromFunc("dmesg", getLinuxDmesg)  //nolint:errcheck
	getLinuxKprobeEvents(fb)                    //nolint:errcheck
	getLinuxTracingAvailableEvents(fb)          //nolint:errcheck
	getLinuxTracingAvailableFilterFunctions(fb) //nolint:errcheck
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

func getLinuxDmesg() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(getSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/dmesg")
	return getHTTPData(sysProbeClient, url)
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

func getSystemProbeConntrackCached() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(getSystemProbeSocketPath())
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/debug/conntrack/cached")
	return getHTTPData(sysProbeClient, url)
}

func getSystemProbeConntrackHost() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(getSystemProbeSocketPath())
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/debug/conntrack/host")
	return getHTTPData(sysProbeClient, url)
}

func getSystemProbeBTFLoaderInfo() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(getSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/ebpf_btf_loader_info")
	return getHTTPData(sysProbeClient, url)
}

func getSystemProbeSelinuxSestatus() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(getSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/selinux_sestatus")
	return getHTTPData(sysProbeClient, url)
}

func getSystemProbeSelinuxSemoduleList() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(getSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/selinux_semodule_list")
	return getHTTPData(sysProbeClient, url)
}
