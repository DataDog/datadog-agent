// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package elfstream

import (
	"context"
	"debug/elf" //nolint:depguard // ELF encoding structs are not exposed by safeelf.
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// readByteOrder checks the identification bytes, which precede any
// byte-order-dependent field.
func readByteOrder(input io.ReaderAt) (binary.ByteOrder, error) {
	var identification [elf.EI_NIDENT]byte
	if _, err := input.ReadAt(identification[:], 0); err != nil {
		return nil, err
	}
	if string(identification[:4]) != elf.ELFMAG {
		return nil, errors.New("not an ELF file")
	}
	switch elf.Class(identification[elf.EI_CLASS]) {
	case elf.ELFCLASS64:
	case elf.ELFCLASS32:
		return nil, unsupported("32-bit ELF")
	default:
		return nil, errors.New("invalid ELF class")
	}
	// EV_CURRENT is the only version ELF defines.
	if identification[elf.EI_VERSION] != byte(elf.EV_CURRENT) {
		return nil, errors.New("invalid ELF identification version")
	}
	switch elf.Data(identification[elf.EI_DATA]) {
	case elf.ELFDATA2LSB:
		return binary.LittleEndian, nil
	case elf.ELFDATA2MSB:
		return binary.BigEndian, nil
	default:
		return nil, errors.New("invalid ELF byte order")
	}
}

// readELF64 reads the ELF header and the section and program header tables.
// Every read is bounded by size, so truncated tables fail as read errors.
func readELF64(ctx context.Context, input io.ReaderAt, inputSize int64) (*ELFStream, error) {
	s := &ELFStream{input: io.NewSectionReader(input, 0, inputSize)}
	if err := s.readHeader(); err != nil {
		return nil, err
	}
	if err := s.readSections(ctx, inputSize); err != nil {
		return nil, err
	}
	hdr := &s.elfHeader
	s.programHeaders = make([]elf.Prog64, hdr.Phnum)
	if len(s.programHeaders) != 0 {
		if err := binary.Read(io.NewSectionReader(s.input, int64(hdr.Phoff), int64(hdr.Phnum)*phdrSize), s.byteOrder, s.programHeaders); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *ELFStream) readHeader() error {
	byteOrder, err := readByteOrder(s.input)
	if err != nil {
		return err
	}
	s.byteOrder = byteOrder
	if err := binary.Read(io.NewSectionReader(s.input, 0, ehdrSize), byteOrder, &s.elfHeader); err != nil {
		return err
	}

	hdr := &s.elfHeader
	if hdr.Version != uint32(elf.EV_CURRENT) {
		return fmt.Errorf("invalid ELF version %d", hdr.Version)
	}
	// Only linked executables and shared objects have the address layout this
	// writer preserves; relocatable objects are not supported.
	if elf.Type(hdr.Type) != elf.ET_EXEC && elf.Type(hdr.Type) != elf.ET_DYN {
		return unsupported("ELF type %d", hdr.Type)
	}
	// Tables are decoded with fixed ELF64 record sizes: smaller records can't
	// hold the ELF64 fields; larger ones are valid but not handled.
	if hdr.Ehsize < ehdrSize || hdr.Shentsize < shdrSize || (hdr.Phnum != 0 && hdr.Phentsize < phdrSize) {
		return errors.New("ELF header entry sizes are too small")
	}
	if hdr.Ehsize != ehdrSize || hdr.Shentsize != shdrSize || (hdr.Phnum != 0 && hdr.Phentsize != phdrSize) {
		return unsupported("ELF header entry sizes")
	}
	// Extended section/program numbering is not implemented.
	if hdr.Shnum == 0 || hdr.Shnum >= uint16(elf.SHN_LORESERVE) || hdr.Phnum == math.MaxUint16 {
		return unsupported("missing or extended section/program numbering")
	}
	if hdr.Shstrndx == 0 || hdr.Shstrndx >= hdr.Shnum {
		return errors.New("invalid ELF section name table index")
	}
	return nil
}

// readSections reads every section header and name, including for sections
// whose bytes will be omitted: output keeps every slot so indices stay valid.
func (s *ELFStream) readSections(ctx context.Context, inputSize int64) error {
	hdr := &s.elfHeader
	sectionHeaders := make([]elf.Section64, hdr.Shnum)
	if err := binary.Read(io.NewSectionReader(s.input, int64(hdr.Shoff), int64(hdr.Shnum)*shdrSize), s.byteOrder, sectionHeaders); err != nil {
		return err
	}
	// Slot zero must be the reserved null section. Without extended numbering,
	// rejected above, its other fields are meaningless; post-link tools can leave
	// stale values there, so write it zeroed.
	if elf.SectionType(sectionHeaders[0].Type) != elf.SHT_NULL {
		return errors.New("invalid ELF null section")
	}
	sectionHeaders[0] = elf.Section64{}
	nameTableHeader := sectionHeaders[hdr.Shstrndx]
	if elf.SectionType(nameTableHeader.Type) != elf.SHT_STRTAB || nameTableHeader.Flags&uint64(elf.SHF_COMPRESSED) != 0 || nameTableHeader.Size == 0 {
		return errors.New("invalid ELF section name table")
	}
	// Bound the allocation before reading the table.
	if !withinInput(nameTableHeader.Off, nameTableHeader.Size, inputSize) {
		return fmt.Errorf("ELF section name table is outside the input: %w", io.ErrUnexpectedEOF)
	}
	if nameTableHeader.Size > maxSectionNames {
		return unsupported("section name table of %d bytes exceeds %d", nameTableHeader.Size, maxSectionNames)
	}
	nameTable := make([]byte, int(nameTableHeader.Size))
	if _, err := s.input.ReadAt(nameTable, int64(nameTableHeader.Off)); err != nil {
		return err
	}
	if nameTable[0] != 0 {
		return errors.New("invalid ELF section name table")
	}
	nameTableText := string(nameTable)
	s.inputSectionNames = nameTable
	s.sections = make([]outputSection, len(sectionHeaders))
	for i, sectionHeader := range sectionHeaders {
		if err := ctx.Err(); err != nil {
			return err
		}
		if uint64(sectionHeader.Name) >= uint64(len(nameTable)) {
			return fmt.Errorf("invalid section %d name", i)
		}
		nameLen := strings.IndexByte(nameTableText[int(sectionHeader.Name):], 0)
		if nameLen < 0 {
			return fmt.Errorf("invalid section %d name", i)
		}
		name := nameTableText[int(sectionHeader.Name) : int(sectionHeader.Name)+nameLen]
		s.sections[i] = outputSection{header: sectionHeader, name: name, inputOffset: sectionHeader.Off}
	}
	return nil
}

// checkRetainedSections validates the sections Write will copy, after
// replacements: links name an input section, alignment is a power of two, bytes
// are inside the input, and compressed sections hold a compression header.
// Metadata of omitted sections is deliberately not checked.
func (s *ELFStream) checkRetainedSections(inputSize int64) error {
	for i := range s.sections {
		sec := &s.sections[i]
		if sec.typ() == elf.SHT_NOBITS {
			continue
		}
		// Links must target input slots; replacement sections have none.
		if sec.header.Link >= uint32(s.elfHeader.Shnum) {
			return fmt.Errorf("invalid retained section %d link", i)
		}
		// Layout rounds offsets with (pos+a-1) &^ (a-1), which needs a power of
		// two; the ELF spec requires it too. 0 and 1 mean no alignment.
		if align := sec.header.Addralign; align != 0 && align&(align-1) != 0 {
			return fmt.Errorf("invalid retained section %d alignment", i)
		}
		// Fail on a known truncation before the upload starts.
		if sec.inputSize != 0 && !withinInput(sec.inputOffset, sec.inputSize, inputSize) {
			return fmt.Errorf("section %d is outside the input", i)
		}
		// Compressed contents are copied as is; only the header must fit.
		compressed := sec.header.Flags&uint64(elf.SHF_COMPRESSED) != 0
		if compressed && sec.header.Size < chdrSize {
			return fmt.Errorf("invalid compression header in section %d", i)
		}
		// Symbol tables are arrays of 24-byte entries starting with the null
		// symbol, and Info (the first global) is within the table.
		if isSymbolTable(sec.typ()) && !compressed && !validSymbolTable(sec.header) {
			return fmt.Errorf("invalid symbol table %d layout", i)
		}
	}
	return nil
}

func validSymbolTable(header elf.Section64) bool {
	return header.Entsize == symSize && header.Size >= symSize && header.Size%symSize == 0 &&
		uint64(header.Info) <= header.Size/symSize
}

func withinInput(offset, length uint64, inputSize int64) bool {
	return inputSize >= 0 && offset <= uint64(inputSize) && length <= uint64(inputSize)-offset
}
