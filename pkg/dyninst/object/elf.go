// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"compress/zlib"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"

	dlvdwarf "github.com/go-delve/delve/pkg/dwarf"
	"github.com/go-delve/delve/pkg/dwarf/loclist"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// ElfFile is a struct that contains the data for an ELF file.
//
// This object is safe for concurrent use.
type ElfFile struct {
	// Underlying is the underlying elf file.
	Underlying *safeelf.File

	// osFile is the file we'll close when we're done.
	osFile *os.File

	dwarfSections *DebugSections
	dwarfData     *dwarf.Data
	architecture  bininspect.GoArch

	// Maps the compile unit entry offset to the version of the DWARF spec
	// that was used to parse it.
	unitVersions map[dwarf.Offset]uint8
}

// TextSectionHeader implements File.
func (e *ElfFile) TextSectionHeader() (*safeelf.SectionHeader, error) {
	for _, s := range e.Underlying.Sections {
		if s.Name == ".text" {
			return &s.SectionHeader, nil
		}
	}
	return nil, fmt.Errorf("text section not found")
}

// Architecture implements File.
func (e *ElfFile) Architecture() Architecture {
	return e.architecture
}

// DwarfData implements File.
func (e *ElfFile) DwarfData() *dwarf.Data {
	return e.dwarfData
}

// DwarfSections implements File.
func (e *ElfFile) DwarfSections() *DebugSections {
	return e.dwarfSections
}

// LoclistReader implements File.
func (e *ElfFile) LoclistReader() (*LoclistReader, error) {
	if e.dwarfSections.Loc == nil {
		return nil, fmt.Errorf("no loclist section found")
	}

	reader2 := loclist.NewDwarf2Reader(
		e.dwarfSections.Loc,
		int(e.architecture.PointerSize()),
	)
	return &LoclistReader{
		reader2:   reader2,
		maxOffset: len(e.dwarfSections.Loc) - 1,
		unitVersionGetter: func(unit *dwarf.Entry) (uint8, bool) {
			unitVersion, ok := e.unitVersions[unit.Offset]
			return unitVersion, ok
		},
	}, nil
}

// Close implements File.
func (e *ElfFile) Close() error {
	return e.osFile.Close()
}

var _ File = (*ElfFile)(nil)

// PointerSize implements File.
func (e *ElfFile) PointerSize() uint8 {
	return uint8(e.architecture.PointerSize())
}

// IsStrippedBinaryError returns true if the error is due to a stripped binary.
func IsStrippedBinaryError(err error) bool {
	var decodeErr dwarf.DecodeError
	return errors.As(err, &decodeErr)
}

// NewElfObject creates a new Binary from an elf file.
//
// Note that the returned ElfFile should be considered to be the owner
// of this file after this function returns. If an error occurs, the file
// will be closed.
func NewElfObject(f *os.File) (_ *ElfFile, retErr error) {
	defer func() {
		if retErr != nil {
			if closeErr := f.Close(); closeErr != nil {
				retErr = fmt.Errorf(
					"%w; failed to close file: %w", retErr, closeErr,
				)
			}
		}
	}()
	elfFile, err := safeelf.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to load elf file: %w", err)
	}

	// The safeelf package also has functionality to load the DWARF sections
	// but it doesn't provide enough control to get our hands on the sections
	// we need to access later.
	//
	// In the future we should consider folding this code into the safeelf
	// package. For now we're avoiding that in part because we're not taking
	// all the same care to deal with relocations. We will, however, recover
	// from panics like that code would.
	defer func() {
		r := recover()
		switch r := r.(type) {
		case nil:
			// carry on
		case error:
			retErr = fmt.Errorf("panic loading DWARF sections: %w", r)
		default:
			retErr = fmt.Errorf("panic loading DWARF sections: %v", r)
		}
	}()
	arch, err := bininspect.GetArchitecture(elfFile)
	if err != nil {
		return nil, err
	}
	ds, err := loadDebugSections(elfFile, f)
	if err != nil {
		return nil, err
	}
	if ds.Info == nil {
		return nil, fmt.Errorf("no .debug_info section found")
	}
	d, err := ds.loadDwarfData()
	if err != nil {
		return nil, err
	}
	unitVersions := dlvdwarf.ReadUnitVersions(ds.Info)
	return &ElfFile{
		Underlying:    elfFile,
		osFile:        f,
		dwarfSections: ds,
		dwarfData:     d,
		architecture:  arch,
		unitVersions:  unitVersions,
	}, nil
}

// TODO: Build a constructor that can write the decompressed sections to disk
// and generally uses an mmap'd file to avoid keeping anything resident in the
// heap. The elf.Section.Data() method does the decompression and eager loading
// but elf.Section.Open() just sets up the decompression and hands back a reader
// which could be used to load the section into a file we then mmap or something
// like that.

func loadDebugSections(elfFile *safeelf.File, f io.ReaderAt) (*DebugSections, error) {
	dwarfSuffix := func(s *safeelf.Section) string {
		const debug = ".debug_"
		const zdebug = ".zdebug_"
		switch {
		case strings.HasPrefix(s.Name, debug):
			return s.Name[len(debug):]
		case strings.HasPrefix(s.Name, zdebug):
			return s.Name[len(zdebug):]
		default:
			return ""
		}
	}
	// There are many DWARf sections, but these are the ones
	// the debug/dwarf package started with.
	var ds DebugSections
	for _, s := range elfFile.Sections {
		suffix := dwarfSuffix(s)
		if suffix == "" {
			continue
		}
		sd := ds.getSection(suffix)
		if sd == nil {
			continue
		}
		if *sd != nil {
			return nil, fmt.Errorf("section %s already loaded", s.Name)
		}

		cr, err := sectionData(s, elfFile, f)
		if err != nil {
			return nil, fmt.Errorf("failed to get section data for %s: %w", s.Name, err)
		}

		// TODO: Figure out whether it is important that we aren't applying
		// relocations here. The elf code that loads dwarf in `f.DWARF()` does
		// apply relocations, so it's possible that we're not getting the same
		// results as the elf code. See [0] for the code we're not executing
		// like the stdlib when loading this data.
		//
		// 0: https://github.com/golang/go/blob/db55b83c/src/debug/elf/file.go#L1351-L1377
		data, err := cr.data(f)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get section data for %s: %w", s.Name, err,
			)
		}
		*sd = data
	}
	return &ds, nil
}

type compressionFormat uint8

const (
	compressionFormatUnknown compressionFormat = iota
	compressionFormatNone
	compressionFormatZlib
)

type compressedFileRange struct {
	format compressionFormat
	// The offset of the compressed data in the file.
	// Note that this takes into account any compression header.
	offset int64
	// The length of the compressed data.
	compressedLength int64
	// The length of the uncompressed data.
	uncompressedLength int64
}

func (r compressedFileRange) data(fileReaderAt io.ReaderAt) ([]byte, error) {
	// We don't want to let an invalid section header cause us to load
	// too much data into memory.
	const maxSectionSize = 512 << 20 // 512MiB

	if r.uncompressedLength > maxSectionSize {
		return nil, fmt.Errorf(
			"section has a decompressed size of %d bytes, "+
				"exceeding the maximum of %d bytes",
			r.uncompressedLength,
			maxSectionSize,
		)
	}

	var reader io.Reader
	switch r.format {
	case compressionFormatUnknown:
		return nil, fmt.Errorf("unknown compression format")
	case compressionFormatNone:
		reader = io.NewSectionReader(fileReaderAt, r.offset, r.compressedLength)
	case compressionFormatZlib:
		input := io.NewSectionReader(
			fileReaderAt,
			r.offset,
			r.compressedLength,
		)

		zrd, err := zlib.NewReader(input)
		if err != nil {
			return nil, fmt.Errorf("failed to create zlib reader: %w", err)
		}
		reader = zrd
	}

	data := make([]byte, r.uncompressedLength)
	_, err := io.ReadFull(reader, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read section data: %w", err)
	}
	return data, nil
}

func sectionData(
	s *safeelf.Section,
	elfFile *safeelf.File,
	fileReaderAt io.ReaderAt,
) (compressedFileRange, error) {
	var r compressedFileRange
	if s.Type == elf_SHT_NOBITS {
		return r, nil
	}

	// If the section doesn't have the compressed flag set, it still might
	// be compressed with the "gnu" compression scheme. This is somewhat
	// outdated for elf binaries produced by a modern Go toolchain, but it's
	// the scheme used for compression in Mach-O binaries on macos.
	//
	// See https://github.com/golang/go/blob/d166a0b0/src/debug/elf/file.go#L134-L149
	if s.Flags&elf_SHF_COMPRESSED == 0 {
		if !strings.HasPrefix(s.Name, ".zdebug") {
			return compressedFileRange{
				format:             compressionFormatNone,
				offset:             int64(s.Offset),
				compressedLength:   int64(s.Size),
				uncompressedLength: int64(s.Size),
			}, nil
		}
		sr := io.NewSectionReader(fileReaderAt, int64(s.Offset), int64(s.Size))

		const gnuCompressionMagic = "ZLIB\x00\x00\x00\x00"
		const gnuCompressionHeaderLength = 12 // magic + 4-byte big endian uncompressed length
		b := make([]byte, gnuCompressionHeaderLength)
		n, _ := sr.ReadAt(b, 0)
		if n != 12 || string(b[:8]) != gnuCompressionMagic {
			return compressedFileRange{
				format:             compressionFormatUnknown,
				offset:             int64(s.Offset + gnuCompressionHeaderLength),
				compressedLength:   int64(s.Size),
				uncompressedLength: int64(s.Size),
			}, nil
		}

		r.format = compressionFormatZlib
		r.uncompressedLength = int64(binary.BigEndian.Uint32(b[8:12]))
		r.compressedLength = int64(s.Size) - gnuCompressionHeaderLength
		r.offset = int64(s.Offset) + gnuCompressionHeaderLength
		return r, nil
	}

	// Parse out the compression header.
	//
	// See https://github.com/golang/go/blob/d166a0b0/src/debug/elf/file.go#L557-L579
	if s.Flags&elf_SHF_ALLOC != 0 {
		return r, fmt.Errorf("SHF_COMPRESSED applies only to non-allocable sections")
	}
	sr := io.NewSectionReader(fileReaderAt, int64(s.Offset), int64(s.FileSize))

	var bo binary.ByteOrder
	switch elfFile.File.Data {
	case elf_ELFDATA2LSB:
		bo = binary.LittleEndian
	case elf_ELFDATA2MSB:
		bo = binary.BigEndian
	default:
		return r, fmt.Errorf("unknown elf data encoding: %#v", elfFile.File.Data)
	}
	// Read the compression header.
	switch elfFile.File.Class {
	case elf_ELFCLASS32:
		var ch elf_Chdr32
		chdata := make([]byte, unsafe.Sizeof(ch))
		if _, err := sr.ReadAt(chdata, 0); err != nil {
			return r, fmt.Errorf("failed to read compression header: %w", err)
		}
		ct := elf_CompressionType(bo.Uint32(chdata[unsafe.Offsetof(ch.Type):]))
		switch ct {
		case elf_COMPRESS_ZLIB:
			r.format = compressionFormatZlib
			r.offset = int64(s.Offset) + int64(unsafe.Sizeof(ch))
			r.compressedLength = int64(s.SectionHeader.FileSize) - int64(unsafe.Sizeof(ch))
			r.uncompressedLength = int64(bo.Uint64(chdata[unsafe.Offsetof(ch.Size):]))
			return r, nil
		default:
			return r, fmt.Errorf("unknown compression type: %#v %x", ct, ct)
		}
	case elf_ELFCLASS64:
		var ch elf_Chdr64
		chdata := make([]byte, unsafe.Sizeof(ch))
		if _, err := sr.ReadAt(chdata, 0); err != nil {
			return r, err
		}
		ct := elf_CompressionType(bo.Uint32(chdata[unsafe.Offsetof(ch.Type):]))
		switch ct {
		case elf_COMPRESS_ZLIB:
			r.format = compressionFormatZlib
			r.offset = int64(s.Offset) + int64(unsafe.Sizeof(ch))
			r.compressedLength = int64(s.SectionHeader.FileSize) - int64(unsafe.Sizeof(ch))
			r.uncompressedLength = int64(bo.Uint64(chdata[unsafe.Offsetof(ch.Size):]))
			return r, nil
		default:
			return r, fmt.Errorf("unknown compression type: %#v %x", ct, chdata)
		}
	default:
		return r, fmt.Errorf("unknown elf class: %#v", elfFile.File.Class)
	}
}
