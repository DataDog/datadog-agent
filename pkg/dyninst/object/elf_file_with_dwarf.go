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

	dlvdwarf "github.com/go-delve/delve/pkg/dwarf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
)

// InMemoryLoader is an elf file loader that stores decompressed section
// data in memory.
type InMemoryLoader struct {
	_ struct{} // prevent instantiation
}

// NewInMemoryLoader creates an InMemoryLoader that will load elf files and
// DWARF data from disk and use anonymous memory mappings for the compressed
// section data. Importantly these sections are not part of the Go heap, and
// thus do not contribute to the heap size that the Go runtime tracks to guide
// garbage collection, but they are in RAM and will count towards the RSS used
// by the various OOM killers.
func NewInMemoryLoader() *InMemoryLoader {
	return &InMemoryLoader{}
}

// Load loads an elf file from the given path.
func (l *InMemoryLoader) Load(path string) (FileWithDwarf, error) {
	f, err := OpenElfFileWithDwarf(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ElfFileWithDwarf is a struct that contains the data for an ELF file.
//
// This object is safe for concurrent use.
type ElfFileWithDwarf struct {
	decompressingMMappingElfFile
	dwarfData
}

var _ FileWithDwarf = (*ElfFileWithDwarf)(nil)

// OpenElfFileWithDwarf opens an elf file from a path, using anonymous memory
// mappings for the compressed sections.
func OpenElfFileWithDwarf(path string) (*ElfFileWithDwarf, error) {
	mmf, err := openMMappingElfFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load elf file: %w", err)
	}
	return newElfFileWithDwarf(mmf, anonymousMmapCompressedSectionLoader{})
}

func newElfFileWithDwarf(
	mmf MMappingElfFile,
	loader compressedSectionLoader,
) (*ElfFileWithDwarf, error) {
	ret := &ElfFileWithDwarf{
		decompressingMMappingElfFile: decompressingMMappingElfFile{
			MMappingElfFile: mmf,
			sectionLoader:   loader,
		},
	}
	if err := ret.dwarfData.init(&ret.decompressingMMappingElfFile); err != nil {
		return nil, fmt.Errorf("failed to load dwarf data: %w", err)
	}
	return ret, nil
}

// InMemoryElfFileWithDwarf is an ElfFileWithDwarf that stores all the
// decompressed section data in memory on the heap and reads other data from
// the elf file on demand.
type InMemoryElfFileWithDwarf struct {
	*InMemoryElfFile
	dwarfData
}

// NewInMemoryElfFileWithDwarf creates a new InMemoryElfFileWithDwarf from the
// given InMemoryElfFile.
//
// Generally this should be used for testing purposes only as it is not using
// optimized decompression or disk caching.
func NewInMemoryElfFileWithDwarf(f *InMemoryElfFile) (*InMemoryElfFileWithDwarf, error) {
	ret := &InMemoryElfFileWithDwarf{
		InMemoryElfFile: f,
	}
	if err := ret.dwarfData.init(f); err != nil {
		return nil, fmt.Errorf("failed to load dwarf data: %w", err)
	}
	return ret, nil
}

// Close implements File.
//
// Note that various items returned from methods on the ElfFile are not usable
// after the ElfFile is closed.
func (e *ElfFileWithDwarf) Close() error {
	for _, s := range e.debugSections.sections() {
		if s != nil {
			s.Close()
		}
	}
	return e.decompressingMMappingElfFile.Close()
}

type dwarfData struct {
	debugSections DebugSections
	reader        loclist.Reader

	// By making the dwarfData part of the ElfFileWithDwarf struct and
	// transitively also making the dwarf.Data part of the dwarfData struct, we
	// can ensure that the ElfFileWithDwarf is not finalized while the
	// dwarf.Data is in use.
	dwarfData dwarf.Data

	unitHeaders []dwarfutil.CompileUnitHeader
}

var _ Dwarf = (*dwarfData)(nil)

// DwarfData implements Dwarf.
//
// Note that the returned dwarf.Data is not usable after the ElfFile is closed.
func (d *dwarfData) DwarfData() *dwarf.Data {
	return &d.dwarfData
}

// DebugSections implements File.
//
// Note that the returned DebugSections are not usable after the ElfFile is
// closed.
func (d *dwarfData) DebugSections() *DebugSections {
	return &d.debugSections
}

// LoclistReader implements File.
//
// Note that the returned loclist.Reader is not usable after the ElfFile is
// closed.
func (d *dwarfData) LoclistReader() *loclist.Reader {
	return &d.reader
}

func (d *dwarfData) UnitHeaders() []dwarfutil.CompileUnitHeader {
	return d.unitHeaders
}

// IsStrippedBinaryError returns true if the error is due to a stripped binary.
func IsStrippedBinaryError(err error) bool {
	var decodeErr dwarf.DecodeError
	return errors.As(err, &decodeErr)
}

func (d *dwarfData) init(f File) (retErr error) {
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

	var err error
	d.debugSections, err = loadDebugSections(f)
	if err != nil {
		return err
	}
	info := d.debugSections.Info()
	if info == nil {
		return errors.New("no .debug_info section found")
	}
	dwarfData, err := d.debugSections.loadDwarfData()
	if err != nil {
		return err
	}
	d.dwarfData = *dwarfData
	_, _, _, byteOrder := dlvdwarf.ReadDwarfLengthVersion(info)
	if byteOrder != binary.LittleEndian {
		return fmt.Errorf("unexpected DWARF byte order: %v", byteOrder)
	}
	d.unitHeaders = dwarfutil.ReadCompileUnitHeaders(d.debugSections.Info())
	d.reader = loclist.MakeReader(
		d.debugSections.Loc(),
		d.debugSections.LocLists(),
		d.debugSections.Addr(),
		uint8(f.Architecture().PointerSize()),
		d.unitHeaders,
	)
	return nil
}
