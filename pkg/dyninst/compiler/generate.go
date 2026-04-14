// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	ID               uint32
	Functions        []Function
	Types            []ir.Type
	Throttlers       []Throttler
	GoModuledataInfo ir.GoModuledataInfo
	CommonTypes      ir.CommonTypes
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
	for probeIdx, probe := range program.Probes {
		// Determine which event kind has a condition (if any) across all
		// instances. All instances share the same probe config so conditions
		// are uniform.
		var conditionEventKind ir.EventKind
		for _, inst := range probe.Instances {
			for _, event := range inst.Events {
				if event.Condition != nil {
					conditionEventKind = event.Kind
					break
				}
			}
			if conditionEventKind != 0 {
				break
			}
		}

		// Track throttler indices per event kind so that all instances of
		// the same event kind share a single throttler.
		throttlerByKind := make(map[ir.EventKind]int)
		for _, inst := range probe.Instances {
			for _, event := range inst.Events {
				throttlerIdx, ok := throttlerByKind[event.Kind]
				if !ok {
					throttlerIdx = len(throttlers)
					throttlerByKind[event.Kind] = throttlerIdx
					throttleConfig := probe.GetThrottleConfig()
					periodMs := throttleConfig.GetThrottlePeriodMs()
					periodNs := uint64(periodMs) * uint64(time.Millisecond)
					throttlers = append(throttlers, Throttler{
						PeriodNs: periodNs,
						Budget:   throttleConfig.GetThrottleBudget(),
					})
				}
				throttleMode := computeThrottleMode(event, conditionEventKind)
				for _, injectionPoint := range event.InjectionPoints {
					err := g.addEventHandler(
						injectionPoint,
						throttlerIdx,
						probe.GetCaptureConfig(),
						uint32(probeIdx),
						event.Type,
						event.Kind,
						event.Condition,
						throttleMode,
					)
					if err != nil {
						return Program{}, err
					}
				}
			}
		}
	}
	// Add all the types for which we know the Go runtime type to the
	// queue for processing.
	for _, t := range program.Types {
		if _, ok := t.GetGoRuntimeType(); ok {
			g.typeQueue = append(g.typeQueue, t)
		}
	}
	// Sort the queue to make sure we process types in a deterministic order.
	slices.SortFunc(g.typeQueue, func(a, b ir.Type) int {
		return cmp.Compare(a.GetID(), b.GetID())
	})
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
	slices.SortStableFunc(g.functions, func(a, b Function) int {
		at, aOk := a.ID.(ProcessType)
		bt, bOk := b.ID.(ProcessType)
		switch {
		case !aOk && !bOk:
			return 0
		case !aOk:
			return -1
		case !bOk:
			return 1
		default:
			return cmp.Or(
				cmp.Compare(at.Type.GetName(), bt.Type.GetName()),
				cmp.Compare(at.Type.GetID(), bt.Type.GetID()),
			)

		}
	})
	return Program{
		ID:               uint32(program.ID),
		Functions:        g.functions,
		Types:            types,
		Throttlers:       throttlers,
		GoModuledataInfo: program.GoModuledataInfo,
		CommonTypes:      program.CommonTypes,
	}, nil
}

// computeThrottleMode determines the throttle mode for an event based on
// whether this event or its sibling has a condition.
//
// Note importantly at time of writing only one event can have a condition!
func computeThrottleMode(event *ir.Event, conditionEventKind ir.EventKind) ThrottleMode {
	hasCond := event.Condition != nil
	isReturn := event.Kind == ir.EventKindReturn

	if hasCond {
		// This event has a condition: throttle after condition check.
		// Note: if we later support compound conditions where the entry and
		// return both have conditions, we will need to adjust this to only
		// throttle the return.
		return ThrottleAfterCondCheck
	}
	if conditionEventKind != 0 && conditionEventKind != event.Kind {
		// Sibling event has a condition: skip throttling for this event.
		// For entry with conditional return: don't throttle entry so the return
		// condition can evaluate.
		// For return with conditional entry: unconditional returns never throttle.
		return ThrottleNone
	}
	if isReturn {
		// Unconditional return without sibling condition: never throttle.
		return ThrottleNone
	}
	// Default: throttle at start.
	return ThrottleAtStart
}

// Generates a function called when a probe (represented by the root type)
// is triggered with a particular event (injectionPC). The function
// dispatches expression handlers.
func (g *generator) addEventHandler(
	injectionPoint ir.InjectionPoint,
	throttlerIdx int,
	captureConfig ir.CaptureConfig,
	probeID uint32,
	rootType *ir.EventRootType,
	eventKind ir.EventKind,
	condition *ir.Expression,
	throttleMode ThrottleMode,
) error {
	id := ProcessEvent{
		InjectionPC:         injectionPoint.PC,
		ThrottlerIdx:        throttlerIdx,
		PointerChasingLimit: captureConfig.GetMaxReferenceDepth(),
		CollectionSizeLimit: captureConfig.GetMaxCollectionSize(),
		StringSizeLimit:     captureConfig.GetMaxLength(),
		Frameless:           injectionPoint.Frameless,
		HasAssociatedReturn: injectionPoint.HasAssociatedReturn,
		NoReturnReason:      injectionPoint.NoReturnReason,
		TopPCOffset:         injectionPoint.TopPCOffset,
		ThrottleMode:        throttleMode,
		ProbeID:             probeID,
		EventKind:           eventKind,
		EventRootType:       rootType,
	}
	ops := make([]Op, 0, 3+len(rootType.Expressions))

	// If there's a condition, insert the condition check before
	// PrepareEventRoot so that non-matching events are skipped entirely.
	if condition != nil {
		condFunctionID, err := g.addConditionHandler(injectionPoint.PC, rootType, condition)
		if err != nil {
			return err
		}
		ops = append(ops, CallOp{
			FunctionID: condFunctionID,
		})
	}

	ops = append(ops, PrepareEventRootOp{
		EventRootType: rootType,
	})
	// For generic shape functions, resolve dict entries right after
	// preparing the event root. Each dict entry reads the dictionary
	// pointer from a register, indexes into it, and writes the resolved
	// runtime type into the event output. For return events, bit 7 of
	// DictRegister is set to signal the eBPF handler to read the dict
	// pointer from saved call context instead of a CPU register.
	for _, de := range rootType.DictEntries {
		reg := de.DictRegister
		if eventKind == ir.EventKindReturn {
			reg |= 0x80
		}
		ops = append(ops, ProcessGoDictTypeOp{
			DictIndex:    int32(de.DictIndex),
			DictRegister: reg,
			OutputOffset: de.Offset,
		})
	}
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

// Generates a function that evaluates a condition expression. If the condition
// evaluates to false, the stack machine sets condition_failed and aborts.
func (g *generator) addConditionHandler(
	injectionPC uint64,
	rootType *ir.EventRootType,
	condition *ir.Expression,
) (FunctionID, error) {
	id := ProcessCondition{
		EventRootType: rootType,
		InjectionPC:   injectionPC,
	}
	ops := make([]Op, 0, 5+len(condition.Operations))
	ops = append(ops, ConditionBeginOp{})
	ops = append(ops, ExprPrepareOp{})

	for _, op := range condition.Operations {
		switch op := op.(type) {
		case *ir.LocationOp:
			opsAfter, err := g.EncodeLocationOp(injectionPC, op, ops)
			if err != nil {
				logLocationIssue(
					"error encoding location op for condition: %v", err,
				)
				opsAfter = append(ops, ReturnOp{})
			}
			ops = opsAfter
		case *ir.DereferenceOp:
			ops = append(ops, ExprDereferencePtrOp{
				Bias:      op.Bias,
				Len:       op.ByteSize,
				NilBitIdx: ^uint32(0),
			})
		case *ir.ExprPushOffsetOp:
			ops = append(ops, ExprPushOffsetOp{ByteSize: op.ByteSize})
		case *ir.ExprLoadLiteralOp:
			ops = append(ops, ExprLoadLiteralOp{Data: op.Data})
		case *ir.ExprReadStringOp:
			ops = append(ops, ExprReadStringOp{MaxLen: op.MaxLen})
		case *ir.ExprCmpEqBaseOp:
			ops = append(ops, ExprCmpEqBaseOp{ByteSize: op.ByteSize})
		case *ir.ExprCmpEqStringOp:
			ops = append(ops, ExprCmpEqStringOp{})
		case *ir.ConditionCheckOp:
			ops = append(ops, ConditionCheckOp{})
		default:
			panic(fmt.Sprintf("unexpected ir.Operation in condition: %#v", op))
		}
	}
	ops = append(ops, ReturnOp{})
	err := g.addFunction(id, ops)
	if err != nil {
		return nil, err
	}
	return id, nil
}

// findDictEntry returns the DictEntry matching the given dictIndex, or nil.
func findDictEntry(rootType *ir.EventRootType, dictIndex int) *ir.DictEntry {
	if dictIndex < 0 {
		return nil
	}
	for i := range rootType.DictEntries {
		if rootType.DictEntries[i].DictIndex == dictIndex {
			return &rootType.DictEntries[i]
		}
	}
	return nil
}

var encodeLocationLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

func logLocationIssue(format string, args ...any) {
	if encodeLocationLogLimiter.Allow() {
		log.Infof("dyninst/compiler: location encoding issue: "+format, args...)
	} else {
		log.Debugf("dyninst/compiler: location encoding issue: "+format, args...)
	}
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

	// Track the size of the last operation to sanity check that we are
	// dereferencing a pointer with the correct size.
	var lastOpSize uint32
	for _, op := range expr.Operations {
		switch op := op.(type) {
		case *ir.LocationOp:
			lastOpSize = op.ByteSize
			opsAfter, err := g.EncodeLocationOp(injectionPC, op, ops)
			// Treat an error as if the location op is not available.
			if err != nil {
				logLocationIssue(
					"error encoding location op for expression %s: %v",
					rootType.Expressions[exprIdx].Name,
					err,
				)
				opsAfter = append(ops, ReturnOp{})
			}
			ops = opsAfter
		case *ir.DereferenceOp:
			const pointerSize = 8
			if lastOpSize != pointerSize {
				return nil, fmt.Errorf("unexpected pointer size: %d", lastOpSize)
			}
			lastOpSize = op.ByteSize
			ops = append(ops, ExprDereferencePtrOp{
				Bias:      op.Bias,
				Len:       op.ByteSize,
				NilBitIdx: 2*exprIdx + 1,
			})
		case *ir.ExprPushOffsetOp:
			ops = append(ops, ExprPushOffsetOp{ByteSize: op.ByteSize})
		case *ir.ExprLoadLiteralOp:
			ops = append(ops, ExprLoadLiteralOp{Data: op.Data})
		case *ir.ExprReadStringOp:
			ops = append(ops, ExprReadStringOp{MaxLen: op.MaxLen})
		case *ir.ExprCmpEqBaseOp:
			ops = append(ops, ExprCmpEqBaseOp{ByteSize: op.ByteSize})
		case *ir.ExprCmpEqStringOp:
			ops = append(ops, ExprCmpEqStringOp{})
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
		// For dict-resolved shape types, emit a dynamic dispatch that
		// tries to call the concrete type's ProcessType, falling back
		// to the shape type's.
		rootExpr := rootType.Expressions[exprIdx]
		dictEntry := findDictEntry(rootType, rootExpr.DictIndex)
		if dictEntry != nil {
			ops = append(ops, CallDictResolvedOp{
				OutputOffset: dictEntry.Offset,
				FallbackFunc: typeFunctionID,
			})
		} else {
			ops = append(ops, CallOp{
				FunctionID: typeFunctionID,
			})
		}
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
		return fmt.Errorf("internal: function `%s` already exists", id)
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
	structureTypeHandler := func(t *ir.StructureType) error {
		ops = make([]Op, 0, 2*len(t.RawFields))
		for field := range t.Fields() {
			elemFunc, elemNeeded, err := g.addTypeHandler(field.Type)
			if err != nil {
				return err
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
		return nil
	}
	switch t := t.(type) {
	case *ir.BaseType:
		// Nothing to process.

	case *ir.GoHMapBucketType:
		if err := structureTypeHandler(t.StructureType); err != nil {
			return fid, needed, err
		}
	case *ir.StructureType:
		if err := structureTypeHandler(t); err != nil {
			return fid, needed, err
		}

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
			ProcessSliceDataRepeatOp{ElemByteLen: t.Element.GetByteSize() - g.typeFuncMetadata[t.Element.GetID()].offsetShift},
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
			ProcessSliceDataRepeatOp{ElemByteLen: t.Element.GetByteSize() - g.typeFuncMetadata[t.Element.GetID()].offsetShift},
			ReturnOp{},
		}

	case *ir.GoStringDataType:
		// Nothing to process.

	// Pointer or fat pointer types.

	case *ir.VoidPointerType:
		// Nothing to process. We don't know what the pointee is.

	case *ir.UnresolvedPointeeType:
		// Nothing to process.

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
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessGoEmptyInterfaceOp{},
			ReturnOp{},
		}

	case *ir.GoInterfaceType:
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessGoInterfaceOp{},
			ReturnOp{},
		}

	case *ir.GoMapType:
		g.typeQueue = append(g.typeQueue, t.HeaderType)
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessPointerOp{
				Pointee: t.HeaderType,
			},
			ReturnOp{},
		}
	case *ir.GoChannelType:
		// TODO: support Go channels

	case *ir.GoSubroutineType:
		// TODO: support Go subroutines

	// Map containers
	case *ir.GoHMapHeaderType:
		needed = true
		flagsOffset, err := offsetOfUint8(t.RawFields, "flags")
		if err != nil {
			return nil, false, err
		}
		bOffset, err := offsetOfUint8(t.RawFields, "B")
		if err != nil {
			return nil, false, err
		}
		bucketsOffset, err := offsetOfUint8(t.RawFields, "buckets")
		if err != nil {
			return nil, false, err
		}
		oldBucketsOffset, err := offsetOfUint8(t.RawFields, "oldbuckets")
		if err != nil {
			return nil, false, err
		}
		ops = []Op{
			ProcessGoHmapOp{
				BucketsType:      t.BucketsType,
				BucketType:       t.BucketType,
				FlagsOffset:      flagsOffset,
				BOffset:          bOffset,
				BucketsOffset:    bucketsOffset,
				OldBucketsOffset: oldBucketsOffset,
			},
			ReturnOp{},
		}
		g.typeQueue = append(
			g.typeQueue,
			t.BucketsType,
			t.BucketType,
			t.BucketType.KeyType,
			t.BucketType.ValueType,
		)
	case *ir.GoSwissMapGroupsType:
		dataOffset, err := offsetOfUint8(t.RawFields, "data")
		if err != nil {
			return nil, false, err
		}
		lengthMaskOffset, err := offsetOfUint8(t.RawFields, "lengthMask")
		if err != nil {
			return nil, false, err
		}
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessGoSwissMapGroupsOp{
				DataOffset:       uint8(dataOffset),
				LengthMaskOffset: uint8(lengthMaskOffset),
				GroupSlice:       t.GroupSliceType,
				Group:            t.GroupType,
			},
			ReturnOp{},
		}
		g.typeQueue = append(g.typeQueue, t.GroupSliceType, t.GroupType)
	case *ir.GoSwissMapHeaderType:
		directoryPtrOffset, err := offsetOfUint8(t.RawFields, "dirPtr")
		if err != nil {
			return nil, false, err
		}
		directoryLenOffset, err := offsetOfUint8(t.RawFields, "dirLen")
		if err != nil {
			return nil, false, err
		}
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessGoSwissMapOp{
				TablePtrSlice: t.TablePtrSliceType,
				Group:         t.GroupType,
				DirPtrOffset:  uint8(directoryPtrOffset),
				DirLenOffset:  uint8(directoryLenOffset),
			},
			ReturnOp{},
		}
		g.typeQueue = append(g.typeQueue, t.TablePtrSliceType, t.GroupType)
	case *ir.EventRootType:
		// EventRootType is handled by event and expression processing functions
		// family.
		return nil, false, errors.New("internal: unexpected EventRootType")

	default:
		panic(fmt.Sprintf("unexpected ir.Type to handle: %#v", t))
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
	collectFields := func(fields []ir.Field, offset uint32) error {
		for _, field := range fields {
			if err := collectPieces(field.Type, offset+field.Offset); err != nil {
				return err
			}
		}
		return nil
	}
	collectPieces = func(t ir.Type, offset uint32) error {
		var err error
		switch t := t.(type) {
		case *ir.StructureType:
			if err := collectFields(t.RawFields, offset); err != nil {
				return err
			}

		case *ir.ArrayType:
			for i := range t.Count {
				err = collectPieces(t.Element, offset+uint32(i)*uint32(t.Element.GetByteSize()))
				if err != nil {
					return err
				}
			}

		// Base or pointer types.
		case *ir.BaseType, *ir.GoChannelType, *ir.PointerType, *ir.VoidPointerType, *ir.GoMapType, *ir.GoSubroutineType:
			pieces = append(pieces, memoryLayoutPiece{
				PaddedOffset: offset,
				Size:         uint32(t.GetByteSize()),
			})

		// Structure-like types.
		case *ir.GoEmptyInterfaceType:
			err = collectFields(t.RawFields, offset)
		case *ir.GoHMapBucketType:
			err = collectPieces(t.StructureType, offset)
		case *ir.GoHMapHeaderType:
			err = collectPieces(t.StructureType, offset)
		case *ir.GoInterfaceType:
			err = collectFields(t.RawFields, offset)
		case *ir.GoSliceHeaderType:
			err = collectPieces(t.StructureType, offset)
		case *ir.GoStringHeaderType:
			err = collectPieces(t.StructureType, offset)

		// Types that should never be stored in registers nor stack.
		case *ir.EventRootType:
			err = fmt.Errorf("internal: unexpected EventRootType: %#v", t)
		case *ir.GoSliceDataType:
			err = fmt.Errorf("internal: unexpected GoSliceDataType: %#v", t)
		case *ir.GoStringDataType:
			err = fmt.Errorf("internal: unexpected GoStringDataType: %#v", t)
		case *ir.GoSwissMapGroupsType:
			err = fmt.Errorf("internal: unexpected GoSwissMapGroupsType: %#v", t)
		case *ir.GoSwissMapHeaderType:
			err = fmt.Errorf("internal: unexpected GoSwissMapHeaderType: %#v", t)
		default:
			panic(fmt.Sprintf("unexpected ir.Type for layout: %#v", t))
		}
		return err
	}
	err := collectPieces(t, 0)
	if err != nil {
		return nil, err
	}
	return pieces, nil
}

func offsetOf(fields []ir.Field, name string) (uint32, error) {
	for _, field := range fields {
		if field.Name == name {
			return field.Offset, nil
		}
	}
	return 0, fmt.Errorf("internal: field `%s` not found", name)
}

func offsetOfUint8(fields []ir.Field, name string) (uint8, error) {
	offset, err := offsetOf(fields, name)
	if err != nil {
		return 0, err
	}
	if offset > math.MaxUint8 {
		return 0, fmt.Errorf("offset of %s overflows uint8: %d", name, offset)
	}
	return uint8(offset), nil
}

// hasDuplicateInterfacePieces returns true if an interface type has both
// pieces claiming the same register. Interface types have two distinct
// pointers (type/itab and data) that can never have the same value, so
// duplicate registers indicate invalid DWARF. Seen on ARM64 with go1.26rc1.
func hasDuplicateInterfacePieces(typ ir.Type, pieces []ir.Piece) bool {
	switch typ.(type) {
	case *ir.GoInterfaceType, *ir.GoEmptyInterfaceType:
		// Interfaces always have exactly 2 pieces
		if len(pieces) == 2 && pieces[0].Op == pieces[1].Op {
			return true
		}
	}
	return false
}

// `ops` is used as an output buffer for the encoded instructions.
func (g *generator) EncodeLocationOp(pc uint64, op *ir.LocationOp, ops []Op) ([]Op, error) {
outer:
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

		if op.Variable.Type.GetByteSize() == 0 {
			// Nothing needs to be read.
			return ops, nil
		}

		// Check if the matching location list is unavailable. If so, by
		// breaking here we'll make sure we don't mark the variable as
		// available.
		//
		// Also check for duplicate interface pieces where both claim the same
		// register (seen on ARM64 with go1.26rc1). Interface types have two
		// distinct pointers that can never share a register.
		if len(loclist.Pieces) == 0 ||
			hasDuplicateInterfacePieces(op.Variable.Type, loclist.Pieces) {
			break
		}

		layoutPieces, err := g.typeMemoryLayout(op.Variable.Type)
		if err != nil {
			return nil, err
		}
		layoutIdx := 0
		origLen := len(ops)
		for _, piece := range loclist.Pieces {
			if layoutIdx >= len(layoutPieces) {
				return nil, fmt.Errorf(
					"mismatch between loclist pieces and type memory layout for %s: %s",
					op.Variable.Name, op.Variable.Type.GetName(),
				)
			}
			paddedOffset := layoutPieces[layoutIdx].PaddedOffset
			nextLayoutIdx := layoutIdx
			for nextLayoutIdx < len(layoutPieces) &&
				layoutPieces[nextLayoutIdx].PaddedOffset-paddedOffset < uint32(piece.Size) {
				nextLayoutIdx++
			}
			// Layout pieces in [layoutIdx, nextLayoutIdx) range correspond to current locPiece.
			layoutIdx = nextLayoutIdx

			switch p := piece.Op.(type) {
			case nil:
				// If this piece is unavailable, only bail out if it
				// overlaps with the requested byte range. This avoids
				// rejecting narrowed field captures (e.g. foo.bar) when an
				// unrelated field in the same parent struct is unavailable.
				pieceEnd := paddedOffset + piece.Size
				if op.Offset < pieceEnd && paddedOffset < op.Offset+op.ByteSize {
					// Overlaps with requested range — variable is
					// partially unavailable, treat as unavailable.
					// Discard any ops emitted for earlier pieces.
					ops = ops[:origLen]
					break outer
				}
			case ir.Register:
				// Register pieces are small and map to individual layout
				// pieces. Check whether this piece's padded position falls
				// within the requested range.
				if op.Offset <= paddedOffset && paddedOffset < op.Offset+op.ByteSize {
					if piece.Size > 8 {
						return nil, fmt.Errorf("unsupported register size: %d", piece.Size)
					}
					ops = append(ops, ExprReadRegisterOp{
						Register:     p.RegNo,
						Size:         uint8(piece.Size),
						OutputOffset: paddedOffset - op.Offset,
					})
				}
			case ir.Cfa:
				// CFA pieces represent contiguous memory on the stack that
				// already has correct padding. Compute the overlap between
				// this piece's range and the requested range, then read just
				// that portion.
				pieceEnd := paddedOffset + piece.Size
				reqEnd := op.Offset + op.ByteSize
				overlapStart := max(op.Offset, paddedOffset)
				overlapEnd := min(reqEnd, pieceEnd)
				if overlapStart < overlapEnd {
					cfaOff := p.CfaOffset + int32(overlapStart-paddedOffset)
					ops = append(ops, ExprDereferenceCfaOp{
						Offset:       cfaOff,
						Len:          overlapEnd - overlapStart,
						OutputOffset: overlapStart - op.Offset,
					})
				}
			case ir.Addr:
				return nil, errUnsupportedAddrLocationOp
			default:
				return nil, fmt.Errorf(
					"internal error: unexpected piece op: %#v (%T)", p, p,
				)
			}
		}
		return ops, nil
	}
	// Variable is not available, just return. Expression ops are allowed to "return early" on error.
	ops = append(ops, ReturnOp{})
	return ops, nil
}

var errUnsupportedAddrLocationOp = errors.New("unsupported addr location op")
