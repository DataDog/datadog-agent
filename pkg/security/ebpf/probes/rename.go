// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// renameProbes holds the list of probes used to track file rename events
var renameProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "hook_vfs_rename",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "hook_do_renameat2",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "rethook_do_renameat2",
		},
	},
}

func getRenameProbes(fentry bool) []*manager.Probe {
	renameProbes = append(renameProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "rename",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	renameProbes = append(renameProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "renameat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	renameProbes = append(renameProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "renameat2",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	return renameProbes
}
