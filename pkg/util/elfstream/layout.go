// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package elfstream

import (
	"debug/elf" //nolint:depguard // ELF encoding structs are not exposed by safeelf.
	"errors"
	"math"
)

// planLayout sets the output header fields and every section offset. Output
// order: ELF header, program headers, section headers, then section payloads.
func (s *ELFStream) planLayout() error {
	// Added sections may need extended numbering, which is not implemented.
	if len(s.sections) >= int(elf.SHN_LORESERVE) {
		return unsupported("too many output sections")
	}
	if err := s.appendSectionNames(); err != nil {
		return err
	}
	hdr := &s.elfHeader
	hdr.Phoff, hdr.Phentsize, hdr.Shoff, hdr.Shnum = 0, phdrSize, ehdrSize+uint64(len(s.programHeaders))*phdrSize, uint16(len(s.sections))
	if len(s.programHeaders) != 0 {
		hdr.Phoff = ehdrSize
	}
	return s.assignSectionOffsets()
}

// appendSectionNames keeps the input name table, so existing Name offsets stay
// valid, and appends names only for added or replaced sections.
func (s *ELFStream) appendSectionNames() error {
	outputNames := s.inputSectionNames
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if hasName(s.inputSectionNames, sec.header.Name, sec.name) {
			continue
		}
		sec.header.Name = uint32(len(outputNames))
		outputNames = append(outputNames, sec.name...)
		outputNames = append(outputNames, 0)
	}
	nameTableSection := &s.sections[s.elfHeader.Shstrndx]
	nameTableSection.ownBytes, nameTableSection.inputSize = outputNames, 0
	nameTableSection.header.Size = uint64(len(outputNames))
	return nil
}

// hasName reports whether offset names name in the NUL-terminated table.
func hasName(table []byte, offset uint32, name string) bool {
	end := uint64(offset) + uint64(len(name))
	return end < uint64(len(table)) && string(table[offset:end]) == name && table[end] == 0
}

// assignSectionOffsets aligns payloads after the header tables and computes the
// output size. NOBITS sections keep their size but take no file bytes.
func (s *ELFStream) assignSectionOffsets() error {
	position := s.elfHeader.Shoff + uint64(len(s.sections))*shdrSize
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if sec.typ() == elf.SHT_NOBITS {
			// Go ELF readers still read a NOBITS Size as a signed length.
			if sec.header.Size > math.MaxInt64 {
				return errors.New("omitted section size exceeds reader limit")
			}
			sec.header.Off = position
			continue
		}
		align := max(sec.header.Addralign, 1)
		if align > maxAlignment {
			return unsupported("section %d alignment exceeds %d", i, maxAlignment)
		}
		if position > math.MaxInt64-(align-1) {
			return errors.New("output size overflow")
		}
		position = (position + align - 1) &^ (align - 1) // align is a power of two.
		if sec.header.Size > math.MaxInt64-position {
			return errors.New("output size overflow")
		}
		sec.header.Off = position
		position += sec.header.Size
	}
	s.outputSize = position
	return nil
}

// planProgramHeaders keeps segment addresses for address lookup, but maps no
// file bytes, except the generated headers when a LOAD covered them.
func (s *ELFStream) planProgramHeaders() error {
	headersLoad, err := s.unmapSegments()
	if err != nil {
		return err
	}
	return s.relocatePHDR(headersLoad)
}

// unmapSegments clears the file bytes of every segment, and returns the index of
// the first LOAD that covered the headers, now mapping the generated headers,
// or -1.
func (s *ELFStream) unmapSegments() (int, error) {
	hdr := &s.elfHeader
	headersSize := ehdrSize + uint64(hdr.Phnum)*phdrSize
	headersLoad := -1
	for i := range s.programHeaders {
		prog := &s.programHeaders[i]
		coversHeaders := prog.Type == uint32(elf.PT_LOAD) && prog.Off == 0 && prog.Filesz >= hdr.Phoff+uint64(hdr.Phnum)*phdrSize
		if prog.Align != 0 && prog.Align&(prog.Align-1) != 0 {
			return -1, errors.New("invalid program alignment")
		}
		if prog.Type == uint32(elf.PT_INTERP) {
			// There is no interpreter path in the output; keep the slot.
			*prog = elf.Prog64{Type: uint32(elf.PT_NULL)}
			continue
		}
		// Off and Vaddr must stay congruent modulo Align.
		prog.Off, prog.Filesz = 0, 0
		if prog.Align > 1 {
			prog.Off = prog.Vaddr % prog.Align
		}
		if !coversHeaders {
			continue
		}
		prog.Off, prog.Filesz = 0, headersSize
		prog.Memsz = max(prog.Memsz, prog.Filesz)
		if headersLoad < 0 {
			headersLoad = i
		}
	}
	return headersLoad, nil
}

// relocatePHDR points PT_PHDR at the generated program headers, inside
// headerLoad, or drops it when no LOAD covers them.
func (s *ELFStream) relocatePHDR(headersLoad int) error {
	hdr := &s.elfHeader
	for i := range s.programHeaders {
		prog := &s.programHeaders[i]
		if prog.Type != uint32(elf.PT_PHDR) {
			continue
		}
		if headersLoad < 0 {
			prog.Type = uint32(elf.PT_NULL)
			continue
		}
		headersSegment := s.programHeaders[headersLoad]
		if headersSegment.Vaddr > math.MaxUint64-ehdrSize || headersSegment.Paddr > math.MaxUint64-ehdrSize {
			return errors.New("program header address overflow")
		}
		prog.Off, prog.Filesz = ehdrSize, uint64(hdr.Phnum)*phdrSize
		prog.Vaddr, prog.Paddr, prog.Memsz = headersSegment.Vaddr+ehdrSize, headersSegment.Paddr+ehdrSize, prog.Filesz
	}
	return nil
}
