// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// MMappingElfFile is an ElfFile that supports mmapping sections into memory.
type MMappingElfFile struct {
	InMemoryElfFile
	f *os.File
}

var _ File = (*MMappingElfFile)(nil)

// OpenMMappingElfFile creates a new MMappingElfFile for the given path.
func OpenMMappingElfFile(path string) (*MMappingElfFile, error) {
	ef, err := openMMappingElfFile(path)
	if err != nil {
		return nil, err
	}
	return &ef, nil
}

func openMMappingElfFile(path string) (MMappingElfFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return MMappingElfFile{}, err
	}
	return makeMMappingElfFile(f)
}

// Close closes the underlying file descriptor.
func (m *MMappingElfFile) Close() error {
	return errors.Join(m.f.Close(), m.InMemoryElfFile.Close())
}

// SectionData loads a section from the object file.
//
// It will return an error if the section is compressed.
func (m *MMappingElfFile) SectionData(s *safeelf.SectionHeader) (SectionData, error) {
	// Delegate to SectionDataRange to fail hard on compressed sections rather
	// than delegating to the underlying elf file that will do decompression to
	// RAM transparently. Generally we don't want to accidentally decompress
	// when using the MMappingElfFile.
	return m.SectionDataRange(s, 0, s.Size)
}

// SectionDataRange implements SectionLoader.
func (m *MMappingElfFile) SectionDataRange(
	s *safeelf.SectionHeader, offset, length uint64,
) (SectionData, error) {
	if s.Flags&safeelf.SHF_COMPRESSED != 0 {
		return nil, errors.New("mmapping compressed sections is not supported")
	}
	if offset+length > s.Size {
		return nil, fmt.Errorf("out of section range: %d+%d > %d", offset, length, s.Size)
	}
	offset += s.Offset
	return m.mmap(offset, length)
}

// CompressedSectionError is returned when attempting to load a range of a
// compressed section or when trying to load a compressed section from a
// SectionLoader that does not support decompression.
type CompressedSectionError safeelf.SectionHeader

func (e *CompressedSectionError) Error() string {
	return fmt.Sprintf("section %s (0x%x) is compressed", e.Name, e.Addr)
}

// mmappedData is a portion of a file that has been mmapped into memory.
// Call Close() to release resources.
type mmappedData struct {
	data    []byte
	mmaped  []byte
	cleanup runtime.Cleanup
}

// Data returns the data of the mmapped section.
//
// The returned data is valid until the MMappedData is closed; users must take
// care to retain a reference to the MMappedData in order to call Close.
func (m *mmappedData) Data() []byte {
	return m.data
}

// makeMMappingElfFile creates a new MMappingElfFile for the given file.
//
// Note that this passes ownership of the file to the returned MMappingElfFile:
// calling Close on the MMappingElfFile will close the file.
func makeMMappingElfFile(f *os.File) (MMappingElfFile, error) {
	inMemory, err := makeInMemoryElfFile(f)
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return MMappingElfFile{}, fmt.Errorf("%w: (failed to close file: %w)", err, closeErr)
		}
		return MMappingElfFile{}, err
	}
	return MMappingElfFile{
		InMemoryElfFile: inMemory,
		f:               f,
	}, nil
}

func (m *MMappingElfFile) mmap(offset uint64, size uint64) (*mmappedData, error) {
	// The offset must be page-aligned for mmap to work
	pageSize := uint64(syscall.Getpagesize())
	alignedOffset := (offset / pageSize) * pageSize
	offsetDelta := offset - alignedOffset
	mmaped, err := syscall.Mmap(
		int(m.f.Fd()),
		int64(alignedOffset),
		int(size+offsetDelta),
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap section: %w", err)
	}
	return newMMappedData(mmaped[offsetDelta:], mmaped), nil
}

func newMMappedData(data, mmaped []byte) *mmappedData {
	md := &mmappedData{
		data:   data,
		mmaped: mmaped,
	}
	md.cleanup = runtime.AddCleanup(md, munmapCleanup, md.mmaped)
	return md
}

func munmapCleanup(m []byte) {
	_ = syscall.Munmap(m) // ignore errors
}

// Close unmaps the section from memory.
func (m *mmappedData) Close() error {
	if m.mmaped == nil {
		return nil
	}
	m.cleanup.Stop()
	runtime.KeepAlive(m) // out of an abundance of caution
	err := syscall.Munmap(m.mmaped)
	m.mmaped = nil
	return err
}
