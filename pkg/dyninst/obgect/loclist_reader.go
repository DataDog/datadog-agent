// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"debug/dwarf"
	"fmt"

	"github.com/go-delve/delve/pkg/dwarf/loclist"
)

// LoclistReader is a reader that can be used to read loclist entries.
// The reader is not safe for concurrent use.
type LoclistReader struct {
	reader2           *loclist.Dwarf2Reader
	maxOffset         int
	unitVersionGetter func(unit *dwarf.Entry) (uint8, bool)

	valid       bool
	baseAddress uint64
}

// Next advances the reader by populating the provided entry.
// Returns true if successful, false if not.
func (r *LoclistReader) Next(e *loclist.Entry) (ret bool) {
	if !r.valid {
		return false
	}
	// The logic underneath the reader is not particularly panic safe
	// in the face of invalid input, so we recover here to avoid and
	// just bail on iteration if a panic occurs.
	defer func() {
		if r := recover(); r != nil {
			ret = false
		}
		if !ret {
			r.valid = false
		}
	}()
	for r.reader2.Next(e) {
		if e.BaseAddressSelection() {
			r.baseAddress = e.HighPC
			continue
		}
		e.LowPC += r.baseAddress
		e.HighPC += r.baseAddress
		return true
	}
	return false
}

// Seek seeks to the given offset in the given unit.
func (r *LoclistReader) Seek(unit *dwarf.Entry, offset int64) error {
	unitVersion, ok := r.unitVersionGetter(unit)
	if !ok {
		return fmt.Errorf(
			"no unit version found for unit at offset 0x%x", unit.Offset,
		)
	}

	// TODO: Support Dwarf 5 loclists. The code in the delve library is
	// there, but it doesn't export the needed iterator. See [0].
	//
	// [0]: https://github.com/go-delve/delve/blob/0b7bffc7/pkg/dwarf/loclist/dwarf5_loclist.go
	if unitVersion < 2 || unitVersion > 4 {
		return fmt.Errorf(
			"unit version %d is not supported for unit at offset 0x%x",
			unitVersion, unit.Offset,
		)
	}
	off := int(offset)
	if off > r.maxOffset {
		return fmt.Errorf(
			"offset out of bounds for loclist (%d > %d)",
			off, r.maxOffset,
		)
	}

	r.reader2.Seek(off)
	r.valid = true
	return nil
}
