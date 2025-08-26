// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package object abstracts the loading of debugging sections from an object
// file (such as an ELF file).
package object

import (
	"debug/dwarf"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// Architecture is the architecture of the object file.
type Architecture = bininspect.GoArch

// File is an interface that represents an object file.
type File interface {
	io.Closer
	SectionLoader

	// Architecture returns the architecture of the object file.
	Architecture() Architecture

	// Symbols returns the symbols of the object file.
	Symbols() ([]safeelf.Symbol, error)
}

// Dwarf is an interface that represents an object file with DWARF information.
type Dwarf interface {
	// DebugSections returns the DWARF sections.
	DebugSections() *DebugSections
	// DwarfData returns the DWARF data.
	DwarfData() *dwarf.Data
	// LoclistReader returns a reader that can be used to read
	// loclist entries. The reader is not safe for concurrent use.
	LoclistReader() *loclist.Reader
}

// FileWithDwarf is an interface that represents an object file with DWARF
// information.
type FileWithDwarf interface {
	File
	Dwarf
}

// SectionLoader is an interface that represents an object file that can load
// sections from the object file.
type SectionLoader interface {
	// Section returns the section header for the given name.
	Section(name string) *safeelf.SectionHeader
	// SectionHeaders returns the section headers of the object file.
	SectionHeaders() []*safeelf.SectionHeader
	// SectionData loads portions of a section from the object file.
	//
	// Note that if the section was compressed, the returned data will be
	// decompressed.
	SectionData(section *safeelf.SectionHeader) (SectionData, error)
	// SectionDataRange loads a range of a section from the object file.
	//
	// Note that if the section was compressed, this method will return an
	// error.
	SectionDataRange(section *safeelf.SectionHeader, offset, length uint64) (SectionData, error)
}
