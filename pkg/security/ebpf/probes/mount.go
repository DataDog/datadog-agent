// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// mountProbes holds the list of probes used to track mount points events
var mountProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/attach_recursive_mnt",
			EBPFFuncName: "kprobe_attach_recursive_mnt",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/propagate_mnt",
			EBPFFuncName: "kprobe_propagate_mnt",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/security_sb_umount",
			EBPFFuncName: "kprobe_security_sb_umount",
		},
	},
}

func getMountProbes() []*manager.Probe {
	mountProbes = append(mountProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "mount",
	}, EntryAndExit, true)...)
	mountProbes = append(mountProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "umount",
	}, EntryAndExit)...)
	return mountProbes
}
