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
func (s *ELFStream) selectSections(keepNames []string) error {
	selected := s.selectPrimarySections(keepNames)
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
func (s *ELFStream) selectPrimarySections(keepNames []string) []bool {
	keepByName := make(map[string]bool, len(keepNames))
	for _, name := range keepNames {
		keepByName[name] = true
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
		if typ == elf.SHT_SYMTAB || i == int(s.elfHeader.Shstrndx) {
			selected[i] = true
			continue
		}
		// objcopy's path removes .gdb_index too.
		if sec.name != ".gdb_index" {
			selected[i] = keepByName[sec.name] || isDebugSection(sec.name) || typ == elf.SHT_NOTE
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
		linkIndex := sec.header.Link
		if linkIndex == 0 || linkIndex >= uint32(len(s.sections)) || s.sections[linkIndex].typ() != elf.SHT_STRTAB {
			return fmt.Errorf("invalid symbol table %q", sec.name)
		}
		// Section names are appended to the name table during layout, which
		// cannot also hold symbol names.
		if linkIndex == uint32(s.elfHeader.Shstrndx) {
			return unsupported("symbols share the section name table")
		}
		selected[linkIndex] = true
	}
	return nil
}

// retainDebugRelocations keeps relocations that target retained, nonempty
// sections. Go's debug/elf, for one, applies .rela.debug_* to non-ET_EXEC files,
// so dropping them would change how DWARF reads compared with the input.
func (s *ELFStream) retainDebugRelocations(selected []bool) error {
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if !isRelocation(sec.typ()) {
			continue
		}
		// A target outside the input is not retained, so the relocation is not
		// either; like other omitted metadata, it is not an error.
		targetIndex := sec.header.Info
		if targetIndex < uint32(len(s.sections)) && selected[targetIndex] && s.sections[targetIndex].header.Size != 0 {
			selected[i] = true
		}
		// Not kept for its target, but it can still be kept by name (KeepSections).
		if !selected[i] {
			continue
		}
		// Entries refer to symbols by index in the Link table; Link 0 means they
		// refer to none. Emitted against an omitted (NOBITS) table, they would
		// make DWARF readers that apply relocations fail, so the table must be
		// kept. Real .rela.debug_* sections link .symtab, which always is.
		linkIndex := sec.header.Link
		if linkIndex == 0 {
			continue
		}
		if linkIndex >= uint32(len(s.sections)) || !selected[linkIndex] {
			return fmt.Errorf("invalid or omitted relocation symbol table in section %d", i)
		}
		if typ := s.sections[linkIndex].typ(); typ != elf.SHT_SYMTAB && typ != elf.SHT_DYNSYM {
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
			sec.inputSize = sec.header.Size
			continue
		}
		sec.omit()
	}
}

// applySections adds or replaces sections with supplied bytes, such as Go tables
// extracted from a larger section.
func (s *ELFStream) applySections(supplied []StreamSection) error {
	for _, section := range supplied {
		if section.Name == "" || strings.ContainsRune(section.Name, 0) {
			return errors.New("invalid additional section name")
		}
		replacement := outputSection{
			name:     section.Name,
			ownBytes: section.Data,
			header: elf.Section64{
				Type:      uint32(elf.SHT_PROGBITS),
				Addr:      section.Address,
				Size:      uint64(len(section.Data)),
				Addralign: 8,
			},
		}
		existing := s.sectionIndex(section.Name)
		if existing == -1 {
			s.sections = append(s.sections, replacement)
			continue
		}
		// Replaces the input section of that name, such as a Go binary's own
		// .gopclntab, at the same index. Symbol, string and name tables can't be
		// replaced: the plan still patches and appends into them.
		if existing == int(s.elfHeader.Shstrndx) || isSymbolOrStringTable(s.sections[existing].typ()) {
			return fmt.Errorf("cannot replace section %q", section.Name)
		}
		s.sections[existing] = replacement
		s.omitRelocationsOf(existing)
	}
	return nil
}

// omitRelocationsOf omits kept relocations of section index, which describe the
// input bytes, not the supplied ones. This is an edge case: it only happens when
// a caller replaces a section that is also kept, such as a debug section. The
// uploader only replaces .gopclntab, whose relocations are never kept.
func (s *ELFStream) omitRelocationsOf(targetIndex int) {
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if isRelocation(sec.typ()) && int(sec.header.Info) == targetIndex {
			sec.omit()
		}
	}
}

// omit turns sec into NOBITS: its header stays, with no bytes in the output.
func (sec *outputSection) omit() {
	sec.header.Type = uint32(elf.SHT_NOBITS)
	sec.header.Flags &^= uint64(elf.SHF_COMPRESSED)
	sec.inputSize = 0
}

func isRelocation(typ elf.SectionType) bool {
	return typ == elf.SHT_REL || typ == shtRela
}

func isSymbolTable(typ elf.SectionType) bool {
	return typ == elf.SHT_SYMTAB || typ == elf.SHT_DYNSYM
}

func isSymbolOrStringTable(typ elf.SectionType) bool {
	return typ == elf.SHT_SYMTAB || typ == elf.SHT_DYNSYM || typ == elf.SHT_STRTAB
}

func isDebugSection(name string) bool {
	return strings.HasPrefix(name, ".debug_") || strings.HasPrefix(name, ".zdebug_") ||
		name == ".gnu_debuglink" || name == ".gnu_debugaltlink" || name == ".gnu_debugdata"
}
