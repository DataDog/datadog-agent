// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import (
	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"
)

// tcProbes holds the list of probes used to track network flows
var tcProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "classifier_ingress",
		},
		NetworkDirection: manager.Ingress,
		TCFilterProtocol: unix.ETH_P_ALL,
		KeepProgramSpec:  true,
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "classifier_egress",
		},
		NetworkDirection: manager.Egress,
		TCFilterProtocol: unix.ETH_P_ALL,
		KeepProgramSpec:  true,
	},
}

// GetTCProbes returns the list of TCProbes
func GetTCProbes() []*manager.Probe {
	return tcProbes
}

// GetAllTCProgramFunctions returns the list of TC classifier sections
func GetAllTCProgramFunctions() []string {
	output := []string{
		"classifier_dns_request_parser",
		"classifier_dns_request",
	}

	for _, tcProbe := range GetTCProbes() {
		output = append(output, tcProbe.EBPFFuncName)
	}

	for _, flowProbe := range getFlowProbes() {
		output = append(output, flowProbe.EBPFFuncName)
	}

	for _, netDeviceProbe := range getNetDeviceProbes() {
		output = append(output, netDeviceProbe.EBPFFuncName)
	}

	return output
}

func getTCTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: "classifier_router",
			Key:           TCDNSRequestKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "classifier_dns_request",
			},
		},
		{
			ProgArrayName: "classifier_router",
			Key:           TCDNSRequestParserKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "classifier_dns_request_parser",
			},
		},
	}
}
