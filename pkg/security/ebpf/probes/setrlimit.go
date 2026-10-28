// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

const (
	// SetrlimitKprobeFunctionName is the kprobe/fentry flavor of the setrlimit security hook
	SetrlimitKprobeFunctionName = "hook_security_task_setrlimit"
	// SetrlimitLSMFunctionName is the BPF LSM flavor of the setrlimit security hook
	SetrlimitLSMFunctionName = "hook_lsm_task_setrlimit"
)

// GetBPFLSMProgramFunctions returns the programs that are BPF LSM variants of kprobe/fentry hooks
func GetBPFLSMProgramFunctions() []string {
	return []string{SetrlimitLSMFunctionName}
}

// GetBPFLSMReplacedProgramFunctions returns the kprobe/fentry hooks that are replaced by a BPF LSM program
func GetBPFLSMReplacedProgramFunctions() []string {
	return []string{SetrlimitKprobeFunctionName}
}

func getSetrlimitProbes(fentry bool) []*manager.Probe {
	var setrlimitProbes []*manager.Probe
	setrlimitProbes = append(setrlimitProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "setrlimit",
	}, fentry, EntryAndExit)...)
	setrlimitProbes = append(setrlimitProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "prlimit64",
	}, fentry, EntryAndExit)...)

	// Add the security hook for setrlimit, either as a kprobe/fentry or as a BPF LSM program.
	// Only one of them is loaded, see GetBPFLSMProgramFunctions and GetBPFLSMReplacedProgramFunctions.
	setrlimitProbes = append(setrlimitProbes,
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: SetrlimitKprobeFunctionName,
			},
		},
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: SetrlimitLSMFunctionName,
			},
		},
	)

	return setrlimitProbes
}
