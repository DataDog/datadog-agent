// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"reflect"
)

const (
	// tflag extra star indicates the name has an extra leading '*', which must
	// be stripped. Matches runtime tflag bits.
	tflagExtraStar = 1 << 1
	tflagUncommon  = 1 << 0
	kindMask       = (1 << 5) - 1
)

// TODO: Validate the layout by checking for internal consistency of the
// abi types.
//
// Unfortunately, it's not trivial to recompute this table from a binary without
// dwarf. You might think that something that could be done would be to have a
// hard-coded bootstrapping layout of just the basic type field and the struct
// type and their offsets and then use the type table to validate the internal
// consistency of the abi types. However, this turns out not to always work
// because the the go compiler can eliminate the core ABI type information
// because they don't make their way into interfaces.
//
// Another option that'd be sufficient for our purposes would be to just
// validate the sanity of the hard-coded layout for the binary we're looking at.
// There are almost definitely some types we know will appear due to the runtime
// and we could ensure that we find them and that internally when we use the
// acessors we get the correct information. If anything turns out to be bogus,
// we can just say that this is a future version of Go for which the type table
// is not supported.

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L11-L45
type typeLayout struct {
	size int

	sizeOff      int // u64
	tflagOff     int // u8
	kindOff      int // u8
	strOff       int // u32
	ptrToThisOff int // u32
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L268-L274
type arrayTypeLayout struct {
	extraSize int

	elemPtrOff int // *Type
	lenOff     int // uintptr
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L297-L302
type chanTypeLayout struct {
	extraSize int

	elemPtrOff int // *Type
	dirOff     int // ChanDir (int)
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L484-L499
type funcTypeLayout struct {
	extraSize int

	inCountOff  int // u16
	outCountOff int // u16
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/map.go#L32-L43
type swissMapTypeLayout struct {
	extraSize int

	keyPtrOff    int // *Type
	elemPtrOff   int // *Type
	bucketPtrOff int // *Type
}

// See https://github.com/golang/go/blob/f7d167fe/src/internal/abi/map_noswiss.go#L10-L11
type hMapTypeLayout struct {
	extraSize int

	keyPtrOff    int // *Type
	elemPtrOff   int // *Type
	bucketPtrOff int // *Type
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L543-L545
type ptrTypeLayout struct {
	extraSize int

	elemPtrOff int // *Type
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L479-L481
type sliceTypeLayout struct {
	extraSize int

	elemPtrOff int // *Type
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L558-L562
type structTypeLayout struct {
	extraSize int

	pkgPathOff int // Name (*Bytes)
	fieldsOff  int // []StructField
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L447-L451
type interfaceTypeLayout struct {
	extraSize int

	pkgPathOff int // Name (*Bytes)
	methodsOff int // []IMethod
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L225-L233
type uncommonTypeLayout struct {
	size int

	pkgPathOff int // NameOff
	mcountOff  int // u16
	xcountOff  int // u16
	moffOff    int // u32
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L217
type methodLayout struct {
	size int

	methodNameOff int // u32
	methodMtypOff int // u32
	methodIFnOff  int // u32
	methodTFnOff  int // u32
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L262-L266
type imethodLayout struct {
	size int

	nameOff int // u32
	mtypOff int // u32
}

// See https://github.com/golang/go/blob/bd885401/src/internal/abi/type.go#L548-L552
type structFieldLayout struct {
	size int

	structFieldNameOff    int // Name (*Bytes)
	structFieldTypePtrOff int // *Type
	structFieldOffsetOff  int // uintptr
}

// Layout configuration for parsing Go type metadata from the types blob.
//
// Note that it's an incomplete list of fields.
type abiLayout struct {
	_type      typeLayout
	array      arrayTypeLayout
	_chan      chanTypeLayout
	_func      funcTypeLayout
	swissMap   swissMapTypeLayout
	hMap       hMapTypeLayout
	_ptr       ptrTypeLayout
	slice      sliceTypeLayout
	_struct    structTypeLayout
	_interface interfaceTypeLayout

	uncommon    uncommonTypeLayout
	method      methodLayout
	imethod     imethodLayout
	structField structFieldLayout
}

var hardCodedLayout = abiLayout{
	_type: typeLayout{
		size: 48,

		sizeOff:      0,
		tflagOff:     20,
		kindOff:      23,
		strOff:       40,
		ptrToThisOff: 44,
	},
	array: arrayTypeLayout{
		extraSize: 24,

		elemPtrOff: 0,
		lenOff:     16,
	},
	_chan: chanTypeLayout{
		extraSize: 16,

		elemPtrOff: 0,
		dirOff:     8,
	},
	_func: funcTypeLayout{
		extraSize: 8,

		inCountOff:  0,
		outCountOff: 2,
	},
	swissMap: swissMapTypeLayout{
		extraSize: 64,

		keyPtrOff:    0,
		elemPtrOff:   8,
		bucketPtrOff: 16,
	},
	hMap: hMapTypeLayout{
		extraSize: 40,

		keyPtrOff:    0,
		elemPtrOff:   8,
		bucketPtrOff: 16,
	},
	_ptr: ptrTypeLayout{
		extraSize: 8,

		elemPtrOff: 0,
	},
	slice: sliceTypeLayout{
		extraSize: 8,

		elemPtrOff: 0,
	},
	_struct: structTypeLayout{
		extraSize: 32,

		pkgPathOff: 0,
		fieldsOff:  8,
	},
	_interface: interfaceTypeLayout{
		extraSize: 32,

		pkgPathOff: 0,
		methodsOff: 8,
	},
	uncommon: uncommonTypeLayout{
		size: 16,

		pkgPathOff: 0,
		mcountOff:  4,
		xcountOff:  6,
		moffOff:    8,
	},
	method: methodLayout{
		size: 16,

		methodNameOff: 0,
		methodMtypOff: 4,
		methodIFnOff:  8,
		methodTFnOff:  12,
	},
	imethod: imethodLayout{
		size: 8,

		nameOff: 0,
		mtypOff: 4,
	},
	structField: structFieldLayout{
		size: 24,

		structFieldNameOff:    0,
		structFieldTypePtrOff: 8,
		structFieldOffsetOff:  16,
	},
}

func validateBaseTypeData(tb *Table, typeBase int) bool {
	tl := &tb._type
	lb, ub := typeBase, typeBase+tl.size
	if lb < 0 || ub < 0 || ub > len(tb.data) {
		return false
	}
	return true
}

func (l *abiLayout) kindSize(kind reflect.Kind, mapType mapType) int {
	size := l._type.size
	switch kind {
	case reflect.Array:
		size += l.array.extraSize
	case reflect.Chan:
		size += l._chan.extraSize
	case reflect.Func:
		size += l._func.extraSize
	case reflect.Pointer:
		size += l._ptr.extraSize
	case reflect.Slice:
		size += l.slice.extraSize
	case reflect.Map:
		switch mapType {
		case mapTypeSwiss:
			size += l.swissMap.extraSize
		case mapTypeHmap:
			size += l.hMap.extraSize
		}
	case reflect.Struct:
		size += l._struct.extraSize
	case reflect.Interface:
		size += l._interface.extraSize
	}
	return size
}
