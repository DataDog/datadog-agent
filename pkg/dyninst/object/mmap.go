// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// MMappingElfFile is an ElfFile that supports mmapping sections into memory.
type MMappingElfFile struct {
	Elf *safeelf.File
	f   *os.File
}

// MMappedData is a portion of a file that has been mmapped into memory.
// Call Close() to release resources.
type MMappedData struct {
	Data   []byte
	mmaped []byte
}

// OpenMMappingElfFile creates a new MMappingElfFile for the given path.
func OpenMMappingElfFile(path string) (*MMappingElfFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	ef, err := newMMappingElfFile(f)
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w: (failed to close file: %w)", err, closeErr)
		}
		return nil, err
	}
	return ef, nil
}

// newMMappingElfFile creates a new MMappingElfFile for the given file.
//
// Note that this passes ownership of the file to the returned MMappingElfFile:
// calling Close on the MMappingElfFile will close the file.
func newMMappingElfFile(f *os.File) (*MMappingElfFile, error) {
	elfFile, err := safeelf.NewFile(f)
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w: (failed to close file: %w)", err, closeErr)
		}
		return nil, err
	}
	return &MMappingElfFile{
		Elf: elfFile,
		f:   f,
	}, nil
}

// Close closes the underlying file descriptor.
func (m *MMappingElfFile) Close() error {
	return m.f.Close()
}

// MMap mmaps a portion of the file into memory.
func (m *MMappingElfFile) MMap(
	section *safeelf.Section, offset, size uint64,
) (*MMappedData, error) {
	if section.Flags&safeelf.SHF_COMPRESSED != 0 {
		return nil, fmt.Errorf("mmapping compressed sections is not supported")
	}
	if offset+size > section.Size {
		return nil, fmt.Errorf("out of section range: %d+%d > %d", offset, size, section.Size)
	}
	offset += section.Offset

	return m.mmap(offset, size)
}

func (m *MMappingElfFile) mmap(offset uint64, size uint64) (*MMappedData, error) {
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
	md := &MMappedData{
		Data:   mmaped[offsetDelta:],
		mmaped: mmaped,
	}
	runtime.SetFinalizer(md, (*MMappedData).Close)
	return md, nil
}

// Close unmaps the section from memory.
func (m *MMappedData) Close() error {
	if m.mmaped == nil {
		return nil
	}
	err := syscall.Munmap(m.mmaped)
	m.mmaped = nil
	return err
}
