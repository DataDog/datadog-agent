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
func (s *ELFStream) addSymbols(ctx context.Context, requested []StreamSymbol) error {
	if len(requested) == 0 {
		return nil
	}
	markers, longestMarker, err := s.markerEntries(requested)
	if err != nil {
		return err
	}
	// Markers go in the static symbol table; their names are in its string table.
	symtab, strtab, err := s.getSymbolTables()
	if err != nil {
		return err
	}
	markerOffsets, err := s.findMarkerOffsets(ctx, strtab, markers, longestMarker)
	if err != nil {
		return err
	}
	handled, err := s.patchMarkerSymbols(ctx, symtab, markerOffsets, markers)
	if err != nil {
		return err
	}
	return s.appendMissingMarkers(symtab, strtab, requested, markers, handled)
}

// markerEntries resolves each requested marker's owning section and builds the
// symbol entry it should become.
func (s *ELFStream) markerEntries(requested []StreamSymbol) (map[string]elf.Sym64, int, error) {
	markers := make(map[string]elf.Sym64, len(requested))
	longestMarker := 0
	for _, marker := range requested {
		ownerIndex := s.sectionIndex(marker.Section)
		// The name is appended to the string table, so it can't hold a NUL; the
		// string-table scan finds names no longer than its read buffer. Index 0
		// is the null section, -1 a missing one.
		if marker.Name == "" || len(marker.Name) > streamBufferSize || strings.ContainsRune(marker.Name, 0) || ownerIndex <= 0 {
			return nil, 0, fmt.Errorf("invalid additional symbol %q", marker.Name)
		}
		owner := s.sections[ownerIndex].header
		// Markers have size 0, so an address at the section end is valid.
		if marker.Value < owner.Addr || marker.Value-owner.Addr > owner.Size {
			return nil, 0, fmt.Errorf("symbol %q is outside section %q", marker.Name, marker.Section)
		}
		markers[marker.Name] = elf.Sym64{Info: byte(elf.STB_GLOBAL) << 4, Shndx: uint16(ownerIndex), Value: marker.Value}
		longestMarker = max(longestMarker, len(marker.Name))
	}
	return markers, longestMarker, nil
}

// getSymbolTables returns the first static symbol table and its string table.
// For a stripped input it adds empty ones: a null entry, with Info=1 marking
// where globals start.
func (s *ELFStream) getSymbolTables() (*outputSection, *outputSection, error) {
	symtabIndex := -1
	for i := range s.sections {
		if s.sections[i].typ() == elf.SHT_SYMTAB {
			symtabIndex = i
			break
		}
	}
	if symtabIndex < 0 {
		symtabIndex = len(s.sections)
		symtab := outputSection{
			name:     ".symtab",
			ownBytes: make([]byte, symSize), // null symbol.
			header: elf.Section64{
				Type:      uint32(elf.SHT_SYMTAB),
				Size:      symSize,
				Link:      uint32(symtabIndex + 1), // the .strtab appended next.
				Info:      1,                       // index of the first global symbol.
				Addralign: 8,
				Entsize:   symSize,
			},
		}
		strtab := outputSection{
			name:     ".strtab",
			ownBytes: []byte{0}, // Offset 0 is the empty name.
			header: elf.Section64{
				Type:      uint32(elf.SHT_STRTAB),
				Size:      1,
				Addralign: 1,
			},
		}
		s.sections = append(s.sections, symtab, strtab)
	}
	symtab := &s.sections[symtabIndex]
	// checkRetainedSections validated the table's layout. Markers are patched
	// in place and appended, which needs it uncompressed.
	if symtab.header.Flags&uint64(elf.SHF_COMPRESSED) != 0 {
		return nil, nil, unsupported("compressed static symbol table")
	}
	// The string table is scanned raw and appended to, and st_name is a 32-bit
	// offset: uncompressed and at most 4 GiB. Offset 0 is the empty name.
	strtab := &s.sections[symtab.header.Link]
	// A string table may be empty, but appended names need offset 0 to stay the
	// empty name.
	if strtab.header.Size == 0 {
		return nil, nil, unsupported("empty symbol string table")
	}
	if strtab.header.Flags&uint64(elf.SHF_COMPRESSED) != 0 || strtab.header.Size > math.MaxUint32 {
		return nil, nil, unsupported("symbol string table layout")
	}
	return symtab, strtab, nil
}

// findMarkerOffsets returns the string-table offsets that name a marker.
// A name can be the suffix of a longer string, so every string's suffix is
// checked.
func (s *ELFStream) findMarkerOffsets(ctx context.Context, strtab *outputSection, markers map[string]elf.Sym64, longestMarker int) (map[uint32]string, error) {
	markerOffsets := make(map[uint32]string)
	markerFound := make(map[string]bool, len(markers))
	if strtab.inputSize == 0 {
		return markerOffsets, nil
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(s.input, int64(strtab.inputOffset), int64(strtab.inputSize)), streamBufferSize)
	// For a string longer than the read buffer, the end of its previous chunk,
	// so the suffix check still sees a whole marker name.
	chunkTail := make([]byte, 0, 2*longestMarker)
	for offset := uint64(0); offset < strtab.inputSize; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk, err := reader.ReadSlice(0) // One string and its NUL, or a full buffer.
		offset += uint64(len(chunk))
		if errors.Is(err, bufio.ErrBufferFull) {
			chunkTail = append(chunkTail[:0], chunk[len(chunk)-min(len(chunk), longestMarker):]...)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("invalid symbol string table: %w", err)
		}

		suffix := chunk[:len(chunk)-1]
		// A string split across refills: when its last piece is shorter than a
		// marker name, the name may start in the previous chunk.
		if len(chunkTail) != 0 && len(suffix) < longestMarker {
			suffix = append(chunkTail, suffix...)
		}

		nulOffset := offset - 1 // Offset of the string's NUL.
		for name := range markers {
			if !bytes.HasSuffix(suffix, []byte(name)) {
				continue
			}
			// A marker name appears once in real string tables. A crafted table
			// repeating it would otherwise grow markerOffsets with its size.
			if markerFound[name] {
				return nil, unsupported("%q appears more than once in the symbol string table", name)
			}
			markerFound[name] = true
			markerOffsets[uint32(nulOffset-uint64(len(name)))] = name
		}
		chunkTail = chunkTail[:0]
	}
	return markerOffsets, nil
}

// patchMarkerSymbols records, in input order, a symbol override for the existing
// symbol named like each marker, and returns the markers it handled.
func (s *ELFStream) patchMarkerSymbols(ctx context.Context, symtab *outputSection, markerOffsets map[uint32]string, markers map[string]elf.Sym64) (map[string]bool, error) {
	handled := make(map[string]bool)
	if symtab.inputSize == 0 {
		return handled, nil
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(s.input, int64(symtab.inputOffset), int64(symtab.inputSize)), streamBufferSize)
	var entry [symSize]byte
	for offset := uint64(0); offset < symtab.inputSize; offset += symSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(reader, entry[:]); err != nil {
			return nil, err
		}
		name, ok := markerOffsets[s.byteOrder.Uint32(entry[:4])] // st_name comes first.
		if !ok {
			continue
		}
		// Marker symbols are unique: a second one is not a layout this writer
		// understands, and would make the override list grow with the input.
		if handled[name] {
			return nil, unsupported("more than one %q symbol", name)
		}
		var existing elf.Sym64
		if _, err := binary.Decode(entry[:], s.byteOrder, &existing); err != nil {
			return nil, err
		}
		// Keep the name, binding and type; point it at the marker.
		target := markers[name]
		existing.Shndx, existing.Value, existing.Size = target.Shndx, target.Value, 0
		symtab.symbolOverrides = append(symtab.symbolOverrides, symbolOverride{offset: offset, entry: existing})
		handled[name] = true
	}
	return handled, nil
}

// appendMissingMarkers appends the markers not handled yet, in caller order, after
// all existing entries; their names are appended to the string table.
func (s *ELFStream) appendMissingMarkers(symtab, strtab *outputSection, requested []StreamSymbol, markers map[string]elf.Sym64, handled map[string]bool) error {
	for _, marker := range requested {
		if handled[marker.Name] {
			continue
		}
		target := markers[marker.Name]
		if strtab.header.Size+uint64(len(marker.Name))+1 > math.MaxUint32 {
			return errors.New("symbol string table size overflow")
		}
		target.Name = uint32(strtab.header.Size)
		strtab.appended = append(strtab.appended, marker.Name...)
		strtab.appended = append(strtab.appended, 0)
		strtab.header.Size += uint64(len(marker.Name)) + 1
		var encoded bytes.Buffer
		if err := binary.Write(&encoded, s.byteOrder, target); err != nil {
			return err
		}
		symtab.appended = append(symtab.appended, encoded.Bytes()...)
		symtab.header.Size += symSize
		handled[marker.Name] = true
	}
	return nil
}
