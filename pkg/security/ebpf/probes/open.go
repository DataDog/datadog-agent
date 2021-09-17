// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// openProbes holds the list of probes used to track file open events
var openProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/vfs_truncate",
			EBPFFuncName: "kprobe_vfs_truncate",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/vfs_open",
			EBPFFuncName: "kprobe_vfs_open",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/do_dentry_open",
			EBPFFuncName: "kprobe_do_dentry_open",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/io_openat2",
			EBPFFuncName: "kprobe_io_openat2",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/io_openat2",
			EBPFFuncName: "kretprobe_io_openat2",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/filp_close",
			EBPFFuncName: "kprobe_filp_close",
		},
	},
}

func getOpenProbes() []*manager.Probe {
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "open",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "creat",
	}, EntryAndExit)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "open_by_handle_at",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "truncate",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "openat",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "openat2",
	}, EntryAndExit)...)
	return openProbes
}
