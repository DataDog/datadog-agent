// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// ioctlProbes holds the list of probes used to track ioctl events
var ioctlProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "kprobe_do_vfs_ioctl",
		},
	},
}

func getIoctlProbes() []*manager.Probe {
	return ioctlProbes
}
