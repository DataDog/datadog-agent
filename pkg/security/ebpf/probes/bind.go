// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getBindProbes(fentry bool) []*manager.Probe {
	var bindProbes []*manager.Probe
	bindProbes = append(bindProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "bind",
	}, fentry, EntryAndExit)...)

	bindProbes = append(bindProbes, &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "hook_security_socket_bind",
		},
	})

	return bindProbes
}
