// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"encoding/binary"
	"fmt"
)

// UncommonType is present for types with extra information (such as methods).
type UncommonType struct {
	pkgPath uint32
	mcount  uint16
	xcount  uint16
	moff    uint32
}

func (t *Type) uncommonOffset() (int, bool) {
	if (t.tflag() & tflagUncommon) == 0 {
		return 0, false
	}
	return int(t.id) + t.tbl.kindSize(t.Kind(), t.tbl.mapType()), true
}

func (t *Type) uncommonType() (UncommonType, bool) {
	off, ok := t.uncommonOffset()
	if !ok {
		return UncommonType{}, false
	}
	utl := &t.tbl.uncommon
	uncommon := UncommonType{}
	uncommon.pkgPath = binary.LittleEndian.Uint32(t.tbl.data[off+utl.pkgPathOff:])
	uncommon.mcount = binary.LittleEndian.Uint16(t.tbl.data[off+utl.mcountOff:])
	uncommon.xcount = binary.LittleEndian.Uint16(t.tbl.data[off+utl.xcountOff:])
	uncommon.moff = binary.LittleEndian.Uint32(t.tbl.data[off+utl.moffOff:])
	return uncommon, true
}

// Method is a method stored in UncommonType.
type Method struct {
	Name NameOff
	Mtyp TypeID
}

func parseMethod(tbl *Table, off int) (Method, error) {
	ml := &tbl.abiLayout.method
	if off < 0 || off+ml.size > len(tbl.data) {
		return Method{}, fmt.Errorf("parseMethod: offset %d out of bounds", off)
	}
	nameOff := binary.LittleEndian.Uint32(tbl.data[off+ml.methodNameOff:])
	mtyp := binary.LittleEndian.Uint32(tbl.data[off+ml.methodMtypOff:])
	return Method{Name: NameOff(nameOff), Mtyp: TypeID(mtyp)}, nil
}

// Methods returns the UncommonType methods if present. It appends to buf.
func (t *Type) Methods(buf []Method) ([]Method, error) {
	off, ok := t.uncommonOffset()
	if !ok {
		return nil, nil
	}
	ut, ok := t.uncommonType()
	if !ok || ut.mcount == 0 {
		return nil, nil
	}
	if len(buf) == 0 && cap(buf) < int(ut.mcount) {
		buf = make([]Method, 0, ut.mcount)
	}
	base := off + int(ut.moff)
	for i := 0; i < int(ut.mcount); i++ {
		moff := base + i*t.tbl.abiLayout.method.size
		m, err := parseMethod(t.tbl, moff)
		if err != nil {
			name := t.Name().Name()
			return nil, fmt.Errorf("parse method of %s at %d: %w", name, moff, err)
		}
		buf = append(buf, m)
	}
	return buf, nil
}
