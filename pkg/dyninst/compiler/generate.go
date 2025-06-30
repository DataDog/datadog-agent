// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// Function represents stack machine function.
type Function struct {
	ID  FunctionID
	Ops []Op
}

// Throttler corresponds to a throttler instance with specified limits.
type Throttler struct {
	PeriodNs uint64
	Budget   int64
}

// Program represents stack machine program.
type Program struct {
	ID         uint32
	Functions  []Function
	Types      []ir.Type
	Throttlers []Throttler
}

type generator struct {
	// Queue of interesting types that need a `ProcessType` function.
	typeQueue []ir.Type
	// Metadata for `ProcessType` functions.
	typeFuncMetadata map[ir.TypeID]typeFuncMetadata

	functionReg map[FunctionID]struct{}
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

// GenerateProgram generates stack machine program for a given IR program.
func GenerateProgram(program *ir.Program) (Program, error) {
	g := generator{
		typeFuncMetadata: make(map[ir.TypeID]typeFuncMetadata, len(program.Types)),
		functionReg:      make(map[FunctionID]struct{}),
	}
	err := g.addFunction(ChasePointers{}, []Op{
		ChasePointersOp{},
		ReturnOp{},
	})
	if err != nil {
		return Program{}, err
	}
	throttlers := make([]Throttler, 0, len(program.Probes))
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			for _, injectionPoint := range event.InjectionPoints {
				err := g.addEventHandler(
					injectionPoint,
					len(throttlers),
					probe.GetCaptureConfig().GetMaxReferenceDepth(),
					event.Type,
				)
				if err != nil {
					return Program{}, err
				}
			}
			// We throttle each event individually, across all its injection points.
			throttleConfig := probe.GetThrottleConfig()
			periodMs := throttleConfig.GetThrottlePeriodMs()
			periodNs := uint64(periodMs) * uint64(time.Millisecond)
			throttlers = append(throttlers, Throttler{
				PeriodNs: periodNs,
				Budget:   throttleConfig.GetThrottleBudget(),
			})
		}
	}
	for len(g.typeQueue) > 0 {
		_, _, err := g.addTypeHandler(g.typeQueue[0])
		if err != nil {
			return Program{}, err
		}
		g.typeQueue = g.typeQueue[1:]
	}
	types := make([]ir.Type, 0, len(program.Types))
	for _, t := range program.Types {
		types = append(types, t)
	}
	return Program{
		ID:         uint32(program.ID),
		Functions:  g.functions,
		Types:      types,
		Throttlers: throttlers,
	}, nil
}

// Generates a function called when a probe (represented by the root type)
// is triggered with a particular event (injectionPC). The function
// dispatches expression handlers.
func (g *generator) addEventHandler(
	injectionPoint ir.InjectionPoint,
	throttlerIdx int,
	pointerChasingLimit uint32,
	rootType *ir.EventRootType,
) error {
	id := ProcessEvent{
		InjectionPC:         injectionPoint.PC,
		ThrottlerIdx:        throttlerIdx,
		PointerChasingLimit: pointerChasingLimit,
		Frameless:           injectionPoint.Frameless,
		EventRootType:       rootType,
	}
	ops := make([]Op, 0, 2+len(rootType.Expressions))
	ops = append(ops, PrepareEventRootOp{
		EventRootType: rootType,
	})
	for i := range rootType.Expressions {
		exprFunctionID, err := g.addExpressionHandler(injectionPoint.PC, rootType, uint32(i))
		if err != nil {
			return err
		}
		ops = append(ops, CallOp{
			FunctionID: exprFunctionID,
		})
	}
	ops = append(ops, ReturnOp{})
	return g.addFunction(id, ops)
}

// Generates a function that evaluates an expression (at exprIdx in the root type)
// at specific user program counter (injectionPC).
func (g *generator) addExpressionHandler(injectionPC uint64, rootType *ir.EventRootType, exprIdx uint32) (FunctionID, error) {
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
			var err error
			ops, err = g.EncodeLocationOp(injectionPC, op, ops)
			if err != nil {
				return nil, err
			}
		default:
			panic(fmt.Sprintf("unexpected ir.Operation: %#v", op))
		}
	}
	ops = append(ops, ExprSaveOp{
		EventRootType: rootType,
		ExprIdx:       exprIdx,
	})
	typeFunctionID, needed, err := g.addTypeHandler(expr.Type)
	if err != nil {
		return nil, err
	}
	if needed {
		ops = append(ops, CallOp{
			FunctionID: typeFunctionID,
		})
	}
	ops = append(ops, ReturnOp{})
	err = g.addFunction(id, ops)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (g *generator) addFunction(id FunctionID, ops []Op) error {
	if _, ok := g.functionReg[id]; ok {
		return errors.Errorf("internal: function `%s` already exists", id)
	}
	if _, ok := ops[len(ops)-1].(ReturnOp); !ok {
		return errors.New("internal: last op must be a return")
	}
	g.functionReg[id] = struct{}{}
	g.functions = append(g.functions, Function{
		ID:  id,
		Ops: ops,
	})
	return nil
}

// Generate `ProcessType` function called to process data of a given type,
// after it has been read to output buffer. Function is only generated if
// there is something to do with the data of the given type (e.g. pointers
// that have to be chased). Returns function ID or nil and whether the
// function was generated.
func (g *generator) addTypeHandler(t ir.Type) (FunctionID, bool, error) {
	fid := ProcessType{
		Type: t,
	}
	if m, ok := g.typeFuncMetadata[t.GetID()]; ok {
		return fid, m.needed, nil
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
			elemFunc, elemNeeded, err := g.addTypeHandler(field.Type)
			if err != nil {
				return nil, false, err
			}
			if !elemNeeded {
				continue
			}
			needed = true
			if offsetShift < field.Offset {
				ops = append(ops, IncrementOutputOffsetOp{Value: field.Offset - offsetShift})
			}
			ops = append(ops, CallOp{FunctionID: elemFunc})
			offsetShift = field.Offset + g.typeFuncMetadata[field.Type.GetID()].offsetShift
		}
		ops = append(ops, ReturnOp{})

	// Sequential containers

	case *ir.ArrayType:
		elemFunc, elemNeeded, err := g.addTypeHandler(t.Element)
		if err != nil {
			return nil, false, err
		}
		if !elemNeeded {
			break
		}
		needed = true
		offsetShift = uint32(t.GetByteSize())
		ops = []Op{
			ProcessArrayDataPrepOp{ArrayByteLen: t.GetByteSize()},
			CallOp{
				FunctionID: elemFunc,
			},
			ProcessSliceDataRepeatOp{ElemByteLen: t.Element.GetByteSize()},
			ReturnOp{},
		}

	case *ir.GoSliceDataType:
		elemFunc, elemNeeded, err := g.addTypeHandler(t.Element)
		if err != nil {
			return nil, false, err
		}
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
			ProcessSliceDataRepeatOp{ElemByteLen: t.Element.GetByteSize()},
			ReturnOp{},
		}

	case *ir.GoStringDataType:
		// Nothing to process.

	// Pointer or fat pointer types.

	case *ir.PointerType:
		g.typeQueue = append(g.typeQueue, t.Pointee)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessPointerOp{
				Pointee: t.Pointee,
			},
			ReturnOp{},
		}

	case *ir.GoSliceHeaderType:
		g.typeQueue = append(g.typeQueue, t.Data)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessSliceOp{SliceData: t.Data},
			ReturnOp{},
		}

	case *ir.GoStringHeaderType:
		g.typeQueue = append(g.typeQueue, t.Data)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessStringOp{
				StringData: t.Data,
			},
			ReturnOp{},
		}

	case *ir.GoEmptyInterfaceType:
	case *ir.GoInterfaceType:
		// TODO: support Go interfaces

	case *ir.GoMapType:
		// TODO: support Go maps

	case *ir.GoChannelType:
		// TODO: support Go channels

	case *ir.GoSubroutineType:
		// TODO: support Go subroutines

	// Map containers
	case *ir.GoHMapHeaderType:
	case *ir.GoHMapBucketType:
	case *ir.GoSwissMapGroupsType:
	case *ir.GoSwissMapHeaderType:
		// TODO: support Go maps

	case *ir.EventRootType:
		// EventRootType is handled by event and expression processing functions
		// family.
		return nil, false, errors.New("internal: unexpected EventRootType")

	default:
		panic(fmt.Sprintf("unexpected ir.Type: %#v", t))
	}

	g.typeFuncMetadata[t.GetID()] = typeFuncMetadata{
		needed:      needed,
		offsetShift: offsetShift,
	}
	if needed {
		err := g.addFunction(fid, ops)
		if err != nil {
			return nil, false, err
		}
	}
	return fid, needed, nil
}

type memoryLayoutPiece struct {
	PaddedOffset uint32
	Size         uint32
}

// TODO: move padded offset calculation for location pieces into IR generation.
// Breaks down a type memory into (data size, padding size) pairs. Padding size
// may be zero, and consecutive data pieces with zero padding may not be
// coalesced. Only supports data that may be stored in registers and on stack.
func (g *generator) typeMemoryLayout(t ir.Type) ([]memoryLayoutPiece, error) {
	var pieces []memoryLayoutPiece
	var collectPieces func(t ir.Type, offset uint32) error
	collectPieces = func(t ir.Type, offset uint32) error {
		var err error
		switch t := t.(type) {
		case *ir.StructureType:
			for _, field := range t.Fields {
				err = collectPieces(field.Type, offset+field.Offset)
				if err != nil {
					return err
				}
			}

		case *ir.ArrayType:
			for i := range t.Count {
				err = collectPieces(t.Element, offset+uint32(i)*uint32(t.Element.GetByteSize()))
				if err != nil {
					return err
				}
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
			err = collectPieces(t.UnderlyingStructure, offset)
		case *ir.GoHMapBucketType:
			err = collectPieces(t.StructureType, offset)
		case *ir.GoHMapHeaderType:
			err = collectPieces(t.StructureType, offset)
		case *ir.GoInterfaceType:
			err = collectPieces(t.UnderlyingStructure, offset)
		case *ir.GoSliceHeaderType:
			err = collectPieces(t.StructureType, offset)
		case *ir.GoStringHeaderType:
			err = collectPieces(t.StructureType, offset)

		// Types that should never be stored in registers nor stack.
		case *ir.EventRootType:
			err = errors.Errorf("internal: unexpected EventRootType: %#v", t)
		case *ir.GoSliceDataType:
			err = errors.Errorf("internal: unexpected GoSliceDataType: %#v", t)
		case *ir.GoStringDataType:
			err = errors.Errorf("internal: unexpected GoStringDataType: %#v", t)
		case *ir.GoSwissMapGroupsType:
			err = errors.Errorf("internal: unexpected GoSwissMapGroupsType: %#v", t)
		case *ir.GoSwissMapHeaderType:
			err = errors.Errorf("internal: unexpected GoSwissMapHeaderType: %#v", t)
		default:
			panic(fmt.Sprintf("unexpected ir.Type: %#v", t))
		}
		return err
	}
	err := collectPieces(t, 0)
	if err != nil {
		return nil, err
	}
	return pieces, nil
}

// `ops` is used as an output buffer for the encoded instructions.
func (g *generator) EncodeLocationOp(pc uint64, op *ir.LocationOp, ops []Op) ([]Op, error) {
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
		layoutPieces, err := g.typeMemoryLayout(op.Variable.Type)
		if err != nil {
			return nil, err
		}
		layoutIdx := 0
		if len(loclist.Pieces) == 0 {
			// Variable has loclist entry for relevant PC range, but it is still unavailable.
			break
		}
		for _, piece := range loclist.Pieces {
			paddedOffset := layoutPieces[layoutIdx].PaddedOffset
			nextLayoutIdx := layoutIdx
			for nextLayoutIdx < len(layoutPieces) && layoutPieces[nextLayoutIdx].PaddedOffset-paddedOffset < uint32(piece.Size) {
				nextLayoutIdx++
			}
			// Layout pieces in [layoutIdx, nextLayoutIdx) range correspond to current locPiece.
			layoutIdx = nextLayoutIdx
			if op.Offset <= paddedOffset && paddedOffset < op.Offset+op.ByteSize {
				switch p := piece.Op.(type) {
				case ir.Register:
					if piece.Size > 8 {
						return nil, fmt.Errorf("unsupported register size: %d", piece.Size)
					}
					ops = append(ops, ExprReadRegisterOp{
						Register:     p.RegNo,
						Size:         uint8(piece.Size),
						OutputOffset: paddedOffset - op.Offset,
					})
				case ir.Cfa:
					ops = append(ops, ExprDereferenceCfaOp{
						Offset:       p.CfaOffset,
						Len:          piece.Size,
						OutputOffset: paddedOffset - op.Offset,
					})
				case ir.Addr:
					return nil, fmt.Errorf("unsupported addr location op")
				}
			}
		}
		return ops, nil
	}
	// Variable is not available, just return. Expression ops are allowed to "return early" on error.
	ops = append(ops, ReturnOp{})
	return ops, nil
}
