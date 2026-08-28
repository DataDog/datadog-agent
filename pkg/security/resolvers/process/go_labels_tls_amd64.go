// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package process

import (
	"fmt"

	"go.opentelemetry.io/ebpf-profiler/asm/amd"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	"go.opentelemetry.io/ebpf-profiler/nativeunwind/elfunwindinfo"
)

// extractTLSGOffset returns the offset from the thread pointer (fsbase) at which
// the Go runtime stores the current g.
//
// Most normal amd64 Go binaries use -8, but statically linked ones end up at -80,
// and the value is ultimately picked by the linker from the size of the
// executable's TLS block. Rather than guess it from ELF metadata, decode it from
// the runtime's own g-load sequence: symbol names come from .gopclntab, which the
// runtime needs for tracebacks and which `-ldflags=-s -w` therefore cannot strip.
//
// Ported from the OTel eBPF profiler's interpreter/go/tls_amd64.go (DataDog fork);
// update both together.
func extractTLSGOffset(f *pfelf.File) (int32, error) {
	pclntab, err := elfunwindinfo.NewGopclntab(f)
	if err != nil {
		return 0, err
	}
	defer pclntab.Close()

	const symbolName = "runtime.stackcheck"

	// Dump of assembler code for function runtime.stackcheck:
	// 0x0000000000470080 <+0>:     mov    %fs:0xfffffffffffffff8,%rax
	// Binaries built with -buildmode=pie have a different assembly code for stackcheck with 2 movs:
	//  0x00000000007ec320 <+0>:	mov    $0xfffffffffffffff8,%rcx
	//  0x00000000007ec327 <+7>:	mov    %fs:(%rcx),%rax
	// In some binaries offset is stored relative to RIP:
	// 0x000000000017e34c0 <+0>: 	mov    0x40b9af9(%rip),%rcx        # 589cfc0 <runtime.tlsg@@Base+0x589cfc0>
	// 0x000000000017e34c7 <+7>:	mov    %fs:(%rcx),%rax
	sym, err := pclntab.LookupSymbol(libpf.SymbolName(symbolName))
	if err != nil {
		return 0, err
	}

	sz := int(min(sym.Size, 128))
	code, err := f.VirtualMemory(int64(sym.Address), sz, sz)
	if err != nil {
		return 0, err
	}

	offset, err := amd.ExtractTLSOffset(code, uint64(sym.Address), f)
	if err != nil {
		// Fall back to the conventional offset: decoding failing is far narrower
		// than "we could not look at the code at all", and -8 is right for every
		// internally linked binary. The caller logs and keeps going.
		return -8, fmt.Errorf("symbol '%s': %w", symbolName, errDecodeSymbol)
	}
	return offset, nil
}
