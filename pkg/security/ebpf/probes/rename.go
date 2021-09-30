// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// renameProbes holds the list of probes used to track file rename events
var renameProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/vfs_rename",
			EBPFFuncName: "kprobe_vfs_rename",
		},
	},
}

func getRenameProbes() []*manager.Probe {
	renameProbes = append(renameProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "rename",
	}, EntryAndExit)...)
	renameProbes = append(renameProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "renameat",
	}, EntryAndExit)...)
	renameProbes = append(renameProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "renameat2",
	}, EntryAndExit)...)
	return renameProbes
}
