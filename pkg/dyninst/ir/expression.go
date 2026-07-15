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
	ExprStatusAbsent    ExprStatus = 0 // evaluation failed (unknown reason)
	ExprStatusPresent   ExprStatus = 1 // evaluation succeeded
	ExprStatusNilDeref  ExprStatus = 2 // nil pointer dereference
	ExprStatusOOB       ExprStatus = 3 // index out of bounds
	ExprStatusTruncated ExprStatus = 4 // value present, but collection was truncated to the iteration cap (filter only, today)
)

// ExprStatusBits is the number of bits per entry in the ExprStatusArray.
// Must stay in sync with EXPR_STATUS_BITS in pkg/dyninst/ebpf/stack_machine.h.
const ExprStatusBits = 4

// Expression is a typed and validated set of operations for compilation
// and evaluation.
type Expression struct {
	// The type of the expression.
	Type Type
	// The operations that make up the expression, in reverse-polish notation.
	Operations []ExpressionOp
	// LeafBodies holds the per-leaf sub-expressions for split-event-kind
	// conditions. Indexed by leaf index (matching ConditionLeafEvalOp.LeafIdx
	// and ConditionLeafLoadOp.LeafIdx). Each leaf is compiled to its own
	// SM sub-function so leaf-internal aborts return to the entry-side
	// driver rather than the event handler. Nil for non-split conditions.
	LeafBodies []*Expression
	// IsSplit marks a split-event-kind condition program (the entry-side
	// driver or the return-side AST replay). When true, the compiler
	// skips the implicit ConditionBeginOp prelude and emits
	// ConditionCheckPreserveErrorOp at the tail instead of the regular
	// ConditionCheckOp: the per-leaf record / load ops in such programs
	// manage condition_eval_error directly, and the begin/check
	// arm/clear lifecycle would corrupt that.
	IsSplit bool
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

	// NullAsZero, when true, suppresses the nil-deref abort: on a null
	// pointer the op writes ByteSize zero bytes at sm->offset and
	// continues, instead of setting condition_nil_deref and aborting.
	// Used by contains(map, key) so that contains(nil_map, k) evaluates
	// to bool-false (the zero header passes through into SwissMapLookupOp,
	// which detects dir_ptr == 0 and writes the bool).
	NullAsZero bool
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
//
// The u32 len holds the *original* Go string length (may exceed MaxLen);
// the bytes block holds only the first min(len, MaxLen) bytes — that is
// what the offset advances by. ExprCmpStringOp uses the true length for
// length-sensitive semantics (eq length-check, lexicographic length tie-break)
// and clamps byte access to MaxLen so a truncated LHS sharing the literal's
// prefix never compares equal to the literal.
type ExprReadStringOp struct {
	MaxLen uint16
}

func (*ExprReadStringOp) irOp() {}

// ExprCmpBaseOp pops the LHS offset from the data stack, compares ByteSize
// bytes at LHS vs RHS (current offset) using Op + Kind, and writes a bool
// result (0 or 1) at the current offset. Used both for base-type comparison
// and for the 8-byte leading-pointer comparison that implements `== nil` /
// `!= nil` on nullable types (pointer, map, slice, interface).
type ExprCmpBaseOp struct {
	Op       CmpOp
	Kind     CmpKind
	ByteSize uint8
}

func (*ExprCmpBaseOp) irOp() {}

// ExprCmpStringOp pops the LHS offset from the data stack and compares two
// length-prefixed strings ([u32 len][bytes...]) using Op. Writes a bool
// result at the current offset. Lt/Le/Gt/Ge use lexicographic byte order;
// shorter strings sort below longer ones when the common prefix matches.
type ExprCmpStringOp struct {
	Op CmpOp
}

func (*ExprCmpStringOp) irOp() {}

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
// matching slot.
//
// Two modes are supported, selected by ExistenceOnly:
//
//   - Default (map index, ExistenceOnly=false): on success the value element
//     is written at sm->offset; on nil map or key-not-found ExprStatusOOB
//     is written and the expression is aborted.
//   - Existence-only (contains(map, key), ExistenceOnly=true): on found,
//     0x01 is written at sm->offset and the value dereference is skipped;
//     on nil map or key-not-found, 0x00 is written and the op continues
//     without setting OOB or aborting. sm->offset is left pointing at the
//     one-byte bool, matching the leaf contract of ExprCmpEqBaseOp.
type SwissMapLookupOp struct {
	// KeyData is the literal key encoded for comparison.
	// Base types: raw little-endian bytes (1–8 bytes).
	// Strings: [u32 len][bytes...] (max 4+MaxStringLiteralLength bytes).
	KeyData []byte

	// IsStringKey indicates the key is a Go string. When false the key is
	// a base type and compared by raw byte equality.
	IsStringKey bool

	// ExistenceOnly switches the op to contains(map, key) semantics: writes
	// a one-byte bool at sm->offset (1 on found, 0 on nil map or absent),
	// and skips the value dereference on key match. See the struct doc
	// comment for full details.
	ExistenceOnly bool

	// KeyByteSize is the in-memory size of the key type in bytes.
	// For base types: 1, 2, 4, or 8.
	// For strings: 16 (the Go string header: ptr + len).
	KeyByteSize uint8

	// ValByteSize is the in-memory size of the value element in bytes.
	// When ExistenceOnly is true this is set to 1 (the bool byte width)
	// and the value dereference is skipped.
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

// PanicUnwindPrepareOp validates that the goroutine has a recovered
// panic and computes the stack-byte-depth bounds of the unwound region
// from gp._panic.{startSP, sp} and gp.stack.hi. On success it writes
// (panic_lo_depth, panic_hi_depth) to the event header. On any
// validation failure (no panic, goexit, sp out of bounds, etc.) it
// sets condition_failed so probe_run aborts the event.
//
// Emitted as the first expression-time op of the synthesised
// runtime.recovery probe — before any LocationOp / DereferenceOp on
// the panic value, so the unwound-region bounds are available to a
// trailing PanicUnwindEvictSlotsOp.
type PanicUnwindPrepareOp struct{}

func (*PanicUnwindPrepareOp) irOp() {}

// PanicUnwindEvictSlotsOp walks in_progress_calls for the goroutine
// (via header.goid) and zeroes every call_depths_entry_t whose depth
// lies in (panic_lo_depth, panic_hi_depth]. If all slots end up empty
// the per-goroutine map entry is deleted.
//
// Emitted as the trailing op of the runtime.recovery probe's
// expression sequence, after the standard chase pointers op, so the
// BPF state cleanup happens only when the synthetic event has been
// fully assembled.
type PanicUnwindEvictSlotsOp struct{}

func (*PanicUnwindEvictSlotsOp) irOp() {}

// ConditionCheckOp reads a uint8 bool result at the current offset. If false
// (0), it sets the condition_failed flag and aborts the stack machine.
type ConditionCheckOp struct{}

func (*ConditionCheckOp) irOp() {}

// LabelID identifies a jump target within a single compiled condition
// handler. Allocated per condition, starting at 1.
type LabelID uint32

// CondNotOp flips the uint8 bool at the current offset (1 -> 0, 0 -> 1).
type CondNotOp struct{}

func (*CondNotOp) irOp() {}

// CondJumpOp jumps to Target when the uint8 bool at the current offset matches
// Cond. Cond == false is jump-if-false (used for and short-circuit); Cond ==
// true is jump-if-true (used for or short-circuit). The jump does NOT touch
// condition_eval_error — leaves after the jump can still fault, so the error
// arm set by ConditionBeginOp is kept in place until the tail
// ConditionCheckOp runs.
type CondJumpOp struct {
	Cond   bool
	Target LabelID
}

func (*CondJumpOp) irOp() {}

// CondLabelOp marks a jump target. Emits no bytes; just records its PC.
type CondLabelOp struct {
	ID LabelID
}

func (*CondLabelOp) irOp() {}

// ExprPrepareOp resets the SM's expression-result scratch frame to the
// current scratch_buf_len, ready for a fresh expression evaluation. The
// compiler emits this implicitly at the start of every condition handler
// (and again inside split-event-kind drivers between the per-leaf calls
// and the AST replay). At the IR level, ExprPrepareOp lets irgen request
// a re-prepare in the middle of a condition program; without it, the
// scratch frame established by the implicit prelude can be left in
// arbitrary state by per-leaf evaluations.
type ExprPrepareOp struct{}

func (*ExprPrepareOp) irOp() {}

// ConditionStateInitOp clears the per-SM condition_state scratch (uint16)
// to zero. Emitted at the start of a split-event-kind entry-side
// condition driver.
type ConditionStateInitOp struct{}

func (*ConditionStateInitOp) irOp() {}

// ConditionLeafEvalOp instructs the compiler to emit (a) a CallOp to the
// per-leaf SM sub-function for entry leaf LeafIdx and (b) a record op
// that captures the leaf's outcome (boolean / eval-error / nil-deref) as
// a 2-bit status into condition_state[LeafIdx]. Used in the entry-side
// driver before the AST replay.
type ConditionLeafEvalOp struct {
	LeafIdx uint8
}

func (*ConditionLeafEvalOp) irOp() {}

// ConditionLeafLoadOp reads the 2-bit status for entry leaf LeafIdx from
// condition_state and dispatches:
//   - LEAF_FALSE → write 0 at sm->offset, continue.
//   - LEAF_TRUE  → write 1 at sm->offset, continue.
//   - LEAF_EVAL_ERROR → set sm->condition_eval_error, write 1, jump to
//     ErrorTarget (the tail label of the surrounding condition program).
//   - LEAF_NIL_DEREF  → set both condition_eval_error and
//     condition_nil_deref, write 1, jump to ErrorTarget.
//
// The error→jump-to-tail behaviour intentionally bypasses surrounding
// short-circuit and Not ops; once an entry-leaf load surfaces an error
// the eval-error flag must propagate to event.c's header, regardless of
// surrounding boolean operators.
type ConditionLeafLoadOp struct {
	LeafIdx     uint8
	ErrorTarget LabelID
}

func (*ConditionLeafLoadOp) irOp() {}

// ConditionCheckPreserveErrorOp behaves like ConditionCheckOp (sets
// condition_failed when the byte at sm->offset is 0) but does NOT clear
// condition_eval_error. Used at the tail of a split-event-kind condition
// driver so that an eval-error surfaced by ConditionLeafLoadOp during
// AST replay survives to event.c's header surfacing.
type ConditionCheckPreserveErrorOp struct{}

func (*ConditionCheckPreserveErrorOp) irOp() {}

// ExprLoadAddressOp loads an 8-byte address into scratch at sm->offset.
//
// When Variable is non-nil, the op resolves Variable's DWARF location and
// produces the address `<location> + Offset`. The variable must be a memory
// location (CFA + offset); register-only locations are rejected at irgen
// time as a typed Issue.
//
// When Variable is nil, scratch at sm->offset is expected to already hold an
// 8-byte pointer (placed there by an immediately-preceding DereferenceOp
// chain that produced a pointer rather than reading through to the pointee).
// The op then adds PointerBias to those 8 bytes in place.
//
// In both modes the op leaves an 8-byte pointer at sm->offset and does not
// advance sm->offset; ArrayLoopBeginOp consumes the pointer in place.
type ExprLoadAddressOp struct {
	Variable    *Variable
	Offset      uint32
	PointerBias uint32
}

func (*ExprLoadAddressOp) irOp() {}

// Quantifier selects the any vs. all semantics for collection loop ops.
type Quantifier uint8

const (
	// QuantifierAny is the `any` quantifier: result is true if at least one
	// element satisfies the predicate; false if the collection is empty.
	QuantifierAny Quantifier = 1
	// QuantifierAll is the `all` quantifier: result is true if every element
	// satisfies the predicate; true if the collection is empty (vacuous).
	QuantifierAll Quantifier = 2
)

// CollectionPredicateMaxIterations is the cap on per-call iteration count
// inside an any/all loop. Larger collections still short-circuit normally;
// only the case where iteration exhausts the cap without a short-circuit
// result is flagged as an evaluation error.
//
// Must stay in sync with COLLECTION_PREDICATE_MAX_ITERATIONS in
// pkg/dyninst/ebpf/stack_machine.h.
const CollectionPredicateMaxIterations = 4096

// CollectionPredicateMaxElemBytes is the per-iteration scratch budget for
// @it in an any/all loop. Slice/array element size must be ≤ this value;
// for swiss maps the synthetic {key, value} entry (with 8-byte alignment
// between key and value) must also fit. Larger types are rejected at
// irgen time with a typed Issue.
//
// Must stay in sync with the upfront scratch reservation in
// sm_slice_loop_begin / sm_swissmap_loop_begin
// (`1 + CollectionPredicateMaxElemBytes + 16`).
const CollectionPredicateMaxElemBytes = 256

// ArrayLoopBeginOp begins iteration over a Go array. At entry, scratch at
// sm->offset holds an 8-byte pointer to the array base in user memory (placed
// there by an ExprLoadAddressOp). The array contents do NOT enter scratch;
// elements are streamed via bpf_probe_read_user into a fixed scratch slot.
//
// CompileTimeLen is known at irgen time; irgen rejects arrays with
// CompileTimeLen > COLLECTION_PREDICATE_MAX_ITERATIONS (4096), so the BPF
// handler does no runtime too-large check.
//
// Quantifier selects accumulator semantics (Any: init=0, exit on body=1;
// All: init=1, exit on body=0). On entry the op:
//   - stashes data_ptr / initial_len / elem_size / quantifier in
//     sm->slice_loop_state (the End op reads them from there)
//   - writes the initial accumulator byte at sm->offset
//   - if CompileTimeLen == 0, jumps to EndLabel (vacuous result is the init)
//   - reads the first element into the @it scratch slot
//   - falls through to the body
type ArrayLoopBeginOp struct {
	Quantifier     Quantifier
	ElemByteSize   uint32
	CompileTimeLen uint32
	EndLabel       LabelID
}

func (*ArrayLoopBeginOp) irOp() {}

// SliceLoopBeginOp begins iteration over a Go slice. At entry, scratch at
// sm->offset holds the 24-byte slice header (data ptr, len, cap). The op:
//   - reads len from header[8..16]; caps it to
//     COLLECTION_PREDICATE_MAX_ITERATIONS (4096) and records `capped` in
//     slice_loop_state so the End op can surface eval_error if iteration
//     exhausts the cap without short-circuiting
//   - stashes data_ptr / initial_len / elem_size / quantifier in
//     sm->slice_loop_state
//   - writes the initial accumulator byte at sm->offset
//   - if len == 0, jumps to EndLabel
//   - reads the first element into the @it scratch slot
//   - falls through to the body
type SliceLoopBeginOp struct {
	Quantifier   Quantifier
	ElemByteSize uint32
	EndLabel     LabelID
}

func (*SliceLoopBeginOp) irOp() {}

// SwissMapLoopBeginOp begins iteration over a Go swiss-table map. At entry,
// scratch at sm->offset holds the dereferenced map header. The op reads
// dir_ptr / dir_len from it; on a nil map, treats it as empty (jumps to
// EndLabel with accumulator init).
//
// Iteration walks the dir-of-tables, then groups within each table, then
// slots within each group, copying (key, value) into the @it scratch slot
// before invoking the body. Body invocations are bounded by an online counter
// in the End op; cursor advancement that skips empty slots uses a separate
// BPF-side scan budget. On exceeding either cap the loop sets
// condition_eval_error and aborts via sm_return.
//
// The synthetic @it scratch struct lays the key at offset 0 and the value
// at the next 8-byte-aligned offset after the key. Irgen registers this
// synthetic struct in the type catalog so that @it.key / @it.value resolve
// via the normal GetMemberExpr path.
type SwissMapLoopBeginOp struct {
	Quantifier  Quantifier
	KeyByteSize uint32
	ValByteSize uint32
	EndLabel    LabelID

	// Swiss-map header / group / table layout (mirroring SwissMapLookupOp).
	// Iteration walks every slot, so we don't need the hash-related
	// GlobalShiftOffset or the HeaderByteSize (the preceding DereferenceOp
	// already placed the header at sm->offset).
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

func (*SwissMapLoopBeginOp) irOp() {}

// ArrayLoopEndOp closes an ArrayLoopBeginOp loop body. The Begin op stashed
// quantifier/elem_size in sm->slice_loop_state, so the End op reads them
// from there rather than encoding them again in its own params.
//
// On each invocation it:
//   - reads the predicate body's bool result at sm->offset
//   - on a short-circuiting result (Any: 1, All: 0), writes the accumulator,
//     pops the loop state, and exits
//   - on continuation: bumps base_ptr by ElemByteSize (from loop state),
//     decrements remaining; if exhausted, pops state and exits with the
//     current accumulator; otherwise reads the next element via
//     bpf_probe_read_user and jumps back to BodyLabel.
//   - if the loop was capped at MAX_ITERATIONS and exhausted without a
//     short-circuit result, sets condition_eval_error.
//
// Body-side faults (nil deref / OOB) sm_return out of the enclosing condition
// before this op runs; condition_eval_error is already set on that path and
// surfaces in the event header.
type ArrayLoopEndOp struct {
	BodyLabel LabelID
}

func (*ArrayLoopEndOp) irOp() {}

// SliceLoopEndOp closes a SliceLoopBeginOp loop body. Same semantics as
// ArrayLoopEndOp; differs only in the upstream Begin op's setup.
type SliceLoopEndOp struct {
	BodyLabel LabelID
}

func (*SliceLoopEndOp) irOp() {}

// SwissMapLoopEndOp closes a SwissMapLoopBeginOp loop body. In addition to
// the short-circuit-on-result handling shared with the slice/array ops,
// it drives the swiss-map iteration state machine: slot+1 within current
// group, then group+1 within current table, then table+1 in the dir,
// skipping duplicate consecutive table pointers (Go's incremental-growth
// aliasing). After each visited occupied element it increments an online
// iterations counter; if it exceeds COLLECTION_PREDICATE_MAX_ITERATIONS the
// op sets condition_eval_error and aborts via sm_return.
type SwissMapLoopEndOp struct {
	BodyLabel LabelID
}

func (*SwissMapLoopEndOp) irOp() {}

// EmitFilterSliceMarkerOp is the inline-pass op for a filter() call whose
// source is a slice. At entry, scratch at sm->offset holds the 24-byte
// source slice header (ptr, len, cap).
//
// The op:
//   - reads len from header[8..16] and data_ptr from header[0..8].
//   - if len > COLLECTION_PREDICATE_MAX_ITERATIONS, sets
//     sm->pending_expr_status = EXPR_STATUS_TRUNCATED. The trailing
//     compiler-appended ExprSaveOp reads and clears this field.
//   - leaves the first 8 bytes at sm->offset as data_ptr (the wire
//     handle); the compiler tracks lastOpSize = 8 so ExprSaveOp copies
//     these 8 bytes into the event-root expression slot.
//   - if len > 0, calls sm_record_pointer with FilterDataTypeID,
//     data_ptr, and capped byte size
//     min(len, MAX_ITERATIONS) * ElemByteSize. The chase loop's
//     FILTER_DEFERRED arm later invokes the data type's enqueue_pc.
type EmitFilterSliceMarkerOp struct {
	FilterDataTypeID TypeID
	ElemByteSize     uint32
}

func (*EmitFilterSliceMarkerOp) irOp() {}

// EmitFilterMapMarkerOp is the inline-pass op for a filter() call whose
// source is a map. At entry, scratch at sm->offset holds the raw
// `map[K]V` user-space pointer (no DereferenceOp precedes this op, in
// contrast to any/all). The op:
//   - reads map_header_addr at sm->offset (the wire handle is already
//     there; lastOpSize = 8).
//   - if map_header_addr != 0, does a one-shot bpf_probe_read_user(8,
//     map_header_addr + UsedFieldOffset) to obtain the `used` count
//     and sets sm->pending_expr_status = EXPR_STATUS_TRUNCATED if
//     used > COLLECTION_PREDICATE_MAX_ITERATIONS.
//   - if map_header_addr != 0, calls sm_record_pointer with
//     FilterDataTypeID, map_header_addr, and SwissHeaderSize. The
//     enqueue_pc init op does its own bpf_probe_read_user of the map
//     header to recover dirPtr / dirLen.
type EmitFilterMapMarkerOp struct {
	FilterDataTypeID TypeID
	SwissHeaderSize  uint32
	UsedFieldOffset  uint32
}

func (*EmitFilterMapMarkerOp) irOp() {}

// InitFilterSliceLoopOp initializes the deferred slice-filter loop. Runs
// inside the data type's enqueue_pc, after sm_chase_pointer's
// FILTER_DEFERRED arm has populated sm->di_0 with the chase item:
// di_0.address is the source data pointer and di_0.length is the capped
// byte size (already min(len, MAX_ITERATIONS) * elem_size from the
// marker op).
//
// On entry: in_progress=false on a fresh enqueue_pc invocation.
// The op:
//   - reads remaining = di_0.length / ElemByteSize.
//   - sets filter_loop_state {data_ptr, remaining, output_index=0,
//     elem_size, data_type_id, in_progress=true}.
//   - if remaining == 0, jumps to EndLabel (loop body not entered).
//
// IterScratchBudget is the worst-case scratch-buffer bytes a single
// iteration may consume — @it + predicate-body scratch + emit-element
// overhead — computed by irgen at compile time from the predicate body's
// emitted ops. Used by the per-iteration scratch_buf_bounds_check.
type InitFilterSliceLoopOp struct {
	ElemByteSize      uint32
	IterScratchBudget uint32
	EndLabel          LabelID
}

func (*InitFilterSliceLoopOp) irOp() {}

// FilterSliceLoopStepOp closes the slice-filter loop body. Reads the
// predicate result byte at sm->offset; on true, calls the emit helper
// (which writes a per-element data item, then runs the element type's
// handler against the just-emitted payload to chase nested pointers
// inside the element). On false, skips emit. Always advances data_ptr
// and remaining. If remaining > 0, prepares @it for the next iteration
// (incl. an iteration-budget bounds check and a flush if needed) and
// jumps to BodyLabel. Otherwise falls through (the enqueue_pc returns
// to the chase loop).
type FilterSliceLoopStepOp struct {
	ElemByteSize  uint32
	ElementTypeID TypeID
	BodyLabel     LabelID
}

func (*FilterSliceLoopStepOp) irOp() {}

// InitFilterMapLoopOp initializes the deferred map-filter loop. Runs
// inside the map data type's enqueue_pc. sm->di_0.address is the raw
// map[K]V pointer. The op does its own bpf_probe_read_user of the map
// header (swiss_header_size bytes at di_0.address) to recover dirPtr
// and dirLen, then initializes the swiss-map iteration cursor.
//
// On a nil-deref reading the header, emits the failure sentinel and
// jumps to EndLabel. Otherwise sets up filter_loop_state for map
// iteration and falls through to the body (or jumps to EndLabel if
// dir_len/used indicate an empty map).
type InitFilterMapLoopOp struct {
	KeyByteSize       uint32
	ValByteSize       uint32
	ValOffsetInPair   uint32
	IterScratchBudget uint32
	EndLabel          LabelID

	// Swiss-map header / group / table layout — same shape as
	// SwissMapLoopBeginOp.
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

func (*InitFilterMapLoopOp) irOp() {}

// FilterMapLoopStepOp closes the map-filter loop body. Same semantics as
// FilterSliceLoopStepOp, but emits one data item per passing (key, value)
// pair via the map-specific emit helper (which reads key and value
// separately from the swiss-map slot's distinct field offsets), then
// runs the key and value type handlers against the just-emitted payload
// at offset 0 and ValOffsetInPair respectively. The compiler lowering
// accounts for each handler's offsetShift when inserting the
// IncrementOutputOffsetOp between them.
type FilterMapLoopStepOp struct {
	KeyTypeID   TypeID
	ValueTypeID TypeID
	BodyLabel   LabelID
}

func (*FilterMapLoopStepOp) irOp() {}
