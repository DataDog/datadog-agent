// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file is a copy of interpreter/threadcontext/tlsaccess.go of the
// OpenTelemetry eBPF profiler as of PR #1229
// (https://github.com/open-telemetry/opentelemetry-ebpf-profiler/pull/1229),
// Copyright The OpenTelemetry Authors, SPDX-License-Identifier: Apache-2.0,
// plus the tlsExport constant and the data struct that resolveTLSAccess returns,
// both from that PR's interpreter/threadcontext/threadcontext.go.
//
// It exists because upstream keeps this classification unexported inside a
// package built around interpreter.Loader/interpreter.Data, so there is nothing
// to call from here even once the PR lands. Removing the copy needs upstream to
// expose the static analysis on its own -- resolveTLSAccess and the access
// model it returns -- independently of the interpreter plumbing; until then,
// keep this file a copy so it can be diffed against upstream and dropped whole.
//
// Deviations from upstream, all mechanical:
//   - the package, the imports, and the tlsExport value (this codebase names the
//     symbol otelTLSSymbolName),
//   - VisitRelocations and DynValue are called as the free functions of
//     otel_tls_upstream_pfelf.go, since the pinned pfelf has neither,
//   - one nolint, for a fmt.Errorf this repo's linter wants as errors.New.

//go:build linux

package process

import (
	"debug/elf" //nolint:depguard
	"errors"
	"fmt"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
)

// tlsExport defines the name of the thread info TLS export.
const tlsExport = otelTLSSymbolName

// tlsAccess identifies how the otel_thread_ctx_v1 TLS variable is accessed,
// which determines how its address is resolved at attach time.
type tlsAccess uint8

const (
	// accessTLSDesc: a TLS descriptor (GNU2/desc dialect) whose resolved argument
	// is either a static TP-relative offset or a pointer to a tls_index struct
	// for dynamic TLS. Covers general-dynamic and local-dynamic.
	accessTLSDesc tlsAccess = iota
	// accessLocalExec: the variable lives in the static TLS block and its
	// TP-relative offset is known at load time.
	accessLocalExec
	// accessInitialExec: a GOT slot holds the variable's TP-relative offset,
	// filled in by the dynamic loader.
	accessInitialExec
	// accessLocalDynamic: a GOT tls_index whose module_id is filled in by the
	// loader, but whose in-module offset is the symbol's static value
	// (GNU dialect, local-dynamic).
	accessLocalDynamic
	// accessGlobalDynamic: a GOT tls_index {module_id, offset} pair (GNU dialect,
	// general-dynamic), both words filled in by the dynamic loader.
	accessGlobalDynamic
)

func (a tlsAccess) String() string {
	switch a {
	case accessTLSDesc:
		return "tlsdesc"
	case accessLocalExec:
		return "local-exec"
	case accessInitialExec:
		return "initial-exec"
	case accessLocalDynamic:
		return "local-dynamic"
	case accessGlobalDynamic:
		return "global-dynamic"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(a))
	}
}

type data struct {
	// access selects how the TLS variable address is resolved at attach time.
	access tlsAccess
	// elfAddr is the (unbiased) ELF address of the TLS descriptor or GOT slot
	// used by the initial-exec, tlsdesc and gnu-dynamic access models.
	elfAddr libpf.Address
	// offset is a statically-known offset added to the base resolved at runtime.
	// For local-exec the runtime base is zero, so it holds the full TP-relative
	// offset. For local-dynamic it holds the symbol's static value (the relocation
	// only resolves the module, not the per-variable offset). It is zero for the
	// other models, where the offset is fully resolved at runtime.
	offset uint64
}

// resolveTLSAccess determines how the otel_thread_ctx_v1 TLS variable is
// accessed, covering both TLS dialects (GNU and GNU2/desc) and all four access
// models (local-exec, initial-exec, general-dynamic and local-dynamic).
//
// The access model and dialect are inferred from the relocation type that
// references the symbol:
//   - TLSDESC                        -> general/local-dynamic, GNU2/desc dialect
//   - DTPMOD64 (+ DTPOFF64 GOT slot) -> general-dynamic, GNU dialect
//   - TPOFF64                        -> initial-exec
//   - no relocation, executable      -> local-exec (static TLS block)
//
// The local-dynamic model is special: the relocation does not reference the
// symbol but the module (symbol index 0), because the per-variable offset is
// resolved separately in code. In that case the in-module offset is the
// symbol's static value (offset).
func resolveTLSAccess(ef *pfelf.File, sym *libpf.Symbol) (*data, error) {
	var tlsdescAddr, tpmodAddr, tpoffAddr libpf.Address
	// Module-level relocations (not referencing any symbol) are local-dynamic
	// candidates: we keep the first one of each dialect as a fallback.
	var tlsdescNoSymAddr, tpmodNoSymAddr libpf.Address

	if err := visitRelocations(ef, func(r ElfReloc, symName string,
		relType RelocType) bool {
		switch symName {
		case tlsExport:
			switch relType {
			case RelTLSDESC:
				tlsdescAddr = libpf.Address(r.Off)
			case RelDTPMOD64:
				tpmodAddr = libpf.Address(r.Off)
			case RelTPOFF64:
				tpoffAddr = libpf.Address(r.Off)
			}
			return false
		case "":
			switch relType {
			case RelTLSDESC:
				if tlsdescNoSymAddr == 0 {
					tlsdescNoSymAddr = libpf.Address(r.Off)
				}
			case RelDTPMOD64:
				if tpmodNoSymAddr == 0 {
					tpmodNoSymAddr = libpf.Address(r.Off)
				}
			}
		}
		return true
	}, RelTLSDESC|RelDTPMOD64|RelTPOFF64); err != nil {
		return nil, fmt.Errorf("failed to visit TLS relocations: %v", err)
	}

	switch {
	case tlsdescAddr != 0:
		// General-dynamic, GNU2/desc dialect.
		return &data{access: accessTLSDesc, elfAddr: tlsdescAddr}, nil
	case tpmodAddr != 0:
		// General-dynamic, GNU dialect.
		return &data{access: accessGlobalDynamic, elfAddr: tpmodAddr}, nil
	case tpoffAddr != 0:
		// Initial-exec.
		return &data{access: accessInitialExec, elfAddr: tpoffAddr}, nil
	}

	// Local-dynamic: the symbol is local to a shared object (not preemptible).
	// The module-level relocation provides the module ID at runtime, the in-module
	// offset is the symbol's static value.
	switch {
	case tlsdescNoSymAddr != 0:
		return &data{access: accessTLSDesc, elfAddr: tlsdescNoSymAddr,
			offset: uint64(sym.Address)}, nil
	case tpmodNoSymAddr != 0:
		return &data{access: accessLocalDynamic, elfAddr: tpmodNoSymAddr,
			offset: uint64(sym.Address)}, nil
	}

	// No relocation references the symbol directly.
	if isExecutable(ef) {
		// Local-exec: the variable lives in the main executable's static TLS
		// block and its TP-relative offset is known at load time.
		tlsOffset, err := getStaticTLSOffset(ef, sym)
		if err != nil {
			return nil, fmt.Errorf("failed to get static TLS offset: %v", err)
		}
		return &data{access: accessLocalExec, offset: tlsOffset}, nil
	}

	return nil, errors.New("unsupported TLS model")
}

func isExecutable(ef *pfelf.File) bool {
	switch ef.Type {
	case elf.ET_EXEC:
		// Classic, non-PIE executable.
		return true

	case elf.ET_DYN:
		// Ambiguous: either a shared library or a PIE executable.
		// The DF_1_PIE flag is the canonical discriminator.
		if vals, err := dynValue(ef, elf.DT_FLAGS_1); err == nil {
			for _, v := range vals {
				if v&uint64(elf.DF_1_PIE) != 0 {
					return true
				}
			}
		}
		// Fallback for older toolchains that didn't emit DF_1_PIE:
		// a dynamically-linked executable carries a PT_INTERP segment,
		// whereas a plain shared library does not.
		// This fallback is not perfect:
		// - libc.so.6 is a shared library, but has an interpreter segment.
		// - a statically-linked PIE executable might does not have an interpreter segment.
		for _, p := range ef.Progs {
			if p.Type == elf.PT_INTERP {
				return true
			}
		}
	}

	return false
}

func roundUp(value, alignment uint64) uint64 {
	return (value + alignment - 1) &^ (alignment - 1)
}

func getTLSProg(ef *pfelf.File) *pfelf.Prog {
	for _, prog := range ef.Progs {
		if prog.Type == elf.PT_TLS {
			return &prog
		}
	}
	return nil
}

// getStaticTLSOffset computes the thread-pointer-relative offset of a local-exec
// TLS variable defined in the main executable's static TLS block. sym.Address is
// the symbol's offset within the PT_TLS image.
func getStaticTLSOffset(ef *pfelf.File, sym *libpf.Symbol) (uint64, error) {
	tlsProg := getTLSProg(ef)
	if tlsProg == nil {
		return 0, fmt.Errorf("failed to locate TLS segment") //nolint:perfsprint // kept as upstream wrote it
	}
	align := uint64(tlsProg.Align)
	if align == 0 {
		align = 1
	}

	switch ef.Machine {
	case elf.EM_AARCH64:
		// Variant I: the static TLS block sits above TP, at the first
		// align-aligned offset past a 16-byte reserved area (TCB on glibc,
		// GAP_ABOVE_TP on musl).
		return roundUp(16, align) + uint64(sym.Address), nil
	case elf.EM_X86_64:
		// Variant II: the executable's TLS block sits immediately below TP, its
		// size rounded up to the block alignment.
		return uint64(sym.Address) - roundUp(uint64(tlsProg.Memsz), align), nil
	}
	return 0, fmt.Errorf("unsupported machine: %s", ef.Machine)
}
