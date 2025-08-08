// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"bytes"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"unsafe"

	dlvdwarf "github.com/go-delve/delve/pkg/dwarf"
	"github.com/klauspost/compress/zlib"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// FindTextSectionHeader finds the text section header in an elf file.
func FindTextSectionHeader(f *safeelf.File) (*safeelf.SectionHeader, error) {
	for _, s := range f.Sections {
		if s.Name == ".text" {
			return &s.SectionHeader, nil
		}
	}
	return nil, fmt.Errorf("text section not found")
}

// ElfFile is a struct that contains the data for an ELF file.
//
// This object is safe for concurrent use.
type ElfFile struct {
	// Underlying is the underlying elf file.
	Underlying *MMappingElfFile

	dwarfSections *DebugSections
	dwarfData     dwarf.Data
	architecture  bininspect.GoArch
	reader        loclist.Reader
}

// TextSectionHeader implements File.
func (e *ElfFile) TextSectionHeader() (*safeelf.SectionHeader, error) {
	for _, s := range e.Underlying.Elf.Sections {
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
//
// Note that the returned dwarf.Data is not usable after the ElfFile is closed.
func (e *ElfFile) DwarfData() *dwarf.Data {
	// By making the dwarf.Data part of the ElfFile struct, we can ensure
	// that the ElfFile is not finalized while the dwarf.Data is in use.
	return &e.dwarfData
}

// DwarfSections implements File.
//
// Note that the returned DebugSections are not usable after the ElfFile is
// closed.
func (e *ElfFile) DwarfSections() *DebugSections {
	return e.dwarfSections
}

// LoclistReader implements File.
//
// Note that the returned loclist.Reader is not usable after the ElfFile is
// closed.
func (e *ElfFile) LoclistReader() *loclist.Reader {
	return &e.reader
}

// Close implements File.
//
// Note that various items returned from methods on the ElfFile are not usable
// after the ElfFile is closed.
func (e *ElfFile) Close() error {
	for _, s := range e.dwarfSections.sections() {
		if s != nil {
			s.Close()
		}
	}
	return e.Underlying.Close()
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

// InMemoryElfFileLoader is an elf file loader that stores decompressed section
// data in memory.
type InMemoryElfFileLoader struct {
	_ struct{} // prevent instantiation
}

// Load loads an elf file from the given path.
func (l *InMemoryElfFileLoader) Load(path string) (*ElfFile, error) {
	return OpenElfFile(path)
}

// NewInMemoryElfFileLoader creates an InMemoryElfFileLoader that will load elf
// files from disk and use anonymous memory mappings for the compressed section
// data. Importantly these sections are not part of the Go heap, and thus do not
// contribute to the heap size that the Go runtime tracks to guide garbage
// collection, but they are in RAM and will count towards the RSS used by the
// various OOM killers.
func NewInMemoryElfFileLoader() *InMemoryElfFileLoader {
	return &InMemoryElfFileLoader{}
}

// OpenElfFile opens an elf file from a path, using anonymous memory mappings
// for the compressed sections.
func OpenElfFile(path string) (*ElfFile, error) {
	mmf, err := OpenMMappingElfFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load elf file: %w", err)
	}
	return newElfObject(mmf, anonymousMmapCompressedSectionLoader{})
}

// newElfObject creates a new Binary from an elf file.
//
// Note that this does not take ownership of the file -- it will not
// be closed when the ElfFile is closed.
func newElfObject(mmf *MMappingElfFile, l compressedSectionLoader) (_ *ElfFile, retErr error) {
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
	arch, err := bininspect.GetArchitecture(mmf.Elf)
	if err != nil {
		return nil, err
	}
	ds, err := loadDebugSections(mmf, l)
	if err != nil {
		return nil, err
	}
	info := ds.Info()
	if info == nil {
		return nil, fmt.Errorf("no .debug_info section found")
	}
	d, err := ds.loadDwarfData()
	if err != nil {
		return nil, err
	}
	_, _, dwarfVersion, byteOrder := dlvdwarf.ReadDwarfLengthVersion(info)
	if byteOrder != binary.LittleEndian {
		return nil, fmt.Errorf("unexpected DWARF byte order: %v", byteOrder)
	}
	unitVersions := dlvdwarf.ReadUnitVersions(ds.Info())
	if dwarfVersion >= 5 {
		// Delve unit offset calculations are 1 byte off.
		// If tests fail, and following code now handles DWARF5, just remove this adjustment:
		// https://github.com/go-delve/delve/blob/946e4885b69396512958b2774402e86acd530bbe/pkg/dwarf/parseutil.go#L126-L142
		uv := make(map[dwarf.Offset]uint8, len(unitVersions))
		for k, v := range unitVersions {
			uv[k-1] = v
		}
		unitVersions = uv
	}
	reader := loclist.MakeReader(
		ds.Loc(),
		ds.LocLists(),
		ds.Addr(),
		uint8(arch.PointerSize()),
		unitVersions,
	)
	return &ElfFile{
		Underlying:    mmf,
		dwarfSections: ds,
		dwarfData:     *d,
		architecture:  arch,
		reader:        reader,
	}, nil
}

// TODO: Build a constructor that can write the decompressed sections to disk
// and generally uses an mmap'd file to avoid keeping anything resident in the
// heap. The elf.Section.Data() method does the decompression and eager loading
// but elf.Section.Open() just sets up the decompression and hands back a reader
// which could be used to load the section into a file we then mmap or something
// like that.

func loadDebugSections(
	mef *MMappingElfFile,
	l compressedSectionLoader,
) (_ *DebugSections, retErr error) {
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
	defer func() {
		if retErr == nil {
			return
		}
		for _, s := range ds.sections() {
			if s != nil {
				_ = s.Close()
			}
		}
	}()
	for _, s := range mef.Elf.Sections {
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

		cr, err := readCompressedFileRange(s, mef.Elf, mef.f)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to read compressed file range for %s: %w", s.Name, err,
			)
		}

		// TODO: Figure out whether it is important that we aren't applying
		// relocations here. The elf code that loads dwarf in `f.DWARF()` does
		// apply relocations, so it's possible that we're not getting the same
		// results as the elf code. See [0] for the code we're not executing
		// like the stdlib when loading this data.
		//
		// 0: https://github.com/golang/go/blob/db55b83c/src/debug/elf/file.go#L1351-L1377
		data, err := compressedSection{
			name:                s.Name,
			compressedFileRange: cr,
		}.data(mef, l)
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

// compressedSection represents a section and the information needed to load
// it from the file.
type compressedSection struct {
	name string
	compressedFileRange
}

type compressedFileRange struct {
	format compressionFormat
	// The offset of the compressed data in the file.
	// Note that this takes into account any compression header.
	offset int64
	// The length of the compressed data.
	compressedLength int64
	// The length of the uncompressed data (same as the compressed length for
	// uncompressed sections).
	uncompressedLength int64
}

type compressedSectionLoader interface {
	load(compressedSection, *MMappingElfFile) (SectionData, error)
}

func (r compressedSection) data(
	mef *MMappingElfFile, loader compressedSectionLoader,
) (SectionData, error) {
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

	switch r.format {
	case compressionFormatUnknown:
		return nil, fmt.Errorf("unknown compression format")
	case compressionFormatNone:
		return mef.mmap(uint64(r.offset), uint64(r.uncompressedLength))
	case compressionFormatZlib:
		uncompressedData, err := loader.load(r, mef)
		if err != nil {
			return nil, fmt.Errorf("failed to load section data: %w", err)
		}
		return uncompressedData, nil
	default:
		return nil, fmt.Errorf("unknown compression format: %d", r.format)
	}
}

type anonymousMmapCompressedSectionLoader struct{}

var _ compressedSectionLoader = anonymousMmapCompressedSectionLoader{}

func (anonymousMmapCompressedSectionLoader) load(
	cr compressedSection, mef *MMappingElfFile,
) (_ SectionData, retErr error) {
	md, zrd, err := mmapCompressedSection(cr.compressedFileRange, mef)
	if err != nil {
		return nil, err
	}
	defer md.Close()
	mapped, err := syscall.Mmap(
		0, // fd
		0, // offset
		int(cr.uncompressedLength),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_GROWSUP,
		syscall.MAP_PRIVATE|syscall.MAP_ANONYMOUS,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap section: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = syscall.Munmap(mapped)
		}
	}()
	if _, err := io.ReadFull(zrd, mapped); err != nil {
		return nil, fmt.Errorf("failed to read section data: %w", err)
	}
	if err := zrd.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zlib reader: %w", err)
	}
	return newMMappedData(mapped, mapped), nil
}

func mmapCompressedSection(
	cr compressedFileRange, mef *MMappingElfFile,
) (*MMappedData, io.ReadCloser, error) {
	md, err := mef.mmap(uint64(cr.offset), uint64(cr.compressedLength))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to mmap section: %w", err)
	}
	zrd, err := zlib.NewReader(bytes.NewReader(md.Data()))
	if err != nil {
		_ = md.Close()
		return nil, nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	return md, zrd, nil
}

// readCompressedFileRange determines the compression and file range information
// for a section.
func readCompressedFileRange(
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
