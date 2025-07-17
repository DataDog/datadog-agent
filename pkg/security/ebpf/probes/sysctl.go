// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getSysCtlProbes(cgroup2MountPoint string) []*manager.Probe {
	return []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: SysCtlProbeFunctionName,
			},
			CGroupPath: cgroup2MountPoint,
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_proc_sys_call_handler",
			},
		},
	}
}

// SysCtlProbeFunctionName is the function name of the cgroup/sysctl probe
const SysCtlProbeFunctionName = "cgroup_sysctl"
