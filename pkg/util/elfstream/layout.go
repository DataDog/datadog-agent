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
	h := &s.header
	h.Phoff, h.Phentsize, h.Shoff, h.Shnum = 0, phdrSize, ehdrSize+uint64(len(s.programs))*phdrSize, uint16(len(s.sections))
	if len(s.programs) != 0 {
		h.Phoff = ehdrSize
	}
	return s.assignSectionOffsets()
}

// appendSectionNames keeps the input name table, so existing Name offsets stay
// valid, and appends names only for added or replaced sections.
func (s *ELFStream) appendSectionNames() error {
	newNames := s.names[:len(s.names):len(s.names)] // Appending copies; input names stay intact.
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if hasName(s.names, sec.header.Name, sec.name) {
			continue
		}
		sec.header.Name = uint32(len(newNames))
		newNames = append(newNames, sec.name...)
		newNames = append(newNames, 0)
	}
	namesSec := &s.sections[s.header.Shstrndx]
	namesSec.data, namesSec.sourceSize = newNames, 0
	namesSec.header.Size = uint64(len(newNames))
	return nil
}

// hasName reports whether offset names name in the NUL-terminated table.
func hasName(names []byte, offset uint32, name string) bool {
	end := uint64(offset) + uint64(len(name))
	return end < uint64(len(names)) && string(names[offset:end]) == name && names[end] == 0
}

// assignSectionOffsets aligns payloads after the header tables and computes the
// output size. NOBITS sections keep their size but take no file bytes.
func (s *ELFStream) assignSectionOffsets() error {
	position := s.header.Shoff + uint64(len(s.sections))*shdrSize
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		nobits := sec.typ() == elf.SHT_NOBITS
		if !nobits {
			align := max(sec.header.Addralign, 1)
			if align > maxAlignment {
				return unsupported("section %d alignment exceeds %d", i, maxAlignment)
			}
			if position > math.MaxInt64-(align-1) {
				return errors.New("output size overflow")
			}
			position = (position + align - 1) &^ (align - 1) // align is a power of two.
		}
		sec.header.Off = position
		if !nobits {
			if sec.header.Size > math.MaxInt64-position {
				return errors.New("output size overflow")
			}
			position += sec.header.Size
		} else if sec.header.Size > math.MaxInt64 {
			// Go ELF readers still read a NOBITS Size as a signed length.
			return errors.New("omitted section size exceeds reader limit")
		}
	}
	s.size = position
	return nil
}

// planProgramHeaders keeps segment addresses for address lookup, but maps no
// file bytes, except the generated headers when a LOAD covered them.
func (s *ELFStream) planProgramHeaders() error {
	h := &s.header
	headersSize := ehdrSize + uint64(h.Phnum)*phdrSize
	headerLoad := -1
	for i := range s.programs {
		p := &s.programs[i]
		coversHeaders := p.Type == uint32(elf.PT_LOAD) && p.Off == 0 && p.Filesz >= h.Phoff+uint64(h.Phnum)*phdrSize
		if p.Align != 0 && p.Align&(p.Align-1) != 0 {
			return errors.New("invalid program alignment")
		}
		if p.Type == uint32(elf.PT_INTERP) {
			// There is no interpreter path in the output; keep the slot.
			*p = elf.Prog64{Type: uint32(elf.PT_NULL)}
			continue
		}
		// Off and Vaddr must stay congruent modulo Align.
		p.Off, p.Filesz = 0, 0
		if p.Align > 1 {
			p.Off = p.Vaddr % p.Align
		}
		if coversHeaders {
			p.Off, p.Filesz = 0, headersSize
			p.Memsz = max(p.Memsz, p.Filesz)
			if headerLoad < 0 {
				headerLoad = i
			}
		}
	}
	for i := range s.programs {
		p := &s.programs[i]
		if p.Type != uint32(elf.PT_PHDR) {
			continue
		}
		// PT_PHDR must be covered by a LOAD; without one, drop it.
		if headerLoad < 0 {
			p.Type = uint32(elf.PT_NULL)
			continue
		}
		load := s.programs[headerLoad]
		if load.Vaddr > math.MaxUint64-ehdrSize || load.Paddr > math.MaxUint64-ehdrSize {
			return errors.New("program header address overflow")
		}
		p.Off, p.Filesz = ehdrSize, uint64(h.Phnum)*phdrSize
		p.Vaddr, p.Paddr, p.Memsz = load.Vaddr+ehdrSize, load.Paddr+ehdrSize, p.Filesz
	}
	return nil
}
