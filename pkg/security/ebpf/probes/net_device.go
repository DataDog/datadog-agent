// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// netDeviceProbes holds the list of probes used to track new network devices
var netDeviceProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/veth_newlink",
			EBPFFuncName: "kprobe_veth_newlink",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/register_netdevice",
			EBPFFuncName: "kprobe_register_netdevice",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/dev_get_valid_name",
			EBPFFuncName: "kprobe_dev_get_valid_name",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/dev_new_index",
			EBPFFuncName: "kprobe_dev_new_index",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/dev_new_index",
			EBPFFuncName: "kretprobe_dev_new_index",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/__dev_get_by_index",
			EBPFFuncName: "kprobe___dev_get_by_index",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/__dev_get_by_name",
			EBPFFuncName: "kprobe___dev_get_by_name",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/register_netdevice",
			EBPFFuncName: "kretprobe_register_netdevice",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/dev_change_net_namespace",
			EBPFFuncName: "kprobe_dev_change_net_namespace",
		},
	},
}

func getNetDeviceProbes() []*manager.Probe {
	return netDeviceProbes
}
