// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"debug/elf" //nolint:depguard
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// tlsAccess identifies how the otel_thread_ctx_v1 TLS variable is accessed,
// which determines how its address is resolved at attach time. Mirrors
// DataDog's opentelemetry-ebpf-profiler fork
// (interpreter/threadcontext/tlsaccess.go, PR #1229).
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

// otelTLSAccessData is the static-analysis result: how otel_thread_ctx_v1 is
// accessed, derived from the defining ELF object alone.
type otelTLSAccessData struct {
	// access selects how the TLS variable address is resolved at attach time.
	access tlsAccess
	// elfAddr is the (unbiased) ELF address of the TLS descriptor or GOT slot
	// used by the initial-exec, tlsdesc and gnu-dynamic access models.
	elfAddr uint64
	// offset is added to the base resolved at attach time: the full TP-relative
	// offset for local-exec, the symbol's static value for local-dynamic (whose
	// relocation resolves only the module), zero for the models that resolve
	// the offset entirely at attach time.
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
// Local-dynamic is special: its relocation references the module (symbol index
// 0) rather than the symbol, so the in-module offset comes from the symbol's
// static value instead.
func resolveTLSAccess(ef *safeelf.File, sym *safeelf.Symbol) (*otelTLSAccessData, error) {
	var tlsdescAddr, tpmodAddr, tpoffAddr uint64
	// Module-level relocations (not referencing any symbol) are local-dynamic
	// candidates: we keep the first one of each dialect as a fallback.
	var tlsdescNoSymAddr, tpmodNoSymAddr uint64

	if err := ef.VisitRelocations(func(r safeelf.ElfReloc, symName string,
		relType safeelf.RelocType) bool {
		switch symName {
		case otelTLSSymbolName:
			switch relType {
			case safeelf.RelTLSDESC:
				tlsdescAddr = r.Off
			case safeelf.RelDTPMOD64:
				tpmodAddr = r.Off
			case safeelf.RelTPOFF64:
				tpoffAddr = r.Off
			}
			return false
		case "":
			switch relType {
			case safeelf.RelTLSDESC:
				if tlsdescNoSymAddr == 0 {
					tlsdescNoSymAddr = r.Off
				}
			case safeelf.RelDTPMOD64:
				if tpmodNoSymAddr == 0 {
					tpmodNoSymAddr = r.Off
				}
			}
		}
		return true
	}, safeelf.RelTLSDESC|safeelf.RelDTPMOD64|safeelf.RelTPOFF64); err != nil {
		return nil, fmt.Errorf("failed to visit TLS relocations: %w", err)
	}

	switch {
	case tlsdescAddr != 0:
		return &otelTLSAccessData{access: accessTLSDesc, elfAddr: tlsdescAddr}, nil
	case tpmodAddr != 0:
		return &otelTLSAccessData{access: accessGlobalDynamic, elfAddr: tpmodAddr}, nil
	case tpoffAddr != 0:
		return &otelTLSAccessData{access: accessInitialExec, elfAddr: tpoffAddr}, nil
	}

	// Local-dynamic: the symbol is local to a shared object, so only the
	// module-level relocation is available.
	switch {
	case tlsdescNoSymAddr != 0:
		return &otelTLSAccessData{access: accessTLSDesc, elfAddr: tlsdescNoSymAddr,
			offset: sym.Value}, nil
	case tpmodNoSymAddr != 0:
		return &otelTLSAccessData{access: accessLocalDynamic, elfAddr: tpmodNoSymAddr,
			offset: sym.Value}, nil
	}

	// No relocation references the symbol directly.
	if isExecutable(ef) {
		tlsOffset, err := getStaticTLSOffset(ef, sym)
		if err != nil {
			return nil, fmt.Errorf("failed to get static TLS offset: %w", err)
		}
		return &otelTLSAccessData{access: accessLocalExec, offset: tlsOffset}, nil
	}

	return nil, errors.New("unsupported TLS model")
}

func isExecutable(ef *safeelf.File) bool {
	switch ef.Type {
	case elf.ET_EXEC:
		return true

	case elf.ET_DYN:
		// Ambiguous: shared library or PIE executable. DF_1_PIE is the
		// canonical discriminator.
		if vals, err := ef.DynValue(elf.DT_FLAGS_1); err == nil {
			for _, v := range vals {
				if v&uint64(elf.DF_1_PIE) != 0 {
					return true
				}
			}
		}
		// Older toolchains omit DF_1_PIE: fall back to PT_INTERP, which a
		// dynamically-linked executable carries and a plain shared library does
		// not. Imperfect: libc.so.6 has one, a static PIE may not.
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

func getTLSProg(ef *safeelf.File) *elf.Prog {
	for i := range ef.Progs {
		if ef.Progs[i].Type == elf.PT_TLS {
			return ef.Progs[i]
		}
	}
	return nil
}

// getStaticTLSOffset computes the thread-pointer-relative offset of a local-exec
// TLS variable defined in the main executable's static TLS block. sym.Value is
// the symbol's offset within the PT_TLS image.
func getStaticTLSOffset(ef *safeelf.File, sym *safeelf.Symbol) (uint64, error) {
	tlsProg := getTLSProg(ef)
	if tlsProg == nil {
		return 0, errors.New("failed to locate TLS segment")
	}
	align := tlsProg.Align
	if align == 0 {
		align = 1
	}

	switch ef.Machine {
	case elf.EM_AARCH64:
		// Variant I: the static TLS block sits above TP, at the first
		// align-aligned offset past a 16-byte reserved area (TCB on glibc,
		// GAP_ABOVE_TP on musl).
		return roundUp(16, align) + sym.Value, nil
	case elf.EM_X86_64:
		// Variant II: the executable's TLS block sits immediately below TP, its
		// size rounded up to the block alignment.
		return sym.Value - roundUp(tlsProg.Memsz, align), nil
	}
	return 0, fmt.Errorf("unsupported machine: %s", ef.Machine)
}
