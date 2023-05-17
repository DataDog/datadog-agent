// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// attrProbes holds the list of probes used to track link events
var attrProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "kprobe_security_inode_setattr",
		},
	},
}

func getAttrProbes() []*manager.Probe {
	// chmod
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chmod",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmod",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmodat",
	}, EntryAndExit)...)

	// chown
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chown",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chown16",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchown",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchown16",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchownat",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "lchown",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "lchown16",
	}, EntryAndExit)...)

	// utime
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utime",
	}, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utime32",
	}, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimes",
	}, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimes",
	}, EntryAndExit|ExpandTime32)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimensat",
	}, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimensat",
	}, EntryAndExit|ExpandTime32)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "futimesat",
	}, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "futimesat",
	}, EntryAndExit|ExpandTime32)...)
	return attrProbes
}
