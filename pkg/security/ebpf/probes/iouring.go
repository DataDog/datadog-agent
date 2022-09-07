// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// iouringProbes is the list of probes that are used for iouring monitoring
var iouringProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "tracepoint/io_uring/io_uring_create",
			EBPFFuncName: "io_uring_create"},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/io_ring_ctx_alloc",
			EBPFFuncName: "kretprobe_io_ring_ctx_alloc",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/io_allocate_scq_urings",
			EBPFFuncName: "kprobe_io_allocate_scq_urings",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/io_sq_offload_start",
			EBPFFuncName: "kprobe_io_sq_offload_start",
		},
	},
}
