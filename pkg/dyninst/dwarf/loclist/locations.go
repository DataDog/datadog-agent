// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package loclist

import (
	"debug/dwarf"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// ProcessLocations processes a location field from a DWARF entry, parsing
// location lists.
//
// unit is the DWARF compile unit entry that contains the location field.
// blockRanges are the PC ranges of the lexical block containing the variable.
// typeSize is the size, in bytes, of the type of the variable.
func ProcessLocations(
	locField *dwarf.Field,
	unit *dwarf.Entry,
	reader *Reader,
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
			return nil, errors.New("unexpected default location pieces")
		}
		locations = loclist.Locations

	case dwarf.ClassExprLoc:
		instr, ok := locField.Val.([]byte)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		pieces, err := ParseInstructions(instr, pointerSize, typeSize)
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
