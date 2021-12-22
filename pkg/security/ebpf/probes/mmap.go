// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// mmapProbes holds the list of probes used to track mmap events
var mmapProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/fget",
			EBPFFuncName: "kretprobe_fget",
		},
	},
}

func getMMapProbes() []*manager.Probe {
	mmapProbes = append(mmapProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "mmap",
	}, EntryAndExit)...)
	return mmapProbes
}
