// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

// linkProbes holds the list of probes used to track link events
var linkProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/vfs_link",
	},
}

func getLinkProbe() []*manager.Probe {
	linkProbes = append(linkProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "link",
	}, EntryAndExit)...)
	linkProbes = append(linkProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "linkat",
	}, EntryAndExit)...)
	return linkProbes
}
