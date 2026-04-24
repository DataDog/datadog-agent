// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	"runtime"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// syscallMonitorProbes holds the list of probes used to track syscall events

func getSyscallMonitorProbes() []*manager.Probe {
	return []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "sys_enter",
			},
		},
	}
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

	getSyscall := func(name string) model.Syscall {
		var syscall model.Syscall

		switch name {
		case "exit":
			syscall = model.Amd64SysExit
			if runtime.GOARCH == "arm64" {
				syscall = model.Arm64SysExit
			}
		case "exit_group":
			syscall = model.Amd64SysExitGroup
			if runtime.GOARCH == "arm64" {
				syscall = model.Arm64SysExitGroup
			}
		case "execve":
			syscall = model.Amd64SysExecve
			if runtime.GOARCH == "arm64" {
				syscall = model.Arm64SysExecve
			}
		case "execveat":
			syscall = model.Amd64SysExecveat
			if runtime.GOARCH == "arm64" {
				syscall = model.Arm64SysExecveat
			}
		}
		return syscall
	}

	m.Contents = []ebpf.MapKV{
		{
			Key: syscallTableKey{
				id:  uint64(getSyscall("exit").ToInt()),
				key: 1,
			},
			Value: uint8(1),
		},
		{
			Key: syscallTableKey{
				id:  uint64(getSyscall("exit_group").ToInt()),
				key: 1,
			},
			Value: uint8(1),
		},
		{
			Key: syscallTableKey{
				id:  uint64(getSyscall("execve").ToInt()),
				key: 2,
			},
			Value: uint8(1),
		},
		{
			Key: syscallTableKey{
				id:  uint64(getSyscall("execveat").ToInt()),
				key: 2,
			},
			Value: uint8(1),
		},
	}
	return m
}
