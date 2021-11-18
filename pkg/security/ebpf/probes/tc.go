// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// tcProbes holds the list of probes used to track network flows
var tcProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "classifier/ingress",
			EBPFFuncName: "classifier_ingress",
		},
		CopyProgram:      true,
		NetworkDirection: manager.Ingress,
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "classifier/egress",
			EBPFFuncName: "classifier_egress",
		},
		CopyProgram:      true,
		NetworkDirection: manager.Egress,
	},
}

// GetTCProbes returns the list of TCProbes
func GetTCProbes() []*manager.Probe {
	return tcProbes
}

func getTCTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{

		{
			ProgArrayName: "classifier_router",
			Key:           TCDNSRequestParserKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "classifier/dns_request_parser",
				EBPFFuncName: "classifier_dns_request_parser",
			},
		},
	}
}
