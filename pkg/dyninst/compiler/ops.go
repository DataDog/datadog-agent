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

	// NullAsZero: if the pointer is null, write Len zero bytes at sm->offset
	// and continue instead of aborting with condition_nil_deref. See
	// ir.DereferenceOp.NullAsZero.
	NullAsZero bool
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

// GoContextChainInitOp is emitted at the head of the enqueue_pc subroutine
// for any concrete context.Context implementation IR type (and for pointer
// types whose Pointee is one). It rewrites the just-serialized data item
// header (written by the chase preamble) to use TraceContextType as its
// type, zeroes the first 40 bytes of payload, and initializes the SM's
// go_context_walk state. ImplTypeID is the IR type id of the
// context-impl struct (e.g. context.cancelCtx) — the chain walk's first
// hop uses this directly rather than re-resolving via go_runtime_type,
// because the impl's runtime type isn't always registered in the binary.
// See pkg/dyninst/irgen/trace_context.md.
type GoContextChainInitOp struct {
	baseOp
	ImplTypeID ir.TypeID
}

// GoContextChainHopOp is emitted after GoContextChainInitOp. It executes one
// step of the context-chain walk per dispatch: looks up the current link's
// IR type (using the IR type stashed by INIT for hop 0, otherwise resolving
// via go_runtime_type), tries to extract a dd-trace span via the value-key
// lookup, and either (a) writes the populated trace_context_t and terminates,
// (b) advances to the next link by reading the embedded Context field's
// interface header and self-jumps (sm->pc -= 1), or (c) terminates with
// valid=0 if the chain ends or a depth/error guard fires. Self-jumps up to
// MAX_GO_CONTEXT_DEPTH (32) times.
type GoContextChainHopOp struct {
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

// ProcessGoTimeOp adjusts a captured time.Time in place. It reads the loc
// pointer at LocFieldOffset and, when CacheResolved is true, performs one
// userspace probe-read on the *time.Location cache fields followed by a
// second read on cacheZone.offset. The 8 bytes at LocFieldOffset are then
// overwritten with either the resolved offset (in seconds east of UTC,
// sign-extended to int64) or the sentinel ir.GoTimeUnresolvedOffset
// (INT64_MIN) when the cache miss path is taken. The op does not enqueue
// the loc pointer for chasing.
type ProcessGoTimeOp struct {
	baseOp
	WallFieldOffset       uint32
	ExtFieldOffset        uint32
	LocFieldOffset        uint32
	CacheResolved         bool
	CacheStartOffset      uint32
	CacheEndOffset        uint32
	CacheZoneOffset       uint32
	ZoneOffsetFieldOffset uint32
	ZoneOffsetFieldSize   uint32
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

// ExprLoadDurationOp writes 8 bytes of (return_ktime_ns - entry_ktime_ns)
// at the current scratch offset. On non-return probes, where those
// timestamps are equal, it marks the enclosing expression's status as
// absent and aborts expression evaluation. It does not advance the
// offset or push onto the data stack — callers that use it as a
// comparison operand should follow with ExprPushOffsetOp{ByteSize: 8}.
type ExprLoadDurationOp struct {
	baseOp
	// ExprStatusIdx is the expression index for writing status-absent
	// on non-return probes; ^0 = none (used by conditions, which report
	// evaluation errors via a different channel).
	ExprStatusIdx uint32
}

type ExprLoadLiteralOp struct {
	baseOp
	Data []byte
}

type ExprReadStringOp struct {
	baseOp
	MaxLen uint16
}

type ExprCmpBaseOp struct {
	baseOp
	Op       ir.CmpOp
	Kind     ir.CmpKind
	ByteSize uint8
}

type ExprCmpStringOp struct {
	baseOp
	Op ir.CmpOp
}

type ExprSliceBoundsCheckOp struct {
	baseOp
	Index         uint32
	ExprStatusIdx uint32 // expression index for writing OOB status; ^0 = none
}

// SwissMapSetupOp reads bytecode params, computes hash, initializes probe state.
type SwissMapSetupOp struct {
	baseOp
	KeyData     []byte
	IsStringKey bool
	KeyByteSize uint8
	ValByteSize uint32

	SeedOffset        uint8
	DirPtrOffset      uint8
	DirLenOffset      uint8
	GlobalShiftOffset uint8

	CtrlOffset      uint8
	SlotsOffset     uint8
	SlotSize        uint16
	KeyInSlotOffset uint8
	ValInSlotOffset uint16

	TableGroupsFieldOffset   uint8
	GroupsDataFieldOffset    uint8
	GroupsLenMaskFieldOffset uint8
	GroupByteSize            uint16

	HeaderByteSize uint32
	ExprStatusIdx  uint32

	// ExistenceOnly switches the op to contains(map, key) semantics. See
	// ir.SwissMapLookupOp for details.
	ExistenceOnly bool
}

// SwissMapAesencOp performs one AESENC round; replays via PC for remaining rounds.
type SwissMapAesencOp struct{ baseOp }

// SwissMapHashFinishOp handles AES hash phase transitions and finalization.
type SwissMapHashFinishOp struct{ baseOp }

// SwissMapProbeOp reads the control word at the current group and computes match bitsets.
type SwissMapProbeOp struct{ baseOp }

// SwissMapCheckSlotOp checks one H2-matching slot against the literal key.
type SwissMapCheckSlotOp struct{ baseOp }

type ConditionBeginOp struct {
	baseOp
}

type ConditionCheckOp struct {
	baseOp
}

// CondNotOp flips the boolean result byte at sm->offset.
type CondNotOp struct {
	baseOp
}

// CondJumpOp branches past Right of a binary And/Or when the preceding leaf's
// result matches Cond (false -> and short-circuit, true -> or short-circuit).
// Label identifies the target, resolved at code-layout time.
type CondJumpOp struct {
	baseOp
	Cond  bool
	Label ir.LabelID
}

// CondLabelOp marks a jump target. Emits no bytes; the layout pass records
// its PC in codeTracker.labelLoc.
type CondLabelOp struct {
	baseOp
	ID ir.LabelID
}

// ConditionStateInitOp clears the per-SM condition_state to zero at the
// start of a split-event-kind entry-side condition driver.
type ConditionStateInitOp struct {
	baseOp
}

// ConditionLeafRecordOp captures the outcome of an entry-side leaf
// (called via SM_OP_CALL just before this op) into a 2-bit slot at
// position LeafIdx of the per-SM condition_state. Reads
// condition_eval_error / condition_nil_deref to detect and classify
// errors; on success reads the boolean byte at sm->offset. Clears both
// error flags so the next leaf's record sees fresh state.
type ConditionLeafRecordOp struct {
	baseOp
	LeafIdx uint8
}

// ConditionLeafLoadOp reads the 2-bit status for entry leaf LeafIdx from
// condition_state and dispatches:
//   - LEAF_FALSE → write 0 at sm->offset.
//   - LEAF_TRUE  → write 1 at sm->offset.
//   - LEAF_EVAL_ERROR / LEAF_NIL_DEREF → set condition_eval_error (and
//     nil_deref for the latter), write 1 at sm->offset, jump to Label.
//     The jump bypasses surrounding short-circuit and Not ops so the
//     eval-error flag survives to event.c's header surfacing.
type ConditionLeafLoadOp struct {
	baseOp
	LeafIdx uint8
	Label   ir.LabelID
}

// ConditionCheckPreserveErrorOp behaves like ConditionCheckOp (sets
// condition_failed when the byte at sm->offset is 0) but does NOT clear
// condition_eval_error. Used at the tail of split-event-kind condition
// drivers so an eval-error surfaced by ConditionLeafLoadOp during AST
// replay survives to event.c's header surfacing.
type ConditionCheckPreserveErrorOp struct {
	baseOp
}

// ConditionLeafCompleteOp clears condition_eval_error. Emitted at the
// tail of a per-leaf SM sub-function on the success path so the driver's
// ConditionLeafRecordOp can tell completion from abort. Abort paths in
// the leaf bypass this op via sm_return, leaving condition_eval_error
// armed by the leaf's prelude ConditionBeginOp.
type ConditionLeafCompleteOp struct {
	baseOp
}

// ExprAdvanceOffsetOp shifts sm->offset by Offset bytes. Used by the
// LocationOp lowering for `@it` (any/all loop iterator) to position
// sm->offset at a specific field within the @it scratch slot before the
// body's ExprPushOffsetOp/ExprCmpBaseOp sequence reads it.
type ExprAdvanceOffsetOp struct {
	baseOp
	Offset uint32
}

// ExprLoadAddressOp writes an 8-byte address at sm->offset.
//
// When LocationKind == ExprAddressFromCfa, the bytecode params carry CFA-base
// information for the variable; the BPF handler emits cfa + Offset.
//
// When LocationKind == ExprAddressInPlace, the bytecode is parameterless;
// the BPF handler expects an 8-byte pointer already at sm->offset and adds
// PointerBias to it in place.
type ExprLoadAddressOp struct {
	baseOp
	LocationKind ExprAddressLocationKind
	// CfaOffset is used when LocationKind == ExprAddressFromCfa. It is the
	// final byte offset (already including any DWARF location offset).
	CfaOffset uint32
	// PointerBias is added to the produced pointer regardless of kind.
	PointerBias uint32
}

// ExprAddressLocationKind selects how ExprLoadAddressOp computes the address.
type ExprAddressLocationKind uint8

const (
	// ExprAddressInPlace adds PointerBias to a pointer already at sm->offset.
	ExprAddressInPlace ExprAddressLocationKind = 1
	// ExprAddressFromCfa computes cfa + CfaOffset + PointerBias.
	ExprAddressFromCfa ExprAddressLocationKind = 2
)

// ArrayLoopBeginOp begins iteration over a Go array; see ir.ArrayLoopBeginOp.
type ArrayLoopBeginOp struct {
	baseOp
	Quantifier     ir.Quantifier
	ElemByteSize   uint32
	CompileTimeLen uint32
	EndLabel       ir.LabelID
}

// ArrayLoopEndOp closes an array loop body; see ir.ArrayLoopEndOp.
type ArrayLoopEndOp struct {
	baseOp
	BodyLabel ir.LabelID
}

// SliceLoopBeginOp begins iteration over a Go slice; see ir.SliceLoopBeginOp.
type SliceLoopBeginOp struct {
	baseOp
	Quantifier   ir.Quantifier
	ElemByteSize uint32
	EndLabel     ir.LabelID
}

// SliceLoopEndOp closes a slice loop body; see ir.SliceLoopEndOp.
type SliceLoopEndOp struct {
	baseOp
	BodyLabel ir.LabelID
}

// SwissMapLoopBeginOp begins iteration over a Go swiss-table map; see
// ir.SwissMapLoopBeginOp.
type SwissMapLoopBeginOp struct {
	baseOp
	Quantifier  ir.Quantifier
	KeyByteSize uint32
	ValByteSize uint32
	EndLabel    ir.LabelID

	DirPtrOffset             uint8
	DirLenOffset             uint8
	CtrlOffset               uint8
	SlotsOffset              uint8
	KeyInSlotOffset          uint8
	ValInSlotOffset          uint16
	SlotSize                 uint16
	GroupByteSize            uint16
	TableGroupsFieldOffset   uint8
	GroupsDataFieldOffset    uint8
	GroupsLenMaskFieldOffset uint8
}

// SwissMapLoopEndOp closes a swiss-map loop body; see ir.SwissMapLoopEndOp.
type SwissMapLoopEndOp struct {
	baseOp
	BodyLabel ir.LabelID
}

// PanicUnwindPrepareOp validates the recovered panic and computes
// (panic_lo_depth, panic_hi_depth) into the event header. Sets
// condition_failed on validation failure so probe_run aborts the
// event. Carries no operands.
type PanicUnwindPrepareOp struct {
	baseOp
}

// PanicUnwindEvictSlotsOp walks in_progress_calls[goid] and zeroes
// every call_depths_entry_t whose depth lies in
// (panic_lo_depth, panic_hi_depth]. Carries no operands.
type PanicUnwindEvictSlotsOp struct {
	baseOp
}

// EmitFilterSliceMarkerOp is the inline-pass op for a top-level
// filter(slice, pred) expression; see ir.EmitFilterSliceMarkerOp.
type EmitFilterSliceMarkerOp struct {
	baseOp
	FilterDataTypeID ir.TypeID
	ElemByteSize     uint32
}

// EmitFilterMapMarkerOp is the inline-pass op for a top-level
// filter(map, pred) expression; see ir.EmitFilterMapMarkerOp.
type EmitFilterMapMarkerOp struct {
	baseOp
	FilterDataTypeID ir.TypeID
	SwissHeaderSize  uint32
	UsedFieldOffset  uint32
}

// InitFilterSliceLoopOp is the first op inside a filter slice data
// type's enqueue_pc; see ir.InitFilterSliceLoopOp.
type InitFilterSliceLoopOp struct {
	baseOp
	ElemByteSize      uint32
	IterScratchBudget uint32
	EndLabel          ir.LabelID
}

// EmitFilterSliceElementOp emits a per-passing-element data item for a
// slice filter. It runs after the body has written a 1 byte at
// sm->offset and a preceding CondJumpIfFalse has skipped past this op
// when the predicate was false. On a true predicate the op reads the
// current source element from filter_loop_state.data_ptr into a fresh
// data-item payload at the buffer tail (header type =
// filter_data_type_id, length = ElemByteSize, address = output_index),
// then sets sm->offset to the just-emitted payload start so the
// trailing CallOp{ElementHandler} can chase nested pointers. Increments
// output_index. Handles flush-and-retry on buffer-full and emits a
// failure sentinel on read fault.
type EmitFilterSliceElementOp struct {
	baseOp
	ElemByteSize uint32
}

// FilterSliceAdvanceOp closes the slice filter loop body. Advances
// data_ptr / remaining; if remaining > 0 it reserves room for the next
// iteration's @it (flush if needed), reads the next element into @it
// scratch, and jumps to BodyLabel. Otherwise falls through.
type FilterSliceAdvanceOp struct {
	baseOp
	ElemByteSize uint32
	BodyLabel    ir.LabelID
}

// InitFilterMapLoopOp is the first op inside a filter map data type's
// enqueue_pc; see ir.InitFilterMapLoopOp.
type InitFilterMapLoopOp struct {
	baseOp
	KeyByteSize       uint32
	ValByteSize       uint32
	ValOffsetInPair   uint32
	IterScratchBudget uint32
	EndLabel          ir.LabelID

	DirPtrOffset             uint8
	DirLenOffset             uint8
	CtrlOffset               uint8
	SlotsOffset              uint8
	KeyInSlotOffset          uint8
	ValInSlotOffset          uint16
	SlotSize                 uint16
	GroupByteSize            uint16
	TableGroupsFieldOffset   uint8
	GroupsDataFieldOffset    uint8
	GroupsLenMaskFieldOffset uint8
}

// EmitFilterMapElementOp emits a per-passing-(k,v) data item for a map
// filter; analogous to EmitFilterSliceElementOp but reads key and value
// separately from the swiss-map slot's distinct field offsets and lays
// them out as [key][padding to 8-byte-align][value].
type EmitFilterMapElementOp struct {
	baseOp
	KeyByteSize     uint32
	ValByteSize     uint32
	ValOffsetInPair uint32
}

// FilterMapAdvanceOp closes the map filter loop body. Advances the
// swiss-map slot cursor (consume after a successful step), checks
// for end of iteration, reads the next (k, v) pair if available, and
// jumps to BodyLabel.
type FilterMapAdvanceOp struct {
	baseOp
	BodyLabel ir.LabelID
}

//revive:enable:exported
