// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

// rmdirProbes holds the list of probes used to track file rmdir events
var rmdirProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/security_inode_rmdir",
	},
}

func getRmdirProbe() []*manager.Probe {
	rmdirProbes = append(rmdirProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "rmdir",
	}, EntryAndExit)...)
	return rmdirProbes
}
