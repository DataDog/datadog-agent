// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	"github.com/DataDog/ebpf/manager"
)

// mountProbes holds the list of probes used to track mount points events
var mountProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/attach_recursive_mnt",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/propagate_mnt",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/security_sb_umount",
	},
}

func getMountProbes() []*manager.Probe {
	mountProbes = append(mountProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "mount",
	}, EntryAndExit, true)...)
	mountProbes = append(mountProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "umount",
	}, EntryAndExit)...)
	return mountProbes
}
