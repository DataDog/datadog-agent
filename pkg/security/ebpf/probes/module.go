// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

func getModuleProbes(fentry bool) []*manager.Probe {
	// moduleProbes holds the list of probes used to track kernel module events
	var moduleProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_kernel_read_file",
			},
		},

		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "module_load",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "kprobe_parse_args",
			},
		},
	}

	if !fentry {
		moduleProbes = append(moduleProbes, &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_kernel_module_from_file",
			},
		})
	}

	moduleProbes = append(moduleProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "init_module",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	moduleProbes = append(moduleProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "finit_module",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	moduleProbes = append(moduleProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "delete_module",
	}, fentry, EntryAndExit|SupportFentry|SupportFexit)...)
	return moduleProbes
}
