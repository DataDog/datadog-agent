// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

// ioctlProbes holds the list of probes used to track ioctl events
var ioctlProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/do_vfs_ioctl",
	},
}

func getIoctlProbes() []*manager.Probe {
	return ioctlProbes
}
