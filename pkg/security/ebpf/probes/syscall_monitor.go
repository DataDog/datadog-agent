// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import (
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// syscallMonitorProbes holds the list of probes used to track syscall events
var syscallMonitorProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFFuncName: "sys_enter",
		},
	},
}

func getSyscallMonitorProbes() []*manager.Probe {
	return syscallMonitorProbes
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
