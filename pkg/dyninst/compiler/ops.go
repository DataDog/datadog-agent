// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import "github.com/DataDog/datadog-agent/pkg/dyninst/ir"

// Op is a logical eBPF stack machine operation.
type Op interface {
	logicalOp() // marker
}

type baseOp struct{}

func (baseOp) logicalOp() {}

//revive:disable:exported

// Execution flow operations. Note there are no jumps or conditional jumps.
// Instead, ops implementations are allowed to (conditionally) reset stack machine
// program counter back to itself, effectively looping. They can also use data stack
// to control this behavior, for example to track loop counter.

type CallOp struct {
	baseOp
	FunctionID FunctionID
}

type ReturnOp struct {
	baseOp
}

// Guard op that fails, immediately terminating the stack machine programs.
type IllegalOp struct {
	baseOp
}

// Advances the output offset by the given constant.
type IncrementOutputOffsetOp struct {
	baseOp
	Value uint32
}

// Expression evaluation operations.
// These operations are executed in a chain, starting from prepare op, and ending
// with a save op. Each intermediate op is allowed to return early to the caller,
// resulting in unset presence bit, interpretted as evaluation failure.

type ExprPrepareOp struct {
	baseOp
}

type ExprSaveOp struct {
	baseOp
	EventRootType *ir.EventRootType
	// The index of the expression in the event root type.
	ExprIdx uint32
}

type ExprDereferenceCfaOp struct {
	baseOp
	Offset       int32
	Len          uint32
	OutputOffset uint32
}

type ExprReadRegisterOp struct {
	baseOp
	Register     uint8
	Size         uint8
	OutputOffset uint32
}

type ExprDereferencePtrOp struct {
	baseOp
	Bias uint32
	Len  uint32
}

// Special type processing ops, that evaluate data of a specific type (already
// read into ringbuf), possibly adjusting it, and enqueueing pointers-equivalents
// for further chasing. E.g. processing interface resolves runtime type into ir
// type, records it in the ringbuf, and then enqueues pointer using the resolved
// type.

type ProcessPointerOp struct {
	baseOp
	Pointee ir.Type
}

type ProcessSliceOp struct {
	baseOp
	SliceData *ir.GoSliceDataType
}

type ProcessArrayDataPrepOp struct {
	baseOp
	ArrayByteLen uint32
}

type ProcessSliceDataPrepOp struct {
	baseOp
}

type ProcessSliceDataRepeatOp struct {
	baseOp
	ElemByteLen uint32
}

type ProcessStringOp struct {
	baseOp
	StringData *ir.GoStringDataType
}

type ProcessGoEmptyInterfaceOp struct {
	baseOp
}

type ProcessGoInterfaceOp struct {
	baseOp
}

type ProcessGoHmapOp struct {
	baseOp
	BucketsArray ir.ArrayType
}

type ProcessGoSwissMapOp struct {
	baseOp
	TablePtrSlice ir.GoSliceHeaderType
	Group         ir.Type
}

type ProcessGoSwissMapGroupsOp struct {
	baseOp
	Group ir.Type
}

// Top level ops.

type ChasePointersOp struct {
	baseOp
}

type PrepareEventRootOp struct {
	baseOp
	EventRootType *ir.EventRootType
}

//revive:enable:exported
