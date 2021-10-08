// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// mkdirProbes holds the list of probes used to track mkdir events
var mkdirProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/vfs_mkdir",
			EBPFFuncName: "kprobe_vfs_mkdir",
		},
	},
}

func getMkdirProbes() []*manager.Probe {
	mkdirProbes = append(mkdirProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "mkdir",
	}, EntryAndExit)...)
	mkdirProbes = append(mkdirProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "mkdirat",
	}, EntryAndExit)...)
	return mkdirProbes
}
