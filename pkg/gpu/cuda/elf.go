// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Some of the code of this file is based on the debug/elf package from the Go standard library.

package cuda

import (
	//nolint:depguard
	// we need the original elf package for all the types/consts
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"unsafe"
)

// stringBufferSize is the size of the reusable buffer used to read strings from ELF sections
const stringBufferSize = 256

// errorUnsupported is returned when an unsupported section is found, which indicates we should skip that section
var errorUnsupported = errors.New("unsupported section parsing")

// lazySectionReader is a lazy reader for ELF sections, with reduced number of allocations.
// It's oriented around an iterator pattern, where only one section is read (and allocated) at a time.
type lazySectionReader struct {
	reader              io.ReaderAt
	err                 error
	stringBuffer        []byte
	sectionHeaderBuffer []byte
	currentSection      *elfSection
}

// newLazySectionReader a new lazySectionReader instance.
func newLazySectionReader(reader io.ReaderAt) *lazySectionReader {
	return &lazySectionReader{
		reader:         reader,
		stringBuffer:   make([]byte, stringBufferSize),
		currentSection: new(elfSection),
	}
}

func (l *lazySectionReader) Err() error {
	return l.err
}

// Iterate reads the ELF sections and yields them one by one. After iterating
// through all sections, the error field l.err is set if an error occurred.
// Part of the code has been adapted from the NewFile method in the debug/elf package of the Go standard library.
func (l *lazySectionReader) Iterate() iter.Seq[*elfSection] {
	sr := io.NewSectionReader(l.reader, 0, 1<<63-1)

	// Read and decode ELF identifier
	var ident [16]uint8
	if _, err := l.reader.ReadAt(ident[0:], 0); err != nil {
		l.err = err
		return nil
	}
	if ident[0] != '\x7f' || ident[1] != 'E' || ident[2] != 'L' || ident[3] != 'F' {
		l.err = fmt.Errorf("bad magic number %v", ident[0:4])
		return nil
	}

	cls := elf.Class(ident[elf.EI_CLASS])
	switch cls {
	case elf.ELFCLASS32:
	case elf.ELFCLASS64:
		// ok
	default:
		l.err = fmt.Errorf("unknown ELF class: %v", cls)
		return nil
	}

	data := elf.Data(ident[elf.EI_DATA])
	var bo binary.ByteOrder
	switch data {
	case elf.ELFDATA2LSB:
		bo = binary.LittleEndian
	case elf.ELFDATA2MSB:
		bo = binary.BigEndian
	default:
		l.err = fmt.Errorf("unknown data encoding %v", data)
		return nil
	}

	version := elf.Version(ident[elf.EI_VERSION])
	if version != elf.EV_CURRENT {
		l.err = fmt.Errorf("unknown ELF version %v", version)
		return nil
	}

	// Read ELF file header
	var phoff int64
	var phentsize, phnum int
	var shoff int64
	var shentsize, shnum, shstrndx int
	switch cls {
	case elf.ELFCLASS32:
		var hdr elf.Header32
		data := make([]byte, unsafe.Sizeof(hdr))
		if _, err := sr.ReadAt(data, 0); err != nil {
			l.err = err
			return nil
		}
		if v := elf.Version(bo.Uint32(data[unsafe.Offsetof(hdr.Version):])); v != version {
			l.err = fmt.Errorf("mismatched ELF version: %v", v)
			return nil
		}
		phoff = int64(bo.Uint32(data[unsafe.Offsetof(hdr.Phoff):]))
		phentsize = int(bo.Uint16(data[unsafe.Offsetof(hdr.Phentsize):]))
		phnum = int(bo.Uint16(data[unsafe.Offsetof(hdr.Phnum):]))
		shoff = int64(bo.Uint32(data[unsafe.Offsetof(hdr.Shoff):]))
		shentsize = int(bo.Uint16(data[unsafe.Offsetof(hdr.Shentsize):]))
		shnum = int(bo.Uint16(data[unsafe.Offsetof(hdr.Shnum):]))
		shstrndx = int(bo.Uint16(data[unsafe.Offsetof(hdr.Shstrndx):]))
	case elf.ELFCLASS64:
		var hdr elf.Header64
		data := make([]byte, unsafe.Sizeof(hdr))
		if _, err := sr.ReadAt(data, 0); err != nil {
			l.err = err
			return nil
		}
		if v := elf.Version(bo.Uint32(data[unsafe.Offsetof(hdr.Version):])); v != version {
			l.err = fmt.Errorf("mismatched ELF version: %v", v)
			return nil
		}
		phoff = int64(bo.Uint64(data[unsafe.Offsetof(hdr.Phoff):]))
		phentsize = int(bo.Uint16(data[unsafe.Offsetof(hdr.Phentsize):]))
		phnum = int(bo.Uint16(data[unsafe.Offsetof(hdr.Phnum):]))
		shoff = int64(bo.Uint64(data[unsafe.Offsetof(hdr.Shoff):]))
		shentsize = int(bo.Uint16(data[unsafe.Offsetof(hdr.Shentsize):]))
		shnum = int(bo.Uint16(data[unsafe.Offsetof(hdr.Shnum):]))
		shstrndx = int(bo.Uint16(data[unsafe.Offsetof(hdr.Shstrndx):]))
	}

	if shoff < 0 {
		l.err = fmt.Errorf("invalid shoff: %v", shoff)
		return nil
	}
	if phoff < 0 {
		l.err = fmt.Errorf("invalid phoff: %v", phoff)
		return nil
	}

	if shoff == 0 && shnum != 0 {
		l.err = fmt.Errorf("invalid ELF shnum for shoff=0: %v", shnum)
		return nil
	}

	if shnum > 0 && shstrndx >= shnum {
		l.err = fmt.Errorf("invalid ELF shstrndx: %v", shstrndx)
		return nil
	}

	var wantPhentsize, wantShentsize int
	switch cls {
	case elf.ELFCLASS32:
		wantPhentsize = 8 * 4
		wantShentsize = 10 * 4
	case elf.ELFCLASS64:
		wantPhentsize = 2*4 + 6*8
		wantShentsize = 4*4 + 6*8
	}
	if phnum > 0 && phentsize < wantPhentsize {
		l.err = fmt.Errorf("invalid ELF phentsize: %v", phentsize)
		return nil
	}

	// If the number of sections is greater than or equal to SHN_LORESERVE
	// (0xff00), shnum has the value zero and the actual number of section
	// header table entries is contained in the sh_size field of the section
	// header at index 0.
	if shoff > 0 && shnum == 0 {
		var typ, link uint32
		_, _ = sr.Seek(shoff, io.SeekStart)
		switch cls {
		case elf.ELFCLASS32:
			sh := new(elf.Section32)
			if err := binary.Read(sr, bo, sh); err != nil {
				l.err = err
				return nil
			}
			shnum = int(sh.Size)
			typ = sh.Type
			link = sh.Link
		case elf.ELFCLASS64:
			sh := new(elf.Section64)
			if err := binary.Read(sr, bo, sh); err != nil {
				l.err = err
				return nil
			}
			shnum = int(sh.Size)
			typ = sh.Type
			link = sh.Link
		}
		if elf.SectionType(typ) != elf.SHT_NULL {
			l.err = fmt.Errorf("invalid type of the initial section at offset %d: %v", shoff, elf.SectionType(typ))
			return nil
		}

		if shnum < int(elf.SHN_LORESERVE) {
			l.err = fmt.Errorf("invalid ELF shnum contained in sh_size at offset %d: %v", shoff, shnum)
			return nil
		}

		// If the section name string table section index is greater than or
		// equal to SHN_LORESERVE (0xff00), this member has the value
		// SHN_XINDEX (0xffff) and the actual index of the section name
		// string table section is contained in the sh_link field of the
		// section header at index 0.
		if shstrndx == int(elf.SHN_XINDEX) {
			shstrndx = int(link)
			if shstrndx < int(elf.SHN_LORESERVE) {
				l.err = fmt.Errorf("invalid ELF shstrndx contained in sh_link at offset %d: %v", shoff, shstrndx)
				return nil
			}
		}
	}

	if shnum > 0 && shentsize < wantShentsize {
		l.err = fmt.Errorf("invalid ELF shentsize: %v", shentsize)
		return nil
	}

	// allocate the buffer for the section header, only once
	l.sectionHeaderBuffer = make([]byte, shentsize)

	var nameSection *elfSection
	var err error
	if shstrndx > 0 {
		sectOffset := shoff + int64(shstrndx)*int64(shentsize)
		err = l.readSection(sr, cls, bo, sectOffset)
		if err != nil {
			l.err = fmt.Errorf("cannot parse names section at offset %d: %w", sectOffset, err)
			return nil
		}

		// allocate a new currentSection to avoid overwriting the nameSection
		nameSection = l.currentSection
		l.currentSection = new(elfSection)

		if nameSection.Type != elf.SHT_STRTAB {
			l.err = fmt.Errorf("invalid ELF section name string table type at offset %d: %v", sectOffset, nameSection.Type)
			return nil
		}
	}

	return func(yield func(*elfSection) bool) {
		for i := 0; i < shnum; i++ {
			off := shoff + int64(i)*int64(shentsize)
			err = l.readSection(sr, cls, bo, off)
			if err != nil {
				if errors.Is(err, errorUnsupported) {
					continue // Skip unsupported sections
				}

				l.err = fmt.Errorf("cannot read section at offset %d: %w", off, err)
				return
			}

			if nameSection != nil {
				var ok bool
				l.currentSection.nameBytes, ok = l.getString(nameSection.Reader(), int64(l.currentSection.nameOffset))
				if !ok {
					l.err = fmt.Errorf("bad section name index at offset=%d: %v", shoff, l.currentSection.nameOffset)
					return
				}
			}

			if !yield(l.currentSection) {
				return
			}
		}
	}
}

// readSection reads the section header at the given offset and initializes the current section.
// Part of the code has been taken from the NewFile method in the debug/elf package of the Go standard library.
func (l *lazySectionReader) readSection(reader io.ReaderAt, cls elf.Class, bo binary.ByteOrder, offset int64) error {
	// sectionHeaderBuffer is already allocated with the correct size based on the header size
	// attribute of the ELF file, so we can reuse it for every section.
	if _, err := reader.ReadAt(l.sectionHeaderBuffer, offset); err != nil {
		return err
	}

	switch cls {
	case elf.ELFCLASS32:
		var sh elf.Section32
		l.currentSection.nameOffset = bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Name):])
		l.currentSection.SectionHeader = elf.SectionHeader{
			Type:      elf.SectionType(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Type):])),
			Flags:     elf.SectionFlag(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Flags):])),
			Addr:      uint64(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Addr):])),
			Offset:    uint64(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Off):])),
			FileSize:  uint64(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Size):])),
			Link:      bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Link):]),
			Info:      bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Info):]),
			Addralign: uint64(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Addralign):])),
			Entsize:   uint64(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Entsize):])),
		}
	case elf.ELFCLASS64:
		var sh elf.Section64
		l.currentSection.nameOffset = bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Name):])
		l.currentSection.SectionHeader = elf.SectionHeader{
			Type:      elf.SectionType(bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Type):])),
			Flags:     elf.SectionFlag(bo.Uint64(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Flags):])),
			Offset:    bo.Uint64(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Off):]),
			FileSize:  bo.Uint64(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Size):]),
			Addr:      bo.Uint64(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Addr):]),
			Link:      bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Link):]),
			Info:      bo.Uint32(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Info):]),
			Addralign: bo.Uint64(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Addralign):]),
			Entsize:   bo.Uint64(l.sectionHeaderBuffer[unsafe.Offsetof(sh.Entsize):]),
		}
	}
	if int64(l.currentSection.Offset) < 0 {
		return fmt.Errorf("invalid section offset at offset=%d: %v", offset, int64(l.currentSection.Offset))
	}
	if int64(l.currentSection.FileSize) < 0 {
		return fmt.Errorf("invalid section size at offset=%d: %v", offset, int64(l.currentSection.FileSize))
	}
	l.currentSection.fileReader = reader

	if l.currentSection.Flags&elf.SHF_COMPRESSED != 0 {
		// Ignore compressed sections
		return errorUnsupported
	}

	l.currentSection.Size = l.currentSection.FileSize

	return nil
}

func (l *lazySectionReader) getString(reader *io.SectionReader, start int64) ([]byte, bool) {
	dstOffset := 0
	srcOffset := start

	for ; srcOffset-start < stringBufferSize; srcOffset++ {
		if _, err := reader.ReadAt(l.stringBuffer[dstOffset:dstOffset+1], int64(srcOffset)); err != nil {
			return nil, false
		}
		if l.stringBuffer[dstOffset] == 0 {
			break
		}
		dstOffset++
	}

	return l.stringBuffer[:dstOffset], true
}

// elfSection represents an ELF section, with the header and name.
// Note that the name in the SectionHeader is not filled, one should use the nameBytes field.
type elfSection struct {
	elf.SectionHeader

	// fileReader holds a reader for the entire ELF file, used to read the section data
	// without allocating a reader if it's not needed
	fileReader io.ReaderAt

	// sectReader is the reader for the section data, initialized lazily
	sectReader *io.SectionReader

	nameOffset uint32
	nameBytes  []byte
}

func (s *elfSection) Reader() *io.SectionReader {
	if s.sectReader == nil {
		s.sectReader = io.NewSectionReader(s.fileReader, int64(s.Offset), int64(s.FileSize))
	}

	return s.sectReader
}

func (s *elfSection) Name() string {
	return string(s.nameBytes)
}
