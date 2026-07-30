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

// exprStatusIdxNone signals that a generated stack-machine op is not
// associated with any per-expression status slot — it mirrors the
// EXPR_STATUS_IDX_NONE sentinel in pkg/dyninst/ebpf/stack_machine.h.
const exprStatusIdxNone uint32 = ^uint32(0)

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
	NumProbes        uint32
	GoModuledataInfo ir.GoModuledataInfo
	GoMapHashInfo    ir.GoMapHashInfo
	CommonTypes      ir.CommonTypes
	IsARM64          bool
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
		// probe_id on call_depths_entry_t is uint16 (the rest of the
		// 16-bit budget there carries condition_state for split-event-kind
		// conditions). Only probes that participate in entry/return
		// pairing read or write that field — line probes never touch
		// in_progress_calls and can have arbitrary probe_id. Reject only
		// when a probe with HasAssociatedReturn would have an index that
		// can't fit. Real-world programs have tens to hundreds of probes;
		// this guard exists for stress-test paths (e.g. symdb-generated
		// all-methods probes) so they can co-exist with split conditions
		// when the count happens to fit.
		if probeIdx >= 1<<16 {
			for _, inst := range probe.Instances {
				for _, event := range inst.Events {
					for _, injectionPoint := range event.InjectionPoints {
						if injectionPoint.HasAssociatedReturn {
							return Program{}, fmt.Errorf(
								"too many probes (%d): probe_id must fit in uint16 "+
									"for method probes that pair entry with return; "+
									"got probe %d with HasAssociatedReturn",
								len(program.Probes), probeIdx,
							)
						}
					}
				}
			}
		}
		// Determine which event kind has a condition across all instances,
		// and whether the condition is split (both entry and return events
		// carry a condition; happens when the user's compound condition
		// references variables from both kinds — see irgen's
		// analyzedCondition.splitCondition).
		var conditionEventKind ir.EventKind
		var entryHasCond, returnHasCond bool
		for _, inst := range probe.Instances {
			for _, event := range inst.Events {
				if event.Condition == nil {
					continue
				}
				switch event.Kind {
				case ir.EventKindEntry:
					entryHasCond = true
				case ir.EventKindReturn:
					returnHasCond = true
				}
				if conditionEventKind == 0 {
					conditionEventKind = event.Kind
				}
			}
		}
		splitCondition := entryHasCond && returnHasCond

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
				throttleMode := computeThrottleMode(event, conditionEventKind, splitCondition)
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
		NumProbes:        uint32(len(program.Probes)),
		GoModuledataInfo: program.GoModuledataInfo,
		GoMapHashInfo:    program.GoMapHashInfo,
		CommonTypes:      program.CommonTypes,
		IsARM64:          program.IsARM64,
	}, nil
}

// computeThrottleMode determines the throttle mode for an event based on
// whether this event or its sibling has a condition.
//
// For non-split conditions: at most one event per probe carries a
// condition, and that event throttles after its check; the sibling skips
// throttling so the condition decides.
//
// For split conditions (splitCondition == true): both events carry a
// condition. The entry condition is the gate for whether the return
// event will fire (entry's condition_failed=true → no in_progress_calls
// insertion → return sees CALL_DEPTHS_ABSENT and is suppressed). To
// avoid double-throttling the same logical probe firing, the entry
// uses ThrottleNone and the return is the canonical decision point with
// ThrottleAfterCondCheck.
func computeThrottleMode(
	event *ir.Event,
	conditionEventKind ir.EventKind,
	splitCondition bool,
) ThrottleMode {
	hasCond := event.Condition != nil
	isReturn := event.Kind == ir.EventKindReturn

	if splitCondition {
		// Both events carry a condition. Throttle only the return so
		// the user's per-second budget isn't halved.
		if isReturn {
			return ThrottleAfterCondCheck
		}
		return ThrottleNone
	}
	if hasCond {
		// This event has a condition: throttle after condition check.
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
		// StringSizeLimit is forwarded as configured. The BPF stack
		// machine clamps to MAX_DATA_ITEM_SIZE before serialization
		// so an oversized maxLength produces a truncated capture
		// rather than a silent skip; see pkg/dyninst/ebpf/stack_machine.h.
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
//
// Split-event-kind conditions are signalled by condition.IsSplit (set by
// irgen when building either the entry-side driver or the return-side AST
// replay). In that case the implicit ConditionBeginOp prelude is skipped
// — the per-leaf record/load ops manage condition_eval_error directly.
// When the Expression has LeafBodies (the entry-side driver emits them;
// the return-side replay does not), this also generates one
// ProcessConditionLeaf sub-function per body before emitting the main
// handler. The driver's IR ConditionLeafEvalOp then lowers to a CallOp +
// ConditionLeafRecordOp pair pointing at the corresponding leaf function.
func (g *generator) addConditionHandler(
	injectionPC uint64,
	rootType *ir.EventRootType,
	condition *ir.Expression,
) (FunctionID, error) {
	// Generate per-leaf sub-functions first so the driver's
	// ConditionLeafEvalOp can call them.
	leafFnIDs := make([]FunctionID, len(condition.LeafBodies))
	for i, body := range condition.LeafBodies {
		fnID, err := g.addConditionLeafHandler(injectionPC, rootType, uint8(i), body)
		if err != nil {
			return nil, err
		}
		leafFnIDs[i] = fnID
	}

	id := ProcessCondition{
		EventRootType: rootType,
		InjectionPC:   injectionPC,
	}
	ops := make([]Op, 0, 5+len(condition.Operations))
	if !condition.IsSplit {
		ops = append(ops, ConditionBeginOp{})
	}
	ops = append(ops, ExprPrepareOp{})
	ops, err := g.appendConditionOps(injectionPC, condition.Operations, ops, leafFnIDs)
	if err != nil {
		return nil, err
	}
	ops = append(ops, ReturnOp{})
	if err := g.addFunction(id, ops); err != nil {
		return nil, err
	}
	return id, nil
}

// addConditionLeafHandler generates the SM sub-function for one entry-side
// leaf of a split-event-kind condition.
//
// The leaf signals its outcome to the driver's ConditionLeafRecord op via
// condition_eval_error:
//   - On success: ConditionBeginOp arms condition_eval_error at the prelude;
//     the leaf's body writes a boolean byte at sm->offset; the trailing
//     ConditionLeafCompleteOp clears the flag, so the record op sees
//     eval_error=false and reads the boolean.
//   - On abort (nil deref / OOB / deref fail): the existing condition error
//     paths call sm_return without clearing the flag, so the record op sees
//     eval_error=true and encodes an error status from condition_nil_deref.
func (g *generator) addConditionLeafHandler(
	injectionPC uint64,
	rootType *ir.EventRootType,
	leafIdx uint8,
	body *ir.Expression,
) (FunctionID, error) {
	id := ProcessConditionLeaf{
		EventRootType: rootType,
		InjectionPC:   injectionPC,
		LeafIdx:       leafIdx,
	}
	ops := make([]Op, 0, 4+len(body.Operations))
	ops = append(ops, ConditionBeginOp{})
	ops = append(ops, ExprPrepareOp{})
	ops, err := g.appendConditionOps(injectionPC, body.Operations, ops, nil /* no nested leaves */)
	if err != nil {
		return nil, err
	}
	ops = append(ops, ConditionLeafCompleteOp{})
	ops = append(ops, ReturnOp{})
	if err := g.addFunction(id, ops); err != nil {
		return nil, err
	}
	return id, nil
}

// appendConditionOps lowers a slice of IR ExpressionOps into compiler Ops
// and appends them to `ops`. Used by both the main condition driver and
// per-leaf sub-functions so they share the same translation table.
func (g *generator) appendConditionOps(
	injectionPC uint64,
	irOps []ir.ExpressionOp,
	ops []Op,
	leafFnIDs []FunctionID,
) ([]Op, error) {
	for _, op := range irOps {
		switch op := op.(type) {
		case *ir.LocationOp:
			opsAfter, err := g.EncodeLocationOp(injectionPC, op, exprStatusIdxNone, ops)
			if err != nil {
				logLocationIssue(
					"error encoding location op for condition: %v", err,
				)
				opsAfter = append(ops, ReturnOp{})
			}
			ops = opsAfter
		case *ir.DereferenceOp:
			ops = append(ops, ExprDereferencePtrOp{
				Bias:          op.Bias,
				Len:           op.ByteSize,
				ExprStatusIdx: exprStatusIdxNone,
				NullAsZero:    op.NullAsZero,
			})
		case *ir.ExprPushOffsetOp:
			ops = append(ops, ExprPushOffsetOp{ByteSize: op.ByteSize})
		case *ir.ExprLoadLiteralOp:
			ops = append(ops, ExprLoadLiteralOp{Data: op.Data})
		case *ir.ExprReadStringOp:
			ops = append(ops, ExprReadStringOp{MaxLen: op.MaxLen})
		case *ir.ExprCmpBaseOp:
			ops = append(ops, ExprCmpBaseOp{
				Op:       op.Op,
				Kind:     op.Kind,
				ByteSize: op.ByteSize,
			})
		case *ir.ExprCmpStringOp:
			ops = append(ops, ExprCmpStringOp{Op: op.Op})
		case *ir.SliceBoundsCheckOp:
			ops = append(ops, ExprSliceBoundsCheckOp{
				Index:         op.Index,
				ExprStatusIdx: exprStatusIdxNone,
			})
		case *ir.SwissMapLookupOp:
			ops = append(ops, swissMapOps(op, exprStatusIdxNone)...)
		case *ir.ConditionCheckOp:
			ops = append(ops, ConditionCheckOp{})
		case *ir.CondNotOp:
			ops = append(ops, CondNotOp{})
		case *ir.CondJumpOp:
			ops = append(ops, CondJumpOp{Cond: op.Cond, Label: op.Target})
		case *ir.CondLabelOp:
			ops = append(ops, CondLabelOp{ID: op.ID})
		case *ir.ExprPrepareOp:
			ops = append(ops, ExprPrepareOp{})
		case *ir.ConditionStateInitOp:
			ops = append(ops, ConditionStateInitOp{})
		case *ir.ConditionLeafEvalOp:
			if int(op.LeafIdx) >= len(leafFnIDs) {
				return nil, fmt.Errorf(
					"internal: ConditionLeafEvalOp leaf index %d out of range (%d leaves)",
					op.LeafIdx, len(leafFnIDs),
				)
			}
			ops = append(ops,
				CallOp{FunctionID: leafFnIDs[op.LeafIdx]},
				ConditionLeafRecordOp{LeafIdx: op.LeafIdx},
			)
		case *ir.ConditionLeafLoadOp:
			ops = append(ops, ConditionLeafLoadOp{
				LeafIdx: op.LeafIdx,
				Label:   op.ErrorTarget,
			})
		case *ir.ConditionCheckPreserveErrorOp:
			ops = append(ops, ConditionCheckPreserveErrorOp{})
		case *ir.ExprLoadAddressOp:
			ops = appendExprLoadAddress(ops, injectionPC, op)
		case *ir.ArrayLoopBeginOp:
			ops = append(ops, ArrayLoopBeginOp{
				Quantifier:     op.Quantifier,
				ElemByteSize:   op.ElemByteSize,
				CompileTimeLen: op.CompileTimeLen,
				EndLabel:       op.EndLabel,
			})
		case *ir.ArrayLoopEndOp:
			ops = append(ops, ArrayLoopEndOp{
				BodyLabel: op.BodyLabel,
			})
		case *ir.SliceLoopBeginOp:
			ops = append(ops, SliceLoopBeginOp{
				Quantifier:   op.Quantifier,
				ElemByteSize: op.ElemByteSize,
				EndLabel:     op.EndLabel,
			})
		case *ir.SliceLoopEndOp:
			ops = append(ops, SliceLoopEndOp{
				BodyLabel: op.BodyLabel,
			})
		case *ir.SwissMapLoopBeginOp:
			ops = append(ops, SwissMapLoopBeginOp{
				Quantifier:               op.Quantifier,
				KeyByteSize:              op.KeyByteSize,
				ValByteSize:              op.ValByteSize,
				EndLabel:                 op.EndLabel,
				DirPtrOffset:             op.DirPtrOffset,
				DirLenOffset:             op.DirLenOffset,
				CtrlOffset:               op.CtrlOffset,
				SlotsOffset:              op.SlotsOffset,
				KeyInSlotOffset:          op.KeyInSlotOffset,
				ValInSlotOffset:          op.ValInSlotOffset,
				SlotSize:                 op.SlotSize,
				GroupByteSize:            op.GroupByteSize,
				TableGroupsFieldOffset:   op.TableGroupsFieldOffset,
				GroupsDataFieldOffset:    op.GroupsDataFieldOffset,
				GroupsLenMaskFieldOffset: op.GroupsLenMaskFieldOffset,
			})
		case *ir.SwissMapLoopEndOp:
			ops = append(ops, SwissMapLoopEndOp{
				BodyLabel: op.BodyLabel,
			})
		default:
			panic(fmt.Sprintf("unexpected ir.Operation in condition: %#v", op))
		}
	}
	return ops, nil
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
			opsAfter, err := g.EncodeLocationOp(injectionPC, op, exprIdx, ops)
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
				Bias:          op.Bias,
				Len:           op.ByteSize,
				ExprStatusIdx: exprIdx,
				NullAsZero:    op.NullAsZero,
			})
		case *ir.ExprPushOffsetOp:
			ops = append(ops, ExprPushOffsetOp{ByteSize: op.ByteSize})
		case *ir.ExprLoadLiteralOp:
			ops = append(ops, ExprLoadLiteralOp{Data: op.Data})
		case *ir.ExprReadStringOp:
			ops = append(ops, ExprReadStringOp{MaxLen: op.MaxLen})
		case *ir.ExprCmpBaseOp:
			ops = append(ops, ExprCmpBaseOp{
				Op:       op.Op,
				Kind:     op.Kind,
				ByteSize: op.ByteSize,
			})
		case *ir.ExprCmpStringOp:
			ops = append(ops, ExprCmpStringOp{Op: op.Op})
		case *ir.SliceBoundsCheckOp:
			// After the bounds check, the scratch still starts with the
			// data pointer (8 bytes). Update lastOpSize so the following
			// DereferenceOp sees a pointer-sized value.
			lastOpSize = 8
			ops = append(ops, ExprSliceBoundsCheckOp{
				Index:         op.Index,
				ExprStatusIdx: exprIdx,
			})
		case *ir.SwissMapLookupOp:
			// The lookup writes the value element at sm->offset on success.
			lastOpSize = op.ValByteSize
			ops = append(ops, swissMapOps(op, exprIdx)...)
		case *ir.PanicUnwindPrepareOp:
			ops = append(ops, PanicUnwindPrepareOp{})
		case *ir.PanicUnwindEvictSlotsOp:
			ops = append(ops, PanicUnwindEvictSlotsOp{})
		// any/all loop and conditional-control ops. These are emitted by
		// emitAnyAllLoop and the predicate-body lowering when an any/all
		// (or its desugared contains form) appears in template-segment or
		// capture-expression position. Encoding mirrors appendConditionOps;
		// the loop / control flow leaves a single boolean byte at sm->offset
		// when it completes, which ExprSaveOp then records as the bool
		// expression result. lastOpSize is left untouched: none of these ops
		// is followed by an ir.DereferenceOp in the IR sequences emitAnyAllLoop
		// produces.
		case *ir.CondNotOp:
			ops = append(ops, CondNotOp{})
		case *ir.CondJumpOp:
			ops = append(ops, CondJumpOp{Cond: op.Cond, Label: op.Target})
		case *ir.CondLabelOp:
			ops = append(ops, CondLabelOp{ID: op.ID})
		case *ir.ExprLoadAddressOp:
			ops = appendExprLoadAddress(ops, injectionPC, op)
		case *ir.ArrayLoopBeginOp:
			ops = append(ops, ArrayLoopBeginOp{
				Quantifier:     op.Quantifier,
				ElemByteSize:   op.ElemByteSize,
				CompileTimeLen: op.CompileTimeLen,
				EndLabel:       op.EndLabel,
			})
		case *ir.ArrayLoopEndOp:
			ops = append(ops, ArrayLoopEndOp{
				BodyLabel: op.BodyLabel,
			})
		case *ir.SliceLoopBeginOp:
			ops = append(ops, SliceLoopBeginOp{
				Quantifier:   op.Quantifier,
				ElemByteSize: op.ElemByteSize,
				EndLabel:     op.EndLabel,
			})
		case *ir.SliceLoopEndOp:
			ops = append(ops, SliceLoopEndOp{
				BodyLabel: op.BodyLabel,
			})
		case *ir.SwissMapLoopBeginOp:
			ops = append(ops, SwissMapLoopBeginOp{
				Quantifier:               op.Quantifier,
				KeyByteSize:              op.KeyByteSize,
				ValByteSize:              op.ValByteSize,
				EndLabel:                 op.EndLabel,
				DirPtrOffset:             op.DirPtrOffset,
				DirLenOffset:             op.DirLenOffset,
				CtrlOffset:               op.CtrlOffset,
				SlotsOffset:              op.SlotsOffset,
				KeyInSlotOffset:          op.KeyInSlotOffset,
				ValInSlotOffset:          op.ValInSlotOffset,
				SlotSize:                 op.SlotSize,
				GroupByteSize:            op.GroupByteSize,
				TableGroupsFieldOffset:   op.TableGroupsFieldOffset,
				GroupsDataFieldOffset:    op.GroupsDataFieldOffset,
				GroupsLenMaskFieldOffset: op.GroupsLenMaskFieldOffset,
			})
		case *ir.SwissMapLoopEndOp:
			ops = append(ops, SwissMapLoopEndOp{
				BodyLabel: op.BodyLabel,
			})
		case *ir.EmitFilterSliceMarkerOp:
			// Inline-pass marker for filter(slice, pred). Leaves the
			// 8-byte source data_ptr at sm->offset; the trailing
			// ExprSaveOp copies those 8 bytes into the event-root slot.
			lastOpSize = 8
			ops = append(ops, EmitFilterSliceMarkerOp{
				FilterDataTypeID: op.FilterDataTypeID,
				ElemByteSize:     op.ElemByteSize,
			})
		case *ir.EmitFilterMapMarkerOp:
			// Inline-pass marker for filter(map, pred). Leaves the
			// 8-byte source map pointer at sm->offset.
			lastOpSize = 8
			ops = append(ops, EmitFilterMapMarkerOp{
				FilterDataTypeID: op.FilterDataTypeID,
				SwissHeaderSize:  op.SwissHeaderSize,
				UsedFieldOffset:  op.UsedFieldOffset,
			})
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

	case *ir.DurationType:
		// Nothing to process; the ExprLoadDurationOp writes the value
		// directly at the expression's result offset.

	case *ir.TraceContextType:
		// Synthetic type. The enqueue subroutine is the chain-walk
		// program emitted at the StructureType case below for any
		// concrete context.Context implementation; this branch only
		// exists for the IR-visitor to be exhaustive.

	case *ir.GoHMapBucketType:
		if err := structureTypeHandler(t.StructureType); err != nil {
			return fid, needed, err
		}
	case *ir.GoTimeType:
		// time.Time is special-cased: the runtime resolves the *Location
		// pointer to a UTC offset and overwrites the captured pointer
		// slot in place. We deliberately do not invoke
		// structureTypeHandler here, which would chase the loc pointer
		// (potentially recursing through Location → []zone → strings).
		needed = true
		offsetShift = 0
		ops = []Op{
			ProcessGoTimeOp{
				WallFieldOffset:       t.WallFieldOffset,
				ExtFieldOffset:        t.ExtFieldOffset,
				LocFieldOffset:        t.LocFieldOffset,
				CacheResolved:         t.CacheResolved,
				CacheStartOffset:      t.CacheStartOffset,
				CacheEndOffset:        t.CacheEndOffset,
				CacheZoneOffset:       t.CacheZoneOffset,
				ZoneOffsetFieldOffset: t.ZoneOffsetFieldOffset,
				ZoneOffsetFieldSize:   t.ZoneOffsetFieldSize,
			},
			ReturnOp{},
		}

	case *ir.StructureType:
		if err := structureTypeHandler(t); err != nil {
			return fid, needed, err
		}
	case *ir.GoContextImplementationType:
		// An impl that is not a chain link (no parent context or key/value
		// payload) has nothing to walk, so capture its fields via the normal
		// struct-descent program like any other struct.
		if !t.HasChainData() {
			if err := structureTypeHandler(t.StructureType); err != nil {
				return fid, needed, err
			}
			break
		}
		// Concrete context.Context implementations (cancelCtx, valueCtx,
		// timerCtx, …) override the normal struct-descent program with a
		// chain-walk subroutine. INIT rewrites the just-serialized data
		// item header to TraceContextType and zeros the first 40 bytes
		// of payload; HOP performs one chain step per dispatch and
		// self-jumps until done. See pkg/dyninst/irgen/trace_context.md.
		needed = true
		offsetShift = 0
		ops = []Op{
			GoContextChainInitOp{ImplTypeID: t.GetID()},
			GoContextChainHopOp{},
			ReturnOp{},
		}
	case *ir.DDTraceSpanType:
		// The wrapper carries trace-correlation metadata for the BPF
		// chain walk; byte-level capture goes through the embedded
		// *StructureType via the standard struct-descent program (we
		// never directly chase this as anything other than a normal
		// pointee struct).
		if err := structureTypeHandler(t.StructureType); err != nil {
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
		// Pointers to context.Context implementations (e.g. *cancelCtx,
		// *valueCtx) are handled specially: enqueue_pc runs the chain-walk
		// directly here, bypassing the normal ProcessPointerOp chain that
		// would (a) cost a ttl decrement and (b) only land on the
		// underlying struct's chase one step later. The chain walk needs
		// the impl pointer (which is the value stored at the chase-
		// preamble buffer slot for this pointer-typed item), not the
		// address of the pointer itself, so SM_OP_GO_CONTEXT_CHAIN_INIT's
		// behavior on a pointer-typed item is to read sm->di_0's payload
		// (8 bytes containing the user-memory pointer) and use it as the
		// chain start. See pkg/dyninst/irgen/trace_context.md.
		if impl, isImpl := t.Pointee.(*ir.GoContextImplementationType); isImpl && impl.HasChainData() {
			needed = true
			offsetShift = 0
			ops = []Op{
				GoContextChainInitOp{ImplTypeID: impl.GetID()},
				GoContextChainHopOp{},
				ReturnOp{},
			}
			break
		}
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

	case *ir.GoFilteredSliceType:
		// The filter handle is 8 bytes (a source pointer). The decoder
		// uses the type ID to find the per-element data items; BPF has
		// no per-handle processing to do. We do need the data type's
		// enqueue_pc generated, so push it onto the type queue.
		g.typeQueue = append(g.typeQueue, t.Data)
		// needed stays false: no ProcessType call is required for the
		// handle, but addTypeHandler will still register an empty
		// function (matching how unused types are emitted today).

	case *ir.GoFilteredMapType:
		// Same as GoFilteredSliceType.
		g.typeQueue = append(g.typeQueue, t.Data)

	case *ir.GoFilteredSliceDataType:
		// Lower the stored EnqueueOps as the data type's enqueue_pc.
		// This is the deferred filter loop body.
		elemFunc, elemNeeded, err := g.addTypeHandler(t.Element)
		if err != nil {
			return fid, needed, err
		}
		lowered, err := lowerExpressionOps(g, t.EnqueueOps, elemFunc, elemNeeded)
		if err != nil {
			return fid, needed, err
		}
		needed = true
		offsetShift = 0
		ops = lowered

	case *ir.GoFilteredMapDataType:
		keyFunc, keyNeeded, err := g.addTypeHandler(t.KeyType)
		if err != nil {
			return fid, needed, err
		}
		valFunc, valNeeded, err := g.addTypeHandler(t.ValueType)
		if err != nil {
			return fid, needed, err
		}
		lowered, err := lowerMapFilterEnqueueOps(
			g, t.EnqueueOps,
			keyFunc, keyNeeded, t.KeyType,
			valFunc, valNeeded, t.ValueType,
			t.ValOffsetInPair,
		)
		if err != nil {
			return fid, needed, err
		}
		needed = true
		offsetShift = 0
		ops = lowered

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
		// The context-impl and dd-trace-span wrappers lay out exactly like the
		// struct they wrap; the extra metadata they carry is irrelevant to
		// register/stack layout.
		case *ir.GoContextImplementationType:
			if err := collectFields(t.RawFields, offset); err != nil {
				return err
			}
		case *ir.DDTraceSpanType:
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
		case *ir.BaseType, *ir.DurationType, *ir.GoChannelType, *ir.PointerType, *ir.VoidPointerType, *ir.GoMapType, *ir.GoSubroutineType:
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
		case *ir.GoTimeType:
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
// exprStatusIdx identifies the expression for writing a status-absent
// flag at runtime (used by expression lowering); conditions pass ^0 to
// indicate none.
func (g *generator) EncodeLocationOp(
	pc uint64, op *ir.LocationOp, exprStatusIdx uint32, ops []Op,
) ([]Op, error) {
	// @duration is a synthetic variable without DWARF locations. Its IR
	// LocationOp is always emitted with Offset=0 and ByteSize=8, and
	// resolves at BPF eval time to (ktime_ns - entry_ktime_ns) via a
	// dedicated opcode. The caller (condition or expression lowering)
	// wraps this output in the same way it would for a base type, so
	// the same ExprPushOffsetOp/ExprLoadLiteral/ExprCmpBase sequence
	// works regardless of the LHS origin.
	if op.Variable != nil && op.Variable.Role == ir.VariableRoleDuration {
		ops = append(ops, ExprLoadDurationOp{
			ExprStatusIdx: exprStatusIdx,
		})
		return ops, nil
	}
	// @it (any/all loop iterator): the bytes are already at sm->offset
	// in the loop's scratch slot. We just need to shift sm->offset by
	// op.Offset so the following ExprPushOffsetOp / ExprCmpBaseOp reads
	// the right field within @it. ByteSize is implicit in the following
	// PushOffsetOp{ByteSize} the caller emits.
	if op.Variable != nil && op.Variable.Role == ir.VariableRoleLoopIt {
		// Always emit an advance — the body needs sm->offset re-anchored
		// to the loop scratch slot on entry to every sub-expression, even
		// when Offset == 0. Previous body ops (PushOffset, CmpBase, etc.)
		// may have moved sm->offset away from the slot.
		//
		// LoopBaseOffset distinguishes the map @value variable (which
		// lives at the 8-byte-aligned value offset inside the slot) from
		// the @it variable (which lives at offset 0).
		ops = append(ops, ExprAdvanceOffsetOp{
			Offset: op.Variable.LoopBaseOffset + op.Offset,
		})
		return ops, nil
	}
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

// swissMapOps returns the 5-opcode sequence for a swiss map lookup.
func swissMapOps(op *ir.SwissMapLookupOp, exprStatusIdx uint32) []Op {
	return []Op{
		SwissMapSetupOp{
			KeyData:                  op.KeyData,
			IsStringKey:              op.IsStringKey,
			KeyByteSize:              op.KeyByteSize,
			ValByteSize:              op.ValByteSize,
			SeedOffset:               op.SeedOffset,
			DirPtrOffset:             op.DirPtrOffset,
			DirLenOffset:             op.DirLenOffset,
			GlobalShiftOffset:        op.GlobalShiftOffset,
			CtrlOffset:               op.CtrlOffset,
			SlotsOffset:              op.SlotsOffset,
			SlotSize:                 op.SlotSize,
			KeyInSlotOffset:          op.KeyInSlotOffset,
			ValInSlotOffset:          op.ValInSlotOffset,
			TableGroupsFieldOffset:   op.TableGroupsFieldOffset,
			GroupsDataFieldOffset:    op.GroupsDataFieldOffset,
			GroupsLenMaskFieldOffset: op.GroupsLenMaskFieldOffset,
			GroupByteSize:            op.GroupByteSize,
			HeaderByteSize:           op.HeaderByteSize,
			ExprStatusIdx:            exprStatusIdx,
			ExistenceOnly:            op.ExistenceOnly,
		},
		SwissMapAesencOp{},
		SwissMapHashFinishOp{},
		SwissMapProbeOp{},
		SwissMapCheckSlotOp{},
	}
}

var errUnsupportedAddrLocationOp = errors.New("unsupported addr location op")

// appendExprLoadAddress lowers an ir.ExprLoadAddressOp to compiler ops.
//
// In-place mode (op.Variable == nil): emits a single ExprLoadAddressOp that
// adds PointerBias to the 8-byte pointer already at sm->offset.
//
// Variable mode (op.Variable != nil): the variable's DWARF location must be
// fully CFA-based at this PC (a single CFA piece spanning all bytes from
// op.Offset upward). Register pieces are rejected — the address of a
// register-resident value cannot be taken. If the variable is unavailable
// or non-CFA, we emit a ReturnOp to leave the expression as absent (same
// fallback EncodeLocationOp uses).
func appendExprLoadAddress(ops []Op, pc uint64, op *ir.ExprLoadAddressOp) []Op {
	if op.Variable == nil {
		return append(ops, ExprLoadAddressOp{
			LocationKind: ExprAddressInPlace,
			PointerBias:  op.PointerBias,
		})
	}
	for _, loclist := range op.Variable.Locations {
		if pc < loclist.Range[0] || pc >= loclist.Range[1] {
			continue
		}
		// We require a single CFA piece covering op.Offset. Multi-piece or
		// register-backed locations have no representable address.
		if len(loclist.Pieces) != 1 {
			break
		}
		p, ok := loclist.Pieces[0].Op.(ir.Cfa)
		if !ok {
			break
		}
		if loclist.Pieces[0].Size == 0 {
			break
		}
		cfaOff := uint32(int64(p.CfaOffset) + int64(op.Offset))
		return append(ops, ExprLoadAddressOp{
			LocationKind: ExprAddressFromCfa,
			CfaOffset:    cfaOff,
			PointerBias:  op.PointerBias,
		})
	}
	// Variable is not available or not address-able. Return early.
	return append(ops, ReturnOp{})
}

// lowerExpressionOps lowers the IR EnqueueOps stored on a
// GoFilteredSliceDataType into compiler ops, producing the data type's
// enqueue_pc body. It is the slice-filter analog to
// appendConditionOps + addExpressionHandler combined: it handles the
// union of ops that can appear in a filter's enqueue_pc (predicate body
// ops + InitFilterSliceLoopOp / FilterSliceLoopStepOp + flow control).
//
// elemFunc / elemNeeded come from addTypeHandler(elementType); the
// step-op lowering inserts a CallOp into elemFunc only if elemNeeded.
// The lowered sequence terminates with ReturnOp.
func lowerExpressionOps(
	_ *generator,
	irOps []ir.ExpressionOp,
	elemFunc FunctionID,
	elemNeeded bool,
) ([]Op, error) {
	ops := make([]Op, 0, len(irOps)+8)
	for _, op := range irOps {
		switch op := op.(type) {
		// Predicate-body ops (shared with appendConditionOps).
		case *ir.LocationOp:
			// In a filter enqueue_pc the only valid LocationOp roles are
			// @it (loop iterator). The body's bytes are at sm->offset in
			// the loop scratch slot; emit an ExprAdvanceOffsetOp by the
			// variable's loop base + field offset.
			if op.Variable == nil || op.Variable.Role != ir.VariableRoleLoopIt {
				return nil, fmt.Errorf(
					"filter enqueue_pc: LocationOp must target @it (VariableRoleLoopIt), got role %v",
					op.Variable.Role,
				)
			}
			ops = append(ops, ExprAdvanceOffsetOp{
				Offset: op.Variable.LoopBaseOffset + op.Offset,
			})
		case *ir.DereferenceOp:
			ops = append(ops, ExprDereferencePtrOp{
				Bias:          op.Bias,
				Len:           op.ByteSize,
				ExprStatusIdx: exprStatusIdxNone,
				NullAsZero:    op.NullAsZero,
			})
		case *ir.SliceBoundsCheckOp:
			ops = append(ops, ExprSliceBoundsCheckOp{
				Index:         op.Index,
				ExprStatusIdx: exprStatusIdxNone,
			})
		case *ir.ExprPushOffsetOp:
			ops = append(ops, ExprPushOffsetOp{ByteSize: op.ByteSize})
		case *ir.ExprLoadLiteralOp:
			ops = append(ops, ExprLoadLiteralOp{Data: op.Data})
		case *ir.ExprReadStringOp:
			ops = append(ops, ExprReadStringOp{MaxLen: op.MaxLen})
		case *ir.ExprCmpBaseOp:
			ops = append(ops, ExprCmpBaseOp{
				Op:       op.Op,
				Kind:     op.Kind,
				ByteSize: op.ByteSize,
			})
		case *ir.ExprCmpStringOp:
			ops = append(ops, ExprCmpStringOp{Op: op.Op})
		case *ir.SwissMapLookupOp:
			ops = append(ops, swissMapOps(op, exprStatusIdxNone)...)
		case *ir.ConditionCheckOp:
			ops = append(ops, ConditionCheckOp{})
		case *ir.CondNotOp:
			ops = append(ops, CondNotOp{})
		case *ir.CondJumpOp:
			ops = append(ops, CondJumpOp{Cond: op.Cond, Label: op.Target})
		case *ir.CondLabelOp:
			ops = append(ops, CondLabelOp{ID: op.ID})

		// Filter-specific ops.
		case *ir.InitFilterSliceLoopOp:
			ops = append(ops, InitFilterSliceLoopOp{
				ElemByteSize:      op.ElemByteSize,
				IterScratchBudget: op.IterScratchBudget,
				EndLabel:          op.EndLabel,
			})
		case *ir.FilterSliceLoopStepOp:
			// Lower the step op into the explicit branch-emit-call-advance
			// sequence described in the plan. The predicate body has left
			// a 1-byte result at sm->offset; a CondJumpIfFalse over the
			// emit + element handler avoids per-element chasing when the
			// predicate rejected the element. After the call, the advance
			// op bumps data_ptr / remaining and jumps back to BodyLabel.
			skipCallLabel := ir.LabelID(0)
			if elemNeeded {
				// Allocate a fresh label local to this enqueue_pc by
				// using a sentinel ID derived from BodyLabel + 1000000.
				// Since labels are scoped to a function, we just need
				// uniqueness within the enqueue_pc; we already have
				// EndLabel + BodyLabel allocated from the irgen
				// labelAllocator at type-synthesis time. A simple
				// scheme: use BodyLabel + 0x40000000 as a "skip-call"
				// marker. To stay safe we pick a label that's clearly
				// out of range of normal allocation: BodyLabel +
				// 0x10000.
				skipCallLabel = op.BodyLabel + 0x10000
				ops = append(ops, CondJumpOp{
					Cond:  false, // jump when predicate result == 0
					Label: skipCallLabel,
				})
				ops = append(ops, EmitFilterSliceElementOp{
					ElemByteSize: op.ElemByteSize,
				})
				ops = append(ops, CallOp{FunctionID: elemFunc})
				ops = append(ops, CondLabelOp{ID: skipCallLabel})
			} else {
				// Element type has no nested pointers to chase. Just
				// conditionally emit; no call needed. The emit op itself
				// no-ops when the predicate is false (it reads the
				// result byte directly).
				//
				// To keep the BPF op semantics uniform, we still emit a
				// CondJumpIfFalse over the EmitOp so the emit only runs
				// on a true predicate. Allocate a skipEmit label.
				skipEmitLabel := op.BodyLabel + 0x10000
				ops = append(ops, CondJumpOp{
					Cond:  false,
					Label: skipEmitLabel,
				})
				ops = append(ops, EmitFilterSliceElementOp{
					ElemByteSize: op.ElemByteSize,
				})
				ops = append(ops, CondLabelOp{ID: skipEmitLabel})
			}
			ops = append(ops, FilterSliceAdvanceOp{
				ElemByteSize: op.ElemByteSize,
				BodyLabel:    op.BodyLabel,
			})
		default:
			return nil, fmt.Errorf(
				"unexpected ir.Operation in filter slice enqueue_pc: %T", op,
			)
		}
	}
	ops = append(ops, ReturnOp{})
	return ops, nil
}

// lowerMapFilterEnqueueOps is the map-filter analog of
// lowerExpressionOps. It handles the same predicate-body ops, plus
// InitFilterMapLoopOp / FilterMapLoopStepOp.
func lowerMapFilterEnqueueOps(
	g *generator,
	irOps []ir.ExpressionOp,
	keyFunc FunctionID, keyNeeded bool, keyType ir.Type,
	valFunc FunctionID, valNeeded bool, valType ir.Type,
	valOffsetInPair uint32,
) ([]Op, error) {
	ops := make([]Op, 0, len(irOps)+8)
	for _, op := range irOps {
		switch op := op.(type) {
		// Predicate-body ops.
		case *ir.LocationOp:
			if op.Variable == nil || op.Variable.Role != ir.VariableRoleLoopIt {
				return nil, fmt.Errorf(
					"filter map enqueue_pc: LocationOp must target @it/@value (VariableRoleLoopIt), got role %v",
					op.Variable.Role,
				)
			}
			ops = append(ops, ExprAdvanceOffsetOp{
				Offset: op.Variable.LoopBaseOffset + op.Offset,
			})
		case *ir.DereferenceOp:
			ops = append(ops, ExprDereferencePtrOp{
				Bias:          op.Bias,
				Len:           op.ByteSize,
				ExprStatusIdx: exprStatusIdxNone,
				NullAsZero:    op.NullAsZero,
			})
		case *ir.SliceBoundsCheckOp:
			ops = append(ops, ExprSliceBoundsCheckOp{
				Index:         op.Index,
				ExprStatusIdx: exprStatusIdxNone,
			})
		case *ir.ExprPushOffsetOp:
			ops = append(ops, ExprPushOffsetOp{ByteSize: op.ByteSize})
		case *ir.ExprLoadLiteralOp:
			ops = append(ops, ExprLoadLiteralOp{Data: op.Data})
		case *ir.ExprReadStringOp:
			ops = append(ops, ExprReadStringOp{MaxLen: op.MaxLen})
		case *ir.ExprCmpBaseOp:
			ops = append(ops, ExprCmpBaseOp{
				Op:       op.Op,
				Kind:     op.Kind,
				ByteSize: op.ByteSize,
			})
		case *ir.ExprCmpStringOp:
			ops = append(ops, ExprCmpStringOp{Op: op.Op})
		case *ir.SwissMapLookupOp:
			ops = append(ops, swissMapOps(op, exprStatusIdxNone)...)
		case *ir.ConditionCheckOp:
			ops = append(ops, ConditionCheckOp{})
		case *ir.CondNotOp:
			ops = append(ops, CondNotOp{})
		case *ir.CondJumpOp:
			ops = append(ops, CondJumpOp{Cond: op.Cond, Label: op.Target})
		case *ir.CondLabelOp:
			ops = append(ops, CondLabelOp{ID: op.ID})

		// Filter-specific map ops.
		case *ir.InitFilterMapLoopOp:
			ops = append(ops, InitFilterMapLoopOp{
				KeyByteSize:              op.KeyByteSize,
				ValByteSize:              op.ValByteSize,
				ValOffsetInPair:          op.ValOffsetInPair,
				IterScratchBudget:        op.IterScratchBudget,
				EndLabel:                 op.EndLabel,
				DirPtrOffset:             op.DirPtrOffset,
				DirLenOffset:             op.DirLenOffset,
				CtrlOffset:               op.CtrlOffset,
				SlotsOffset:              op.SlotsOffset,
				KeyInSlotOffset:          op.KeyInSlotOffset,
				ValInSlotOffset:          op.ValInSlotOffset,
				SlotSize:                 op.SlotSize,
				GroupByteSize:            op.GroupByteSize,
				TableGroupsFieldOffset:   op.TableGroupsFieldOffset,
				GroupsDataFieldOffset:    op.GroupsDataFieldOffset,
				GroupsLenMaskFieldOffset: op.GroupsLenMaskFieldOffset,
			})
		case *ir.FilterMapLoopStepOp:
			// Same as the slice step but emits a (k, v) pair and
			// invokes the key and value handlers separately, with an
			// IncrementOutputOffsetOp between them that accounts for
			// the key handler's offsetShift.
			skipCallLabel := op.BodyLabel + 0x10000
			ops = append(ops, CondJumpOp{
				Cond:  false,
				Label: skipCallLabel,
			})
			ops = append(ops, EmitFilterMapElementOp{
				KeyByteSize:     uint32(0), // filled below
				ValByteSize:     uint32(0),
				ValOffsetInPair: valOffsetInPair,
			})
			// The EmitFilterMapElementOp's KeyByteSize/ValByteSize
			// come from InitFilterMapLoopOp above which already set
			// them via the BPF filter_loop_state — we reuse those at
			// runtime, so the EmitOp doesn't strictly need them.
			// However, for symmetry with the slice case we encode
			// them via the op so the BPF runtime has a self-contained
			// reference.
			//
			// Realize the values now by walking back to the matching
			// init op. Simpler: hoist them up via op.KeyTypeID/
			// op.ValueTypeID instead.
			keySize := keyType.GetByteSize()
			valSize := valType.GetByteSize()
			ops[len(ops)-1] = EmitFilterMapElementOp{
				KeyByteSize:     keySize,
				ValByteSize:     valSize,
				ValOffsetInPair: valOffsetInPair,
			}
			// sm->offset is now at the payload start (key bytes).
			keyShift := uint32(0)
			if keyNeeded {
				ops = append(ops, CallOp{FunctionID: keyFunc})
				keyShift = g.typeFuncMetadata[keyType.GetID()].offsetShift
			}
			if valNeeded {
				if valOffsetInPair > keyShift {
					ops = append(ops, IncrementOutputOffsetOp{
						Value: valOffsetInPair - keyShift,
					})
				}
				ops = append(ops, CallOp{FunctionID: valFunc})
			}
			ops = append(ops, CondLabelOp{ID: skipCallLabel})
			ops = append(ops, FilterMapAdvanceOp{
				BodyLabel: op.BodyLabel,
			})
		default:
			return nil, fmt.Errorf(
				"unexpected ir.Operation in filter map enqueue_pc: %T", op,
			)
		}
	}
	ops = append(ops, ReturnOp{})
	return ops, nil
}
