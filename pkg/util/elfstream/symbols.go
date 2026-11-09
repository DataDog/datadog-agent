// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package elfstream

import (
	"bufio"
	"bytes"
	"context"
	"debug/elf" //nolint:depguard // ELF encoding structs are not exposed by safeelf.
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// addSymbols adds marker symbols, such as Go's go:func.* and runtime.text, like
// objcopy's --strip-symbol plus --add-symbol. Existing symbols with a marker's
// name are patched in place rather than removed, so symbol indices used by
// relocations stay valid; missing markers are appended as zero-sized globals.
// The symbol and string tables are scanned with fixed buffers, not loaded.
func (s *ELFStream) addSymbols(ctx context.Context, symbols []StreamSymbol) error {
	if len(symbols) == 0 {
		return nil
	}
	wanted, longestName, err := s.validateSymbolMarkers(symbols)
	if err != nil {
		return err
	}
	tab, strtab, err := s.getSymbolTables()
	if err != nil {
		return err
	}
	names, err := s.findSymbolNameOffsets(ctx, strtab, wanted, longestName)
	if err != nil {
		return err
	}
	found, err := s.planSymbolPatches(ctx, tab, names, wanted)
	if err != nil {
		return err
	}
	return s.appendMissingSymbols(tab, strtab, symbols, wanted, found)
}

// validateSymbolMarkers resolves each marker's section and builds its entry.
func (s *ELFStream) validateSymbolMarkers(symbols []StreamSymbol) (map[string]elf.Sym64, int, error) {
	wanted := make(map[string]elf.Sym64, len(symbols))
	longestName := 0
	for _, sym := range symbols {
		index := s.getSectionIndex(sym.Section)
		if sym.Name == "" || strings.ContainsRune(sym.Name, 0) || index <= 0 {
			return nil, 0, fmt.Errorf("invalid additional symbol %q", sym.Name)
		}
		sec := s.sections[index].header
		// Markers have zero extent, so an address at the section end is valid.
		if sym.Value < sec.Addr || sym.Value-sec.Addr > sec.Size {
			return nil, 0, fmt.Errorf("symbol %q is outside section %q", sym.Name, sym.Section)
		}
		wanted[sym.Name] = elf.Sym64{Info: byte(elf.STB_GLOBAL) << 4, Shndx: uint16(index), Value: sym.Value}
		longestName = max(longestName, len(sym.Name))
	}
	return wanted, longestName, nil
}

// getSymbolTables returns the first static symbol table and its string table.
// For a stripped input it adds empty ones: a null entry, with Info=1 marking
// where globals start.
func (s *ELFStream) getSymbolTables() (*outputSection, *outputSection, error) {
	tabIndex := -1
	for i := range s.sections {
		if s.sections[i].typ() == elf.SHT_SYMTAB {
			tabIndex = i
			break
		}
	}
	if tabIndex < 0 {
		tabIndex = len(s.sections)
		s.sections = append(s.sections, outputSection{name: ".symtab", data: make([]byte, symSize), header: elf.Section64{
			Type: uint32(elf.SHT_SYMTAB), Size: symSize, Link: uint32(tabIndex + 1), Info: 1, Addralign: 8, Entsize: symSize,
		}}, outputSection{name: ".strtab", data: []byte{0}, header: elf.Section64{Type: uint32(elf.SHT_STRTAB), Size: 1, Addralign: 1}})
	}
	tab := &s.sections[tabIndex]
	if tab.header.Entsize != symSize || tab.header.Size < symSize || tab.header.Size%symSize != 0 || uint64(tab.header.Info) > tab.header.Size/symSize || tab.header.Flags&uint64(elf.SHF_COMPRESSED) != 0 {
		return nil, nil, unsupported("static symbol table layout")
	}
	strtab := &s.sections[tab.header.Link]
	if strtab.typ() != elf.SHT_STRTAB || strtab.header.Flags&uint64(elf.SHF_COMPRESSED) != 0 || strtab.header.Size == 0 || strtab.header.Size > math.MaxUint32 {
		return nil, nil, unsupported("symbol string table layout")
	}
	return tab, strtab, nil
}

// findSymbolNameOffsets returns the string-table offsets that name a marker.
// A name can be the suffix of a longer string, so every string's suffix is
// checked; when a string spans buffer refills, the previous chunk's end is kept.
func (s *ELFStream) findSymbolNameOffsets(ctx context.Context, strtab *outputSection, wanted map[string]elf.Sym64, longestName int) (map[uint32]string, error) {
	names := make(map[uint32]string)
	if strtab.sourceSize != 0 {
		r := bufio.NewReaderSize(io.NewSectionReader(s.input, int64(strtab.source), int64(strtab.sourceSize)), streamBufferSize)
		var offset uint64
		previous := make([]byte, 0, 2*longestName)
		for offset < strtab.sourceSize {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			part, err := r.ReadSlice(0)
			if err == nil {
				tail := part[:len(part)-1]
				if len(tail) < longestName {
					tail = append(previous, tail...)
				}
				for name := range wanted {
					if bytes.HasSuffix(tail, []byte(name)) {
						names[uint32(offset+uint64(len(part)-1)-uint64(len(name)))] = name
					}
				}
				previous = previous[:0]
			} else if errors.Is(err, bufio.ErrBufferFull) {
				previous = append(previous[:0], part[len(part)-min(len(part), longestName):]...)
			} else {
				return nil, fmt.Errorf("invalid symbol string table: %w", err)
			}
			offset += uint64(len(part))
		}
	}
	return names, nil
}

// planSymbolPatches records, in input order, a patch for every symbol named
// like a marker. Write copies around the patched entries.
func (s *ELFStream) planSymbolPatches(ctx context.Context, tab *outputSection, names map[uint32]string, wanted map[string]elf.Sym64) (map[string]bool, error) {
	found := make(map[string]bool)
	if tab.sourceSize != 0 {
		r := bufio.NewReaderSize(io.NewSectionReader(s.input, int64(tab.source), int64(tab.sourceSize)), streamBufferSize)
		var raw [symSize]byte
		for offset := uint64(0); offset < tab.sourceSize; offset += symSize {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if _, err := io.ReadFull(r, raw[:]); err != nil {
				return nil, err
			}
			if name, ok := names[s.order.Uint32(raw[:4])]; ok { // st_name comes first.
				var sym elf.Sym64
				if _, err := binary.Decode(raw[:], s.order, &sym); err != nil {
					return nil, err
				}
				// Keep the name, binding and type; point it at the marker.
				value := wanted[name]
				sym.Shndx, sym.Value, sym.Size = value.Shndx, value.Value, 0
				tab.patches = append(tab.patches, symbolPatch{offset: offset, value: sym})
				found[name] = true
			}
		}
	}
	return found, nil
}

// appendMissingSymbols appends the remaining markers, in caller order, after
// all existing entries; their names are appended to the string table.
func (s *ELFStream) appendMissingSymbols(tab, strtab *outputSection, symbols []StreamSymbol, wanted map[string]elf.Sym64, found map[string]bool) error {
	for _, extra := range symbols {
		if found[extra.Name] {
			continue
		}
		value := wanted[extra.Name]
		if strtab.header.Size+uint64(len(extra.Name))+1 > math.MaxUint32 {
			return errors.New("symbol string table size overflow")
		}
		value.Name = uint32(strtab.header.Size)
		strtab.tail = append(strtab.tail, extra.Name...)
		strtab.tail = append(strtab.tail, 0)
		strtab.header.Size += uint64(len(extra.Name)) + 1
		var encoded bytes.Buffer
		if err := binary.Write(&encoded, s.order, value); err != nil {
			return err
		}
		tab.tail = append(tab.tail, encoded.Bytes()...)
		tab.header.Size += symSize
		found[extra.Name] = true
	}
	return nil
}
