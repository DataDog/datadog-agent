// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file is a copy of the additions that PR #1229 of the OpenTelemetry eBPF
// profiler (https://github.com/open-telemetry/opentelemetry-ebpf-profiler/pull/1229)
// makes to libpf/pfelf/file.go, Copyright The OpenTelemetry Authors,
// SPDX-License-Identifier: Apache-2.0.
//
// It exists only until the profiler revision the agent pins carries them: the
// pinned pfelf classifies no TPOFF64 relocation, does not report which
// relocation type matched, and has no DynValue -- all three of which
// resolveTLSAccess needs. The same PR adds libpf.Symbol.Info, the symbol type
// findOTelTLSSymbol needs to tell a TLS symbol from a same-named data one; that
// field is not copied here, the symbol walk of otel_tls.go filling in for it.
//
// To remove once the pin includes PR #1229:
//   - delete this file,
//   - visitRelocations(ef, visitor, mask) -> ef.VisitRelocations(visitor, mask),
//   - dynValue(ef, tag)                   -> ef.DynValue(tag),
//   - ElfReloc, RelocType, RelTLSDESC & co. -> their pfelf.* equivalents,
//   - use pfelf for symbol lookup instead of safeelf. Having no Info is the only
//     reason why otel_tls depends on safeelf.
//
// Deviations from upstream, kept to what a copy living outside pfelf needs:
//   - the two File methods are free functions taking the file, and the
//     relocation walk streams through the exported Section.ReadAt instead of
//     pfelf's unexported elfReader,
//   - getString and maxBytesLargeSection are copied from the same upstream file,
//     being pfelf-private helpers this code calls,
//   - VisitTLSRelocations, whose only change upstream is to adapt to the new
//     visitor signature, is left out: nothing here calls it,
//   - a file-level nolint for misspell, which reads the ELF relocation-section
//     spelling used throughout as a typo of "real".

//go:build linux

//nolint:misspell // RELA is the ELF term throughout, not a typo of "real"
package process

import (
	"bytes"
	"debug/elf" //nolint:depguard
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"

	"go.opentelemetry.io/ebpf-profiler/libpf/pfbufio"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfunsafe"
)

// maxBytesLargeSection is the maximum section size accepted for a section read
// whole into memory.
const maxBytesLargeSection = 16 * 1024 * 1024

// ElfReloc is a 64-bit RELA relocation entry.
type ElfReloc = elf.Rela64

// RelocType represents an architecture-independent relocation type.
// Multiple values can be combined with bitwise OR to match several types.
type RelocType uint32

const (
	// RelTLSDESC matches TLSDESC relocations (R_AARCH64_TLSDESC, R_X86_64_TLSDESC).
	RelTLSDESC RelocType = 1 << iota
	// RelDTPMOD64 matches DTPMOD64 relocations (R_AARCH64_TLS_DTPMOD64, R_X86_64_DTPMOD64).
	RelDTPMOD64
	// RelTPOFF64 matches TP-relative relocations (R_AARCH64_TLS_TPREL64, R_X86_64_TPOFF64)
	RelTPOFF64
)

// classifyRelocAarch64 returns the RelocType for an AARCH64 relocation.
func classifyRelocAarch64(rela ElfReloc) RelocType {
	switch elf.R_AARCH64(rela.Info & 0xffff) {
	case elf.R_AARCH64_TLSDESC:
		return RelTLSDESC
	case elf.R_AARCH64_TLS_DTPMOD64:
		return RelDTPMOD64
	case elf.R_AARCH64_TLS_TPREL64:
		return RelTPOFF64
	default:
		return 0
	}
}

// classifyRelocX86_64 returns the RelocType for an X86_64 relocation.
func classifyRelocX86_64(rela ElfReloc) RelocType {
	switch elf.R_X86_64(rela.Info & 0xffff) {
	case elf.R_X86_64_TLSDESC:
		return RelTLSDESC
	case elf.R_X86_64_DTPMOD64:
		return RelDTPMOD64
	case elf.R_X86_64_TPOFF64:
		return RelTPOFF64
	default:
		return 0
	}
}

// visitRelocations visits all relocations whose type matches the relTypes
// bitmask and provides the relocation, symbol name and matched RelocType to the
// visitor. The visitor can return false to stop iteration.
func visitRelocations(f *pfelf.File, visitor func(ElfReloc, string, RelocType) bool,
	relTypes RelocType) error {
	var classify func(ElfReloc) RelocType
	switch f.Machine {
	case elf.EM_AARCH64:
		classify = classifyRelocAarch64
	case elf.EM_X86_64:
		classify = classifyRelocX86_64
	default:
		return nil
	}
	var err error
	if err = f.LoadSections(); err != nil {
		return err
	}

	for i := range f.Sections {
		section := &f.Sections[i]
		// NOTE: SHT_REL is not relevant for the archs that we care about
		if section.Type == elf.SHT_RELA {
			cont, err := visitRelocationsForSection(f, visitor, classify, relTypes, section)
			if err != nil {
				return err
			}
			if !cont {
				return nil
			}
		}
	}

	return nil
}

func visitRelocationsForSection(f *pfelf.File, visitor func(ElfReloc, string, RelocType) bool,
	classify func(ElfReloc) RelocType, relTypes RelocType,
	relaSection *pfelf.Section,
) (bool, error) {
	if relaSection.Link >= uint32(len(f.Sections)) {
		return false, fmt.Errorf("rela section link is invalid (%d/%d)",
			relaSection.Link, len(f.Sections))
	}
	if relaSection.Size%uint64(unsafe.Sizeof(elf.Rela64{})) != 0 {
		return false, errors.New("relocation section size isn't multiple of rela64 struct")
	}

	symtabSection := &f.Sections[relaSection.Link]
	if symtabSection.Link >= uint32(len(f.Sections)) {
		return false, fmt.Errorf("symtab section link is invalid (%d/%d)",
			symtabSection.Link, len(f.Sections))
	}
	if symtabSection.Size%uint64(unsafe.Sizeof(elf.Sym64{})) != 0 {
		return false, errors.New("symbol section size isn't multiple of sym64 struct")
	}

	strtabSection := &f.Sections[symtabSection.Link]
	if strtabSection.Size > maxBytesLargeSection {
		return false, fmt.Errorf("string table too big (%d bytes)", strtabSection.Size)
	}

	strtabData, err := strtabSection.Data(uint(strtabSection.Size))
	if err != nil {
		return false, fmt.Errorf("failed to read string table: %w", err)
	}

	rdr := pfbufio.NewReader(relaSection, 0, int64(relaSection.Size))
	defer pfbufio.PutReader(rdr)

	rela := &elf.Rela64{}
	sym := &elf.Sym64{}
	symSz := int64(unsafe.Sizeof(elf.Sym64{}))
	for {
		if _, err := rdr.Read(pfunsafe.FromPointer(rela)); err != nil {
			if err != io.EOF {
				return false, fmt.Errorf("failed to read relocation: %w", err)
			}
			break
		}
		relType := classify(*rela)
		if relType&relTypes == 0 {
			continue
		}
		symNo := int64(rela.Info >> 32)
		n, err := symtabSection.ReadAt(pfunsafe.FromPointer(sym), symNo*symSz)
		if err != nil || n != int(symSz) {
			return false, fmt.Errorf("failed to read relocation symbol: %w", err)
		}

		symStr, ok := getString(strtabData, int(sym.Name))
		if !ok {
			return false, errors.New("failed to get relocation name string")
		}

		if !visitor(*rela, symStr, relType) {
			return false, nil
		}
	}
	runtime.KeepAlive(f)

	return true, nil
}

// dynValue returns the values listed for the given tag in the file's dynamic
// program header. It returns an empty slice (and no error) when the tag is not
// present.
func dynValue(f *pfelf.File, tag elf.DynTag) ([]uint64, error) {
	var vals []uint64
	for i := range f.Progs {
		p := &f.Progs[i]
		if p.ProgHeader.Type != elf.PT_DYNAMIC || p.Filesz <= 0 {
			continue
		}
		rdr := pfbufio.NewReader(p, 0, int64(p.Filesz))
		var dyn elf.Dyn64
		for {
			if _, err := rdr.Read(pfunsafe.FromPointer(&dyn)); err != nil {
				break
			}
			if elf.DynTag(dyn.Tag) == tag {
				vals = append(vals, dyn.Val)
			}
		}
		pfbufio.PutReader(rdr)
	}
	return vals, nil
}

// getString extracts a string from an ELF string table.
func getString(section []byte, start int) (string, bool) {
	if start < 0 || start >= len(section) {
		return "", false
	}
	slen := bytes.IndexByte(section[start:], 0)
	if slen < 0 {
		return "", false
	}
	return string(section[start : start+slen]), true
}
