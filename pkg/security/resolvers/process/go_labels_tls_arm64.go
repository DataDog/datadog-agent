// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package process

import (
	"encoding/binary"
	"errors"
	"fmt"

	"go.opentelemetry.io/ebpf-profiler/asm/arm"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	"go.opentelemetry.io/ebpf-profiler/nativeunwind/elfunwindinfo"
	"golang.org/x/arch/arm64/arm64asm"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// runtime.load_g starts by loading runtime.iscgo before deciding how to
// retrieve the current goroutine pointer:
//
//	https://github.com/golang/go/blob/6885bad7dd86880be/src/runtime/tls_arm64.s#L11
//
// On Linux/arm64 the prologue is typically:
//
//	0x000000000007f260 <+0>:     adrp    x27, 0x1c2000 <runtime.mheap_+101440>
//	0x000000000007f264 <+4>:     ldrsb   x0, [x27, #284]
//	0x000000000007f268 <+8>:     cbz     x0, 0x7f278 <runtime.load_g+24>
//
// Contrary to CGO_ENABLED that can be set at build time by the user, runtime.iscgo
// is a runtime variable that defaults to false and is set to true only when runtime/cgo
// is actually linked.
//
// see https://github.com/open-telemetry/opentelemetry-ebpf-profiler/issues/1455 for more details.
//
// When iscgo is true, load_g reads g from TLS.
// When iscgo is false, load_g returns immediately and the runtime keeps g in r28 instead.
//
// We decode emitted assembly for `MOVB runtime.iscgo(SB), R0` to recover the absolute address of
// runtime.iscgo, then read that byte. A non-zero value means the runtime itself
// would take the TLS path.
func extractRuntimeIsCgo(f *pfelf.File, b []byte, pc int64) (bool, int, error) {
	const prologueSize = 2 * 4 // ADRP + LDRSB, one instruction each

	if len(b) < prologueSize {
		return false, 0, errors.New("code too short for runtime.iscgo prologue")
	}

	adrp, err := arm64asm.Decode(b[0:4])
	if err != nil {
		return false, 0, fmt.Errorf("error while decoding first instruction: %w", err)
	}
	if adrp.Op != arm64asm.ADRP {
		return false, 0, fmt.Errorf("expected ADRP, got %v", adrp.Op)
	}

	ldrsb, err := arm64asm.Decode(b[4:8])
	if err != nil {
		return false, 0, fmt.Errorf("error while decoding second instruction: %w", err)
	}
	if ldrsb.Op != arm64asm.LDRSB {
		return false, 0, fmt.Errorf("expected LDRSB, got %v", ldrsb.Op)
	}

	pcrel, ok := arm.DecodeImmediate(adrp.Args[1])
	if !ok {
		return false, 0, errors.New("failed to decode ADRP page address")
	}
	page := (pc + pcrel) & ^0xFFF

	mem, ok := ldrsb.Args[1].(arm64asm.MemImmediate)
	if !ok {
		return false, 0, fmt.Errorf("unexpected LDRSB operand type %T", ldrsb.Args[1])
	}
	offset, ok := arm.DecodeImmediate(mem)
	if !ok {
		return false, 0, errors.New("failed to decode LDRSB memory offset")
	}
	addr := page + offset

	runtimeIscgo, err := f.VirtualMemory(addr, 1, 1)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read runtime.iscgo: %w", err)
	}
	return runtimeIscgo[0] != 0, prologueSize, nil
}

// extractTLSGOffset returns the offset from the thread pointer (tpidr_el0) at
// which the Go runtime stores the current g, or 0 when the runtime does not keep
// g in TLS at all — on arm64 a !iscgo program keeps it in R28 only, and never
// writes the TLS slot. eBPF falls back to that register when the offset is 0.
//
// Symbol names come from .gopclntab, which the runtime needs for tracebacks and
// which `-ldflags=-s -w` therefore cannot strip.
//
// Ported from the OTel eBPF profiler's interpreter/go/tls_arm64.go (DataDog fork);
// update both together.
//
// https://github.com/golang/go/blob/6885bad7dd86880be/src/runtime/tls_arm64.s#L11
//
//	Gets compiled into:
//	0x000000000007f260 <+0>:     adrp    x27, 0x1c2000 <runtime.mheap_+101440>
//	0x000000000007f264 <+4>:     ldrsb   x0, [x27, #284]
//	0x000000000007f268 <+8>:     cbz     x0, 0x7f278 <runtime.load_g+24>
//	0x000000000007f26c <+12>:    mrs     x0, tpidr_el0
//	0x000000000007f270 <+16>:    mov     x27, #0x30                      // #48
//	0x000000000007f274 <+20>:    ldr     x28, [x0, x27]
//	0x000000000007f278 <+24>:    ret
//
// And, when compiled with -buildmode=pie:
//
//	0x00000000000c2290 <+0>:	adrp	x27, 0x2ca000 <runtime.itabTableInit+3072>
//	0x00000000000c2294 <+4>:	ldrsb	x0, [x27, #1766]
//	0x00000000000c2298 <+8>:	cbz	x0, 0xc22ac <runtime.load_g+28>
//	0x00000000000c229c <+12>:	mrs	x0, tpidr_el0
//	0x00000000000c22a0 <+16>:	movz	x27, #0x0, lsl #16
//	0x00000000000c22a4 <+20>:	movk	x27, #0x10
//	0x00000000000c22a8 <+24>:	ldr	x28, [x0, x27]
//	0x00000000000c22ac <+28>:	ret
func hasTPRelativeRelocation(f *pfelf.File, addr int64) (bool, error) {
	found := false
	err := visitRelocations(f, func(reloc ElfReloc, _ string, _ RelocType) bool {
		if reloc.Off == uint64(addr) {
			found = true
			return false
		}
		return true
	}, RelTPOFF64)
	return found, err
}

func goRuntimeTLSGOffsetFromTLSProgramHeader(f *pfelf.File) (int32, error) {
	for i := range f.Progs {
		prog := &f.Progs[i]
		if prog.Type != safeelf.PT_TLS {
			continue
		}
		if prog.Memsz > 1<<31-1 {
			return 0, fmt.Errorf("PT_TLS memsz %d overflows int32", prog.Memsz)
		}
		return int32(prog.Memsz), nil
	}
	return 0, errors.New("PT_TLS program header not found")
}

func extractRuntimeTLSGOffsetFromSlot(f *pfelf.File, b []byte, pc int64) (int32, bool, error) {
	// Some external linkers leave the runtime.tlsg access as an ADRP+LDR
	// sequence instead of relaxing it into MOVZ/MOVK immediates:
	//
	//	mrs	x0, tpidr_el0
	//	adrp	x27, runtime.tlsg@PAGE
	//	ldr	x27, [x27, runtime.tlsg@PAGEOFF]
	//	ldr	x28, [x0, x27]
	//
	// Decode the address of runtime.tlsg, read the offset stored there, and
	// verify that it is then used as the index in the final g load.
	if len(b) < 3*4 {
		return 0, false, nil
	}

	adrp, err := arm64asm.Decode(b[0:4])
	if err != nil {
		return 0, false, err
	}
	if adrp.Op != arm64asm.ADRP {
		return 0, false, nil
	}

	baseReg, ok := adrp.Args[0].(arm64asm.Reg)
	if !ok {
		return 0, false, nil
	}
	pcrel, ok := arm.DecodeImmediate(adrp.Args[1])
	if !ok {
		return 0, false, nil
	}

	loadOffset, err := arm64asm.Decode(b[4:8])
	if err != nil {
		return 0, false, err
	}
	if loadOffset.Op != arm64asm.LDR {
		return 0, false, nil
	}
	loadOffsetDst, ok := loadOffset.Args[0].(arm64asm.Reg)
	if !ok || loadOffsetDst != baseReg {
		return 0, false, nil
	}
	loadOffsetMem, ok := loadOffset.Args[1].(arm64asm.MemImmediate)
	if !ok || arm64asm.Reg(loadOffsetMem.Base) != baseReg {
		return 0, false, nil
	}
	pageOff, ok := arm.DecodeImmediate(loadOffsetMem)
	if !ok {
		return 0, false, nil
	}

	loadG, err := arm64asm.Decode(b[8:12])
	if err != nil {
		return 0, false, err
	}
	if loadG.Op != arm64asm.LDR {
		return 0, false, nil
	}
	loadGDst, ok := loadG.Args[0].(arm64asm.Reg)
	if !ok || loadGDst != arm64asm.X28 {
		return 0, false, nil
	}
	loadGMem, ok := loadG.Args[1].(arm64asm.MemExtend)
	if !ok || arm64asm.Reg(loadGMem.Base) != arm64asm.X0 || loadGMem.Index != baseReg {
		return 0, false, nil
	}

	addr := ((pc + pcrel) & ^int64(0xfff)) + pageOff
	data, err := f.VirtualMemory(addr, 8, 8)
	if err != nil {
		return 0, true, fmt.Errorf("failed to read runtime.tlsg: %w", err)
	}
	offset := binary.LittleEndian.Uint64(data)
	if offset == 0 {
		// In PIE binaries, older/different external linkers may leave a
		// R_AARCH64_TLS_TPREL64 dynamic relocation for this slot. The on-disk
		// value is zero and the dynamic linker fills in the real TP-relative
		// offset at load time. Go's runtime.tlsg slot is the final word in the
		// executable's TLS block, matching the immediate offset emitted when the
		// linker relaxes this access.
		hasRelocation, err := hasTPRelativeRelocation(f, addr)
		if err != nil {
			return 0, true, fmt.Errorf("failed to inspect runtime.tlsg relocation: %w", err)
		}
		if hasRelocation {
			offset, err := goRuntimeTLSGOffsetFromTLSProgramHeader(f)
			if err != nil {
				return 0, true, fmt.Errorf("failed to derive runtime.tlsg offset: %w", err)
			}
			return offset, true, nil
		}
	}
	if offset > 1<<31-1 {
		return 0, true, fmt.Errorf("runtime.tlsg offset %d overflows int32", offset)
	}
	return int32(offset), true, nil
}

func extractTLSGOffset(f *pfelf.File) (int32, error) {
	pclntab, err := elfunwindinfo.NewGopclntab(f)
	if err != nil {
		return 0, err
	}
	defer pclntab.Close()

	symbolName := "runtime.load_g.abi0"
	sym, err := pclntab.LookupSymbol(libpf.SymbolName(symbolName))
	if err != nil {
		// Use runtime.load_g as backup, if we can not identify
		// runtime.load_g.abi0. This can happen, if runtime.load_g.abi0
		// is inlined into runtime.load_g.
		symbolName = "runtime.load_g"
		sym, err = pclntab.LookupSymbol(libpf.SymbolName(symbolName))
		if err != nil {
			return 0, err
		}
	}

	pc := int64(sym.Address)
	b, err := f.VirtualMemory(pc, 32, 32)
	if err != nil {
		return 0, err
	}
	isCgo, consumed, err := extractRuntimeIsCgo(f, b, pc)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errRuntimeIsCgoUnavailable, err)
	}
	if !isCgo {
		// g lives in R28 only.
		return 0, nil
	}

	for off := consumed; len(b[off:]) >= 4; off += 4 {
		i, err := arm64asm.Decode(b[off : off+4])
		if err != nil {
			return 0, err
		}
		switch i.Op {
		case arm64asm.MOV:
			imm, ok := i.Args[1].(arm64asm.Imm64)
			if ok {
				return int32(imm.Imm), nil
			}
		case arm64asm.ADRP:
			if offset, ok, err := extractRuntimeTLSGOffsetFromSlot(f, b[off:], pc+int64(off)); ok || err != nil {
				return offset, err
			}
		case arm64asm.MOVK:
			// when compiled with -buildmode=pie, mov instruction is split into two instructions: movz and movk
			// movz is used to zero the register and set bits 16-31, while movk is used to set the lower 16 bits:
			// movz x27, #0x0, lsl #16
			// movk x27, #0x10
			// For now, we'll just decode the immediate value from the movk instruction since the one from the movz
			// instruction seems to always be 0.
			imm, ok := arm.DecodeImmediate(i.Args[1])
			if ok {
				return int32(imm), nil
			}
		}
	}
	return 0, fmt.Errorf("symbol '%s': %w", symbolName, errDecodeSymbol)
}
