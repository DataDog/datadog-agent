// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// sharedProbes is the list of probes that are shared across multiple events
var sharedProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/filename_create",
			EBPFFuncName: "kprobe_filename_create",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/mnt_want_write",
			EBPFFuncName: "kprobe_mnt_want_write",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/mnt_want_write_file",
			EBPFFuncName: "kprobe_mnt_want_write_file",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/mnt_want_write_file_path",
			EBPFFuncName: "kprobe_mnt_want_write_file_path",
		},
	},
}
