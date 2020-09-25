// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probes

import "github.com/DataDog/ebpf/manager"

// allProbes contain the list of all the probes of the runtime security module
var allProbes []*manager.Probe

// AllProbes returns the list of all the probes of the runtime security module
func AllProbes() []*manager.Probe {
	if len(allProbes) > 0 {
		return allProbes
	}

	allProbes = append(allProbes, getAttrProbes()...)
	allProbes = append(allProbes, getExecProbes()...)
	allProbes = append(allProbes, getLinkProbe()...)
	allProbes = append(allProbes, getMkdirProbes()...)
	allProbes = append(allProbes, getMountProbes()...)
	allProbes = append(allProbes, getOpenProbes()...)
	allProbes = append(allProbes, getRenameProbes()...)
	allProbes = append(allProbes, getRmdirProbe()...)
	allProbes = append(allProbes, sharedProbes...)
	allProbes = append(allProbes, getUnlinkProbes()...)
	allProbes = append(allProbes, getXattrProbes()...)

	// Syscall monitor
	allProbes = append(allProbes, &manager.Probe{
		UID:     SecurityAgentUID,
		Section: "tracepoint/raw_syscalls/sys_enter",
	})

	return allProbes
}
