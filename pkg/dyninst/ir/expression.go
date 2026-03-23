// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

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

// ConditionCheckOp reads a uint8 bool result at the current offset. If false
// (0), it sets the condition_failed flag and aborts the stack machine.
type ConditionCheckOp struct{}

func (*ConditionCheckOp) irOp() {}
