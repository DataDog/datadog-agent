// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package execinfomanager

import (
	"debug/elf"

	aa "golang.org/x/arch/arm64/arm64asm"

	"go.opentelemetry.io/ebpf-profiler/asm/arm"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	sdtypes "go.opentelemetry.io/ebpf-profiler/nativeunwind/stackdeltatypes"
	"go.opentelemetry.io/ebpf-profiler/support"
)

// regFP is the arm64 frame-pointer register (x29) number
const regFP = 29

// regLR is the arm64 link register (x30) number
const regLR = 30

// synthesizeIntervalData creates synthetic stack deltas if possible.
// Currently supported for ARM64 vDSO only.
func synthesizeIntervalData(ef *pfelf.File) sdtypes.IntervalData {
	if ef.Machine == elf.EM_AARCH64 {
		soname, err := ef.DynString(elf.DT_SONAME)
		if err == nil && soname[0] == "linux-vdso.so.1" {
			return createVDSOSyntheticRecordArm64(ef)
		}
	}
	return sdtypes.IntervalData{}
}

// createVDSOSyntheticRecordArm64 creates generated stack-delta records for ARM64 vDSO.
// ARM64 kernel vDSO does not have proper `.eh_frame` section, so we synthesize it here.
// This assumes LR based unwinding for most of the vDSO. Additionally the following
// synthetization is done:
//   - if matching STP/LDP is found within a dynamic symbol, an unwind rule with
//     is created and the frame size is extracted
//   - the sigreturn helper is detected and signal unwind info is associated for it
func createVDSOSyntheticRecordArm64(ef *pfelf.File) sdtypes.IntervalData {
	deltas := sdtypes.StackDeltaArray{}
	deltas = append(deltas, sdtypes.StackDelta{Address: 0, Info: sdtypes.UnwindInfoLR})
	_ = ef.VisitDynamicSymbols(func(sym libpf.Symbol) bool {
		addr := uint64(sym.Address)
		if sym.Name == "__kernel_rt_sigreturn" {
			deltas = append(
				deltas,
				sdtypes.StackDelta{Address: addr, Info: sdtypes.UnwindInfoSignal},
				sdtypes.StackDelta{Address: addr + sym.Size, Info: sdtypes.UnwindInfoLR},
			)
			return true
		}
		// Determine if LR is on stack
		code := make([]byte, sym.Size)
		if _, err := ef.ReadVirtualMemory(code, int64(sym.Address)); err != nil {
			return true
		}

		var frameStart uint64
		var frameSize int
		for offs := uint64(0); offs < sym.Size; offs += 4 {
			inst, err := aa.Decode(code[offs:])
			if err != nil {
				continue
			}
			switch inst.Op {
			case aa.RET:
				return true
			case aa.STP:
				if reg, ok := arm.Xreg2num(inst.Args[0]); !ok || reg != regFP {
					continue
				}
				if reg, ok := arm.Xreg2num(inst.Args[1]); !ok || reg != regLR {
					continue
				}
				imm, ok := arm.DecodeImmediate(inst.Args[2])
				if !ok {
					continue
				}
				imm = -imm
				if imm < 1024 {
					frameStart = offs + 4
					frameSize = int(imm)
				}
			case aa.LDP:
				if reg, ok := arm.Xreg2num(inst.Args[0]); !ok || reg != regFP {
					continue
				}
				if reg, ok := arm.Xreg2num(inst.Args[1]); !ok || reg != regLR {
					continue
				}
				if frameStart == 0 {
					return true
				}
				deltas = append(
					deltas,
					sdtypes.StackDelta{
						Address: addr + frameStart,
						Info: sdtypes.UnwindInfo{
							BaseReg:    support.UnwindRegFp,
							Param:      int32(frameSize),
							AuxBaseReg: support.UnwindRegFp,
							AuxParam:   8,
						},
					},
					sdtypes.StackDelta{Address: addr + offs + 4, Info: sdtypes.UnwindInfoLR},
				)
				frameStart = 0
			}
		}
		return true
	})
	deltas.Sort()

	return sdtypes.IntervalData{Deltas: deltas}
}
