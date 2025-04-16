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

// GetPerfEventProbes returns the list of perf event Probes
func GetPerfEventProbes() []*manager.Probe {
	return []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "network_stats_worker",
			},
			SampleFrequency:   1,
			PerfEventType:     unix.PERF_TYPE_SOFTWARE,
			PerfEventConfig:   unix.PERF_COUNT_SW_CPU_CLOCK,
			PerfEventCPUCount: 1,
		},
	}
}
