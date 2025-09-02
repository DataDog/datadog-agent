// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package dwarfutil provides utilities for working with DWARF debug info.
package dwarfutil

import (
	"debug/dwarf"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// PCRange corresponds to a range of program counters (PCs). Ranges are
// inclusive of the start and exclusive of the end.
type PCRange = [2]uint64

// ProcessLocations processes a location field from a DWARF entry, parsing
// location lists.
//
// unit is the DWARF compile unit entry that contains the location field.
// blockRanges are the PC ranges of the lexical block containing the variable.
// typeSize is the size, in bytes, of the type of the variable.
func ProcessLocations(
	locField *dwarf.Field,
	unit *dwarf.Entry,
	reader *loclist.Reader,
	blockRanges []ir.PCRange,
	typeSize uint32,
	pointerSize uint8,
) ([]ir.Location, error) {
	var locations []ir.Location
	switch locField.Class {
	case dwarf.ClassLocListPtr:
		offset, ok := locField.Val.(int64)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		loclist, err := reader.Read(unit, offset, typeSize)
		if err != nil {
			return nil, err
		}
		if len(loclist.Default) > 0 {
			return nil, fmt.Errorf("unexpected default location pieces")
		}
		locations = loclist.Locations

	case dwarf.ClassExprLoc:
		instr, ok := locField.Val.([]byte)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		pieces, err := loclist.ParseInstructions(instr, pointerSize, typeSize)
		if err != nil {
			return nil, err
		}
		for _, r := range blockRanges {
			locations = append(locations, ir.Location{
				Range:  r,
				Pieces: pieces,
			})
		}
	default:
		return nil, fmt.Errorf(
			"unexpected %s class: %s",
			locField.Attr, locField.Class,
		)
	}

	return locations, nil
}

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
