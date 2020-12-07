// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

// xattrProbes holds the list of probes used to track xattr events
var xattrProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/vfs_setxattr",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/vfs_removexattr",
	},
}

func getXattrProbes() []*manager.Probe {
	xattrProbes = append(xattrProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "setxattr",
	}, EntryAndExit)...)
	xattrProbes = append(xattrProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "fsetxattr",
	}, EntryAndExit)...)
	xattrProbes = append(xattrProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "lsetxattr",
	}, EntryAndExit)...)
	xattrProbes = append(xattrProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "removexattr",
	}, EntryAndExit)...)
	xattrProbes = append(xattrProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "fremovexattr",
	}, EntryAndExit)...)
	xattrProbes = append(xattrProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "lremovexattr",
	}, EntryAndExit)...)
	return xattrProbes
}
