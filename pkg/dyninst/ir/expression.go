// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

// ExprStatus is the per-expression evaluation status stored in the
// ExprStatusArray at the start of event root data. Each expression gets
// ExprStatusBits bits.
type ExprStatus uint8

const (
	ExprStatusAbsent   ExprStatus = 0 // evaluation failed (unknown reason)
	ExprStatusPresent  ExprStatus = 1 // evaluation succeeded
	ExprStatusNilDeref ExprStatus = 2 // nil pointer dereference
	ExprStatusOOB      ExprStatus = 3 // index out of bounds
)

// ExprStatusBits is the number of bits per entry in the ExprStatusArray.
// Currently 2; can be expanded to 4 for alignment if more statuses are needed.
const ExprStatusBits = 2

// Expression is a typed and validated set of operations for compilation
// and evaluation.
type Expression struct {
	// The type of the expression.
	Type Type
	// The operations that make up the expression, in reverse-polish notation.
	Operations []ExpressionOp
}

var (
	_ ExpressionOp = (*LocationOp)(nil)
	_ ExpressionOp = (*DereferenceOp)(nil)
)

// ExpressionOp is an operation that can be performed on an expression.
type ExpressionOp interface {
	irOp() // marker
}

// LocationOp references data of Size bytes
// at Offset in Variable.
type LocationOp struct {
	// The variable that is referenced.
	Variable *Variable

	// The offset in bytes from the start of the variable to extract.
	Offset uint32

	// The size of the data to extract in bytes.
	ByteSize uint32
}

func (*LocationOp) irOp() {}

// DereferenceOp dereferences a pointer and extracts data at an offset.
type DereferenceOp struct {
	// Bias is the offset in bytes to apply to the dereferenced address.
	Bias uint32

	// ByteSize is the size in bytes to extract after dereferencing.
	ByteSize uint32
}

func (*DereferenceOp) irOp() {}

// ExprPushOffsetOp pushes the current scratch offset onto the data stack
// and advances the offset by ByteSize bytes.
type ExprPushOffsetOp struct {
	ByteSize uint32
}

func (*ExprPushOffsetOp) irOp() {}

// ExprLoadLiteralOp writes literal bytes from the compiled bytecode into
// scratch at the current offset.
type ExprLoadLiteralOp struct {
	Data []byte
}

func (*ExprLoadLiteralOp) irOp() {}

// ExprReadStringOp materializes a Go string from its header (ptr+len) already
// in scratch at the current offset. It pushes the offset onto the data stack,
// overwrites the header with [u32 len][bytes...], and advances the offset.
type ExprReadStringOp struct {
	MaxLen uint16
}

func (*ExprReadStringOp) irOp() {}

// ExprCmpEqBaseOp pops the LHS offset from the data stack, compares ByteSize
// bytes at LHS vs RHS (current offset), and writes a bool result (0 or 1) at
// the current offset.
type ExprCmpEqBaseOp struct {
	ByteSize uint8
}

func (*ExprCmpEqBaseOp) irOp() {}

// ExprCmpEqStringOp pops the LHS offset from the data stack and compares two
// length-prefixed strings ([u32 len][bytes...]). Writes a bool result at the
// current offset.
type ExprCmpEqStringOp struct{}

func (*ExprCmpEqStringOp) irOp() {}

// SliceBoundsCheckOp checks that a compile-time index is within the runtime
// length of a Go slice. It expects the scratch buffer at the current offset
// to contain the first 16 bytes of the slice header: [data_ptr (8), len (8)].
// The len field is at a fixed offset of 8 bytes (we only support 64-bit
// targets). If index >= len, it writes ExprStatusOOB and aborts the expression.
type SliceBoundsCheckOp struct {
	Index uint32 // compile-time index to validate against runtime len
}

func (*SliceBoundsCheckOp) irOp() {}

// SwissMapLookupOp performs a Go swiss table map key lookup. At runtime the
// scratch buffer at sm->offset contains the map header (already dereferenced).
// The op computes the hash of the compile-time literal key using the per-map
// seed and per-process hash secret, then probes the swiss table to find the
// matching slot. On success the value element is written at sm->offset. On
// failure (key not found or nil map) ExprStatusOOB is written and the
// expression is aborted.
type SwissMapLookupOp struct {
	// KeyData is the literal key encoded for comparison.
	// Base types: raw little-endian bytes (1–8 bytes).
	// Strings: [u32 len][bytes...] (max 4+MaxStringLiteralLength bytes).
	KeyData []byte

	// IsStringKey indicates the key is a Go string. When false the key is
	// a base type and compared by raw byte equality.
	IsStringKey bool

	// KeyByteSize is the in-memory size of the key type in bytes.
	// For base types: 1, 2, 4, or 8.
	// For strings: 16 (the Go string header: ptr + len).
	KeyByteSize uint8

	// ValByteSize is the in-memory size of the value element in bytes.
	ValByteSize uint32

	// Map header field offsets (from DWARF, vary by Go version).
	SeedOffset        uint8
	DirPtrOffset      uint8
	DirLenOffset      uint8
	GlobalShiftOffset uint8

	// Group layout.
	CtrlOffset      uint8
	SlotsOffset     uint8
	SlotSize        uint16 // size of one slot (key + elem with alignment)
	KeyInSlotOffset uint8  // offset of key within slot
	ValInSlotOffset uint16 // offset of elem within slot

	// Table struct → groupsReference field layout.
	TableGroupsFieldOffset   uint8
	GroupsDataFieldOffset    uint8
	GroupsLenMaskFieldOffset uint8

	// GroupByteSize is the total byte size of one group (ctrl word + all slots).
	GroupByteSize uint16

	// HeaderByteSize is the byte size of the map header struct (written by the
	// preceding DereferenceOp). Used to compute where key data starts in the
	// scratch buffer, replacing the implicit buf_offset_1 coupling.
	HeaderByteSize uint32
}

func (*SwissMapLookupOp) irOp() {}

// ConditionCheckOp reads a uint8 bool result at the current offset. If false
// (0), it sets the condition_failed flag and aborts the stack machine.
type ConditionCheckOp struct{}

func (*ConditionCheckOp) irOp() {}
