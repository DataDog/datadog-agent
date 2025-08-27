// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"fmt"
	"io"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// InMemoryElfFile is a thin wrapper around a safeelf.File that implements
// the File interface. It delegates to the elf package for section reading
// and decompression.
type InMemoryElfFile struct {
	*safeelf.File
	sectionHeaders []*safeelf.SectionHeader
	architecture   Architecture
}

var _ File = (*InMemoryElfFile)(nil)

// NewInMemoryElfFile creates a new InMemoryElfFile from f. Note that the
// responsibility for closing continues to lie with the caller even if f
// implements io.Closer.
func NewInMemoryElfFile(f io.ReaderAt) (*InMemoryElfFile, error) {
	ef, err := makeInMemoryElfFile(f)
	if err != nil {
		return nil, err
	}
	return &ef, nil
}

func makeInMemoryElfFile(f io.ReaderAt) (InMemoryElfFile, error) {
	elfFile, err := safeelf.NewFile(f)
	if err != nil {
		return InMemoryElfFile{}, err
	}
	arch, err := bininspect.GetArchitecture(elfFile)
	if err != nil {
		return InMemoryElfFile{}, fmt.Errorf("failed to get architecture: %w", err)
	}
	sectionHeaders := make([]*safeelf.SectionHeader, len(elfFile.Sections))
	for i, s := range elfFile.Sections {
		sectionHeaders[i] = &s.SectionHeader
	}
	return InMemoryElfFile{
		File:           elfFile,
		sectionHeaders: sectionHeaders,
		architecture:   arch,
	}, nil
}

// Architecture returns the architecture of the object file.
func (e *InMemoryElfFile) Architecture() Architecture {
	return e.architecture
}

// Section returns the section header for the given name.
func (e *InMemoryElfFile) Section(name string) *safeelf.SectionHeader {
	section := e.File.Section(name)
	if section == nil {
		return nil
	}
	return &section.SectionHeader
}

// SectionHeaders returns the section headers of the object file.
func (e *InMemoryElfFile) SectionHeaders() []*safeelf.SectionHeader {
	return e.sectionHeaders
}

// SectionData loads a section from the object file.
//
// It will decompress the section if it is compressed.
func (e *InMemoryElfFile) SectionData(
	sh *safeelf.SectionHeader,
) (SectionData, error) {
	section, err := sectionWithHeader(e.File, sh)
	if err != nil {
		return nil, err
	}
	data, err := section.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read section data: %w", err)
	}
	return inMemorySectionData(data), nil
}

// SectionDataRange loads a range of a section from the object file.
//
// It will return an error if the section is compressed.
func (e *InMemoryElfFile) SectionDataRange(
	sh *safeelf.SectionHeader, offset, length uint64,
) (SectionData, error) {
	section, err := sectionWithHeader(e.File, sh)
	if err != nil {
		return nil, err
	}
	if sh.Flags&safeelf.SHF_COMPRESSED != 0 {
		return nil, (*CompressedSectionError)(sh)
	}
	if offset > sh.Size {
		return nil, fmt.Errorf("offset %d is greater than section size %d", offset, sh.Size)
	}
	if offset+length > sh.Size {
		return nil, fmt.Errorf("offset %d + length %d is greater than section size %d", offset, length, sh.Size)
	}
	sectionData, err := section.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read section data: %w", err)
	}
	if len(sectionData) != int(sh.Size) {
		return nil, fmt.Errorf("section data length %d does not match section size %d", len(sectionData), sh.Size)
	}
	// Don't retain the entire section data in memory if we're only loading a
	// small range.
	if length < uint64(len(sectionData)/4) {
		newBuf := make([]byte, length)
		copy(newBuf, sectionData[:length])
		sectionData = newBuf
	}
	return inMemorySectionData(sectionData), nil
}

func sectionWithHeader(e *safeelf.File, sh *safeelf.SectionHeader) (*safeelf.Section, error) {
	if idx := slices.IndexFunc(e.Sections, func(s *safeelf.Section) bool {
		return &s.SectionHeader == sh
	}); idx != -1 {
		return e.Sections[idx], nil
	}
	return nil, fmt.Errorf("section %s (0x%x) not found", sh.Name, sh.Addr)
}

type inMemorySectionData []byte

func (d inMemorySectionData) Close() error { return nil }
func (d inMemorySectionData) Data() []byte { return d }
