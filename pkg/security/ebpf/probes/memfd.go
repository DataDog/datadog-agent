// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getMemfdProbes(fentry bool) []*manager.Probe {
	memfdProbes := appendSyscallProbes(nil, fentry, EntryAndExit, false, "memfd_create")
	memfdProbes = append(memfdProbes,
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_memfd_fcntl",
			}},
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_shmem_fcntl",
			}},
	)
	return memfdProbes
}
