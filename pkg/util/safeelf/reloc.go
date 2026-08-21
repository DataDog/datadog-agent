// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file is derived from libpf/pfelf/file.go of the OpenTelemetry eBPF
// profiler (https://github.com/open-telemetry/opentelemetry-ebpf-profiler),
// Copyright The OpenTelemetry Authors, SPDX-License-Identifier: Apache-2.0.
//
// Modifications: the visitor reads sections through debug/elf instead of
// pfelf's own ELF reader, TPOFF64 relocations are classified as well, the
// matched RelocType is reported to the visitor, and relocation sections
// without a usable symbol or string table are skipped instead of failing.

package safeelf

import (
	"debug/elf" //nolint:depguard
	"encoding/binary"
	"errors"
	"fmt"
)

// ElfReloc is a 64-bit RELA relocation entry (Off, Info, Addend).
//
//nolint:misspell // RELA is the ELF term, not a typo of "real"
type ElfReloc = elf.Rela64

// RelocType represents an architecture-independent relocation type. Multiple
// values can be combined with bitwise OR to match several types at once.
type RelocType uint32

const (
	// RelTLSDESC matches TLSDESC relocations (R_AARCH64_TLSDESC, R_X86_64_TLSDESC).
	RelTLSDESC RelocType = 1 << iota
	// RelDTPMOD64 matches DTPMOD64 relocations (R_AARCH64_TLS_DTPMOD64, R_X86_64_DTPMOD64).
	RelDTPMOD64
	// RelTPOFF64 matches TP-relative relocations (R_AARCH64_TLS_TPREL64, R_X86_64_TPOFF64).
	RelTPOFF64
)

func classifyRelocAarch64(info uint64) RelocType {
	switch elf.R_AARCH64(uint32(info)) {
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

func classifyRelocX86_64(info uint64) RelocType {
	switch elf.R_X86_64(uint32(info)) {
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

// VisitRelocations visits every SHT_RELA relocation in f whose type matches
// the relTypes bitmask, calling visitor with the relocation, the referenced
// symbol's name (empty when the relocation references no symbol) and the
// matched RelocType. Returning false stops the iteration.
//
//nolint:misspell // RELA is the ELF term, not a typo of "real"
func (f *File) VisitRelocations(visitor func(ElfReloc, string, RelocType) bool,
	relTypes RelocType,
) (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		err = fmt.Errorf("visiting ELF relocations panicked: %s", r)
	}()

	var classify func(uint64) RelocType
	switch f.Machine {
	case elf.EM_AARCH64:
		classify = classifyRelocAarch64
	case elf.EM_X86_64:
		classify = classifyRelocX86_64
	default:
		return nil
	}

	for _, sec := range f.Sections {
		if sec.Type != elf.SHT_RELA { //nolint:misspell
			continue
		}
		cont, err := visitRelocationSection(f.ByteOrder, f.Sections, sec, classify, relTypes, visitor)
		if err != nil {
			return err
		}
		if !cont {
			return nil
		}
	}
	return nil
}

// visitRelocationSection visits the relocations in one SHT_RELA section.
// Sections with no usable symbol/string table (e.g. a static binary's
// .rela.plt, whose IRELATIVE entries reference no symbol and so may carry
// sh_link == 0) are skipped rather than failing: they cannot hold any
// symbol-driven relocation.
//
//nolint:misspell // RELA is the ELF term, not a typo of "real"
func visitRelocationSection(order binary.ByteOrder, sections []*elf.Section, sec *elf.Section,
	classify func(uint64) RelocType, relTypes RelocType,
	visitor func(ElfReloc, string, RelocType) bool,
) (bool, error) {
	if sec.Link == 0 || sec.Link >= uint32(len(sections)) {
		return true, nil
	}
	symtab := sections[sec.Link]
	if symtab.Link >= uint32(len(sections)) {
		return true, nil
	}
	strtab := sections[symtab.Link]

	data, err := sec.Data()
	if err != nil {
		return true, nil //nolint:nilerr // best-effort: skip this section if it can't be read
	}
	symtabData, err := symtab.Data()
	if err != nil {
		return true, nil //nolint:nilerr // best-effort: skip this section if its symtab can't be read
	}
	strtabData, err := strtab.Data()
	if err != nil {
		return true, nil //nolint:nilerr // best-effort: skip this section if its strtab can't be read
	}

	const relocEntSize = 24 // r_offset, r_info, r_addend, 8 bytes each
	for off := 0; off+relocEntSize <= len(data); off += relocEntSize {
		entry := ElfReloc{
			Off:    order.Uint64(data[off:]),
			Info:   order.Uint64(data[off+8:]),
			Addend: int64(order.Uint64(data[off+16:])),
		}

		relType := classify(entry.Info)
		if relType&relTypes == 0 {
			continue
		}

		symName, err := symbolNameAt(order, symtabData, strtabData, entry.Info>>32)
		if err != nil {
			return false, err
		}

		if !visitor(entry, symName, relType) {
			return false, nil
		}
	}
	return true, nil
}

// symbolNameAt returns the name of the Elf64_Sym entry at index symIdx within
// symtabData, resolved against strtabData. Index 0 (no symbol) returns "".
func symbolNameAt(order binary.ByteOrder, symtabData, strtabData []byte, symIdx uint64) (string, error) {
	if symIdx == 0 {
		return "", nil
	}
	const symEntSize = uint64(Sym64Size) // 24: st_name(4) st_info(1) st_other(1) st_shndx(2) st_value(8) st_size(8)
	off := symIdx * symEntSize
	if off+4 > uint64(len(symtabData)) {
		return "", fmt.Errorf("symbol index %d out of range", symIdx)
	}
	nameOff := order.Uint32(symtabData[off:])
	return getNulString(strtabData, nameOff)
}

func getNulString(data []byte, off uint32) (string, error) {
	if uint64(off) >= uint64(len(data)) {
		return "", errors.New("string offset out of range")
	}
	end := off
	for end < uint32(len(data)) && data[end] != 0 {
		end++
	}
	return string(data[off:end]), nil
}
