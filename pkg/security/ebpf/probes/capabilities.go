// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"
)

func getCapabilitiesMonitoringProbes() []*manager.Probe {
	return []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_capable",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "rethook_security_capable",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_override_creds",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_revert_creds",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "capabilities_usage_ticker",
			},
			SampleFrequency:   1,
			PerfEventType:     unix.PERF_TYPE_SOFTWARE,
			PerfEventConfig:   unix.PERF_COUNT_SW_CPU_CLOCK,
			PerfEventCPUCount: 1,
		},
	}
}

// GetCapabilitiesMonitoringProgramFunctions returns the capabilities monitoring functions
func GetCapabilitiesMonitoringProgramFunctions() []string {
	return []string{
		"hook_security_capable",
		"rethook_security_capable",
		"hook_override_creds",
		"hook_revert_creds",
		"capabilities_usage_ticker",
	}
}
