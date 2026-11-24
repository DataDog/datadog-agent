// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && ebpf_bindata

// Package tests holds tests related files
package tests

import (
	"encoding/binary"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/safchain/baloum/pkg/baloum"
	"github.com/stretchr/testify/require"
)

func TestFlagApprover(t *testing.T) {
	var ctx baloum.StdContext
	code, err := newVM(t).RunProgram(&ctx, "test/flag_approver")
	if err != nil || code != 1 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

const (
	syscallStateDiscarded = int64(2)
	syscallStateSampled   = int64(3)
)

func newSamplerTestVM(t *testing.T, spec *ebpf.CollectionSpec) *baloum.VM {
	return baloum.NewVM(spec, baloum.Opts{
		Logger: &testLogger{t: t, trace: trace},
		Fncs: baloum.Fncs{
			KtimeGetNS: func(_ *baloum.VM) (uint64, error) {
				return 1, nil
			},
		},
		RawFncs: map[asm.BuiltinFunc]func(*baloum.VM, *asm.Instruction) error{
			asm.FnGetPrandomU32: func(vm *baloum.VM, _ *asm.Instruction) error {
				ret := baloum.RawContext{Regs: vm.Regs()}
				ret.Regs[asm.R0] = 42
				ret.SetRegs(vm)
				return nil
			},
			asm.FnGetCurrentTask: func(vm *baloum.VM, _ *asm.Instruction) error {
				ret := baloum.RawContext{Regs: vm.Regs()}
				ret.Regs[asm.R0] = 0
				ret.SetRegs(vm)
				return nil
			},
			asm.FnGetCurrentComm: func(vm *baloum.VM, _ *asm.Instruction) error {
				ret := baloum.RawContext{Regs: vm.Regs()}
				ret.Regs[asm.R0] = 0
				ret.SetRegs(vm)
				return nil
			},
		},
	})
}

func TestApproveBindConnectSample(t *testing.T) {
	spec := loadColSpec(t)
	for _, mapName := range []string{"connect_samples", "excluded_cgroups", "proc_cache", "pid_cache"} {
		m := spec.Maps[mapName].Copy()
		m.MaxEntries = 16
		spec.Maps[mapName] = m
	}
	vm := newSamplerTestVM(t, spec)
	procBuf := vm.Heap().AllocWith(make([]byte, 512))

	run := func(cgroupID, excludedCgroupID, port uint64) int64 {
		context := make([]byte, 4*8)
		binary.LittleEndian.PutUint64(context[0:8], procBuf)
		binary.LittleEndian.PutUint64(context[8:16], cgroupID)
		binary.LittleEndian.PutUint64(context[16:24], excludedCgroupID)
		binary.LittleEndian.PutUint64(context[24:32], port)
		ctx := baloum.RawContext{}
		ctx.Regs[asm.R1] = vm.Heap().AllocWith(context)
		code, err := vm.RunProgram(&ctx, "test/approve_bind_connect_sample")
		require.NoError(t, err)
		return code
	}

	require.Equal(t, syscallStateSampled, run(100, 0, 443), "first connect for unexcluded cgroup should be sampled")
	require.Equal(t, syscallStateDiscarded, run(100, 0, 443), "duplicate connect for same endpoint should be discarded")
	require.Equal(t, syscallStateDiscarded, run(100, 100, 8443), "connect for excluded cgroup should be discarded")
}

func TestOpenApproversSample(t *testing.T) {
	for _, tc := range []struct {
		name         string
		hostEnabled  uint64
		expectedHost int64
	}{
		{name: "host-disabled", hostEnabled: 0, expectedHost: syscallStateDiscarded},
		{name: "host-enabled", hostEnabled: 1, expectedHost: syscallStateSampled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := loadColSpec(t)
			for _, mapName := range []string{"open_samples", "excluded_cgroups", "proc_cache", "pid_cache", "pid_path_keys"} {
				m := spec.Maps[mapName].Copy()
				m.MaxEntries = 16
				spec.Maps[mapName] = m
			}
			constants := map[string]uint64{
				"event_sampling_open_enabled":      1,
				"event_sampling_open_host_enabled": tc.hostEnabled,
			}
			for _, progSpec := range spec.Programs {
				for i := range progSpec.Instructions {
					ins := &progSpec.Instructions[i]
					if val, ok := constants[ins.Reference()]; ok && ins.IsConstantLoad(asm.DWord) {
						ins.Constant = int64(val)
					}
				}
			}

			vm := newSamplerTestVM(t, spec)
			syscallBuf := vm.Heap().AllocWith(make([]byte, 512))
			procBuf := vm.Heap().AllocWith(make([]byte, 512))

			run := func(fromTerminateWalk, cgroupID, excludedCgroupID uint64) int64 {
				context := make([]byte, 5*8)
				binary.LittleEndian.PutUint64(context[0:8], syscallBuf)
				binary.LittleEndian.PutUint64(context[8:16], fromTerminateWalk)
				binary.LittleEndian.PutUint64(context[16:24], procBuf)
				binary.LittleEndian.PutUint64(context[24:32], cgroupID)
				binary.LittleEndian.PutUint64(context[32:40], excludedCgroupID)
				ctx := baloum.RawContext{}
				ctx.Regs[asm.R1] = vm.Heap().AllocWith(context)
				code, err := vm.RunProgram(&ctx, "test/open_approvers_sample")
				require.NoError(t, err)
				return code
			}

			require.Equal(t, syscallStateDiscarded, run(1, 100, 0), "terminate_walk open must not be sampled")
			require.Equal(t, syscallStateSampled, run(0, 100, 0), "vfs_open in container cgroup 100 should be sampled")
			require.Equal(t, syscallStateDiscarded, run(0, 100, 0), "duplicate vfs_open in same cgroup 100 should be discarded")
			require.Equal(t, syscallStateSampled, run(0, 200, 0), "same binary and file in container cgroup 200 must not collide with cgroup 100")
			require.Equal(t, tc.expectedHost, run(0, 300, 300), "excluded host cgroup 300 should respect event_sampling_open_host_enabled")
		})
	}
}
