// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package gotype

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

// ReadTypeLinks reads the runtime's root types, including Go 1.27 packed descriptors.
func ReadTypeLinks(obj object.File) (TypeLinks, error) {
	if section := obj.Section(".typelink"); section != nil {
		data, err := obj.SectionData(section)
		if err != nil {
			return TypeLinks{}, err
		}
		defer data.Close()
		return ParseTypeLinks(data.Data()), nil
	}
	md, err := object.ParseModuleData(obj)
	if err != nil {
		return TypeLinks{}, err
	}
	table, err := NewTable(obj)
	if err != nil {
		return TypeLinks{}, err
	}
	defer table.Close()
	return table.packedTypeLinks(md.TypeDescLen)
}

func (tb *Table) packedTypeLinks(length uint64) (TypeLinks, error) {
	if length < 8 || length > uint64(len(tb.data)) || length > math.MaxUint32 {
		return TypeLinks{}, fmt.Errorf("invalid packed type descriptor length: %d", length)
	}
	var ids []TypeID
	// Match runtime.typelinks: skip the initial pointer, then align each descriptor.
	for offset := uint64(8); offset < length; {
		offset = (offset + 7) &^ 7
		if offset >= length {
			return TypeLinks{}, fmt.Errorf("truncated packed type descriptor at %#x", offset)
		}
		size, err := tb.descriptorSize(TypeID(offset), length-offset)
		if err != nil {
			return TypeLinks{}, err
		}
		ids = append(ids, TypeID(offset))
		offset += size
	}
	return TypeLinks{ids: ids}, nil
}

// descriptorSize matches internal/abi.Type.DescriptorSize, bounded to the prefix.
func (tb *Table) descriptorSize(id TypeID, remaining uint64) (uint64, error) {
	t, err := tb.ParseGoType(id)
	if err != nil {
		return 0, err
	}
	kind := t.Kind()
	if kind < reflect.Bool || kind > reflect.UnsafePointer {
		return 0, fmt.Errorf("invalid packed type kind %d at %#x", kind, id)
	}
	size := uint64(tb.kindSize(kind, tb.mapType()))
	var count, width uint64
	switch kind {
	case reflect.Func:
		header := int(id) + tb._type.size
		count = uint64(binary.LittleEndian.Uint16(tb.data[header+tb._func.inCountOff:])) +
			uint64(binary.LittleEndian.Uint16(tb.data[header+tb._func.outCountOff:])&0x7fff)
		width = 8
	case reflect.Struct:
		count = binary.LittleEndian.Uint64(tb.data[int(id)+tb._type.size+tb._struct.fieldsOff+8:])
		width = uint64(tb.structField.size)
	case reflect.Interface:
		count = binary.LittleEndian.Uint64(tb.data[int(id)+tb._type.size+tb._interface.methodsOff+8:])
		width = uint64(tb.imethod.size)
	}
	if uncommon, ok := t.uncommonType(); ok {
		size += uint64(tb.uncommon.size)
		size += uint64(uncommon.mcount) * uint64(tb.method.size)
	}
	if size > remaining || (width != 0 && count > (remaining-size)/width) {
		return 0, fmt.Errorf("packed type descriptor out of bounds at %#x", id)
	}
	return size + count*width, nil
}
