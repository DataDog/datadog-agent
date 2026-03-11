// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package flare

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare/priviledged"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
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

	_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_audit.log"), getLinuxAuditLogs)

	if pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enabled") {
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_cached.log"), getSystemProbeConntrackCached)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_host.log"), getSystemProbeConntrackHost)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "ebpf_btf_loader.log"), getSystemProbeBTFLoaderInfo)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "dmesg.log"), priviledged.GetLinuxDmesg)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_sestatus.log"), getSystemProbeSelinuxSestatus)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "selinux_semodule_list.log"), getSystemProbeSelinuxSemoduleList)

		if pkgconfigsetup.SystemProbe().GetBool("discovery.enabled") {
			_ = fb.AddFileFromFunc(filepath.Join("system-probe", "discovery.log"), getSystemProbeDiscoveryState)
		}
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

func getSystemProbeDiscoveryState() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.ModuleURL(sysconfig.DiscoveryModule, "/state")
	return priviledged.GetHTTPData(sysProbeClient, url)
}

// getLinuxAuditLogs reads /var/log/audit/audit.log and returns lines mentioning
// Datadog agent binaries. This is useful for diagnosing SELinux AVC denials.
func getLinuxAuditLogs() ([]byte, error) {
	const auditLogPath = "/var/log/audit/audit.log"

	f, err := os.Open(auditLogPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Binary names to grep for in the audit log
	datadogBinaries := []string{
		"datadog-agent",
		"system-probe",
		"sysprobe",
		"security-agent",
		"process-agent",
		"trace-agent",
	}

	var result bytes.Buffer
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, binary := range datadogBinaries {
			if strings.Contains(line, binary) {
				result.WriteString(line)
				result.WriteByte('\n')
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return result.Bytes(), err
	}
	return result.Bytes(), nil
}
