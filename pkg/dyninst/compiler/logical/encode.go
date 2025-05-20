// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package logical implements the logical encoding of the IR program into eBPF stack machine program.
package logical

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// Function is the logical representation of an eBPF stack machine function.
type Function struct {
	ID  FunctionID
	Ops []Op
}

// Program is the logical representation of an eBPF stack machine program.
type Program struct {
	Functions []Function
	Types     []ir.Type
}

type encoder struct {
	// Queue of interesting types that need a `ProcessType` function.
	typeQueue []ir.Type
	// Metadata for `ProcessType` functions.
	typeFuncMetadata map[ir.TypeID]typeFuncMetadata

	functionReg map[FunctionID]bool
	functions   []Function
}

type typeFuncMetadata struct {
	// Whether a given type needs a `ProcessType` function (has any pointers,
	// interfaces, etc).
	needed bool
	// How much the function shifts the output offset. Must be the same no matter
	// what execution flow the function takes.
	offsetShift uint32
}

// EncodeProgram encodes the IR program into a logical eBPF stack machine program.
func EncodeProgram(program ir.Program) Program {
	e := encoder{
		typeFuncMetadata: make(map[ir.TypeID]typeFuncMetadata, len(program.Types)),
		functionReg:      make(map[FunctionID]bool),
	}
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			for _, injectionPC := range event.InjectionPCs {
				e.addEventHandler(injectionPC, event.Type)
			}
		}
	}
	for len(e.typeQueue) > 0 {
		e.addTypeHandler(e.typeQueue[0])
		e.typeQueue = e.typeQueue[1:]
	}
	types := make([]ir.Type, 0, len(program.Types))
	for _, t := range program.Types {
		types = append(types, t)
	}
	return Program{
		Functions: e.functions,
		Types:     types,
	}
}

// Generates a function called when a probe (represented by the root type)
// is triggered with a particular event (injectionPC). The function
// dispatches expression handlers.
func (e *encoder) addEventHandler(injectionPC uint64, rootType *ir.EventRootType) {
	id := ProcessEvent{
		EventRootType: rootType,
		InjectionPC:   injectionPC,
	}
	ops := make([]Op, 0, 2+len(rootType.Expressions))
	ops = append(ops, PrepareEventRootOp{
		EventRootType: rootType,
	})
	for i := range rootType.Expressions {
		exprFunctionID := e.addExpressionHandler(injectionPC, rootType, uint32(i))
		ops = append(ops, CallOp{
			FunctionID: exprFunctionID,
		})
	}
	ops = append(ops, ReturnOp{})
	e.addFunction(id, ops)
}

// Generates a function that evaluates an expression (at exprIdx in the root type)
// at specific user program counter (injectionPC).
func (e *encoder) addExpressionHandler(injectionPC uint64, rootType *ir.EventRootType, exprIdx uint32) FunctionID {
	id := ProcessExpression{
		EventRootType: rootType,
		ExprIdx:       exprIdx,
		InjectionPC:   injectionPC,
	}
	expr := rootType.Expressions[exprIdx].Expression
	// Approximated capacity, location ops may require more than one instruction.
	ops := make([]Op, 0, 4+len(expr.Operations))
	ops = append(ops, ExprPrepareOp{})
	for _, op := range expr.Operations {
		switch op := op.(type) {
		case *ir.LocationOp:
			ops = e.EncodeLocationOp(injectionPC, op, ops)
		default:
			panic(fmt.Sprintf("unexpected ir.Operation: %#v", op))
		}
	}
	ops = append(ops, ExprSaveOp{
		EventRootType: rootType,
		ExprIdx:       exprIdx,
	})
	typeFunctionID, needed := e.addTypeHandler(expr.Type)
	if needed {
		ops = append(ops, CallOp{
			FunctionID: typeFunctionID,
		})
	}
	ops = append(ops, ReturnOp{})
	e.addFunction(id, ops)
	return id
}

func (e *encoder) addFunction(id FunctionID, ops []Op) {
	if _, ok := e.functionReg[id]; ok {
		panic("function `" + id.PrettyString() + "` already exists")
	}
	if _, ok := ops[len(ops)-1].(ReturnOp); !ok {
		panic("last op must be a return")
	}
	e.functionReg[id] = true
	e.functions = append(e.functions, Function{
		ID:  id,
		Ops: ops,
	})
}

// Generate `ProcessType` function called to process data of a given type,
// after it has been read to output buffer. Function is only generated if
// there is something to do with the data of the given type (e.g. pointers
// that have to be chased). Returns function ID or nil and whether the
// function was generated.
func (e *encoder) addTypeHandler(t ir.Type) (FunctionID, bool) {
	fid := ProcessType{
		Type: t,
	}
	if m, ok := e.typeFuncMetadata[t.GetID()]; ok {
		return fid, m.needed
	}
	// Note we will recursively encode embedded types, which is guaranteed not
	// to cycle back. We will also enqueue more types that may need to be encoded
	// (for pointer chasing), but not recurse immediately.
	needed := false
	offsetShift := uint32(0)
	var ops []Op
	switch t := t.(type) {
	case *ir.BaseType:
		// Nothing to process.

	case *ir.StructureType:
		ops = make([]Op, 0, 2*len(t.Fields))
		for _, field := range t.Fields {
			elemFunc, elemNeeded := e.addTypeHandler(field.Type)
			if !elemNeeded {
				continue
			}
			needed = true
			if offsetShift < field.Offset {
				ops = append(ops, IncrementOutputOffsetOp{Value: field.Offset - offsetShift})
			}
			ops = append(ops, CallOp{FunctionID: elemFunc})
			offsetShift = field.Offset + e.typeFuncMetadata[field.Type.GetID()].offsetShift
		}

	// Sequential containers

	case *ir.ArrayType:
		elemFunc, elemNeeded := e.addTypeHandler(t.Element)
		if !elemNeeded {
			break
		}
		needed = true
		offsetShift = uint32(t.GetByteSize())
		ops = []Op{
			ProcessArrayPrepOp{},
			CallOp{
				FunctionID: elemFunc,
			},
			ProcessArrayRepeatOp{},
			ReturnOp{},
		}

	case *ir.GoSliceDataType:
		elemFunc, elemNeeded := e.addTypeHandler(t.Element)
		if !elemNeeded {
			break
		}
		needed = true
		offsetShift = uint32(t.GetByteSize())
		ops = []Op{
			ProcessSliceDataPrepOp{},
			CallOp{
				FunctionID: elemFunc,
			},
			ProcessSliceDataRepeatOp{},
			ReturnOp{},
		}

	case *ir.GoStringDataType:
		// Nothing to process.

	// Pointer or fat pointer types.

	case *ir.PointerType:
		e.typeQueue = append(e.typeQueue, t.Pointee)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessPointerOp{},
		}

	case *ir.GoSliceHeaderType:
		e.typeQueue = append(e.typeQueue, t.Data)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessSliceOp{},
		}

	case *ir.GoStringHeaderType:
		e.typeQueue = append(e.typeQueue, t.Data)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessStringOp{},
		}

	case *ir.GoEmptyInterfaceType:
	case *ir.GoInterfaceType:
		// TODO: support Go interfaces

	case *ir.GoMapType:
		// TODO: support Go maps

	case *ir.GoChannelType:
		// TODO: support Go channels

	// Map containers
	case *ir.GoHMapHeaderType:
	case *ir.GoHMapBucketType:
	case *ir.GoSwissMapGroupsType:
	case *ir.GoSwissMapHeaderType:
		// TODO: support Go maps

	case *ir.EventRootType:
		// EventRootType is handled by event and expression processing functions
		// family.
		panic("unexpected EventRootType")

	default:
		panic(fmt.Sprintf("unexpected ir.Type: %#v", t))
	}

	e.typeFuncMetadata[t.GetID()] = typeFuncMetadata{
		needed:      needed,
		offsetShift: offsetShift,
	}
	if needed {
		e.addFunction(fid, ops)
	}
	return fid, needed
}

type memoryLayoutPiece struct {
	PaddedOffset uint32
	Size         uint32
}

// TODO: move padded offset calculation for location pieces into IR generation.
// Breaks down a type memory into (data size, padding size) pairs. Padding size
// may be zero, and consecutive data pieces with zero padding may not be
// coalesced. Only supports data that may be stored in registers and on stack.
func (e *encoder) typeMemoryLayout(t ir.Type) []memoryLayoutPiece {
	pieces := make([]memoryLayoutPiece, 0)
	var collectPieces func(t ir.Type, offset uint32)
	collectPieces = func(t ir.Type, offset uint32) {
		switch t := t.(type) {
		case *ir.StructureType:
			for _, field := range t.Fields {
				collectPieces(field.Type, offset+field.Offset)
			}

		case *ir.ArrayType:
			for i := range t.Count {
				collectPieces(t.Element, offset+uint32(i)*uint32(t.Element.GetByteSize()))
			}

		// Base or pointer types.
		case *ir.BaseType:
			pieces = append(pieces, memoryLayoutPiece{
				PaddedOffset: offset,
				Size:         uint32(t.GetByteSize()),
			})
		case *ir.GoChannelType:
			pieces = append(pieces, memoryLayoutPiece{
				PaddedOffset: offset,
				Size:         uint32(t.GetByteSize()),
			})
		case *ir.PointerType:
			pieces = append(pieces, memoryLayoutPiece{
				PaddedOffset: offset,
				Size:         uint32(t.GetByteSize()),
			})
		case *ir.GoMapType:
			pieces = append(pieces, memoryLayoutPiece{
				PaddedOffset: offset,
				Size:         uint32(t.GetByteSize()),
			})

		// Structure-like types.
		case *ir.GoEmptyInterfaceType:
			collectPieces(t.UnderlyingStructure, offset)
		case *ir.GoHMapBucketType:
			collectPieces(t.StructureType, offset)
		case *ir.GoHMapHeaderType:
			collectPieces(t.StructureType, offset)
		case *ir.GoInterfaceType:
			collectPieces(t.UnderlyingStructure, offset)
		case *ir.GoSliceHeaderType:
			collectPieces(t.StructureType, offset)
		case *ir.GoStringHeaderType:
			collectPieces(t.StructureType, offset)

		// Types that should never be stored in registers nor stack.
		case *ir.EventRootType:
			panic(fmt.Sprintf("unexpected EventRootType: %#v", t))
		case *ir.GoSliceDataType:
			panic(fmt.Sprintf("unexpected GoSliceDataType: %#v", t))
		case *ir.GoStringDataType:
			panic(fmt.Sprintf("unexpected GoStringDataType: %#v", t))
		case *ir.GoSwissMapGroupsType:
			panic(fmt.Sprintf("unexpected GoSwissMapGroupsType: %#v", t))
		case *ir.GoSwissMapHeaderType:
			panic(fmt.Sprintf("unexpected GoSwissMapHeaderType: %#v", t))
		default:
			panic(fmt.Sprintf("unexpected ir.Type: %#v", t))
		}
	}
	collectPieces(t, 0)
	return pieces
}

// `ops` is used as an output buffer for the encoded instructions.
func (e *encoder) EncodeLocationOp(pc uint64, op *ir.LocationOp, ops []Op) []Op {
	for _, loclist := range op.Variable.Locations {
		if pc < loclist.Range[0] || pc >= loclist.Range[1] {
			continue
		}
		// NOTE: Tricky.
		// We need to match loclist pieces (representing data stored in registers on stack) with memory
		// layout pieces (represending data stored on heap), so we know how to lay out the data in
		// the output buffer.

		// Consecutive pieces of data stored on heap may be padded. Consecutive pieces of data stored
		// in different registers and/or stack (represented with multiple loclist pieces) are not padded.
		// Consecutive pieces of data stored in the same loclist piece are padded (this only happens when
		// the location is a stack, Go never packs multiple data pieces into same register).
		layoutPieces := e.typeMemoryLayout(op.Variable.Type)
		layoutIdx := 0
		outputOffset := op.Offset
		for _, locPiece := range loclist.Pieces {
			paddedOffset := layoutPieces[layoutIdx].PaddedOffset
			nextLayoutIdx := layoutIdx
			for nextLayoutIdx < len(layoutPieces) && layoutPieces[nextLayoutIdx].PaddedOffset-paddedOffset < uint32(locPiece.Size) {
				nextLayoutIdx++
			}
			// Layout pieces in [layoutIdx, nextLayoutIdx) range correspond to current locPiece.
			layoutIdx = nextLayoutIdx
			if op.Offset <= paddedOffset && paddedOffset < op.Offset+op.Size {
				if outputOffset < paddedOffset {
					ops = append(ops, IncrementOutputOffsetOp{Value: paddedOffset - outputOffset})
					outputOffset = paddedOffset
				}
				if locPiece.InReg {
					ops = append(ops, ExprReadRegisterOp{
						Register: uint8(locPiece.Register),
						Size:     uint8(locPiece.Size),
					})
				} else {
					ops = append(ops, ExprDereferenceCfaOp{
						Offset: uint32(locPiece.StackOffset),
						Len:    uint32(locPiece.Size),
					})
				}
			}
		}
		return ops
	}
	// Variable is not available, just return. Expression ops are allowed to "return early" on error.
	ops = append(ops, ReturnOp{})
	return ops
}
