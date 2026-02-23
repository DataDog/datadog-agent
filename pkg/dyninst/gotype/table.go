// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// TypeID is a 32-bit offset into the Go types blob.
type TypeID = uint32

// Table contains the information about the Go types blob.
type Table struct {
	abiLayout
	mapTypeContainer
	data        []byte
	dataAddress uint64

	closer io.Closer
}

// NewTable constructs a Table from an ELF file.
func NewTable(obj object.File) (*Table, error) {
	moduleData, err := object.ParseModuleData(obj)
	if err != nil {
		return nil, err
	}

	sections := obj.SectionHeaders()
	idx := slices.IndexFunc(sections, func(s *safeelf.SectionHeader) bool {
		return s.Addr <= moduleData.Types && moduleData.Types < s.Addr+s.Size
	})
	if idx == -1 {
		return nil, errors.New("section containing types not found")
	}
	const rodataName = ".rodata"
	rodata := sections[idx]
	if rodata.Name != rodataName {
		return nil, fmt.Errorf(
			"section containing types is not %q, got %q",
			rodataName, rodata.Name,
		)
	}
	var roMap object.SectionData
	if moduleData.Types < rodata.Addr {
		return nil, fmt.Errorf(
			"moduledata.Types offset is less than rodata address: %#x < %#x",
			moduleData.Types, rodata.Addr,
		)
	}
	if moduleData.ETypes < moduleData.Types {
		// Note that this is not really ever going to happen unless we really
		// messed up the parsing of the module data, but pedantically let's
		// be safe, because we're trusting external executable data.
		return nil, fmt.Errorf(
			"moduledata.ETypes offset is less than moduledata.Types: %#x < %#x",
			moduleData.ETypes, moduleData.Types,
		)
	}
	typesOffset := moduleData.Types - rodata.Addr
	typesLen := moduleData.ETypes - moduleData.Types
	if typesOffset+typesLen > rodata.Size {
		return nil, fmt.Errorf(
			"types offset + types length is greater than rodata size: %#x + %#x > %#x",
			typesOffset, typesLen, rodata.Size,
		)
	}
	roMap, err = obj.SectionDataRange(rodata, typesOffset, typesLen)
	if err != nil {
		return nil, fmt.Errorf("error mapping .rodata section: %w", err)
	}

	roData := roMap.Data()
	return &Table{
		abiLayout:   hardCodedLayout,
		data:        roData,
		dataAddress: moduleData.Types,
		closer:      roMap,
	}, nil
}

// DataByteSize returns the size of the data in bytes.
func (tb *Table) DataByteSize() uint64 {
	return uint64(len(tb.data))
}

// Close closes the table.
func (tb *Table) Close() error {
	return tb.closer.Close()
}

// ParseGoType parses a Go type at offset within the table.
func (tb *Table) ParseGoType(id TypeID) (Type, error) {
	offset := int(id)
	if !validateBaseTypeData(tb, offset) {
		return Type{}, fmt.Errorf(
			"type %#[1]x out of bounds: offset %[2]d (%#[2]x), data size %[3]d",
			id, offset, len(tb.data),
		)
	}
	t := Type{tbl: tb, id: id}
	kind := t.Kind()
	if kind == reflect.Map {
		if _, err := tb.getOrSetMapType(id); err != nil {
			return Type{}, err
		}
	}
	// Sanity check that the type is within the bounds of the data so we don't
	// need to bounds check in the accessors and also don't have to worry about
	// panics.
	{
		infoSize := tb.kindSize(kind, tb.mapType())
		if t.tflag()&tflagUncommon != 0 {
			infoSize += tb.uncommon.size
		}
		if offset+int(infoSize) > len(tb.data) {
			return Type{}, fmt.Errorf(
				"type %#[1]x out of bounds: size %[2]d at offset %[3]d (%#[3]x), data size %[4]d",
				id, infoSize, offset, len(tb.data),
			)
		}
	}
	return t, nil
}

// TypeLinks contains the type IDs of the types in the types blob.
type TypeLinks struct {
	ids []TypeID
}

// TypeIDs returns an iterator over the type IDs.
func (tl *TypeLinks) TypeIDs() iter.Seq[TypeID] {
	return slices.Values(tl.ids)
}

// ParseTypeLinks decodes a .typelink section into a slice of TypeID values.
// The typelink section is a stream of 4-byte little-endian offsets into the types blob.
func ParseTypeLinks(typelink []byte) TypeLinks {
	if len(typelink) < 4 {
		return TypeLinks{}
	}
	ids := make([]TypeID, 0, len(typelink)/4)
	for i := 0; i+4 <= len(typelink); i += 4 {
		ids = append(ids, TypeID(binary.LittleEndian.Uint32(typelink[i:])))
	}
	return TypeLinks{ids: ids}
}
