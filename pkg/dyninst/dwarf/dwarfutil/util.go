// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package dwarfutil provides utilities for working with DWARF debug info.
package dwarfutil

import (
	"debug/dwarf"
	"encoding/binary"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// PCRange corresponds to a range of program counters (PCs). Ranges are
// inclusive of the start and exclusive of the end.
type PCRange = [2]uint64

// FilterIncompleteLocationLists filters out location lists that have missing pieces.
func FilterIncompleteLocationLists(lists []ir.Location) []ir.Location {
	var filtered []ir.Location
	for _, loc := range lists {
		if len(loc.Pieces) == 0 {
			continue // Ignore empty location lists.
		}
		allPiecesPresent := true
		for _, piece := range loc.Pieces {
			if piece.Op == nil {
				allPiecesPresent = false
				break
			}
		}
		if allPiecesPresent {
			filtered = append(filtered, loc)
		}
	}
	return filtered
}

// ExploreInlinedPcRangesInSubprogram goes through the children of a subprogram
// and collects the PC ranges of all inlined subroutines found inside it. The
// ranges are non-overlapping and sorted.
//
// Exported for testing.
func ExploreInlinedPcRangesInSubprogram(reader *dwarf.Reader, data *dwarf.Data) ([]PCRange, error) {
	var inlinedRanges []PCRange
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return nil, err
		}
		if IsEntryNull(child) {
			break // End of children for this subprogram.
		}
		// Inlined subroutines are direct children of the subprogram, so we only
		// need to look one level deep.
		reader.SkipChildren()

		if child.Tag != dwarf.TagInlinedSubroutine {
			// We only care about inlined subroutines here.
			continue
		}

		ranges, err := data.Ranges(child)
		if err != nil {
			return nil, err
		}
		inlinedRanges = append(inlinedRanges, ranges...)
	}
	sort.Slice(inlinedRanges, func(i, j int) bool {
		return inlinedRanges[i][0] < inlinedRanges[j][0]
	})
	return inlinedRanges, nil
}

// IsEntryNull determines whether a *dwarf.Entry value is a "null" DIE, which
// are used to denote the end of sub-trees. See DWARF v4 spec, section 2.3.
func IsEntryNull(entry *dwarf.Entry) bool {
	// Every field is 0 in an empty entry.
	return !entry.Children &&
		len(entry.Field) == 0 &&
		entry.Offset == 0 &&
		entry.Tag == dwarf.Tag(0)
}

// CompileUnitHeader represents the header of a compile unit in the DWARF
// debug_info section.
type CompileUnitHeader struct {
	// The offset of the compile unit DIE, which comes after the header.
	Offset  dwarf.Offset
	Version uint8
	// Length of the compile unit, not including the header.
	Length uint64
}

// ReadCompileUnitHeaders reads the headers of compile units from a debug_info
// section.
//
// Inspired from https://github.com/go-delve/delve/blob/fa07b65188ae1293113e41fa9e44f47c061fb509/pkg/dwarf/parseutil.go#L109
func ReadCompileUnitHeaders(data []byte) []CompileUnitHeader {
	var headers []CompileUnitHeader
	off := dwarf.Offset(0)
	for len(data) > 0 {
		length, dwarf64, version, _ := readCUHeaderInfo(data)

		data = data[4:]
		off += 4
		secoffsz := uint64(4)
		if dwarf64 {
			off += 8
			secoffsz = 8
			data = data[8:]
		}

		var headerSize uint64

		switch version {
		case 2, 3, 4:
			headerSize = 3 + secoffsz
		default: // 5 and later?
			unitType := data[2]

			switch unitType {
			case DW_UT_compile, DW_UT_partial:
				headerSize = 4 + secoffsz

			case DW_UT_skeleton, DW_UT_split_compile:
				headerSize = 4 + secoffsz + 8

			case DW_UT_type, DW_UT_split_type:
				headerSize = 4 + secoffsz + 8 + secoffsz
			}
		}

		headers = append(headers, CompileUnitHeader{
			Offset:  off + dwarf.Offset(headerSize),
			Version: version,
			Length:  length - headerSize,
		})

		data = data[length:] // skip contents
		off += dwarf.Offset(length)
	}
	return headers
}

// readCUHeaderInfo reads a some fields from a compile unit header.
//
// Inspired from https://github.com/go-delve/delve/blob/fa07b65188ae1293113e41fa9e44f47c061fb509/pkg/dwarf/parseutil.go#L58
func readCUHeaderInfo(data []byte) (length uint64, dwarf64 bool, version uint8, byteOrder binary.ByteOrder) {
	if len(data) < 4 {
		return 0, false, 0, binary.LittleEndian
	}

	lengthfield := binary.LittleEndian.Uint32(data)
	voff := 4
	if lengthfield == ^uint32(0) {
		dwarf64 = true
		voff = 12
	}

	if voff+1 >= len(data) {
		return 0, false, 0, binary.LittleEndian
	}

	byteOrder = binary.LittleEndian
	x, y := data[voff], data[voff+1]
	switch {
	default:
		fallthrough
	case x == 0 && y == 0:
		version = 0
		byteOrder = binary.LittleEndian
	case x == 0:
		version = y
		byteOrder = binary.BigEndian
	case y == 0:
		version = x
		byteOrder = binary.LittleEndian
	}

	if dwarf64 {
		length = byteOrder.Uint64(data[4:])
	} else {
		length = uint64(byteOrder.Uint32(data))
	}

	return length, dwarf64, version, byteOrder
}

const (
	DW_UT_compile       = 0x1 + iota //nolint:revive
	DW_UT_type                       //nolint:revive
	DW_UT_partial                    //nolint:revive
	DW_UT_skeleton                   //nolint:revive
	DW_UT_split_compile              //nolint:revive
	DW_UT_split_type                 //nolint:revive
)
