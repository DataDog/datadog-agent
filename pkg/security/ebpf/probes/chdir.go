// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getChdirProbes(fentry bool) []*manager.Probe {
	var chdirProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_set_fs_pwd",
			},
		},
	}
	chdirProbes = appendSyscallProbes(chdirProbes, fentry, EntryAndExit, false, "chdir", "fchdir")
	return chdirProbes
}
