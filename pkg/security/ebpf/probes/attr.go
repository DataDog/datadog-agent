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

func getAttrProbes(fentry bool) []*manager.Probe {
	// chmod
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chmod",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmod",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmodat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)

	// chown
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chown",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chown16",
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchown",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchown16",
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchownat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "lchown",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "lchown16",
	}, fentry, EntryAndExit)...)

	// utime
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utime",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utime32",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimes",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimes",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit|ExpandTime32)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimensat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimensat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit|ExpandTime32)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "futimesat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "futimesat",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit|ExpandTime32)...)
	return attrProbes
}
