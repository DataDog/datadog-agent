// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package ptracer

import (
	"debug/elf" //nolint:depguard
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
)

// otelTLSSymbolName is the TLS symbol name defined by OTel spec PR #4947,
// mirroring pkg/security/resolvers/process/otel_tls.go.
const otelTLSSymbolName = "otel_thread_ctx_v1"

// otelTLSExportSize is the expected size of otel_thread_ctx_v1: it holds a
// pointer to the active Thread Local Context Record, not the record itself.
const otelTLSExportSize = 8

// errOTelStaticTLSNotFound means the executable doesn't export
// otel_thread_ctx_v1 through the one access model this reader understands --
// not that resolution itself failed. A process with no OTel instrumentation
// at all, a PIE/dynamically linked one, or one using any TLS access model
// other than local-exec (initial-exec, local/global-dynamic, tlsdesc) all
// resolve to this.
var errOTelStaticTLSNotFound = errors.New("no local-exec otel_thread_ctx_v1 export found")

// resolveOTelStaticTLSOffset computes the thread-pointer-relative offset of
// the otel_thread_ctx_v1 TLS export for a statically linked, non-PIE
// executable (ELF ET_EXEC): the "local-exec" TLS access model. It is the only
// model the static linker leaves behind for a symbol defined in the final
// executable itself, so this needs no relocation/GOT lookup and no read of
// the live process: just the symbol's static value and the TLS segment's
// layout, both from the ELF file on disk.
//
// This mirrors the accessLocalExec branch of resolveTLSAccess/
// getStaticTLSOffset in
// pkg/security/resolvers/process/otel_tls_upstream_access.go (itself a copy
// of upstream OTel eBPF profiler PR #1229), restricted to that one case and
// reimplemented against the stdlib debug/elf instead of pfelf so this
// eBPF-less spike doesn't have to pull that dependency (and the DTV/GOT
// machinery it exists for) into the injected cws-instrumentation binary.
//
// Known gap: unlike upstream, this does not first check that no relocation
// references the symbol. That's safe for a fully static (no PT_INTERP)
// ET_EXEC binary -- there is no dynamic linker at runtime to resolve any
// other model -- but a dynamically linked, non-PIE ET_EXEC binary could in
// principle still be misclassified as local-exec if the toolchain left an
// unrelaxed reference; this has not been observed in practice, since `ld`
// always relaxes TLS references to a symbol defined in the final executable.
func resolveOTelStaticTLSOffset(exePath string) (int64, error) {
	ef, err := elf.Open(exePath)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", exePath, err)
	}
	defer ef.Close()

	if ef.Type != elf.ET_EXEC || ef.Class != elf.ELFCLASS64 || ef.Machine != elf.EM_X86_64 {
		return 0, errOTelStaticTLSNotFound
	}

	sym, err := findOTelStaticTLSSymbol(ef)
	if err != nil {
		return 0, err
	}

	tlsProg := findTLSProgHeader(ef)
	if tlsProg == nil {
		return 0, errOTelStaticTLSNotFound
	}
	align := tlsProg.Align
	if align == 0 {
		align = 1
	}

	// x86_64 Variant II: the executable's static TLS block sits immediately
	// below the thread pointer, its size rounded up to the block alignment.
	memsz := roundUp(tlsProg.Memsz, align)
	return int64(sym.Value) - int64(memsz), nil
}

func findOTelStaticTLSSymbol(ef *elf.File) (*elf.Symbol, error) {
	for _, symsFunc := range []func() ([]elf.Symbol, error){ef.DynamicSymbols, ef.Symbols} {
		syms, err := symsFunc()
		if err != nil {
			continue
		}
		for i := range syms {
			sym := &syms[i]
			if sym.Name != otelTLSSymbolName || elf.ST_TYPE(sym.Info) != elf.STT_TLS {
				continue
			}
			if sym.Size != otelTLSExportSize {
				return nil, fmt.Errorf("%w: unexpected size %d", errOTelStaticTLSNotFound, sym.Size)
			}
			return sym, nil
		}
	}
	return nil, errOTelStaticTLSNotFound
}

func findTLSProgHeader(ef *elf.File) *elf.Prog {
	for _, prog := range ef.Progs {
		if prog.Type == elf.PT_TLS {
			return prog
		}
	}
	return nil
}

func roundUp(value, alignment uint64) uint64 {
	return (value + alignment - 1) &^ (alignment - 1)
}

// otelThreadCtxRecordSize matches sizeof(struct otel_thread_ctx_record_t)
// (span_context.h): 16-byte trace id + 8-byte span id + valid + reserved +
// u16 attrs_data_size.
const otelThreadCtxRecordSize = 16 + 8 + 1 + 1 + 2

// otelThreadCtxValidOffset matches OTEL_THREAD_CTX_VALID_OFFSET.
const otelThreadCtxValidOffset = 16 + 8

// readOTelSpanContextStaticTLS reads the active span/trace id published by a
// statically linked, native (non-Go) OTel-instrumented thread, entirely from
// user space via ptrace + process_vm_readv, mirroring fill_span_context_otel()
// in pkg/security/ebpf/c/include/helpers/span_otel.h for the static-TLS case
// (otel_tls_t.module_id == 0): the record pointer sits directly at
// thread_pointer + tls_offset, with no DTV indirection.
//
// tid must be a thread this tracer already has stopped (PTRACE_GETREGS only
// returns valid register state during a ptrace-stop). tlsOffset is the value
// resolveOTelStaticTLSOffset computed for this thread's tgid.
//
// A zero traceIDHi/Lo/spanID with ok == false means there is no current span
// on this thread right now (either no record was ever published, or the
// instrumented thread is mid-update and the read was torn) -- not an error.
//
// Only works on amd64: the thread pointer here comes from PTRACE_GETREGS's
// fs_base, which syscall.PtraceRegs only exposes on amd64. On arm64 the
// thread pointer is tpidr_el0, which needs PTRACE_GETREGSET(NT_ARM_TLS)
// instead -- see otel_span_context_unsupported.go.
func (t *Tracer) readOTelSpanContextStaticTLS(tid int, tlsOffset int64) (traceIDHi, traceIDLo, spanID uint64, ok bool, err error) {
	var regs syscall.PtraceRegs
	if err := syscall.PtraceGetRegs(tid, &regs); err != nil {
		return 0, 0, 0, false, fmt.Errorf("ptrace getregs tid %d: %w", tid, err)
	}
	threadPointer := regs.Fs_base

	recordAddr := threadPointer + uint64(tlsOffset)

	// valid is checked on both sides of the copy, exactly like
	// fill_span_context_otel(): the instrumented thread clears it while it
	// updates the record, so a torn read is rejected rather than reported.
	validBefore, err := t.readData(tid, recordAddr+otelThreadCtxValidOffset, 1)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("read valid flag: %w", err)
	}
	if validBefore[0] != 1 {
		return 0, 0, 0, false, nil
	}

	record, err := t.readData(tid, recordAddr, otelThreadCtxRecordSize)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("read record at %#x: %w", recordAddr, err)
	}

	validAfter, err := t.readData(tid, recordAddr+otelThreadCtxValidOffset, 1)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("read valid flag (after): %w", err)
	}
	if record[otelThreadCtxValidOffset] != 1 || validAfter[0] != 1 {
		return 0, 0, 0, false, nil
	}

	// The W3C trace/span id bytes are big-endian.
	traceIDHi = binary.BigEndian.Uint64(record[0:8])
	traceIDLo = binary.BigEndian.Uint64(record[8:16])
	spanID = binary.BigEndian.Uint64(record[16:24])
	return traceIDHi, traceIDLo, spanID, true, nil
}
