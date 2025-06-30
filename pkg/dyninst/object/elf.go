// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	dlvdwarf "github.com/go-delve/delve/pkg/dwarf"

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
	// The underlying elf file.
	*safeelf.File

	dwarfSections *DebugSections
	dwarfData     *dwarf.Data
	architecture  bininspect.GoArch

	// Maps the compile unit entry offset to the version of the DWARF spec
	// that was used to parse it.
	unitVersions map[dwarf.Offset]uint8
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
func (e *ElfFile) LoclistReader() (*loclist.Reader, error) {
	// Loclists replace Loc in DWARF 5. Here we do not need to recognize the version,
	// just pick the section that exists.
	var data []byte
	if e.dwarfSections.LocLists != nil {
		data = e.dwarfSections.LocLists
	} else if e.dwarfSections.Loc != nil {
		data = e.dwarfSections.Loc
	} else {
		return nil, fmt.Errorf("no loc/loclist section found")
	}

	return loclist.NewReader(data, e.dwarfSections.Addr, uint8(e.architecture.PointerSize()), func(unit *dwarf.Entry) (uint8, bool) {
		unitVersion, ok := e.unitVersions[unit.Offset]
		return unitVersion, ok
	}), nil
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
func NewElfObject(elfFile *safeelf.File) (_ *ElfFile, retErr error) {
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
	ds, err := loadDebugSections(elfFile)
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
	_, _, dwarfVersion, byteOrder := dlvdwarf.ReadDwarfLengthVersion(ds.Info)
	if byteOrder != binary.LittleEndian {
		return nil, fmt.Errorf("unexpected DWARF byte order: %v", byteOrder)
	}
	unitVersions := dlvdwarf.ReadUnitVersions(ds.Info)
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
	return &ElfFile{
		File:          elfFile,
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

func loadDebugSections(f *safeelf.File) (*DebugSections, error) {
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
	for _, s := range f.Sections {
		suffix := dwarfSuffix(s)
		if suffix == "" {
			continue
		}
		sd, ok := ds.getSection(suffix)
		if !ok {
			continue
		}
		if *sd != nil {
			return nil, fmt.Errorf("section %s already loaded", s.Name)
		}

		// TODO: Figure out whether it is important that we aren't applying
		// relocations here. The elf code that loads dwarf in `f.DWARF()` does
		// apply relocations, so it's possible that we're not getting the same
		// results as the elf code. See [0] for the code we're not executing
		// like the stdlib when loading this data.
		//
		// 0: https://github.com/golang/go/blob/db55b83c/src/debug/elf/file.go#L1351-L1377
		data, err := s.Data()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get section data for %s: %w", s.Name, err,
			)
		}
		*sd = data
	}
	return &ds, nil
}
