// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// flowProbes holds the list of probes used to track network flows
var flowProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/security_socket_bind",
			EBPFFuncName: "kprobe_security_socket_bind",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/security_sk_classify_flow",
			EBPFFuncName: "kprobe_security_sk_classify_flow",
		},
	},
}

func getFlowProbes() []*manager.Probe {
	return flowProbes
}
