// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package elfstream

import (
	"debug/elf" //nolint:depguard // ELF encoding structs are not exposed by safeelf.
	"errors"
	"fmt"
	"strings"
)

// selectSections decides which sections keep their bytes, resolving links
// before any section type changes. It reads headers only.
func (s *ELFStream) selectSections(names []string) error {
	selected := s.selectPrimarySections(names)
	if err := s.retainSymbolStrings(selected); err != nil {
		return err
	}
	if err := s.retainDebugRelocations(selected); err != nil {
		return err
	}
	s.omitUnselectedSections(selected)
	return nil
}

// selectPrimarySections selects debug sections, notes (build IDs), symbol
// tables, the section-name table and the caller's KeepSections.
func (s *ELFStream) selectPrimarySections(names []string) []bool {
	keep := make(map[string]bool, len(names))
	for _, name := range names {
		keep[name] = true
	}
	selected := make([]bool, len(s.sections))
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		typ := sec.typ()
		switch typ {
		case elf.SHT_NOBITS, elf.SHT_GNU_VERSYM, elf.SHT_GNU_VERDEF, elf.SHT_GNU_VERNEED:
			// GNU version tables are never exported, even when named.
			continue
		}
		if typ == elf.SHT_SYMTAB || i == int(s.header.Shstrndx) {
			selected[i] = true
			continue
		}
		// objcopy's path removes .gdb_index too.
		if sec.name != ".gdb_index" {
			selected[i] = keep[sec.name] || isDebugSection(sec.name) || typ == elf.SHT_NOTE
		}
	}
	return selected
}

// retainSymbolStrings selects the string table of each selected symbol table.
func (s *ELFStream) retainSymbolStrings(selected []bool) error {
	for i := range s.sections {
		sec := &s.sections[i]
		if !selected[i] || (sec.typ() != elf.SHT_SYMTAB && sec.typ() != elf.SHT_DYNSYM) {
			continue
		}
		link := sec.header.Link
		if link == 0 || link >= uint32(len(s.sections)) || s.sections[link].typ() != elf.SHT_STRTAB {
			return fmt.Errorf("invalid symbol table %q", sec.name)
		}
		// Section names are appended to the name table during layout, which
		// cannot also hold symbol names.
		if link == uint32(s.header.Shstrndx) {
			return unsupported("symbols share the section name table")
		}
		selected[link] = true
	}
	return nil
}

// retainDebugRelocations keeps relocations that target retained, nonempty
// sections. Go's debug/elf, for one, applies .rela.debug_* to non-ET_EXEC files,
// so dropping them would change how DWARF reads compared with the input.
func (s *ELFStream) retainDebugRelocations(selected []bool) error {
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if sec.typ() != elf.SHT_REL && sec.typ() != shtRela {
			continue
		}
		// A target outside the input is not retained, so the relocation is not
		// either; like other omitted metadata, it is not an error.
		target := sec.header.Info
		if target < uint32(len(s.sections)) && selected[target] && s.sections[target].header.Size != 0 {
			selected[i] = true
		}
		if !selected[i] {
			continue
		}
		// Entries index the symbol table named by Link, so it must be retained.
		// Link 0 means the entries reference no symbols.
		link := sec.header.Link
		if link == 0 {
			continue
		}
		if link >= uint32(len(s.sections)) || !selected[link] {
			return fmt.Errorf("invalid or omitted relocation symbol table in section %d", i)
		}
		if typ := s.sections[link].typ(); typ != elf.SHT_SYMTAB && typ != elf.SHT_DYNSYM {
			return fmt.Errorf("invalid relocation symbol table in section %d", i)
		}
	}
	return nil
}

// omitUnselectedSections turns unselected sections into NOBITS. Each slot keeps
// its index, address and size, so symbols can still point at .text without its
// bytes being uploaded.
func (s *ELFStream) omitUnselectedSections(selected []bool) {
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if selected[i] {
			sec.sourceSize = sec.header.Size
			continue
		}
		sec.header.Type = uint32(elf.SHT_NOBITS)
		sec.header.Flags &^= uint64(elf.SHF_COMPRESSED)
	}
}

// applySections adds or replaces sections with supplied bytes, such as Go tables
// extracted from a larger section.
func (s *ELFStream) applySections(sections []StreamSection) error {
	for _, extra := range sections {
		// An empty name would match the null section; a NUL would hide the rest.
		if extra.Name == "" || strings.ContainsRune(extra.Name, 0) {
			return errors.New("invalid additional section name")
		}
		index := s.getSectionIndex(extra.Name)
		if index == -1 {
			index = len(s.sections)
			s.sections = append(s.sections, outputSection{name: extra.Name})
		} else {
			typ := s.sections[index].typ()
			if index == int(s.header.Shstrndx) || typ == elf.SHT_SYMTAB || typ == elf.SHT_DYNSYM || typ == elf.SHT_STRTAB {
				return fmt.Errorf("cannot replace section %q", extra.Name)
			}
		}
		s.sections[index] = outputSection{name: extra.Name, data: extra.Data, header: elf.Section64{
			Type: uint32(elf.SHT_PROGBITS), Addr: extra.Address, Size: uint64(len(extra.Data)), Addralign: 8,
		}}
		s.omitRelocationsOf(index)
	}
	return nil
}

// omitRelocationsOf omits retained relocations that target section index: they
// describe the input bytes, not the supplied ones.
func (s *ELFStream) omitRelocationsOf(index int) {
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if (sec.typ() == elf.SHT_REL || sec.typ() == shtRela) && int(sec.header.Info) == index {
			sec.header.Type = uint32(elf.SHT_NOBITS)
			sec.header.Flags &^= uint64(elf.SHF_COMPRESSED)
			sec.sourceSize = 0
		}
	}
}

// shtRela is the relocations-with-addends section type; misspell flags its elf name.
const shtRela = elf.SHT_RELA //nolint:misspell

func isDebugSection(name string) bool {
	return strings.HasPrefix(name, ".debug_") || strings.HasPrefix(name, ".zdebug_") ||
		name == ".gnu_debuglink" || name == ".gnu_debugaltlink" || name == ".gnu_debugdata"
}
