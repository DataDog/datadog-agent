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
// leaving the expression status as absent (0).

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
	Bias          uint32
	Len           uint32
	ExprStatusIdx uint32 // expression index for writing nil-deref status; ^0 = none
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

// ProcessGoDictTypeOp resolves a generic shape type parameter to its concrete
// type by reading the runtime dictionary at probe time. The eBPF stack machine:
// 1. Reads the dict pointer from the register specified by DictRegister
// 2. Indexes into the dict array at DictIndex
// 3. Reads the *runtime._type at that slot
// 4. Converts to a types-base offset and records for type resolution
//
// DictRegister encoding:
//   - Bit 7 clear (0-15): read dict pointer from pt_regs register (entry probes)
//   - Bit 7 set (0x80|reg): read dict pointer from saved call context (return probes)
//
// On the entry path, the handler always stashes the dict pointer into
// stack_machine_t.saved_dict_ptr, which event.c propagates through the call
// context so that return probes can use it.
type ProcessGoDictTypeOp struct {
	baseOp
	DictIndex    int32  // flat index into the dictionary array
	DictRegister uint8  // DWARF register number; bit 7 = use saved dict ptr
	OutputOffset uint32 // byte offset within the event root data to write the resolved type
}

// CallDictResolvedOp dynamically dispatches to the concrete type's ProcessType
// function based on a dict-resolved runtime type. It reads the resolved
// *runtime._type offset from the event root data at OutputOffset (where
// ProcessGoDictTypeOp wrote it), looks up the concrete type's enqueue_pc,
// and calls it. If resolution fails, falls back to calling the FallbackFunc
// (the shape type's ProcessType).
type CallDictResolvedOp struct {
	baseOp
	OutputOffset uint32     // byte offset in event root where resolved runtime type was written
	FallbackFunc FunctionID // shape type's ProcessType function ID
}

type ProcessGoHmapOp struct {
	baseOp
	BucketsType      *ir.GoSliceDataType
	BucketType       *ir.GoHMapBucketType
	FlagsOffset      uint8
	BOffset          uint8
	BucketsOffset    uint8
	OldBucketsOffset uint8
}

type ProcessGoSwissMapOp struct {
	baseOp
	TablePtrSlice *ir.GoSliceDataType
	Group         ir.Type
	DirPtrOffset  uint8
	DirLenOffset  uint8
}

type ProcessGoSwissMapGroupsOp struct {
	baseOp
	DataOffset       uint8
	LengthMaskOffset uint8
	GroupSlice       *ir.GoSliceDataType
	Group            ir.Type
}

// Top level ops.

type ChasePointersOp struct {
	baseOp
}

type PrepareEventRootOp struct {
	baseOp
	EventRootType *ir.EventRootType
}

// Condition expression ops.

type ExprPushOffsetOp struct {
	baseOp
	ByteSize uint32
}

type ExprLoadLiteralOp struct {
	baseOp
	Data []byte
}

type ExprReadStringOp struct {
	baseOp
	MaxLen uint16
}

type ExprCmpEqBaseOp struct {
	baseOp
	ByteSize uint8
}

type ExprCmpEqStringOp struct {
	baseOp
}

type ExprSliceBoundsCheckOp struct {
	baseOp
	Index         uint32
	ExprStatusIdx uint32 // expression index for writing OOB status; ^0 = none
}

type ConditionBeginOp struct {
	baseOp
}

type ConditionCheckOp struct {
	baseOp
}

//revive:enable:exported
