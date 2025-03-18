// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package flare

import (
	"path/filepath"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare/priviledged"
)

func addSystemProbePlatformSpecificEntries(fb flaretypes.FlareBuilder) {
	systemProbeConfigBPFDir := pkgconfigsetup.SystemProbe().GetString("system_probe_config.bpf_dir")
	if systemProbeConfigBPFDir != "" {
		fb.RegisterDirPerm(systemProbeConfigBPFDir)
	}

	sysprobeSocketLocation := priviledged.GetSystemProbeSocketPath()
	if sysprobeSocketLocation != "" {
		fb.RegisterDirPerm(filepath.Dir(sysprobeSocketLocation))
	}

	if pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enabled") {
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_cached.log"), getSystemProbeConntrackCached)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_host.log"), getSystemProbeConntrackHost)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "ebpf_btf_loader.log"), getSystemProbeBTFLoaderInfo)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "dmesg.log"), priviledged.GetLinuxDmesg)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_sestatus.log"), getSystemProbeSelinuxSestatus)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_semodule_list.log"), getSystemProbeSelinuxSemoduleList)
	}
}

func getSystemProbeConntrackCached() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/debug/conntrack/cached")
	return priviledged.GetHTTPData(sysProbeClient, url)
}

func getSystemProbeConntrackHost() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/debug/conntrack/host")
	return priviledged.GetHTTPData(sysProbeClient, url)
}

func getSystemProbeBTFLoaderInfo() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/ebpf_btf_loader_info")
	return priviledged.GetHTTPData(sysProbeClient, url)
}

func getSystemProbeSelinuxSestatus() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/selinux_sestatus")
	return priviledged.GetHTTPData(sysProbeClient, url)
}

func getSystemProbeSelinuxSemoduleList() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/selinux_semodule_list")
	return priviledged.GetHTTPData(sysProbeClient, url)
}
