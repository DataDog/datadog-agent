// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"strings"
	"unsafe"
)

// NameOff is a 32-bit offset into the string table.
type NameOff uint32

// Name represents a name in the type table.
//
// See https://github.com/golang/go/blob/98238fd4/src/internal/abi/type.go#L590
type Name struct {
	bytes *byte
	cap   uint32
}

// TypeName represents a name of a type.
//
// It is just like Name but with an extra flag indicating whether the name
// has an extra star (a flag that's set in the type header).
type TypeName struct {
	innerName
	ExtraStar bool
}

type innerName struct {
	Name
}

func removeExtraStar(n string) string {
	if len(n) > 0 && n[0] == '*' {
		return n[1:]
	}
	return n
}

// Name returns the name of the type.
func (tn TypeName) Name() string {
	return strings.Clone(tn.UnsafeName())
}

// UnsafeName is like Name() but does not copy the name from the table
// and does not check whether the name is valid utf-8.
func (tn TypeName) UnsafeName() string {
	n := tn.innerName.Name.UnsafeName()
	if tn.ExtraStar {
		n = removeExtraStar(n)
	}
	return n
}

// IsExported returns "is n exported?"
func (n Name) IsExported() bool { return checkBit(n, 0) }

// HasTag returns true iff there is tag data following this name
func (n Name) HasTag() bool { return checkBit(n, 1) }

// IsEmbedded returns true iff n is embedded (an anonymous field).
func (n Name) IsEmbedded() bool { return checkBit(n, 3) }

func checkBit(n Name, bit uint) bool {
	xp, ok := n.data(0)
	return ok && (*xp)&(1<<bit) != 0
}

// Name returns the name of the type.
func (n Name) Name() string {
	return strings.Clone(n.UnsafeName())
}

// UnsafeName is like Name() but does not copy the name from the table
// and does not check whether the name is valid utf-8.
func (n Name) UnsafeName() string {
	if n.bytes == nil {
		return ""
	}
	off, nameLen, ok := n.readVarint(1)
	if !ok || nameLen < 0 {
		return ""
	}
	dataOff := 1 + off
	bound := dataOff + nameLen
	if bound < 0 || bound >= int(n.cap) {
		return ""
	}
	xp, ok := n.data(dataOff)
	if !ok {
		return ""
	}
	return string(unsafe.String(xp, nameLen))
}

// Tag returns the tag of the name if it has one.
func (n Name) Tag() string {
	if !n.HasTag() {
		return ""
	}
	nameOff, nameLen, ok := n.readVarint(1)
	if !ok || nameLen < 0 {
		return ""
	}
	tagOff, tagLen, ok := n.readVarint(1 + nameOff + nameLen)
	if !ok || tagLen < 0 {
		return ""
	}
	dataOff := 1 + nameOff + nameLen + tagOff
	bound := dataOff + tagLen
	if bound < 0 || bound >= int(n.cap) {
		return ""
	}
	xp, ok := n.data(dataOff)
	if !ok {
		return ""
	}
	return string(unsafe.Slice(xp, tagLen))
}

// ReadVarint parses a varint as encoded by encoding/binary.
// It returns the number of encoded bytes and the encoded value.
func (n Name) readVarint(off int) (consumed int, value int, ok bool) {
	v := 0
	for i := 0; ; i++ {
		xp, ok := n.data(off + i)
		if !ok {
			return 0, 0, false
		}
		x := *xp
		v += int(x&0x7f) << (7 * i)
		if x&0x80 == 0 || i == 5 {
			return i + 1, v, true
		}
	}
}

// IsBlank indicates whether n is "_".
func (n Name) IsBlank() bool {
	if n.bytes == nil {
		return false
	}
	_, l, ok := n.readVarint(1)
	if !ok {
		return false
	}
	xp, ok := n.data(2)
	return ok && l == 1 && *xp == '_'
}

// IsEmpty indicates whether n is empty.
func (n Name) IsEmpty() bool {
	if n.bytes == nil {
		return true
	}
	_, nameLen, ok := n.readVarint(1)
	return ok && nameLen == 0
}

func (n Name) data(off int) (*byte, bool) {
	if n.bytes == nil || off < 0 || off >= int(n.cap) {
		return nil, false
	}
	addr := unsafe.Pointer(uintptr(unsafe.Pointer(n.bytes)) + uintptr(off))
	return (*byte)(addr), true
}

// Resolve resolves the string at the given offset.
func (s NameOff) Resolve(tb *Table) Name {
	// There needs to be at least 2 bytes: 1 for the flags and the min varint.
	dataLen := uint32(len(tb.data))
	if s == 0 || s+2 < s /* overflow */ || uint32(s+2) > dataLen {
		return Name{}
	}
	c := dataLen - uint32(s)
	return Name{bytes: &tb.data[s], cap: c}
}
