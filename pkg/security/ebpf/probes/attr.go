// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getAttrProbes(fentry bool) []*manager.Probe {
	var attrProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_inode_setattr",
			},
		},
	}

	// chmod
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chmod",
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmod",
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmodat",
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "fchmodat2",
	}, fentry, EntryAndExit)...)

	// chown
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "chown",
	}, fentry, EntryAndExit)...)
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
	}, fentry, EntryAndExit)...)
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
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "lchown",
	}, fentry, EntryAndExit)...)
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
	}, fentry, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utime32",
	}, fentry, EntryAndExit)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimes",
	}, fentry, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimes",
	}, fentry, EntryAndExit|ExpandTime32)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimensat",
	}, fentry, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "utimensat",
	}, fentry, EntryAndExit|ExpandTime32)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "futimesat",
	}, fentry, EntryAndExit, true)...)
	attrProbes = append(attrProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "futimesat",
	}, fentry, EntryAndExit|ExpandTime32)...)
	return attrProbes
}
