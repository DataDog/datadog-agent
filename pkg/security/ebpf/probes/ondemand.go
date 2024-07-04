// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"
)

// GetOnDemandProbes returns all the on-demand probes
func GetOnDemandProbes() []*manager.Probe {
	return []*manager.Probe{
		GetOnDemandRegularProbe(),
		GetOnDemandSyscallProbe(),
	}
}

// GetOnDemandRegularProbe returns the on-demand probe used for regular (non-sycall) function hooking
func GetOnDemandRegularProbe() *manager.Probe {
	return &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "hook_on_demand",
		},
		KeepProgramSpec: true,
	}
}

// GetOnDemandSyscallProbe returns the on-demand probe used for sycall function hooking
func GetOnDemandSyscallProbe() *manager.Probe {
	return &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "hook_on_demand_syscall",
		},
		KeepProgramSpec: true,
	}
}
