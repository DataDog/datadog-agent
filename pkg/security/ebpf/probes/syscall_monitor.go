// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// SyscallUsageSysEnterFunctionName is the program that samples the syscalls used per process
	SyscallUsageSysEnterFunctionName = "sys_enter_syscall_usage"
	// SyscallUsageTickerFunctionName is the program that periodically sends the syscalls sampled per process
	SyscallUsageTickerFunctionName = "syscall_usage_ticker"
)

// SyscallUsageFunctionNames returns the programs of the workload profiles v2 syscall sampler
func SyscallUsageFunctionNames() []string {
	return []string{SyscallUsageSysEnterFunctionName, SyscallUsageTickerFunctionName}
}

// syscallMonitorProbes holds the list of probes used to track syscall events

func getSyscallMonitorProbes() []*manager.Probe {
	return []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "sys_enter",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: SyscallUsageSysEnterFunctionName,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: SyscallUsageTickerFunctionName,
			},
			SampleFrequency:   1,
			PerfEventType:     unix.PERF_TYPE_SOFTWARE,
			PerfEventConfig:   unix.PERF_COUNT_SW_CPU_CLOCK,
			PerfEventCPUCount: 1,
		},
	}
}

// SyscallUsageSelectors returns the probes that send the syscalls sampled for workload profiles v2
func SyscallUsageSelectors() []manager.ProbesSelector {
	var selectors []manager.ProbesSelector
	for _, funcName := range SyscallUsageFunctionNames() {
		selectors = append(selectors, &manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: funcName,
			},
		})
	}
	return selectors
}

func getSyscallTableMap() *manager.Map {
	m := &manager.Map{
		Name: "syscall_table",
	}

	// initialize the content of the map with the syscalls ID of the current architecture
	type syscallTableKey struct {
		id  uint64
		key uint64
	}

	m.Contents = []ebpf.MapKV{
		{
			Key: syscallTableKey{
				id:  uint64(model.SysExit),
				key: 1,
			},
			Value: uint8(1),
		},
		{
			Key: syscallTableKey{
				id:  uint64(model.SysExitGroup),
				key: 1,
			},
			Value: uint8(1),
		},
		{
			Key: syscallTableKey{
				id:  uint64(model.SysExecve),
				key: 2,
			},
			Value: uint8(1),
		},
		{
			Key: syscallTableKey{
				id:  uint64(model.SysExecveat),
				key: 2,
			},
			Value: uint8(1),
		},
	}
	return m
}
