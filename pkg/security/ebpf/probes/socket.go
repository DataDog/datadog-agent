// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

func getSocketProbes(fentry bool, cgroup2MountPoint string) []*manager.Probe {
	socketProbes := []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_sock_create",
			},
			CGroupPath: cgroup2MountPoint,
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_sock_release",
			},
			CGroupPath: cgroup2MountPoint,
		},
	}

	socketProbes = append(socketProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "socket",
	}, fentry, EntryAndExit)...)
	return socketProbes
}

// GetAllSocketProgramFunctions returns the list of socket functions
func GetAllSocketProgramFunctions() []string {
	return []string{
		"hook_sock_create",
		"hook_sock_release",
	}
}

// CheckCgroupSocketReturnCode checks if the return code is 1(accept)
//
//nolint:unused,deadcode
func CheckCgroupSocketReturnCode(progSpecs map[string]*ebpf.ProgramSpec) error {
	for _, progSpec := range progSpecs {
		if progSpec.Type == ebpf.CGroupSock {
			if IsProgAllowedToTCActShot(progSpec.Name) {
				continue
			}

			r0 := int32(255)

			for _, inst := range progSpec.Instructions {
				class := inst.OpCode.Class()
				if class.IsJump() {
					if inst.OpCode.JumpOp() == asm.Exit {
						fmt.Printf("code: %d\n", r0)

						if r0 != 1 {
							return fmt.Errorf("cgroup sock program %s is not using 1 as return code, %d, %v", progSpec.Name, r0, progSpec.Instructions)
						}
					}
				} else {
					op := inst.OpCode
					switch op {
					case asm.Mov.Op(asm.ImmSource),
						asm.LoadImmOp(asm.DWord),
						asm.LoadImmOp(asm.Word),
						asm.LoadImmOp(asm.Half),
						asm.LoadImmOp(asm.Byte):
						if inst.Dst == asm.R0 {
							r0 = int32(inst.Constant)
						}
					}
				}
			}
		}
	}

	return nil
}
