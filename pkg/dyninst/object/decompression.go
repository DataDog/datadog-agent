// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"unsafe"

	"github.com/klauspost/compress/zlib"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// decompressingMMappingElfFile wraps an MMappingElfFile and provides access to
// the decompressed section data. It delegates the responsibility of where the
// decompressed data will live (on the heap, in memory, or on disk) to the
// sectionLoader.
type decompressingMMappingElfFile struct {
	MMappingElfFile
	sectionLoader compressedSectionLoader
}

var _ File = (*decompressingMMappingElfFile)(nil)

// Shadows the MMappingElfFile.SectionData method.
func (d *decompressingMMappingElfFile) SectionData(
	s *safeelf.SectionHeader,
) (SectionData, error) {
	cr, err := readCompressedFileRange(s, d.File, d.f)
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
	data, err := compressedSectionMetadata{
		name:                s.Name,
		compressedFileRange: cr,
	}.data(&d.MMappingElfFile, d.sectionLoader)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get section data for %s: %w", s.Name, err,
		)
	}
	return data, nil
}

type compressionFormat uint8

const (
	compressionFormatUnknown compressionFormat = iota
	compressionFormatNone
	compressionFormatZlib
)

// compressedSectionMetadata represents a section and the information needed to
// load it from the file.
type compressedSectionMetadata struct {
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
	load(compressedSectionMetadata, *MMappingElfFile) (SectionData, error)
}

func (r compressedSectionMetadata) data(
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
		return nil, errors.New("unknown compression format")
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
	cr compressedSectionMetadata, mef *MMappingElfFile,
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
) (*mmappedData, io.ReadCloser, error) {
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
//
// Note that we do not rely on the elf package's implementation of the
// compression header parsing and fields in the section header because it
// doesn't report the compression type and it doesn't report where the
// compressed data starts.
func readCompressedFileRange(
	s *safeelf.SectionHeader,
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
		return r, errors.New("SHF_COMPRESSED applies only to non-allocable sections")
	}
	sr := io.NewSectionReader(fileReaderAt, int64(s.Offset), int64(s.FileSize))

	var bo binary.ByteOrder
	switch elfFile.Data {
	case elf_ELFDATA2LSB:
		bo = binary.LittleEndian
	case elf_ELFDATA2MSB:
		bo = binary.BigEndian
	default:
		return r, fmt.Errorf("unknown elf data encoding: %#v", elfFile.Data)
	}
	// Read the compression header.
	switch elfFile.Class {
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
			r.compressedLength = int64(s.FileSize) - int64(unsafe.Sizeof(ch))
			r.uncompressedLength = int64(s.Size)
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
			r.compressedLength = int64(s.FileSize) - int64(unsafe.Sizeof(ch))
			r.uncompressedLength = int64(s.Size)
			return r, nil
		default:
			return r, fmt.Errorf("unknown compression type: %#v %x", ct, chdata)
		}
	default:
		return r, fmt.Errorf("unknown elf class: %#v", elfFile.Class)
	}
}
