// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"debug/dwarf"
	"fmt"
	"iter"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// SectionData holds the data for an object file section.  The data may be
// backed by a memory map.
type SectionData interface {
	// Data returns the data for the section.
	//
	// Beware, the returned data may be a memory map, so it is not safe to
	// modify the returned data (that may result in a segfault) and it is not
	// safe to use once the memory map is closed (which may happen when the
	// SectionData is garbage collected or the object file is closed).
	Data() []byte
	// Close closes the memory map, if any.
	Close() error
}

// DebugSections is a struct that contains the data for each debugging
// data section.
type DebugSections struct {
	// The .debug_abbrev section.
	abbrev SectionData `elf:"abbrev"`
	// The .debug_addr section.
	addr SectionData `elf:"addr"`
	// The .debug_aranges section.
	aranges SectionData `elf:"aranges"`
	// The .debug_info section.
	info SectionData `elf:"info"`
	// The .debug_line section.
	line SectionData `elf:"line"`
	// The .debug_line_str section.
	lineStr SectionData `elf:"line_str"`
	// The .debug_str section.
	str SectionData `elf:"str"`
	// The .debug_str_offsets section.
	strOffsets SectionData `elf:"str_offsets"`
	// The .debug_types section.
	types SectionData `elf:"types"`
	// The .debug_loc section.
	loc SectionData `elf:"loc"`
	// The .debug_loclists section.
	locLists SectionData `elf:"loclists"`
	// The .debug_ranges section.
	ranges SectionData `elf:"ranges"`
	// The .debug_rnglists section.
	rngLists SectionData `elf:"rnglists"`

	// NB: We're intentionally not loading the .debug_frame or .eh_frame
	// sections.
}

func loadDebugSections(f File) (_ DebugSections, retErr error) {
	dwarfSuffix := func(s *safeelf.SectionHeader) string {
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
	for _, s := range f.SectionHeaders() {
		suffix := dwarfSuffix(s)
		if suffix == "" {
			continue
		}
		sd := ds.getSection(suffix)
		if sd == nil {
			continue
		}
		if *sd != nil {
			return DebugSections{}, fmt.Errorf("section %s already loaded", s.Name)
		}

		data, err := f.SectionData(s)
		if err != nil {
			return DebugSections{}, fmt.Errorf("failed to load section data: %w", err)
		}
		*sd = data
	}
	return ds, nil
}

func getData(m SectionData) []byte {
	if m == nil {
		return nil
	}
	return m.Data()
}

// Abbrev returns the .debug_abbrev section.
func (ds *DebugSections) Abbrev() []byte { return getData(ds.abbrev) }

// Addr returns the .debug_addr section.
func (ds *DebugSections) Addr() []byte { return getData(ds.addr) }

// Aranges returns the .debug_aranges section.
func (ds *DebugSections) Aranges() []byte { return getData(ds.aranges) }

// Info returns the .debug_info section.
func (ds *DebugSections) Info() []byte { return getData(ds.info) }

// Line returns the .debug_line section.
func (ds *DebugSections) Line() []byte { return getData(ds.line) }

// LineStr returns the .debug_line_str section.
func (ds *DebugSections) LineStr() []byte { return getData(ds.lineStr) }

// Str returns the .debug_str section.
func (ds *DebugSections) Str() []byte { return getData(ds.str) }

// StrOffsets returns the .debug_str_offsets section.
func (ds *DebugSections) StrOffsets() []byte { return getData(ds.strOffsets) }

// Types returns the .debug_types section.
func (ds *DebugSections) Types() []byte { return getData(ds.types) }

// Loc returns the .debug_loc section.
func (ds *DebugSections) Loc() []byte { return getData(ds.loc) }

// LocLists returns the .debug_loclists section.
func (ds *DebugSections) LocLists() []byte { return getData(ds.locLists) }

// Ranges returns the .debug_ranges section.
func (ds *DebugSections) Ranges() []byte { return getData(ds.ranges) }

// RngLists returns the .debug_rnglists section.
func (ds *DebugSections) RngLists() []byte { return getData(ds.rngLists) }

var sections = []struct {
	name   string
	getter func(*DebugSections) *SectionData
}{
	{"abbrev", func(d *DebugSections) *SectionData { return &d.abbrev }},
	{"addr", func(d *DebugSections) *SectionData { return &d.addr }},
	{"aranges", func(d *DebugSections) *SectionData { return &d.aranges }},
	{"info", func(d *DebugSections) *SectionData { return &d.info }},
	{"line", func(d *DebugSections) *SectionData { return &d.line }},
	{"line_str", func(d *DebugSections) *SectionData { return &d.lineStr }},
	{"str", func(d *DebugSections) *SectionData { return &d.str }},
	{"str_offsets", func(d *DebugSections) *SectionData { return &d.strOffsets }},
	{"types", func(d *DebugSections) *SectionData { return &d.types }},
	{"loc", func(d *DebugSections) *SectionData { return &d.loc }},
	{"loclists", func(d *DebugSections) *SectionData { return &d.locLists }},
	{"ranges", func(d *DebugSections) *SectionData { return &d.ranges }},
	{"rnglists", func(d *DebugSections) *SectionData { return &d.rngLists }},
}

func (ds *DebugSections) getSection(name string) *SectionData {
	for _, s := range sections {
		if s.name == name {
			return s.getter(ds)
		}
	}
	return nil
}

func (ds *DebugSections) sections() iter.Seq2[string, SectionData] {
	return func(yield func(string, SectionData) bool) {
		for _, s := range sections {
			if !yield(s.name, *s.getter(ds)) {
				return
			}
		}
	}
}

// Sections returns an iterator over all the debug sections with their names and
// contents. Note that this is not the sections in the file, but rather all the
// sections that DebugSections supports.
func (ds *DebugSections) Sections() iter.Seq2[string, []byte] {
	return func(yield func(string, []byte) bool) {
		for name, mmap := range ds.sections() {
			if !yield(name, getData(mmap)) {
				return
			}
		}
	}
}

func (ds *DebugSections) loadDwarfData() (*dwarf.Data, error) {
	d, err := dwarf.New(
		ds.Abbrev(),
		ds.Aranges(),
		nil, // frame
		ds.Info(),
		ds.Line(),
		nil, // pubnames
		ds.Ranges(),
		ds.Str(),
	)
	if err != nil {
		return nil, err
	}
	for _, s := range []struct {
		name     string
		contents []byte
	}{
		{name: ".debug_addr", contents: ds.Addr()},
		{name: ".debug_line_str", contents: ds.LineStr()},
		{name: ".debug_str_offsets", contents: ds.StrOffsets()},
		{name: ".debug_rnglists", contents: ds.RngLists()},
	} {
		if len(s.contents) == 0 {
			continue
		}
		if err := d.AddSection(s.name, s.contents); err != nil {
			return nil, err
		}
	}
	return d, nil
}
