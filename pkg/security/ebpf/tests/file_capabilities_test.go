// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && ebpf_bindata

package tests

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/cilium/ebpf/asm"
	"github.com/safchain/baloum/pkg/baloum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cpuVFSCapsFixture(words ...uint32) []byte {
	data := make([]byte, len(words)*4)
	for i, word := range words {
		binary.LittleEndian.PutUint32(data[i*4:], word)
	}
	return data
}

func TestExecFileCapabilitiesLayouts(t *testing.T) {
	const (
		magic     = 0x02000001 // VFS_CAP_REVISION_2 | VFS_CAP_FLAGS_EFFECTIVE
		permitted = uint64(1<<10 | 1<<33)
		inherited = uint64(1<<7 | 1<<37)
	)
	// CPU layouts, not the interleaved permitted/inheritable on-disk xattr.
	linux414 := cpuVFSCapsFixture(magic, 1<<10, 1<<1, 1<<7, 1<<5)
	linux61 := cpuVFSCapsFixture(magic, 1<<10, 1<<1, 1<<7, 1<<5, 0x12345678)
	linux63 := cpuVFSCapsFixture(magic, 0x12345678, 1<<10, 1<<1, 1<<7, 1<<5)
	// A synthetic layout with padding and nonadjacent masks verifies that the
	// reader follows the resolved offsets rather than known kernel layouts.
	custom := cpuVFSCapsFixture(magic, 0x11111111, 0x22222222, 1<<10, 1<<1, 0x33333333, 0x44444444, 1<<7, 1<<5)

	for _, tc := range []struct {
		name        string
		fixture     []byte
		permittedAt uint64
		inheritedAt uint64
		permitted   uint64
		inherited   uint64
		effective   uint32
		available   uint32
		failRead    int
	}{
		{"linux-4.14-20-bytes", linux414, 4, 12, permitted, inherited, 1, 1, 0},
		{"linux-6.1-rootid-last", linux61, 4, 12, permitted, inherited, 1, 1, 0},
		{"linux-6.3-rootid-first", linux63, 8, 16, permitted, inherited, 1, 1, 0},
		{"legacy-permitted-only", cpuVFSCapsFixture(magic, 1<<10, 0, 0, 0), 4, 12, 1 << 10, 0, 1, 1, 0},
		{"modern-permitted-only", cpuVFSCapsFixture(magic, 123, 1<<10, 0, 0, 0), 8, 16, 1 << 10, 0, 1, 1, 0},
		{"legacy-inheritable-only", cpuVFSCapsFixture(magic, 0, 0, 1<<7, 1<<5), 4, 12, 0, inherited, 1, 1, 0},
		{"legacy-empty", cpuVFSCapsFixture(0x02000000, 0, 0, 0, 0), 4, 12, 0, 0, 0, 1, 0},
		{"modern-empty-effective", cpuVFSCapsFixture(magic, 123, 0, 0, 0, 0), 8, 16, 0, 0, 1, 1, 0},
		{"resolved-custom-offsets", custom, 12, 28, permitted, inherited, 1, 1, 0},
		{"missing-offsets", linux414, 0, 0, 0, 0, 0, 0, 0},
		{"missing-permitted-offset", linux414, 0, 12, 0, 0, 0, 0, 0},
		{"missing-inheritable-offset", linux414, 4, 0, 0, 0, 0, 0, 0},
		{"magic-read-failure", linux63, 8, 16, 0, 0, 0, 0, 1},
		{"permitted-read-failure", linux63, 8, 16, 0, 0, 0, 0, 2},
		{"inheritable-read-failure", linux63, 8, 16, 0, 0, 0, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type read struct{ offset, size uint64 }
			var reads []read
			var input uint64
			vm := baloum.NewVM(loadColSpec(t), baloum.Opts{
				Logger: &testLogger{t: t, trace: trace},
				RawFncs: map[asm.BuiltinFunc]func(*baloum.VM, *asm.Instruction) error{
					asm.FnProbeRead: func(vm *baloum.VM, inst *asm.Instruction) error {
						regs := vm.Regs()
						reads = append(reads, read{regs[asm.R3] - input, regs[asm.R2]})
						var result int64
						if len(reads) == tc.failRead {
							result = -14 // EFAULT
						} else if err := baloum.FnProbeReadImpl(vm, inst); err != nil {
							return fmt.Errorf("probe_read outside the kernel-layout fixture: %w", err)
						}
						// Baloum's default probe_read copies bytes but does not set
						// R0; supply the helper's success/error return value too.
						ret := baloum.RawContext{Regs: vm.Regs()}
						ret.Regs[asm.R0] = uint64(result)
						ret.SetRegs(vm)
						return nil
					},
				},
			})
			input = vm.Heap().AllocWith(tc.fixture)
			output := vm.Heap().AllocWith(make([]byte, 24))
			// Use the raw baloum_ctx layout, not StdContext's x86 pt_regs
			// encoding, so the fixture runner is architecture-independent.
			context := make([]byte, 5*8)
			for i, value := range []uint64{input, tc.permittedAt, tc.inheritedAt, output} {
				binary.LittleEndian.PutUint64(context[i*8:], value)
			}
			ctx := baloum.RawContext{}
			ctx.Regs[asm.R1] = vm.Heap().AllocWith(context)
			code, err := vm.RunProgram(&ctx, "test/exec_file_capabilities")
			require.NoError(t, err)
			require.Equal(t, int64(1), code)

			data, err := vm.GetBytes(output, 24)
			require.NoError(t, err)
			assert.Equal(t, tc.permitted, binary.LittleEndian.Uint64(data[0:8]))
			assert.Equal(t, tc.inherited, binary.LittleEndian.Uint64(data[8:16]))
			assert.Equal(t, tc.effective, binary.LittleEndian.Uint32(data[16:20]))
			assert.Equal(t, tc.available, binary.LittleEndian.Uint32(data[20:24]))

			if tc.available != 0 || tc.failRead != 0 {
				wantReads := []read{{0, 4}, {tc.permittedAt, 8}, {tc.inheritedAt, 8}}
				if tc.failRead != 0 {
					wantReads = wantReads[:tc.failRead]
				}
				assert.Equal(t, wantReads, reads)
			} else {
				assert.Empty(t, reads, "unresolved offsets must not read kernel memory")
			}
		})
	}
}
