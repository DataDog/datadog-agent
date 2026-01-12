// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"encoding/binary"
	"fmt"
	"reflect"
)

// Type represents a Go type decoded from the types blob.
type Type struct {
	tbl *Table
	id  TypeID
}

// ID returns the type ID.
func (t *Type) ID() TypeID { return t.id }

// Name decodes the type name.
func (t *Type) Name() TypeName {
	strOff := int(t.id) + t.tbl._type.strOff
	nameOff := u32le(t.tbl.data[strOff:])
	n := NameOff(nameOff).Resolve(t.tbl)
	if n.IsEmpty() {
		return TypeName{}
	}
	tn := TypeName{ExtraStar: (t.tflag() & tflagExtraStar) != 0}
	tn.innerName.Name = n
	return tn
}

// Size returns the ABI size for this type.
func (t *Type) Size() uint64 {
	off := int(t.id) + t.tbl._type.sizeOff
	return u64le(t.tbl.data[off:])
}

// Kind returns the reflect kind byte.
func (t *Type) Kind() reflect.Kind {
	off := int(t.id) + t.tbl._type.kindOff
	return reflect.Kind(t.tbl.data[off] & kindMask)
}

// tflag returns the tflag byte.
func (t *Type) tflag() byte {
	off := int(t.id) + t.tbl._type.tflagOff
	return t.tbl.data[off]
}

// PtrToThis returns the pointer-to-this TypeID if present.
func (t *Type) PtrToThis() (TypeID, bool) {
	off := int(t.id) + t.tbl._type.ptrToThisOff
	ptrToThis := u32le(t.tbl.data[off:])
	return TypeID(ptrToThis), ptrToThis != 0
}

// PkgPathOff returns the package path offset if present.
func (t *Type) PkgPathOff() (r NameOff) {
	if ut, ok := t.uncommonType(); ok && ut.pkgPath != 0 && ut.pkgPath != ^uint32(0) {
		return NameOff(ut.pkgPath)
	}
	switch reflect.Kind(t.Kind() & kindMask) {
	case reflect.Struct:
		pkgPtrOff := int(t.id) + t.tbl._type.size +
			t.tbl._struct.pkgPathOff
		pkgPtr := u64le(t.tbl.data[pkgPtrOff:])
		if pkgPtr != 0 && t.tbl.dataAddress < pkgPtr {
			return NameOff(pkgPtr - t.tbl.dataAddress)
		}
	case reflect.Interface:
		pkgPathPtrOff := int(t.id) + t.tbl._type.size +
			t.tbl._interface.pkgPathOff
		pkgPathPtr := u64le(t.tbl.data[pkgPathPtrOff:])
		if pkgPathPtr != 0 && t.tbl.dataAddress < pkgPathPtr {
			return NameOff(pkgPathPtr - t.tbl.dataAddress)
		}
	}
	return 0
}

// PkgPath returns the package path.
func (t *Type) PkgPath() Name {
	return t.PkgPathOff().Resolve(t.tbl)
}

// AppendReferences will append the references to the passed slice and return it.
func (t *Type) AppendReferences(buf []TypeID) []TypeID {
	if t == nil {
		return buf
	}
	switch reflect.Kind(t.Kind()) {
	case reflect.Array:
		if a, ok := t.Array(); ok {
			if _, elem, err := a.LenElem(); err == nil {
				buf = append(buf, elem)
			}
		}
	case reflect.Chan:
		if c, ok := t.Chan(); ok {
			if _, elem, err := c.DirElem(); err == nil {
				buf = append(buf, elem)
			}
		}
	case reflect.Func:
		if f, ok := t.Func(); ok {
			if in, out, _, err := f.Signature(); err == nil {
				buf = append(buf, in...)
				buf = append(buf, out...)
			}
		}
	case reflect.Map:
		if m, ok := t.Map(); ok {
			if k, v, err := m.KeyValue(); err == nil {
				buf = append(buf, k, v)
			}
		}
	case reflect.Pointer:
		if p, ok := t.Pointer(); ok {
			buf = append(buf, p.Elem())
		}
	case reflect.Slice:
		if s, ok := t.Slice(); ok {
			buf = append(buf, s.Elem())
		}
	case reflect.Struct:
		if s, ok := t.Struct(); ok {
			if fs, err := s.Fields(); err == nil {
				for _, f := range fs {
					buf = append(buf, f.Typ)
				}
			}
		}
	case reflect.Interface:
		if i, ok := t.Interface(); ok {
			if ms, err := i.Methods(nil); err == nil {
				for _, m := range ms {
					buf = append(buf, m.Typ)
				}
			}
		}
	}
	if ptr, ok := t.PtrToThis(); ok && ptr != 0 {
		buf = append(buf, ptr)
	}
	if ms, err := t.Methods(nil); err == nil {
		for _, m := range ms {
			// For eliminated methods, the mtype field is -1.
			if m.Mtyp != 0 && m.Mtyp != ^TypeID(0) {
				buf = append(buf, m.Mtyp)
			}
		}
	}
	return buf
}

type (
	// Struct provides access to struct specific type information.
	Struct struct{ t *Type }
	// Pointer provides access to pointer specific type information.
	Pointer struct{ t *Type }
	// Slice provides access to slice specific type information.
	Slice struct{ t *Type }
	// Map provides access to map specific type information.
	Map struct{ t *Type }
	// Func provides access to function specific type information.
	Func struct{ t *Type }
	// Chan provides access to channel specific type information.
	Chan struct{ t *Type }
	// Array provides access to array specific type information.
	Array struct{ t *Type }
	// Interface provides access to interface specific type information.
	Interface struct{ t *Type }
)

// Struct returns the Struct details if the type is a struct.
func (t *Type) Struct() (Struct, bool) {
	if t.Kind() != reflect.Struct {
		return Struct{}, false
	}
	return Struct{t: t}, true
}

// Pointer returns the Pointer details if the type is a pointer.
func (t *Type) Pointer() (Pointer, bool) {
	if t.Kind() != reflect.Pointer {
		return Pointer{}, false
	}
	return Pointer{t: t}, true
}

// Slice returns the Slice details if the type is a slice.
func (t *Type) Slice() (Slice, bool) {
	if t.Kind() != reflect.Slice {
		return Slice{}, false
	}
	return Slice{t: t}, true
}

// Map returns the Map details if the type is a map.
func (t *Type) Map() (Map, bool) {
	if t.Kind() != reflect.Map {
		return Map{}, false
	}
	return Map{t: t}, true
}

// Func returns the Func details if the type is a function.
func (t *Type) Func() (Func, bool) {
	if t.Kind() != reflect.Func {
		return Func{}, false
	}
	return Func{t: t}, true
}

// Chan returns the Chan details if the type is a channel.
func (t *Type) Chan() (Chan, bool) {
	if t.Kind() != reflect.Chan {
		return Chan{}, false
	}
	return Chan{t: t}, true
}

// Array returns the Array details if the type is an array.
func (t *Type) Array() (Array, bool) {
	if t.Kind() != reflect.Array {
		return Array{}, false
	}
	return Array{t: t}, true
}

// Interface returns the Interface details if the type is an interface.
func (t *Type) Interface() (Interface, bool) {
	if t.Kind() != reflect.Interface {
		return Interface{}, false
	}
	return Interface{t: t}, true
}

// Fields returns the fields of the struct.
func (s Struct) Fields() ([]StructField, error) {
	tbl := s.t.tbl
	sl := &tbl._struct
	sfl := &tbl.structField
	extra := int(s.t.id) + tbl._type.size
	fieldsSliceOff := extra + sl.fieldsOff
	if fieldsSliceOff+24 > len(s.t.tbl.data) {
		return nil, fmt.Errorf(
			"struct fields slice header out of bounds at %#x (dataAddress %#x)",
			fieldsSliceOff, s.t.tbl.dataAddress,
		)
	}
	fieldsPtr := u64le(tbl.data[fieldsSliceOff:])
	if fieldsPtr < tbl.dataAddress {
		return nil, fmt.Errorf(
			"struct fields ptr %#x below dataAddress %#x",
			fieldsPtr, s.t.tbl.dataAddress,
		)
	}
	fieldCount := u64le(s.t.tbl.data[fieldsSliceOff+8:])
	recordsOff := int(fieldsPtr - s.t.tbl.dataAddress)
	if fieldCount == 0 {
		return nil, nil
	}
	upperBound := recordsOff + int(fieldCount)*sfl.size
	if upperBound < 0 || upperBound > len(s.t.tbl.data) {
		return nil, fmt.Errorf(
			"struct fields data out of bounds at %d (dataAddress %#x)",
			upperBound, s.t.tbl.dataAddress,
		)
	}
	var fields []StructField
	for i := range int(fieldCount) {
		rec := recordsOff + i*sfl.size
		namePtr := u64le(tbl.data[rec+sfl.structFieldNameOff:])
		if namePtr < s.t.tbl.dataAddress {
			return nil, fmt.Errorf(
				"struct field name ptr %#x below dataAddress %#x",
				namePtr, s.t.tbl.dataAddress,
			)
		}
		name := NameOff(namePtr - s.t.tbl.dataAddress)
		typPtr := u64le(tbl.data[rec+sfl.structFieldTypePtrOff:])
		if typPtr < s.t.tbl.dataAddress {
			return nil, fmt.Errorf(
				"struct field type ptr %#x below dataAddress %#x",
				typPtr, s.t.tbl.dataAddress,
			)
		}
		foff := u64le(tbl.data[rec+sfl.structFieldOffsetOff:])
		fields = append(fields, StructField{
			Name:   name,
			Typ:    TypeID(uint32(typPtr - s.t.tbl.dataAddress)),
			Offset: foff,
		})
	}
	return fields, nil
}

// Elem returns the element type of the pointer.
func (p Pointer) Elem() TypeID {
	tbl := p.t.tbl
	pl := &tbl._ptr
	extra := int(p.t.id) + tbl._type.size
	elemPtr := u64le(tbl.data[extra+pl.elemPtrOff:])
	if elemPtr < p.t.tbl.dataAddress {
		panic(fmt.Sprintf("pointer elem ptr %#x below dataAddress %#x", elemPtr, p.t.tbl.dataAddress))
	}
	return TypeID(uint32(elemPtr - p.t.tbl.dataAddress))
}

// Elem returns the element type of the slice.
func (s Slice) Elem() TypeID {
	tbl := s.t.tbl
	sl := &tbl.slice
	extra := int(s.t.id) + tbl._type.size
	elemPtr := u64le(tbl.data[extra+sl.elemPtrOff:])
	if elemPtr < s.t.tbl.dataAddress {
		panic(fmt.Sprintf("slice elem ptr %#x below dataAddress %#x", elemPtr, s.t.tbl.dataAddress))
	}
	return TypeID(uint32(elemPtr - s.t.tbl.dataAddress))
}

// KeyValue returns the key and value types of the map.
func (m Map) KeyValue() (key, value TypeID, _ error) {
	tbl := m.t.tbl
	extra := int(m.t.id) + tbl._type.size
	mt, err := tbl.getOrSetMapType(m.t.id)
	if err != nil {
		return 0, 0, err
	}
	switch mt {
	case mapTypeHmap:
		hl := &tbl.hMap
		keyPtr := u64le(tbl.data[extra+hl.keyPtrOff:])
		valuePtr := u64le(tbl.data[extra+hl.elemPtrOff:])
		return TypeID(uint32(keyPtr - tbl.dataAddress)), TypeID(uint32(valuePtr - tbl.dataAddress)), nil
	case mapTypeSwiss:
		sl := &tbl.swissMap
		keyPtr := binary.LittleEndian.Uint64(tbl.data[extra+sl.keyPtrOff:])
		valuePtr := u64le(tbl.data[extra+sl.elemPtrOff:])
		return TypeID(uint32(keyPtr - tbl.dataAddress)), TypeID(uint32(valuePtr - tbl.dataAddress)), nil
	default:
		return 0, 0, fmt.Errorf("unknown map type: %v", mt)
	}
}

// Signature returns the signature of the function.
func (f Func) Signature() (in []TypeID, out []TypeID, variadic bool, err error) {
	tbl := f.t.tbl
	fl := &tbl._func
	header := int(f.t.id) + tbl._type.size
	inCountOff := header + fl.inCountOff
	inCount := binary.LittleEndian.Uint16(tbl.data[inCountOff:])
	outCountOff := inCountOff + 2
	outCount := binary.LittleEndian.Uint16(tbl.data[outCountOff:])
	variadic = (outCount & (1 << 15)) != 0
	if variadic {
		outCount &= (1 << 15) - 1
	}
	paramsOffset := header + fl.extraSize
	if _, ok := f.t.uncommonOffset(); ok {
		paramsOffset += tbl.uncommon.size
	}
	total := int(inCount) + int(outCount)
	if paramsOffset+total*8 > len(tbl.data) {
		return nil, nil, variadic, fmt.Errorf(
			"func params out of bounds at %d", paramsOffset,
		)
	}
	for i := 0; i < int(inCount); i++ {
		ptr := u64le(tbl.data[paramsOffset:])
		if ptr < tbl.dataAddress {
			return nil, nil, variadic, fmt.Errorf(
				"func in param ptr %#x below dataAddress %#x",
				ptr, tbl.dataAddress,
			)
		}
		in = append(in, TypeID(uint32(ptr-tbl.dataAddress)))
		paramsOffset += 8
	}
	for i := 0; i < int(outCount); i++ {
		ptr := u64le(tbl.data[paramsOffset:])
		if ptr < tbl.dataAddress {
			return nil, nil, variadic, fmt.Errorf(
				"func out param ptr %#x below dataAddress %#x",
				ptr, tbl.dataAddress,
			)
		}
		out = append(out, TypeID(uint32(ptr-tbl.dataAddress)))
		paramsOffset += 8
	}
	return in, out, variadic, nil
}

// ChanDir is the direction of a channel.
type ChanDir uint32

const (
	// ChanInvalid is an invalid channel direction.
	ChanInvalid ChanDir = iota
	// ChanRecv is a receive-only channel.
	ChanRecv ChanDir = 1
	// ChanSend is a send-only channel.
	ChanSend ChanDir = 2
	// ChanBoth is a bidirectional channel.
	ChanBoth = 3
)

// DirElem returns the direction and element type of the channel.
func (c Chan) DirElem() (ChanDir, TypeID, error) {
	tbl := c.t.tbl
	cl := &tbl._chan
	extra := int(c.t.id) + tbl._type.size
	elemPtr := u64le(tbl.data[extra+cl.elemPtrOff:])
	if elemPtr < tbl.dataAddress {
		return 0, 0, fmt.Errorf("chan elem ptr %#x below dataAddress %#x", elemPtr, tbl.dataAddress)
	}
	dirU32 := u32le(tbl.data[extra+cl.dirOff:])
	return ChanDir(dirU32), TypeID(uint32(elemPtr - tbl.dataAddress)), nil
}

// LenElem returns the length and element type of the array.
func (a Array) LenElem() (uint64, TypeID, error) {
	tbl := a.t.tbl
	al := &tbl.array
	extra := int(a.t.id) + tbl._type.size
	elemPtr := u64le(tbl.data[extra+al.elemPtrOff:])
	if elemPtr < tbl.dataAddress {
		return 0, 0, fmt.Errorf("array elem ptr %#x below dataAddress %#x", elemPtr, tbl.dataAddress)
	}
	length := u64le(tbl.data[extra+al.lenOff:])
	return length, TypeID(uint32(elemPtr - tbl.dataAddress)), nil
}

// Methods returns the methods of the interface.
func (intf Interface) Methods(buf []IMethod) ([]IMethod, error) {
	tbl := intf.t.tbl
	il := &tbl._interface
	iml := &tbl.imethod
	extra := int(intf.t.id) + tbl._type.size
	methodsSliceOff := extra + il.methodsOff
	methodsSliceDataPtr := u64le(tbl.data[methodsSliceOff:])
	if methodsSliceDataPtr < intf.t.tbl.dataAddress {
		return nil, fmt.Errorf(
			"interface methods slice data ptr %#x below dataAddress %#x",
			methodsSliceDataPtr, intf.t.tbl.dataAddress,
		)
	}
	methodsDataOff := int(methodsSliceDataPtr - tbl.dataAddress)
	methodsSliceLen := int(u64le(tbl.data[methodsSliceOff+8:]))
	if methodsSliceLen == 0 {
		return nil, nil
	}
	if methodsSliceLen < 0 {
		return nil, fmt.Errorf("methodsSliceLen %d is negative", methodsSliceLen)
	}

	upperBound := methodsDataOff + methodsSliceLen*iml.size
	if upperBound < 0 || upperBound > len(tbl.data) {
		return nil, fmt.Errorf(
			"methods data out of bounds at %d", upperBound,
		)
	}
	if len(buf) == 0 && cap(buf) < int(methodsSliceLen) {
		buf = make([]IMethod, 0, methodsSliceLen)
	}
	for i := 0; i < int(methodsSliceLen); i++ {
		off := methodsDataOff + i*iml.size
		nameOff := u32le(tbl.data[off+iml.nameOff:])
		typ := u32le(tbl.data[off+iml.mtypOff:])
		buf = append(buf, IMethod{Name: NameOff(nameOff), Typ: TypeID(typ)})
	}
	return buf, nil
}

// StructField is a field of a struct.
type StructField struct {
	Name   NameOff
	Typ    TypeID
	Offset uint64
}

// IMethod is a method of an interface.
type IMethod struct {
	Name NameOff
	Typ  TypeID
}

func u64le(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data)
}

func u32le(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data)
}
