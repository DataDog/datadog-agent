// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && ebpf_bindata

// Package tests holds tests related files
package tests

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/safchain/baloum/pkg/baloum"
)

// prepareCredentialVM adapts unsupported VM data storage, without changing the
// program's control flow: socket storage is mocked, and binary constant pools
// must be materialized as bytes rather than Baloum's NUL-delimited strings.
func prepareCredentialVM(t *testing.T, vm *baloum.VM) {
	t.Helper()
	vm.Spec.Maps["sk_storage_pid"].Type = ebpf.Hash
	vm.Spec.Maps["sk_storage_pid"].MaxEntries = 1
	for _, prog := range vm.Spec.Programs {
		for i, ins := range prog.Instructions {
			if !strings.HasPrefix(ins.Reference(), ".rodata.cst") {
				continue
			}
			data := vm.Spec.Maps[ins.Reference()].Contents[0].Value.([]byte)
			offset := uint64(ins.Constant) >> 32
			if offset >= uint64(len(data)) || ins.Src != asm.PseudoMapValue {
				t.Fatalf("unexpected constant pool relocation: %v", ins)
			}
			ins.Constant = int64(vm.Heap().AllocWith(data) + offset)
			ins.Src = 0
			prog.Instructions[i] = ins.WithReference("")
		}
	}
}

func TestCredentialRecipientLegacy(t *testing.T) {
	vm := baloum.NewVM(loadColSpec(t), baloum.Opts{
		Logger: &testLogger{t: t, trace: trace},
		RawFncs: map[asm.BuiltinFunc]func(*baloum.VM, *asm.Instruction) error{
			asm.FnGetSocketCookie: func(vm *baloum.VM, _ *asm.Instruction) error {
				ctx := baloum.RawContext{Regs: vm.Regs()}
				ctx.Regs[asm.R0] = 1
				ctx.SetRegs(vm)
				return nil
			},
		},
	})
	prepareCredentialVM(t, vm)
	var ctx baloum.StdContext
	code, err := vm.RunProgram(&ctx, "test/credential_recipient_legacy")
	if err != nil || code != 1 {
		t.Fatalf("unexpected error: %v, return code: %d", err, code)
	}
}

func TestCredentialRecipientSocket(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		family, remote, local := "ipv4", net.ParseIP("169.254.170.2").To4(), net.ParseIP("10.0.0.5").To4()
		if ipv6 {
			family, remote, local = "ipv6", net.ParseIP("fd00:ec2::23").To16(), net.ParseIP("fd00::5").To16()
		}
		expectedTuple := append(append([]byte{}, remote...), local...)
		expectedTuple = binary.BigEndian.AppendUint16(expectedTuple, 80)
		expectedTuple = binary.BigEndian.AppendUint16(expectedTuple, 44000)
		for _, tc := range []struct {
			name           string
			socket         bool
			storage        bool
			otherNamespace bool
			kernelThread   bool
			want           int64
		}{
			{name: "recipient", socket: true, storage: true, want: 456},
			{name: "kernel thread", socket: true, storage: true, kernelThread: true},
			{name: "transit"},
			{name: "untracked socket", socket: true},
			{name: "different namespace", otherNamespace: true},
		} {
			t.Run(family+"/"+tc.name, func(t *testing.T) {
				spec := loadColSpec(t)
				prog := spec.Programs["test_credential_recipient_socket"]
				if prog == nil {
					t.Fatal("missing credential recipient test program")
				}
				offsets := prog.Instructions.ReferenceOffsets()["sk_lookup_pid_enabled"]
				if len(offsets) == 0 {
					t.Fatal("missing sk_lookup_pid_enabled constant")
				}
				for _, offset := range offsets {
					prog.Instructions[offset].Constant = 1
				}
				setReturn := func(vm *baloum.VM, value uint64) {
					ctx := baloum.RawContext{Regs: vm.Regs()}
					ctx.Regs[asm.R0] = value
					ctx.SetRegs(vm)
				}
				var socket uint64
				lookups, releases := 0, 0
				vm := baloum.NewVM(spec, baloum.Opts{
					Logger: &testLogger{t: t, trace: trace},
					RawFncs: map[asm.BuiltinFunc]func(*baloum.VM, *asm.Instruction) error{
						asm.FnSkLookupTcp: func(vm *baloum.VM, _ *asm.Instruction) error {
							lookups++
							regs := vm.Regs()
							tuple, err := vm.GetBytes(regs[asm.R2], regs[asm.R3])
							if err != nil {
								return err
							}
							if !bytes.Equal(expectedTuple, tuple) || regs[asm.R4] != ^uint64(0) || regs[asm.R5] != 0 {
								t.Errorf("incorrect recipient lookup: tuple %x, namespace %x, flags %x", tuple, regs[asm.R4], regs[asm.R5])
							}
							if tc.socket {
								socket = vm.Heap().Alloc(64)
							}
							setReturn(vm, socket)
							return nil
						},
						asm.FnSkStorageGet: func(vm *baloum.VM, _ *asm.Instruction) error {
							if vm.Regs()[asm.R2] != socket {
								t.Error("storage lookup did not use the recipient socket")
							}
							var address uint64
							if tc.storage {
								pid := uint32(456)
								if tc.kernelThread {
									pid = 2
								}
								address = vm.Heap().AllocWith(binary.NativeEndian.AppendUint32(nil, pid))
							}
							setReturn(vm, address)
							return nil
						},
						asm.FnSkRelease: func(vm *baloum.VM, _ *asm.Instruction) error {
							releases++
							if vm.Regs()[asm.R1] != socket {
								t.Error("released a different socket")
							}
							setReturn(vm, 0)
							return nil
						},
					},
				})
				prepareCredentialVM(t, vm)
				// StdContext uses the pt_regs layout, not this test's baloum_ctx.
				var ctx baloum.RawContext
				var arg0 uint64
				if ipv6 {
					arg0 = 1
				}
				data := binary.NativeEndian.AppendUint64(nil, arg0)
				var arg1 uint64
				if tc.otherNamespace {
					arg1 = 1
				}
				ctx.Regs[asm.R1] = vm.Heap().AllocWith(binary.NativeEndian.AppendUint64(data, arg1))
				code, err := vm.RunProgram(&ctx, "test/credential_recipient_socket")
				if err != nil || code != tc.want {
					t.Fatalf("unexpected error: %v, recipient pid: %d, want %d", err, code, tc.want)
				}
				wantReleases := 0
				if tc.socket {
					wantReleases = 1
				}
				wantLookups := 1
				if tc.otherNamespace {
					wantLookups = 0
				}
				if lookups != wantLookups || releases != wantReleases {
					t.Errorf("unexpected socket lookups/releases: %d/%d", lookups, releases)
				}
			})
		}
	}
}

func TestCredentialSource(t *testing.T) {
	for _, family := range []string{"ipv4", "ipv6"} {
		t.Run(family, func(t *testing.T) {
			var ctx baloum.StdContext
			code, err := newVM(t).RunProgram(&ctx, "test/credential_source_"+family)
			if err != nil || code != 1 {
				t.Fatalf("unexpected error: %v, return code: %d", err, code)
			}
		})
	}
}
