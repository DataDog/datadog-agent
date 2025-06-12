// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"debug/dwarf"
	"iter"
)

// DebugSections is a struct that contains the data for each debugging
// data section.
type DebugSections struct {
	// The .debug_abbrev section.
	Abbrev []byte `elf:"abbrev"`
	// The .debug_addr section.
	Addr []byte `elf:"addr"`
	// The .debug_aranges section.
	Aranges []byte `elf:"aranges"`
	// The .debug_info section.
	Info []byte `elf:"info"`
	// The .debug_line section.
	Line []byte `elf:"line"`
	// The .debug_line_str section.
	LineStr []byte `elf:"line_str"`
	// The .debug_str section.
	Str []byte `elf:"str"`
	// The .debug_str_offsets section.
	StrOffsets []byte `elf:"str_offsets"`
	// The .debug_types section.
	Types []byte `elf:"types"`
	// The .debug_loc section.
	Loc []byte `elf:"loc"`
	// The .debug_loclists section.
	LocLists []byte `elf:"loclists"`
	// The .debug_ranges section.
	Ranges []byte `elf:"ranges"`
	// The .debug_rnglists section.
	RngLists []byte `elf:"rnglists"`

	// NB: We're intentionally not loading the .debug_frame or .eh_frame
	// sections.
}

var sections = []struct {
	name   string
	getter func(*DebugSections) *[]byte
}{
	{"abbrev", func(d *DebugSections) *[]byte { return &d.Abbrev }},
	{"addr", func(d *DebugSections) *[]byte { return &d.Addr }},
	{"aranges", func(d *DebugSections) *[]byte { return &d.Aranges }},
	{"info", func(d *DebugSections) *[]byte { return &d.Info }},
	{"line", func(d *DebugSections) *[]byte { return &d.Line }},
	{"line_str", func(d *DebugSections) *[]byte { return &d.LineStr }},
	{"str", func(d *DebugSections) *[]byte { return &d.Str }},
	{"str_offsets", func(d *DebugSections) *[]byte { return &d.StrOffsets }},
	{"types", func(d *DebugSections) *[]byte { return &d.Types }},
	{"loc", func(d *DebugSections) *[]byte { return &d.Loc }},
	{"loclists", func(d *DebugSections) *[]byte { return &d.LocLists }},
	{"ranges", func(d *DebugSections) *[]byte { return &d.Ranges }},
	{"rnglists", func(d *DebugSections) *[]byte { return &d.RngLists }},
}

func (ds *DebugSections) getSection(name string) *[]byte {
	for _, s := range sections {
		if s.name == name {
			return s.getter(ds)
		}
	}
	return nil
}

// Sections returns an iterator over all the debug sections with their names and
// contents. Note that this is not the sections in the file, but rather all the
// sections that DebugSections supports.
func (ds *DebugSections) Sections() iter.Seq2[string, []byte] {
	return func(yield func(string, []byte) bool) {
		for _, s := range sections {
			if !yield(s.name, *s.getter(ds)) {
				return
			}
		}
	}
}

func (ds *DebugSections) loadDwarfData() (*dwarf.Data, error) {
	d, err := dwarf.New(
		ds.Abbrev,
		ds.Aranges,
		nil, // frame
		ds.Info,
		ds.Line,
		nil, // pubnames
		ds.Ranges,
		ds.Str,
	)
	if err != nil {
		return nil, err
	}
	for _, s := range []struct {
		name     string
		contents []byte
	}{
		{name: ".debug_addr", contents: ds.Addr},
		{name: ".debug_line_str", contents: ds.LineStr},
		{name: ".debug_str_offsets", contents: ds.StrOffsets},
		{name: ".debug_rnglists", contents: ds.RngLists},
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
