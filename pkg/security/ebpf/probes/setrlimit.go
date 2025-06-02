// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

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

	// Add the LSM hook for setrlimit
	setrlimitProbes = append(setrlimitProbes, &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "hook_security_task_setrlimit",
		},
	})

	return setrlimitProbes
}
