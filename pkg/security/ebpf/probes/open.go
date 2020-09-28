// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probes

import (
	"github.com/DataDog/ebpf/manager"
)

// openProbes holds the list of probes used to track file open events
var openProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/vfs_truncate",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kretprobe/ovl_d_real",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kretprobe/ovl_dentry_upper",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/do_dentry_open",
	},
}

func getOpenProbes() []*manager.Probe {
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "open",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "creat",
	}, EntryAndExit)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "open_by_handle_at",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "truncate",
	}, EntryAndExit, true)...)
	openProbes = append(openProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "openat",
	}, EntryAndExit, true)...)
	return openProbes
}
