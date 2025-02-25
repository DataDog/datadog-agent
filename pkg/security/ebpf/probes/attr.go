// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getAttrProbes(fentry bool) []*manager.Probe {
	var attrProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_inode_setattr",
			},
		},
	}

	// chmod
	attrProbes = appendSyscallProbes(attrProbes, fentry, EntryAndExit, false, "chmod", "fchmod", "fchmodat", "fchmodat2")

	// chown
	attrProbes = appendSyscallProbes(attrProbes, fentry, EntryAndExit, false, "chown", "chown16", "fchown", "fchown16", "fchownat", "lchown", "lchown16")

	// utime
	attrProbes = appendSyscallProbes(attrProbes, fentry, EntryAndExit, true, "utime", "utimes", "utimensat", "futimesat")
	attrProbes = appendSyscallProbes(attrProbes, fentry, EntryAndExit, false, "utime32")
	attrProbes = appendSyscallProbes(attrProbes, fentry, EntryAndExit|ExpandTime32, false, "utimes", "utimensat", "futimesat")

	return attrProbes
}
