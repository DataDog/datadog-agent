#ifndef __TYPES_H__
#define __TYPES_H__

// Types used to program the stack machine and event processing.

// Throttle mode controls when throttling is applied relative to condition evaluation.
// To be kept in sync with the compiler.ThrottleMode constants.
typedef enum throttle_mode {
  THROTTLE_AT_START = 0,          // Throttle before probe_run (default for unconditional non-return probes)
  THROTTLE_AFTER_COND_CHECK = 1,  // Throttle after condition evaluates to true
  THROTTLE_NONE = 2,              // Never throttle (unconditional returns, entries with conditional returns)
} throttle_mode_t;

typedef struct probe_params {
  uint32_t throttler_idx;
  uint32_t stack_machine_pc;
  uint32_t pointer_chasing_limit;
  uint32_t collection_size_limit;
  uint32_t string_size_limit;
  uint32_t probe_id;
  bool frameless;
  bool has_associated_return;
  char kind; // actually an event_kind_t
  char top_pc_offset;
  char no_return_reason;
  char throttle_mode; // actually a throttle_mode_t
  char __padding[2];
} probe_params_t;

typedef struct throttler_params {
  uint64_t period_ns;
  int64_t budget;
} throttler_params_t;

typedef struct stats {
  uint64_t cpu_ns;
  uint64_t hit_cnt;
  uint64_t throttled_cnt;
  // runtime.recovery probe counters. All zero on cores that haven't
  // observed a recovery firing.
  //
  //  recovery_fires            — number of times the recovery handler ran
  //                              (i.e. uprobe on runtime.recovery hit).
  //  recovery_evicted_frames   — total in_progress_calls slots evicted
  //                              across all recoveries.
  //  recovery_submit_failures  — synthetic-event ringbuf submits that
  //                              failed; we fell back to a RETURN_LOST
  //                              drop notification.
  //  recovery_no_open_calls    — recoveries where the goroutine had no
  //                              entry in in_progress_calls; we short-
  //                              circuited before reading the panic
  //                              chain.
  //  recovery_filtered_goexit  — recoveries we skipped because the
  //                              innermost panic was a runtime.Goexit
  //                              (out of scope for this revision).
  //  recovery_invalid_state    — defensive bails: panic_ptr==0,
  //                              recovered!=1, missing SP fields, or
  //                              lo>=hi. Should normally stay zero.
  uint64_t recovery_fires;
  uint64_t recovery_evicted_frames;
  uint64_t recovery_submit_failures;
  uint64_t recovery_no_open_calls;
  uint64_t recovery_filtered_goexit;
  uint64_t recovery_invalid_state;
} stats_t;

typedef enum dynamic_size_class {
  DYNAMIC_SIZE_CLASS_STATIC = 0,
  DYNAMIC_SIZE_CLASS_SLICE = 1,
  DYNAMIC_SIZE_CLASS_STRING = 2,
  DYNAMIC_SIZE_CLASS_HASHMAP = 3,
  // FILTER_DEFERRED marks per-call-site filter data types whose
  // enqueue_pc runs the deferred filter loop. sm_chase_pointer
  // emits no header and reads no payload for these types — the
  // enqueue_pc itself does all the work. Must stay in sync with
  // ir.DynamicSizeFilterDeferred.
  DYNAMIC_SIZE_CLASS_FILTER_DEFERRED = 4,
} dynamic_size_class_t;

typedef struct type_info {
  dynamic_size_class_t dynamic_size_class;
  uint32_t byte_len;
  uint32_t enqueue_pc;
  int32_t go_context_context_offset;
  int32_t go_context_key_offset;
  int32_t go_context_value_offset;
  int32_t ddtrace_trace_id_offset;
  int32_t ddtrace_span_id_offset;
  int32_t ddtrace_parent_id_offset;
  int32_t ddtrace_span_context_offset;
  int32_t ddtrace_span_context_trace_id_offset;
  uint8_t go_context_is_context;
  uint8_t ddtrace_span_kind;
  uint8_t __padding[2];
} type_info_t;

// trace_context_t is the 40-byte payload layout for synthetic data items of
// IR type TraceContextType. Emitted by SM_OP_GO_CONTEXT_CHAIN_INIT/HOP at
// chase time when a concrete context.Context implementation (cancelCtx,
// valueCtx, …) is dequeued. INIT zeroes the first 40 bytes and rewrites the
// data item header type id; HOP optionally fills in trace_id/span_id/
// parent_id and sets valid=1 if a dd-trace span is found on the chain.
typedef struct trace_context {
  uint64_t trace_id_lower;
  uint64_t trace_id_upper;
  uint64_t span_id;
  uint64_t parent_id;
  uint8_t valid;
  uint8_t __padding[7];
} trace_context_t;

// To be kept in sync with the ir/event_kind.go file.
typedef enum event_kind {
  EVENT_KIND_INVALID = 0,
  EVENT_KIND_ENTRY = 1,
  EVENT_KIND_RETURN = 2,
} event_kind_t;

// To be kept in sync with the ir.NoReturnReason enum in the ir/program.go file.
typedef enum no_return_reason {
  NO_RETURN_REASON_NONE = 0,
  NO_RETURN_REASON_RETURNS_DISABLED = 1,
  NO_RETURN_REASON_LINE_PROBE = 2,
  NO_RETURN_REASON_INLINED = 3,
  NO_RETURN_REASON_NO_BODY = 4,
  NO_RETURN_REASON_IS_RETURN = 5,
} no_return_reason_t;

typedef enum sm_opcode {
  SM_OP_INVALID = 0,
  // Execution flow ops.
  SM_OP_CALL = 1,
  SM_OP_RETURN = 2,
  SM_OP_ILLEGAL = 3,
  // Output offset ops.
  SM_OP_INCREMENT_OUTPUT_OFFSET = 4,
  // Expression ops.
  SM_OP_EXPR_PREPARE = 5,
  SM_OP_EXPR_SAVE = 6,
  SM_OP_EXPR_DEREFERENCE_CFA = 7,
  SM_OP_EXPR_READ_REGISTER = 8,
  SM_OP_EXPR_DEREFERENCE_PTR = 9,
  // Type processing ops.
  SM_OP_PROCESS_POINTER = 10,
  SM_OP_PROCESS_SLICE = 11,
  SM_OP_PROCESS_ARRAY_DATA_PREP = 12,
  SM_OP_PROCESS_SLICE_DATA_PREP = 13,
  SM_OP_PROCESS_SLICE_DATA_REPEAT = 14,
  SM_OP_PROCESS_STRING = 15,
  SM_OP_PROCESS_GO_EMPTY_INTERFACE = 16,
  SM_OP_PROCESS_GO_INTERFACE = 17,
  SM_OP_PROCESS_GO_DICT_TYPE = 18,
  SM_OP_PROCESS_GO_HMAP = 19,
  SM_OP_PROCESS_GO_SWISS_MAP = 20,
  SM_OP_PROCESS_GO_SWISS_MAP_GROUPS = 21,
  // Top level ops.
  SM_OP_CHASE_POINTERS = 22,
  SM_OP_PREPARE_EVENT_ROOT = 23,
  // Condition expression ops.
  SM_OP_EXPR_PUSH_OFFSET = 24,
  SM_OP_EXPR_LOAD_LITERAL = 25,
  SM_OP_EXPR_READ_STRING = 26,
  SM_OP_EXPR_CMP_BASE = 27,
  SM_OP_EXPR_CMP_STRING = 28,
  SM_OP_CONDITION_CHECK = 29,
  SM_OP_CONDITION_BEGIN = 30,
  SM_OP_CALL_DICT_RESOLVED = 31,
  SM_OP_EXPR_SLICE_BOUNDS_CHECK = 32,
  // Swiss map lookup opcodes (decomposed for verifier budget).
  SM_OP_SWISS_MAP_SETUP = 33,
  SM_OP_SWISS_MAP_AESENC = 34,
  SM_OP_SWISS_MAP_HASH_FINISH = 35,
  SM_OP_SWISS_MAP_PROBE = 36,
  SM_OP_SWISS_MAP_CHECK_SLOT = 37,
  // Compound condition opcodes.
  SM_OP_COND_NOT = 38,
  SM_OP_COND_JUMP_IF_FALSE = 39,
  SM_OP_COND_JUMP_IF_TRUE = 40,
  // Writes (sm->start_ns - sm->entry_ktime_ns) as 8 bytes at sm->offset,
  // matching the byte-layout contract of other location ops. Does not
  // advance sm->offset. Callers that use the value as a comparison
  // operand follow with EXPR_PUSH_OFFSET{8} to advance/push.
  SM_OP_EXPR_LOAD_DURATION = 41,
  // Split-event-kind condition ops. Used when a probe's condition
  // expression has leaves that resolve to variables on both the entry
  // and return events. Each entry leaf is compiled to its own SM
  // sub-function so leaf-internal aborts (nil deref / OOB) return to
  // the entry-side driver rather than the event handler. The driver
  // captures each leaf's outcome as a 2-bit status in sm->condition_state
  // and runs an AST replay that lazily reads statuses, preserving the
  // user's short-circuit semantics. See pkg/dyninst/ir/expression.go.
  SM_OP_CONDITION_STATE_INIT = 42,
  SM_OP_CONDITION_LEAF_RECORD = 43,
  SM_OP_CONDITION_LEAF_LOAD = 44,
  SM_OP_CONDITION_CHECK_PRESERVE_ERROR = 45,
  // Emitted at the tail of a per-leaf SM sub-function on the success
  // path. Clears condition_eval_error so the driver's
  // CONDITION_LEAF_RECORD can distinguish success from abort.
  SM_OP_CONDITION_LEAF_COMPLETE = 46,
  // Go context.Context chain-walk opcodes. Together they form the enqueue_pc
  // subroutine for any concrete context.Context implementation IR type:
  //   [SM_OP_GO_CONTEXT_CHAIN_INIT, SM_OP_GO_CONTEXT_CHAIN_HOP, SM_OP_RETURN]
  // INIT runs once after the chase preamble has serialized the
  // implementation's bytes. It rewrites the just-written data item header's
  // type to TraceContextType, zeros the first 40 payload bytes (establishing
  // valid=0), and seeds sm->go_context_walk. HOP runs one chain step per
  // dispatch; if not yet done it self-jumps (sm->pc -= 1) so the next
  // sm_loop iteration re-enters HOP — up to MAX_GO_CONTEXT_DEPTH times.
  // See pkg/dyninst/irgen/trace_context.md for design.
  SM_OP_GO_CONTEXT_CHAIN_INIT = 47,
  SM_OP_GO_CONTEXT_CHAIN_HOP = 48,
  // Resolve the captured time.Time's loc pointer to a UTC offset in
  // seconds, written in place of the loc pointer. See
  // pkg/dyninst/compiler/ops.go: ProcessGoTimeOp.
  SM_OP_PROCESS_GO_TIME = 49,
  // Collection-predicate (any/all) opcodes. See ir.ExprLoadAddressOp,
  // ir.{Array,Slice,SwissMap}Loop{Begin,End}Op.
  SM_OP_EXPR_LOAD_ADDRESS = 50,
  SM_OP_ARRAY_LOOP_BEGIN = 51,
  SM_OP_ARRAY_LOOP_END = 52,
  SM_OP_SLICE_LOOP_BEGIN = 53,
  SM_OP_SLICE_LOOP_END = 54,
  SM_OP_SWISS_MAP_LOOP_BEGIN = 55,
  SM_OP_SWISS_MAP_LOOP_END = 56,
  // Shifts sm->offset by a compile-time immediate. Used by LocationOp
  // lowering for @it to position sm->offset at a field within the loop's
  // @it scratch slot before the body's PushOffset/CmpBase sequence.
  SM_OP_EXPR_ADVANCE_OFFSET = 57,
  // Recovery probe opcodes. PREPARE validates that the goroutine has
  // a recovered panic and computes (lo, hi) depth bounds into the
  // event header; on validation failure it sets condition_failed so
  // the SM aborts and probe_run skips submitting the event. EVICT_SLOTS
  // walks in_progress_calls[goid] and zeros every slot whose depth is
  // in (lo, hi]; deletes the per-goroutine map entry if empty.
  SM_OP_PANIC_UNWIND_PREPARE = 58,
  SM_OP_PANIC_UNWIND_EVICT_SLOTS = 59,
  // Filter (deferred collection-filter) opcodes. See
  // ir.EmitFilter{Slice,Map}MarkerOp, ir.InitFilter{Slice,Map}LoopOp,
  // ir.FilterSliceLoopStepOp, ir.FilterMapLoopStepOp, and the
  // compiler-only EmitFilter{Slice,Map}ElementOp / Filter{Slice,Map}AdvanceOp.
  SM_OP_EMIT_FILTER_SLICE_MARKER = 60,
  SM_OP_EMIT_FILTER_MAP_MARKER = 61,
  SM_OP_INIT_FILTER_SLICE_LOOP = 62,
  SM_OP_EMIT_FILTER_SLICE_ELEMENT = 63,
  SM_OP_FILTER_SLICE_ADVANCE = 64,
  SM_OP_INIT_FILTER_MAP_LOOP = 65,
  SM_OP_EMIT_FILTER_MAP_ELEMENT = 66,
  SM_OP_FILTER_MAP_ADVANCE = 67,
} sm_opcode_t;

// cmp_op_t identifies which comparison SM_OP_EXPR_CMP_BASE /
// SM_OP_EXPR_CMP_STRING performs. Encoded as a single byte in the
// instruction stream — values must stay in sync with ir.CmpOp.
typedef enum cmp_op {
  CMP_EQ = 0,
  CMP_NE = 1,
  CMP_LT = 2,
  CMP_LE = 3,
  CMP_GT = 4,
  CMP_GE = 5,
} cmp_op_t;

// cmp_kind_t tells SM_OP_EXPR_CMP_BASE how to interpret the bytes being
// ordered. Equality and string compares ignore the kind. Encoded as a
// single byte in the instruction stream — values must stay in sync with
// ir.CmpKind.
typedef enum cmp_kind {
  CMP_KIND_UINT = 0,
  CMP_KIND_INT = 1,
} cmp_kind_t;

#ifdef DYNINST_DEBUG
static const char* op_code_name(sm_opcode_t op_code) {
  switch (op_code) {
  case SM_OP_INVALID:
    return "INVALID";
  case SM_OP_CALL:
    return "CALL";
  case SM_OP_RETURN:
    return "RETURN";
  case SM_OP_ILLEGAL:
    return "ILLEGAL";
  case SM_OP_INCREMENT_OUTPUT_OFFSET:
    return "INCREMENT_OUTPUT_OFFSET";
  case SM_OP_EXPR_PREPARE:
    return "EXPR_PREPARE";
  case SM_OP_EXPR_SAVE:
    return "EXPR_SAVE";
  case SM_OP_EXPR_DEREFERENCE_CFA:
    return "EXPR_DEREFERENCE_CFA";
  case SM_OP_EXPR_READ_REGISTER:
    return "EXPR_READ_REGISTER";
  case SM_OP_EXPR_DEREFERENCE_PTR:
    return "EXPR_DEREFERENCE_PTR";
  case SM_OP_PROCESS_POINTER:
    return "PROCESS_POINTER";
  case SM_OP_PROCESS_SLICE:
    return "PROCESS_SLICE";
  case SM_OP_PROCESS_ARRAY_DATA_PREP:
    return "PROCESS_ARRAY_DATA_PREP";
  case SM_OP_PROCESS_SLICE_DATA_PREP:
    return "PROCESS_SLICE_DATA_PREP";
  case SM_OP_PROCESS_SLICE_DATA_REPEAT:
    return "PROCESS_SLICE_DATA_REPEAT";
  case SM_OP_PROCESS_STRING:
    return "PROCESS_STRING";
  case SM_OP_PROCESS_GO_EMPTY_INTERFACE:
    return "PROCESS_GO_EMPTY_INTERFACE";
  case SM_OP_PROCESS_GO_INTERFACE:
    return "PROCESS_GO_INTERFACE";
  case SM_OP_PROCESS_GO_DICT_TYPE:
    return "PROCESS_GO_DICT_TYPE";
  case SM_OP_PROCESS_GO_HMAP:
    return "PROCESS_GO_HMAP";
  case SM_OP_PROCESS_GO_SWISS_MAP:
    return "PROCESS_GO_SWISS_MAP";
  case SM_OP_PROCESS_GO_SWISS_MAP_GROUPS:
    return "PROCESS_GO_SWISS_MAP_GROUPS";
  case SM_OP_CHASE_POINTERS:
    return "CHASE_POINTERS";
  case SM_OP_PREPARE_EVENT_ROOT:
    return "PREPARE_EVENT_ROOT";
  case SM_OP_EXPR_PUSH_OFFSET:
    return "EXPR_PUSH_OFFSET";
  case SM_OP_EXPR_LOAD_LITERAL:
    return "EXPR_LOAD_LITERAL";
  case SM_OP_EXPR_READ_STRING:
    return "EXPR_READ_STRING";
  case SM_OP_EXPR_CMP_BASE:
    return "EXPR_CMP_BASE";
  case SM_OP_EXPR_CMP_STRING:
    return "EXPR_CMP_STRING";
  case SM_OP_CONDITION_CHECK:
    return "CONDITION_CHECK";
  case SM_OP_CONDITION_BEGIN:
    return "CONDITION_BEGIN";
  case SM_OP_CALL_DICT_RESOLVED:
    return "CALL_DICT_RESOLVED";
  case SM_OP_EXPR_SLICE_BOUNDS_CHECK:
    return "EXPR_SLICE_BOUNDS_CHECK";
  case SM_OP_SWISS_MAP_SETUP:
    return "SWISS_MAP_SETUP";
  case SM_OP_SWISS_MAP_AESENC:
    return "SWISS_MAP_AESENC";
  case SM_OP_SWISS_MAP_HASH_FINISH:
    return "SWISS_MAP_HASH_FINISH";
  case SM_OP_SWISS_MAP_PROBE:
    return "SWISS_MAP_PROBE";
  case SM_OP_SWISS_MAP_CHECK_SLOT:
    return "SWISS_MAP_CHECK_SLOT";
  case SM_OP_COND_NOT:
    return "COND_NOT";
  case SM_OP_COND_JUMP_IF_FALSE:
    return "COND_JUMP_IF_FALSE";
  case SM_OP_COND_JUMP_IF_TRUE:
    return "COND_JUMP_IF_TRUE";
  case SM_OP_EXPR_LOAD_DURATION:
    return "EXPR_LOAD_DURATION";
  case SM_OP_CONDITION_STATE_INIT:
    return "CONDITION_STATE_INIT";
  case SM_OP_CONDITION_LEAF_RECORD:
    return "CONDITION_LEAF_RECORD";
  case SM_OP_CONDITION_LEAF_LOAD:
    return "CONDITION_LEAF_LOAD";
  case SM_OP_CONDITION_CHECK_PRESERVE_ERROR:
    return "CONDITION_CHECK_PRESERVE_ERROR";
  case SM_OP_CONDITION_LEAF_COMPLETE:
    return "CONDITION_LEAF_COMPLETE";
  case SM_OP_GO_CONTEXT_CHAIN_INIT:
    return "GO_CONTEXT_CHAIN_INIT";
  case SM_OP_GO_CONTEXT_CHAIN_HOP:
    return "GO_CONTEXT_CHAIN_HOP";
  case SM_OP_PANIC_UNWIND_PREPARE:
    return "PANIC_UNWIND_PREPARE";
  case SM_OP_PANIC_UNWIND_EVICT_SLOTS:
    return "PANIC_UNWIND_EVICT_SLOTS";
  default:
    break;
  }
  return "UNKNOWN";
}
#endif

#endif // __TYPES_H__
