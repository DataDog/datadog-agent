// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package cuda

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"iter"
	"unsafe"
)

type formatError struct {
	off int64
	msg string
	val any
}

func (e *formatError) Error() string {
	msg := e.msg
	if e.val != nil {
		msg += fmt.Sprintf(" '%v' ", e.val)
	}
	msg += fmt.Sprintf("in record at byte %#x", e.off)
	return msg
}

// sliceCap is like SliceCapWithSize but using generics.
func sliceCap[E any](c uint64) int {
	var v E
	size := uint64(unsafe.Sizeof(v))
	return sliceCapWithSize(size, c)
}

// chunk is an arbitrary limit on how much memory we are willing
// to allocate without concern.
const chunk = 10 << 20 // 1

// sliceCapWithSize returns the capacity to use when allocating a slice.
// After the slice is allocated with the capacity, it should be
// built using append. This will avoid allocating too much memory
// if the capacity is large and incorrect.
//
// A negative result means that the value is always too big.
func sliceCapWithSize(size, c uint64) int {
	if int64(c) < 0 || c != uint64(int(c)) {
		return -1
	}
	if size > 0 && c > (1<<64-1)/size {
		return -1
	}
	if c*size > chunk {
		c = chunk / size
		if c == 0 {
			c = 1
		}
	}
	return int(c)
}

type elfSection struct {
	elf.SectionHeader

	reader     *io.SectionReader
	nameOffset uint32
}

type lazyElfSections struct {
	reader io.ReaderAt
	err    error
}

func newLazyElfSections(reader io.ReaderAt) *lazyElfSections {
	return &lazyElfSections{reader: reader}
}

func (l *lazyElfSections) iterate() iter.Seq[*elfSection] {
	sr := io.NewSectionReader(l.reader, 0, 1<<63-1)

	// Read and decode ELF identifier
	var ident [16]uint8
	if _, err := l.reader.ReadAt(ident[0:], 0); err != nil {
		l.err = err
		return nil
	}
	if ident[0] != '\x7f' || ident[1] != 'E' || ident[2] != 'L' || ident[3] != 'F' {
		l.err = &formatError{0, "bad magic number", ident[0:4]}
		return nil
	}

	cls := elf.Class(ident[elf.EI_CLASS])
	switch cls {
	case elf.ELFCLASS32:
	case elf.ELFCLASS64:
		// ok
	default:
		l.err = &formatError{0, "unknown ELF class", cls}
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
		l.err = &formatError{0, "unknown ELF data encoding", data}
		return nil
	}

	version := elf.Version(ident[elf.EI_VERSION])
	if version != elf.EV_CURRENT {
		l.err = &formatError{0, "unknown ELF version", version}
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
			l.err = &formatError{0, "mismatched ELF version", v}
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
			l.err = &formatError{0, "mismatched ELF version", v}
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
		l.err = &formatError{0, "invalid shoff", shoff}
		return nil
	}
	if phoff < 0 {
		l.err = &formatError{0, "invalid phoff", phoff}
		return nil
	}

	if shoff == 0 && shnum != 0 {
		l.err = &formatError{0, "invalid ELF shnum for shoff=0", shnum}
		return nil
	}

	if shnum > 0 && shstrndx >= shnum {
		l.err = &formatError{0, "invalid ELF shstrndx", shstrndx}
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
		l.err = &formatError{0, "invalid ELF phentsize", phentsize}
		return nil
	}

	// If the number of sections is greater than or equal to SHN_LORESERVE
	// (0xff00), shnum has the value zero and the actual number of section
	// header table entries is contained in the sh_size field of the section
	// header at index 0.
	if shoff > 0 && shnum == 0 {
		var typ, link uint32
		sr.Seek(shoff, io.SeekStart)
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
			l.err = &formatError{shoff, "invalid type of the initial section", elf.SectionType(typ)}
			return nil
		}

		if shnum < int(elf.SHN_LORESERVE) {
			l.err = &formatError{shoff, "invalid ELF shnum contained in sh_size", shnum}
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
				l.err = &formatError{shoff, "invalid ELF shstrndx contained in sh_link", shstrndx}
				return nil
			}
		}
	}

	if shnum > 0 && shentsize < wantShentsize {
		l.err = &formatError{0, "invalid ELF shentsize", shentsize}
		return nil
	}

	// Read section headers
	c := sliceCap[elf.Section](uint64(shnum))
	if c < 0 {
		l.err = &formatError{0, "too many sections", shnum}
		return nil
	}

	var nameSection *elfSection
	var err error
	if shstrndx > 0 {
		sectOffset := shoff + int64(shstrndx)*int64(shentsize)
		nameSection, err = l.readSection(sr, cls, bo, sectOffset, int64(shentsize))
		if err != nil {
			l.err = &formatError{sectOffset, "cannot parse names section: %w", err}
			return nil
		}

		if nameSection.Type != elf.SHT_STRTAB {
			l.err = &formatError{sectOffset, "invalid ELF section name string table type", nameSection.Type}
			return nil
		}
	}

	return func(yield func(*elfSection) bool) {
		for i := 0; i < shnum; i++ {
			off := shoff + int64(i)*int64(shentsize)
			var sect *elfSection
			sect, err = l.readSection(sr, cls, bo, off, int64(shentsize))
			if err != nil {
				l.err = err
				return
			}

			if nameSection != nil {
				var ok bool
				sect.Name, ok = getString(nameSection.reader, int64(sect.nameOffset))
				if !ok {
					l.err = &formatError{off, "bad section name index", sect.nameOffset}
					return
				}
			}

			if !yield(sect) {
				return
			}
		}
	}
}

func (l *lazyElfSections) readSection(reader io.ReaderAt, cls elf.Class, bo binary.ByteOrder, offset int64, size int64) (*elfSection, error) {
	shdata := make([]byte, size)
	if _, err := reader.ReadAt(shdata, offset); err != nil {
		return nil, err
	}

	s := new(elfSection)
	switch cls {
	case elf.ELFCLASS32:
		var sh elf.Section32
		s.nameOffset = bo.Uint32(shdata[unsafe.Offsetof(sh.Name):])
		s.SectionHeader = elf.SectionHeader{
			Type:      elf.SectionType(bo.Uint32(shdata[unsafe.Offsetof(sh.Type):])),
			Flags:     elf.SectionFlag(bo.Uint32(shdata[unsafe.Offsetof(sh.Flags):])),
			Addr:      uint64(bo.Uint32(shdata[unsafe.Offsetof(sh.Addr):])),
			Offset:    uint64(bo.Uint32(shdata[unsafe.Offsetof(sh.Off):])),
			FileSize:  uint64(bo.Uint32(shdata[unsafe.Offsetof(sh.Size):])),
			Link:      bo.Uint32(shdata[unsafe.Offsetof(sh.Link):]),
			Info:      bo.Uint32(shdata[unsafe.Offsetof(sh.Info):]),
			Addralign: uint64(bo.Uint32(shdata[unsafe.Offsetof(sh.Addralign):])),
			Entsize:   uint64(bo.Uint32(shdata[unsafe.Offsetof(sh.Entsize):])),
		}
	case elf.ELFCLASS64:
		var sh elf.Section64
		s.nameOffset = bo.Uint32(shdata[unsafe.Offsetof(sh.Name):])
		s.SectionHeader = elf.SectionHeader{
			Type:      elf.SectionType(bo.Uint32(shdata[unsafe.Offsetof(sh.Type):])),
			Flags:     elf.SectionFlag(bo.Uint64(shdata[unsafe.Offsetof(sh.Flags):])),
			Offset:    bo.Uint64(shdata[unsafe.Offsetof(sh.Off):]),
			FileSize:  bo.Uint64(shdata[unsafe.Offsetof(sh.Size):]),
			Addr:      bo.Uint64(shdata[unsafe.Offsetof(sh.Addr):]),
			Link:      bo.Uint32(shdata[unsafe.Offsetof(sh.Link):]),
			Info:      bo.Uint32(shdata[unsafe.Offsetof(sh.Info):]),
			Addralign: bo.Uint64(shdata[unsafe.Offsetof(sh.Addralign):]),
			Entsize:   bo.Uint64(shdata[unsafe.Offsetof(sh.Entsize):]),
		}
	}
	if int64(s.Offset) < 0 {
		return nil, &formatError{offset, "invalid section offset", int64(s.Offset)}
	}
	if int64(s.FileSize) < 0 {
		return nil, &formatError{offset, "invalid section size", int64(s.FileSize)}
	}
	s.reader = io.NewSectionReader(reader, int64(s.Offset), int64(s.FileSize))

	if s.Flags&elf.SHF_COMPRESSED == 0 {
		s.Size = s.FileSize
	} else {
		// Ignore compressed sections
		return nil, nil
	}

	return s, nil
}

func getString(reader *io.SectionReader, start int64) (string, bool) {
	var data []byte
	var b [1]byte

	for ; ; start++ {
		if _, err := reader.ReadAt(b[:], int64(start)); err != nil {
			return "", false
		}
		if b[0] == 0 {
			break
		}
		data = append(data, b[0])
	}

	return string(data), true
}
