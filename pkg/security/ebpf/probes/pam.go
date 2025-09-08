// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

func getPamProbes() []*manager.Probe {
	var pamProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_pam_start",
			},
			BinaryPath: "/lib/aarch64-linux-gnu/libpam.so.0",
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "rethook_pam_start",
			},
			BinaryPath: "/lib/aarch64-linux-gnu/libpam.so.0",
		},
		// {
		// 	ProbeIdentificationPair: manager.ProbeIdentificationPair{
		// 		UID:          SecurityAgentUID,
		// 		EBPFFuncName: "hook_pam_start",
		// 	},
		// 	BinaryPath: "/lib/i386-linux-gnu/libpam.so.0",
		// },
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_pam_set_item",
			},
			BinaryPath: "/lib/aarch64-linux-gnu/libpam.so.0",
		},
	}

	return pamProbes
}
