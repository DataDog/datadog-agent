// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compiler supports compiling probe ir into a stack machine program.
package compiler

//go:generate go run golang.org/x/tools/cmd/stringer -type=Opcode -trimprefix=Opcode

//revive:disable:exported
type Opcode uint8

const (
	OpcodeInvalid Opcode = iota
	OpcodeCall
	OpcodeReturn
	OpcodeIllegal
	OpcodeIncrementOutputOffset
	OpcodeExprPrepare
	OpcodeExprSave
	OpcodeExprDereferenceCfa
	OpcodeExprReadRegister
	OpcodeExprDereferencePtr
	OpcodeProcessPointer
	OpcodeProcessSlice
	OpcodeProcessArrayDataPrep
	OpcodeProcessSliceDataPrep
	OpcodeProcessSliceDataRepeat
	OpcodeProcessString
	OpcodeProcessGoEmptyInterface
	OpcodeProcessGoInterface
	OpcodeProcessGoDictType
	OpcodeProcessGoHmap
	OpcodeProcessGoSwissMap
	OpcodeProcessGoSwissMapGroups
	OpcodeChasePointers
	OpcodePrepareEventRoot
	// Condition expression ops.
	OpcodeExprPushOffset
	OpcodeExprLoadLiteral
	OpcodeExprReadString
	OpcodeExprCmpBase
	OpcodeExprCmpString
	OpcodeConditionCheck
	OpcodeConditionBegin
	OpcodeCallDictResolved
	OpcodeExprSliceBoundsCheck
	OpcodeSwissMapSetup
	OpcodeSwissMapAesenc
	OpcodeSwissMapHashFinish
	OpcodeSwissMapProbe
	OpcodeSwissMapCheckSlot
	// Compound condition opcodes.
	OpcodeCondNot
	OpcodeCondJumpIfFalse
	OpcodeCondJumpIfTrue
	OpcodeExprLoadDuration
	// Split-event-kind condition opcodes (per-leaf 2-bit status + AST replay).
	OpcodeConditionStateInit
	OpcodeConditionLeafRecord
	OpcodeConditionLeafLoad
	OpcodeConditionCheckPreserveError
	OpcodeConditionLeafComplete
	// Go context.Context chain-walk opcodes. The compiler emits the
	// subroutine [ChainInit, ChainHop, Return] as the enqueue_pc for any
	// concrete context.Context implementation IR type. INIT runs once after
	// the chase preamble has serialized the implementation's bytes — it
	// rewrites the just-written data item header's type to TraceContextType,
	// zeros the first 40 bytes of payload (establishing valid=0 baseline),
	// and initializes the SM's go_context_walk state. HOP runs one chain
	// step per dispatch; if not yet done it self-jumps (sm->pc -= 1) so
	// the next sm_loop iteration re-enters HOP. See pkg/dyninst/irgen/trace_context.md.
	OpcodeGoContextChainInit
	OpcodeGoContextChainHop
	// Time decoding.
	OpcodeProcessGoTime
	// Collection-predicate (any/all) opcodes.
	OpcodeExprLoadAddress
	OpcodeArrayLoopBegin
	OpcodeArrayLoopEnd
	OpcodeSliceLoopBegin
	OpcodeSliceLoopEnd
	OpcodeSwissMapLoopBegin
	OpcodeSwissMapLoopEnd
	OpcodeExprAdvanceOffset
	// Recovery probe opcodes — see ir.PanicUnwindPrepareOp /
	// PanicUnwindEvictSlotsOp.
	OpcodePanicUnwindPrepare
	OpcodePanicUnwindEvictSlots
	// Filter (deferred collection-filter) opcodes.
	OpcodeEmitFilterSliceMarker
	OpcodeEmitFilterMapMarker
	OpcodeInitFilterSliceLoop
	OpcodeEmitFilterSliceElement
	OpcodeFilterSliceAdvance
	OpcodeInitFilterMapLoop
	OpcodeEmitFilterMapElement
	OpcodeFilterMapAdvance
)

//revive:enable:exported
