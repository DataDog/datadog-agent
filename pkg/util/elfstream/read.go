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

// getELF64ByteOrder checks the identification bytes, which precede any
// byte-order-dependent field.
func getELF64ByteOrder(input io.ReaderAt) (binary.ByteOrder, error) {
	var ident [elf.EI_NIDENT]byte
	if _, err := input.ReadAt(ident[:], 0); err != nil {
		return nil, err
	}
	if string(ident[:4]) != elf.ELFMAG || ident[elf.EI_CLASS] != byte(elf.ELFCLASS64) || ident[elf.EI_VERSION] != byte(elf.EV_CURRENT) {
		return nil, unsupported("not an ELF64 file")
	}
	switch elf.Data(ident[elf.EI_DATA]) {
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
func readELF64(ctx context.Context, input io.ReaderAt, size int64) (*ELFStream, error) {
	s := &ELFStream{input: io.NewSectionReader(input, 0, size)}
	if err := s.readHeader(); err != nil {
		return nil, err
	}
	if err := s.readSections(ctx, size); err != nil {
		return nil, err
	}
	h := &s.header
	s.programs = make([]elf.Prog64, h.Phnum)
	if len(s.programs) != 0 {
		if err := binary.Read(io.NewSectionReader(s.input, int64(h.Phoff), int64(h.Phnum)*phdrSize), s.order, s.programs); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *ELFStream) readHeader() error {
	order, err := getELF64ByteOrder(s.input)
	if err != nil {
		return err
	}
	s.order = order
	if err := binary.Read(io.NewSectionReader(s.input, 0, ehdrSize), order, &s.header); err != nil {
		return err
	}

	h := &s.header
	// Only linked executables and shared objects have the address layout this
	// writer preserves; relocatable objects are not supported.
	if h.Version != uint32(elf.EV_CURRENT) || (elf.Type(h.Type) != elf.ET_EXEC && elf.Type(h.Type) != elf.ET_DYN) {
		return unsupported("ELF type %d or version %d", h.Type, h.Version)
	}
	// Tables are decoded with fixed ELF64 record sizes.
	if h.Ehsize != ehdrSize || h.Shentsize != shdrSize || (h.Phnum != 0 && h.Phentsize != phdrSize) {
		return unsupported("ELF header entry sizes")
	}
	// Extended section/program numbering is not implemented.
	if h.Shnum == 0 || h.Shnum >= uint16(elf.SHN_LORESERVE) || h.Phnum == math.MaxUint16 {
		return unsupported("missing or extended section/program numbering")
	}
	if h.Shstrndx == 0 || h.Shstrndx >= h.Shnum {
		return errors.New("invalid ELF section name table index")
	}
	return nil
}

// readSections reads every section header and name, including for sections
// whose bytes will be omitted: output keeps every slot so indices stay valid.
func (s *ELFStream) readSections(ctx context.Context, size int64) error {
	h := &s.header
	headers := make([]elf.Section64, h.Shnum)
	if err := binary.Read(io.NewSectionReader(s.input, int64(h.Shoff), int64(h.Shnum)*shdrSize), s.order, headers); err != nil {
		return err
	}
	// Slot zero must be the reserved null section. Without extended numbering,
	// rejected above, its other fields are meaningless; post-link tools can leave
	// stale values there, so write it zeroed.
	if elf.SectionType(headers[0].Type) != elf.SHT_NULL {
		return errors.New("invalid ELF null section")
	}
	headers[0] = elf.Section64{}
	namesHeader := headers[h.Shstrndx]
	if elf.SectionType(namesHeader.Type) != elf.SHT_STRTAB || namesHeader.Flags&uint64(elf.SHF_COMPRESSED) != 0 || namesHeader.Size == 0 {
		return errors.New("invalid ELF section name table")
	}
	// Bound the allocation before reading the table.
	if !fileRange(namesHeader.Off, namesHeader.Size, size) {
		return fmt.Errorf("ELF section name table is outside the input: %w", io.ErrUnexpectedEOF)
	}
	if namesHeader.Size > maxSectionNames {
		return unsupported("section name table of %d bytes exceeds %d", namesHeader.Size, maxSectionNames)
	}
	names := make([]byte, int(namesHeader.Size))
	if _, err := s.input.ReadAt(names, int64(namesHeader.Off)); err != nil {
		return err
	}
	if names[0] != 0 {
		return errors.New("invalid ELF section name table")
	}
	nameString := string(names)
	s.names = names
	s.sections = make([]outputSection, len(headers))
	for i, sh := range headers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if uint64(sh.Name) >= uint64(len(names)) {
			return fmt.Errorf("invalid section %d name", i)
		}
		end := strings.IndexByte(nameString[int(sh.Name):], 0)
		if end < 0 {
			return fmt.Errorf("invalid section %d name", i)
		}
		name := nameString[int(sh.Name) : int(sh.Name)+end]
		s.sections[i] = outputSection{header: sh, name: name, source: sh.Off}
	}
	return nil
}

// checkRetainedSections validates the sections Write will copy, after
// replacements. Metadata of omitted sections is deliberately not checked.
func (s *ELFStream) checkRetainedSections(size int64) error {
	for i := range s.sections {
		sec := &s.sections[i]
		if sec.typ() == elf.SHT_NOBITS {
			continue
		}
		// Links must target input slots; replacement sections have none.
		if sec.header.Link >= uint32(s.header.Shnum) {
			return fmt.Errorf("invalid retained section %d link", i)
		}
		if a := sec.header.Addralign; a != 0 && a&(a-1) != 0 {
			return fmt.Errorf("invalid retained section %d alignment", i)
		}
		// Fail on a known truncation before the upload starts.
		if sec.sourceSize != 0 && !fileRange(sec.source, sec.sourceSize, size) {
			return fmt.Errorf("section %d is outside the input", i)
		}
		// Compressed contents are copied as is; only the header must fit.
		if sec.header.Flags&uint64(elf.SHF_COMPRESSED) != 0 && sec.header.Size < chdrSize {
			return fmt.Errorf("invalid compression header in section %d", i)
		}
	}
	return nil
}

func fileRange(offset, size uint64, inputSize int64) bool {
	return inputSize >= 0 && offset <= uint64(inputSize) && size <= uint64(inputSize)-offset
}
