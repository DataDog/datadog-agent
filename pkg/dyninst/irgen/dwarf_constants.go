// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

// Go-specific type attributes.
//
//nolint:unused
const (
	dwAtGoKind = 0x2900
	dwAtGoKey  = 0x2901
	dwAtGoElem = 0x2902
	// Attribute for DW_TAG_member of a struct type.
	// Nonzero value indicates the struct field is an embedded field.
	dwAtGoEmbeddedField = 0x2903
	dwAtGoRuntimeType   = 0x2904

	dwAtGoPackageName   = 0x2905 // Attribute for DW_TAG_compile_unit
	dwAtGoDictIndex     = 0x2906 // Attribute for DW_TAG_typedef_type, index of the dictionary entry describing the real type of this type shape
	dwAtGoClosureOffset = 0x2907 // Attribute for DW_TAG_variable, offset in the closure struct where this captured variable resides
)

const (
	// See Section 7.12, Table 7.17 of the DWARF v4 spec.
	dwLangGo = 0x16
)

//nolint:unused
const (
	/// See Section 7.16, Table 7.20.
	dwInlNotInlined         = 0x00
	dwInlInlined            = 0x01
	dwInlDeclaredNotInlined = 0x02
	dwInlDeclaredInlined    = 0x03
)

//nolint:unused
const (
	dwAteAddress      = 0x01
	dwAteBoolean      = 0x02
	dwAteComplexFloat = 0x03
	dwAteFloat        = 0x04
	dwAteSigned       = 0x05
	dwAteSignedChar   = 0x06
	dwAteUnsigned     = 0x07
	dwAteUnsignedChar = 0x08

	// DWARF 3.
	dwAteImaginaryFloat = 0x09
	dwAtePackedDecimal  = 0x0a
	dwAteNumericString  = 0x0b
	dwAteEdited         = 0x0c
	dwAteSignedFixed    = 0x0d
	dwAteUnsignedFixed  = 0x0e
	dwAteDecimalFloat   = 0x0f

	// DWARF 4.
	dwAteUTF   = 0x10
	dwAteUCS   = 0x11
	dwAteASCII = 0x12

	dwAteLoUser = 0x80
	dwAteHiUser = 0xff
)
