#ifndef __CONTEXT_H__
#define __CONTEXT_H__

#include "bpf_tracing.h"
#include "types.h"
#include "queue.h"
#include "scratch.h"
#include "chased_pointers_trie.h"
#include "chased_slices.h"

typedef uint32_t type_t;

// Parameters for swiss_map_check_slot, packed for 2-arg global function.
// Defined here (rather than stack_machine.h) so it can be embedded in
// swiss_map_state to avoid a large stack local in sm_loop.
typedef struct swiss_map_slot_params {
  target_ptr_t key_addr;
  target_ptr_t val_addr;
  buf_offset_t key_data_off;
  buf_offset_t result_offset;
  uint32_t val_byte_size;
  uint16_t key_data_len;
  uint8_t key_byte_size;
  uint8_t is_string_key;
  // When set, a key match writes a 1-byte bool at result_offset instead of
  // dereferencing val_addr for val_byte_size bytes. See contains() in the
  // exprlang package.
  uint8_t existence_only;
} swiss_map_slot_params_t;

typedef struct frame_data {
  uint16_t stack_idx;
  uint64_t cfa;
} frame_data_t;

typedef struct resolved_go_interface {
  target_ptr_t addr;
  uint64_t go_runtime_type;
} resolved_go_interface_t;

typedef struct resolved_go_any_type {
  resolved_go_interface_t i;
  type_t type;
  bool has_info;
  type_info_t info;
} resolved_go_any_type_t;

typedef struct pointers_queue_item {
  di_data_item_header_t di;
  uint32_t ttl;
  uint32_t __padding[3];
} pointers_queue_item_t;

DEFINE_QUEUE(pointers, pointers_queue_item_t, 128);

// Sentinel for stack_machine_t::last_submitted_seq meaning "no fragment
// has been submitted yet for this probe invocation". continuation_seq is
// uint16, so 0xFFFF can never collide with a real sequence number.
#define LAST_SUBMITTED_SEQ_NONE ((uint16_t)0xFFFF)

// LEAF_STATUS_* values for the 2-bit per-leaf status entries packed into
// stack_machine_t.condition_state and call_depths_entry_t.condition_state.
// Each entry leaf in a split-event-kind condition produces one of these.
// The encoding is chosen so SM_OP_CONDITION_LEAF_RECORD can derive the
// status directly from condition_eval_error/condition_nil_deref:
//   - completed-and-false → 00 = LEAF_STATUS_FALSE
//   - completed-and-true  → 01 = LEAF_STATUS_TRUE
//   - eval-error          → 10 = LEAF_STATUS_EVAL_ERROR
//   - nil-deref           → 11 = LEAF_STATUS_NIL_DEREF
// SM_OP_CONDITION_LEAF_LOAD reads the 2 bits and (for the error variants)
// also sets condition_eval_error / condition_nil_deref so event.c's
// header surfaces the right flavor.
#define LEAF_STATUS_FALSE 0
#define LEAF_STATUS_TRUE 1
#define LEAF_STATUS_EVAL_ERROR 2
#define LEAF_STATUS_NIL_DEREF 3

// MAX_CONDITION_ENTRY_LEAVES is the maximum number of entry-side leaves
// allowed in a split-event-kind condition. condition_state is a uint16
// storing 2 bits per leaf, so the cap is 8. Keep in sync with
// maxConditionEntryLeaves in pkg/dyninst/irgen/irgen.go.
#define MAX_CONDITION_ENTRY_LEAVES 8
#define CONDITION_LEAF_IDX_MASK (MAX_CONDITION_ENTRY_LEAVES - 1)

#define ENQUEUE_STACK_DEPTH 32
typedef struct stack_machine {
  // Initialized on every entry point.
  uint32_t pc;
  buf_offset_t offset;
  frame_data_t frame_data;

  // Fully owned state.
  uint32_t pc_stack[ENQUEUE_STACK_DEPTH];
  uint32_t pc_stack_pointer;

  uint32_t data_stack[ENQUEUE_STACK_DEPTH];
  uint32_t data_stack_pointer;

  pointers_queue_t pointers_queue;
  chased_pointers_trie_t chased;
  chased_slices_t chased_slices;
  // Remaining pointer chasing limit, given currently processed data item.
  // Maybe 0, in which case data might still be processed (i.e. interface type rewrite),
  // but no further pointers will be chased.
  uint32_t pointer_chasing_ttl;

  uint32_t collection_size_limit;
  uint32_t string_size_limit;

  // Offset of currently visited context object, or zero.
  buf_offset_t go_context_offset;
  // Bitmask for remaining go context values to capture.
  uint64_t go_context_capture_bitmask;

  // State for the Go context.Context chain walk run by
  // SM_OP_GO_CONTEXT_CHAIN_INIT and SM_OP_GO_CONTEXT_CHAIN_HOP. Reset by
  // every INIT invocation. See pkg/dyninst/irgen/trace_context.md.
  struct {
    // Current link being inspected: address of the implementation struct
    // and (for hops 1+) its Go runtime type. Hop 0 reads from
    // current_ir_type instead because INIT seeds it from sm->di_0.type
    // (no go_runtime_type is available without re-resolving the parent
    // interface header from user memory).
    resolved_go_interface_t current;
    // IR type id for hop 0; cleared to 0 after hop 0 consumes it. Hops 1+
    // resolve the IR type via lookup_go_interface(current.go_runtime_type).
    uint32_t current_ir_type;
    // Number of hops the loop is still allowed to take. Starts at
    // MAX_GO_CONTEXT_DEPTH; decremented per successful advance.
    uint32_t depth_remaining;
    // Byte offset in the scratch buffer where the trace_context_t payload
    // sits (i.e. the start of the synthetic data item's payload). Captured
    // by INIT from sm->offset; HOP writes the populated trace_context_t
    // here when it finds a span.
    buf_offset_t data_item_offset;
    // 1 once a span has been extracted, the chain has been exhausted, or
    // an early-termination guard fired. HOP no-ops on subsequent dispatches
    // when set.
    uint8_t done;
    uint8_t __padding[7];
  } go_context_walk;

  // Data about currently evaluated expression results set.
  buf_offset_t expr_results_offset;
  buf_offset_t expr_results_end_offset;
  enum {
    FRAME,
    POINTER,
  } expr_type;
  // Address of the root structure, for evaluating type expressions.
  uint64_t root_addr;

  // Set to true by ConditionCheck when the condition is false.
  bool condition_failed;
  // "Arm" flag for the condition-evaluation error channel.
  //
  // Every condition-leaf abort path (nil deref, OOB, map miss, deref
  // fail, swiss-map probe failure, @duration absent, etc.) sets this
  // directly before calling sm_return — but only when the current
  // expression is a condition (i.e. expr_status_idx == EXPR_STATUS_IDX_NONE).
  // Capture expressions report errors via the per-expression status
  // array (EXPR_STATUS_NIL_DEREF, EXPR_STATUS_OOB, EXPR_STATUS_ABSENT)
  // and must NOT poison this flag. Single-event ConditionBeginOp also
  // arms it at the start of the condition as belt-and-braces.
  //
  // Single-event-kind conditions:
  //   - SM_OP_CONDITION_BEGIN arms it at the start of the condition.
  //   - If any leaf aborts, the abort path also arms it (see above),
  //     so userspace sees a failed condition.
  //   - The tail SM_OP_CONDITION_CHECK clears it iff the full condition
  //     tree ran to completion. The intermediate SM_OP_COND_JUMP_IF_*
  //     ops deliberately do NOT clear it: a short-circuit jump may
  //     skip a leaf that would have aborted, but leaves that fall
  //     through can still fault, so the arm must survive until CHECK.
  //
  // Split-event-kind conditions (entry-side driver and return-side AST
  // replay): each entry leaf is its own SM sub-function. The leaf arms
  // the flag in its prelude, and the success-path
  // SM_OP_CONDITION_LEAF_COMPLETE clears it; abort paths leave it armed
  // (and also arm it directly, per above — important for the
  // return-side replay, which inlines its return leaves and skips
  // SM_OP_CONDITION_BEGIN). The driver's SM_OP_CONDITION_LEAF_RECORD
  // reads the flag to derive a 2-bit status, then clears it before the
  // next leaf. AST-replay SM_OP_CONDITION_LEAF_LOAD sets the flag (and
  // condition_nil_deref) when it dispatches an errored leaf, and the
  // tail SM_OP_CONDITION_CHECK_PRESERVE_ERROR deliberately does NOT
  // clear it so the error survives to event.c.
  //
  // event.c surfaces the flag as header->condition_eval_error (0, 1, 2).
  bool condition_eval_error;
  // Set to true when a nil pointer dereference causes an expression or
  // condition evaluation to abort. Used together with condition_eval_error
  // to distinguish nil-caused failures from other evaluation errors.
  bool condition_nil_deref;
  // condition_state packs up to 8 per-leaf 2-bit statuses for a split-
  // event-kind condition. Bits [2*i, 2*i+1] hold leaf i's status (one of
  // the LEAF_STATUS_* values). Reset to 0 by SM_OP_CONDITION_STATE_INIT
  // at the start of the entry-side driver; written per-leaf by
  // SM_OP_CONDITION_LEAF_RECORD; read per-leaf during AST replay by
  // SM_OP_CONDITION_LEAF_LOAD; copied into the call_depths slot at the
  // existing insertion site in event.c so the return-side program can
  // consult it.
  uint16_t condition_state;
  // Set to true by sm_chase_pointer when scratch_buf_serialize fails due to
  // insufficient buffer space. Checked and cleared by SM_OP_CHASE_POINTERS
  // to trigger a flush-and-continue.
  bool buffer_full;
  // One-shot ExprStatus override read and cleared by SM_OP_EXPR_SAVE.
  // Used by the filter marker ops to surface EXPR_STATUS_TRUNCATED
  // inline (before the chase phase). Co-located with buffer_full so we
  // consume natural alignment padding before saved_dict_ptr rather than
  // growing the struct (which would shift swissmap_loop_state.phase
  // out of the verifier's bounds-tracking window for the existing
  // cold-path reset in sm_run).
  uint8_t pending_expr_status;

  // Dictionary pointer for generic shape functions. Set by
  // SM_OP_PROCESS_GO_DICT_TYPE on entry, propagated through call context
  // for return probes.
  uint64_t saved_dict_ptr;

  // Continuation state. These live in stack_machine_t (which is backed by
  // a per-CPU array map) rather than on global_ctx (which is a stack local
  // in probe_run) because the verifier's combined stack budget across
  // nested subprog calls is 512 bytes and we have very little slack on
  // the probe_run_with_cookie -> sm_loop -> sm_swiss_map_aese path.
  //
  // continuation_seq: how many fragments have been submitted so far.
  uint16_t continuation_seq;
  // continuation_seq of the last *successfully* submitted fragment, or
  // LAST_SUBMITTED_SEQ_NONE if no fragment has been submitted yet. Used to
  // fill last_seq on drop notifications so userspace knows exactly how
  // many fragments to expect when it reconstructs a truncated event.
  uint16_t last_submitted_seq;
  // Set true when a mid-chase flush failed: some fragments reached
  // userspace but a later fragment couldn't be written. probe_run checks
  // this flag after chasing completes and, if set, sends a PARTIAL_*
  // notification and skips the final submit rather than emit a fragment
  // with a gap.
  bool continuation_aborted;
  // Original probe invocation timestamp, shared across all continuation
  // fragments for correlation.
  uint64_t start_ns;
  // Invocation ID. For entry / line / inlined / no-body probes, this is
  // the probe's own start_ns. For return probes, it is the entry's
  // start_ns, pulled from in_progress_calls via call_depths_delete. Drop
  // notifications carry this so userspace can key them by the same
  // invocation identifier as the main-channel fragments.
  uint64_t entry_ktime_ns;

  // Temporary data, stored here to save on stack space.
  uint64_t value_0;
  resolved_go_any_type_t resolved_0, resolved_1;
  buf_offset_t buf_offset_0, buf_offset_1;
  di_data_item_header_t di_0;

  // Swiss map hash scratch space for AES computation.
  struct {
    uint8_t state[16];        // current AESENC/AESE target (active lane)
    uint8_t unscrambled[16];  // original seed state before scramble / custom rk
    uint8_t tmp[16];          // scratch for SubBytes+ShiftRows
    uint8_t lanes[8][16];     // 8 AES lanes for parallel hashing
    uint8_t seeds[8][16];     // 8 scrambled seeds
  } hash_scratch;

  // Swiss map lookup state, persisted across sm_loop iterations.
  // Written by SM_OP_SWISS_MAP_SETUP, read by subsequent opcodes.
  struct {
    // Probe state.
    target_ptr_t groups_data_ptr;
    uint64_t length_mask;
    target_ptr_t group_addr;
    uint64_t h2_matches;
    uint64_t empty_matches;
    uint64_t probe_offset;
    uint64_t probe_index;

    // Result/key locations in scratch buffer.
    buf_offset_t key_data_off;
    buf_offset_t result_offset;
    uint32_t val_byte_size;
    uint32_t expr_status_idx;
    uint16_t key_data_len;

    // Layout parameters (from bytecode).
    uint16_t slot_size;
    uint16_t group_byte_size;
    uint8_t h2;
    uint8_t ctrl_offset;
    uint8_t slots_offset;
    uint8_t key_in_slot_offset;
    uint16_t val_in_slot_offset;
    uint8_t key_byte_size;
    uint8_t is_string_key;
    // When set, the lookup writes a 1-byte bool at result_offset on key
    // match (skipping the value dereference), and converts nil-map /
    // key-not-found into a bool-false result instead of EXPR_STATUS_OOB +
    // abort. See ir.SwissMapLookupOp.ExistenceOnly.
    uint8_t existence_only;

    // Hash computation state.
    uint8_t hash_phase;
    uint8_t aes_rounds_left;  // rounds remaining for current AESENC target
    uint8_t use_aes;
    uint8_t aes_self_keyed;
    uint8_t aes_rk_offset;    // offset into g_swiss_aeskeysched
    uint8_t aes_skip_mc;      // skip MixColumns for current round (arm64 final rounds)
    uint8_t aes_final_skip_mc; // last round of current batch should skip MC
    uint8_t aes_rk_no_advance; // don't advance aes_rk_offset between rounds

    // Multi-lane hash state.
    uint8_t num_lanes;         // total lanes for current hash tier (1,2,4,8)
    uint8_t current_lane;      // index of lane currently being processed
    uint8_t hash_key_len;      // cached key length (clamped to <=255 for uint8)
    uint16_t hash_key_len_full; // full key length (up to 512)
    uint16_t block_offset;     // byte offset into key data for 129+ block loop
    uint16_t blocks_remaining; // number of 128-byte blocks left to process

    // Slot check params — stored here to avoid a 40-byte stack local
    // in sm_loop's CHECK_SLOT case (reduces combined stack usage).
    swiss_map_slot_params_t slot_params;
  } swiss_map_state;

  // Slice-loop iteration state for any/all over a slice or array. Only one
  // such loop can be active at a time (irgen rejects nested any/all), so a
  // single set of fields suffices.
  //
  // `capped` is set true when the collection's length exceeded the
  // iteration cap and we truncated `remaining` to the cap; if the loop
  // exhausts the capped count without short-circuiting, the end op trips
  // eval_error to signal "result inconclusive due to cap".
  struct {
    target_ptr_t data_ptr;
    // Raw 64-bit length read from the collection header. The executable
    // iteration count below is capped before it is stored in remaining.
    uint64_t initial_len;
    uint32_t remaining;
    buf_offset_t accumulator_off;
    uint32_t elem_size;
    uint8_t quantifier;
    bool capped;
  } slice_loop_state;

  // Swiss-map any/all iteration state. Holds the streaming-walk position
  // (dir_idx, group_idx, slot_idx) plus cached layout parameters and the
  // synthetic @it scratch slot location.
  struct {
    target_ptr_t dir_ptr;
    target_ptr_t prev_table_ptr;     // dedup consecutive aliased table pointers
    target_ptr_t groups_data;
    target_ptr_t length_mask;
    uint64_t ctrl;                    // cached ctrl bytes for current group
    target_ptr_t tmp_table_ptr;       // scratch for step_advance, avoids stack local
    uint32_t dir_len;
    uint32_t table_idx;
    uint32_t group_idx;
    uint8_t slot_idx;
    uint8_t loaded_table;             // 1 if (groups_data, length_mask) populated
    // phase encodes whether the next entry to the Begin/End SM_OP handler
    // is a "first entry" (needs init / body-result check) or a back-edge
    // retry from a previous step that returned "keep stepping" (r==2). The
    // SM_OP handlers use `sm->pc -=` to retry themselves while the cursor
    // walks empty slots, so phase distinguishes the two paths without
    // requiring a separate Advance opcode.
    //   0 = fresh entry (Begin: needs init; End: needs short-circuit check)
    //   1 = back-edge retry (skip init / short-circuit, just step again)
    uint8_t phase;
    uint32_t iterations;             // successful body invocations so far
    uint32_t scan_steps;             // cumulative single-step calls; eval_error guard
    buf_offset_t accumulator_off;
    uint32_t end_label_pc;           // jump target on exhaustion / nil-map / short-circuit
    uint32_t body_label_pc;          // jump target on the next found slot (End handler)
    uint32_t key_byte_size;
    uint32_t val_byte_size;
    uint8_t quantifier;

    // Cached layout (re-read on each Begin from the op params).
    uint8_t dir_ptr_offset;
    uint8_t dir_len_offset;
    uint8_t ctrl_offset;
    uint8_t slots_offset;
    uint8_t key_in_slot_offset;
    uint16_t val_in_slot_offset;
    uint16_t slot_size;
    uint16_t group_byte_size;
    uint8_t table_groups_field_offset;
    uint8_t groups_data_field_offset;
    uint8_t groups_len_mask_field_offset;
  } swissmap_loop_state;

  // Scratch offset where the active any/all loop placed its @it bytes.
  // Set by the loop's Begin op and consumed by ExprAdvanceOffsetOp so a
  // body sub-expression can re-anchor sm->offset on @it regardless of
  // where the previous body op left it.
  buf_offset_t cur_loop_it_start;
} stack_machine_t;

// filter_loop_state_t holds per-iteration state for the deferred
// filter() loop. It lives in its own per-CPU map (filter_loop_state_buf
// below) rather than inside stack_machine_t so that:
//   - stack_machine_t doesn't grow large enough to push existing field
//     offsets out of the BPF verifier's bounds-tracking window for
//     cold-path resets via a spilled-and-reloaded pointer.
//   - the filter helpers can re-derive the pointer cheaply via
//     bpf_map_lookup_elem when they need fresh verifier bounds.
//
// One slot is sufficient: filter is leaf-only, the chase queue is FIFO,
// and each filter's enqueue_pc runs to completion before the next
// chase item pops.
typedef struct filter_loop_state {
  target_ptr_t data_ptr;
  uint32_t     remaining;
  uint32_t     elem_size;
  uint32_t     val_offset_in_pair;  // maps only
  uint32_t     val_size;             // maps only
  uint64_t     output_index;
  type_t       data_type_id;
  buf_offset_t it_start;
  uint8_t      last_body_result;
  uint8_t      in_progress;
  uint8_t      is_map;
  // map_slot_returned: 1 if the current map_slot_idx was already read
  // into @it and handed to predicate/emit. On the next entry to
  // sm_filter_map_advance_and_read_noctx, this flag triggers an
  // advance past that slot before scanning. Decoupling read from
  // advance is required because emit reads the slot again from user
  // memory via slot_base = ... + slot_idx * slot_size; advancing
  // slot_idx before emit would make emit read the wrong slot.
  uint8_t      map_slot_returned;
  // Swiss-map walker state, mirroring swissmap_loop_state's layout
  // when is_map == 1. Kept separate so a concurrent any/all loop's
  // swissmap_loop_state isn't clobbered.
  uint64_t map_dir_ptr;
  uint64_t map_dir_len;
  uint64_t map_groups_data;
  uint64_t map_length_mask;
  uint64_t map_ctrl;
  target_ptr_t map_prev_table_ptr;
  target_ptr_t map_tmp_table_ptr;
  uint32_t map_table_idx;
  uint32_t map_group_idx;
  uint8_t  map_slot_idx;
  uint8_t  map_loaded_table;
  uint8_t  __map_pad[6];
  // Swiss-map layout immediates (cached from InitFilterMapLoopOp).
  uint8_t  map_dir_ptr_offset;
  uint8_t  map_dir_len_offset;
  uint8_t  map_ctrl_offset;
  uint8_t  map_slots_offset;
  uint8_t  map_key_in_slot_offset;
  uint16_t map_val_in_slot_offset;
  uint16_t map_slot_size;
  uint16_t map_group_byte_size;
  uint8_t  map_table_groups_field_offset;
  uint8_t  map_groups_data_field_offset;
  uint8_t  map_groups_len_mask_field_offset;
  uint8_t  __map_pad2[3];
} filter_loop_state_t;

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, filter_loop_state_t);
} filter_loop_state_buf SEC(".maps");

// filter_loop_state_load returns the per-CPU filter_loop_state slot.
// Each call re-derives the pointer via bpf_map_lookup_elem so the
// verifier has fresh bounds knowledge — important because the slot
// is much larger than the verifier's bounds-tracking window allows
// across function calls / bpf_loop boundaries.
static filter_loop_state_t* filter_loop_state_load(void) {
  const unsigned long zero = 0;
  return (filter_loop_state_t*)bpf_map_lookup_elem(&filter_loop_state_buf, &zero);
}

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, stack_machine_t);
} stack_machine_buf SEC(".maps");

static stack_machine_t* stack_machine_ctx_load(const probe_params_t* probe_params) {
  const unsigned long zero = 0;
  stack_machine_t* stack_machine =
      (stack_machine_t*)bpf_map_lookup_elem(&stack_machine_buf, &zero);
  if (!stack_machine) {
    return (stack_machine_t*)NULL;
  }
  stack_machine->pc_stack_pointer = 0;
  stack_machine->data_stack_pointer = 0;
  stack_machine->condition_failed = false;
  stack_machine->condition_eval_error = false;
  stack_machine->condition_nil_deref = false;
  stack_machine->condition_state = 0;
  chased_pointers_trie_init(&stack_machine->chased);
  chased_slices_init(&stack_machine->chased_slices);
  stack_machine->pointer_chasing_ttl = probe_params->pointer_chasing_limit;
  stack_machine->collection_size_limit = probe_params->collection_size_limit;
  stack_machine->string_size_limit = probe_params->string_size_limit;
  stack_machine->pointers_queue.len = 0;
  stack_machine->saved_dict_ptr = 0;
  stack_machine->continuation_seq = 0;
  stack_machine->last_submitted_seq = LAST_SUBMITTED_SEQ_NONE;
  stack_machine->continuation_aborted = false;
  // pending_expr_status lives early in the struct so the verifier can
  // prove bounds on a hot-path reset. filter_loop_state lives at the
  // end of the struct and is unconditionally re-initialized by
  // sm_filter_*_init from sm->di_0 every chase, so no hot-path reset
  // is needed for it.
  stack_machine->pending_expr_status = 0;
  // start_ns and entry_ktime_ns are set explicitly by probe_run before use.
  return stack_machine;
}

typedef struct target_stack {
  stack_pcs_t pcs;
  // The in-use length is stored in pcs.len.
  target_ptr_t fps[STACK_DEPTH];
} target_stack_t;

typedef struct stack_walk_ctx {
  // Difference between populate_stack_frame loop index and
  // populated stack size.
  int16_t idx_shift;
  struct pt_regs regs;
  target_stack_t stack;
} stack_walk_ctx_t;

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, stack_walk_ctx_t);
} walk_stack_ctx_buf SEC(".maps");

static stack_walk_ctx_t* stack_walk_ctx_load() {
  const unsigned long zero = 0;
  stack_walk_ctx_t* stack =
      (stack_walk_ctx_t*)bpf_map_lookup_elem(&walk_stack_ctx_buf, &zero);
  if (!stack) {
    return (stack_walk_ctx_t*)NULL;
  }
  stack->idx_shift = 0;
  stack->stack.pcs.len = 0;
  return stack;
}

typedef struct global_ctx {
  // Output and scratch buffer.
  scratch_buf_t* buf;
  // Context for stack matchine.
  stack_machine_t* stack_machine;
  // Context for stack walking.
  stack_walk_ctx_t* stack_walk;
  // Set during goroutine iteration, read during stack machine execution.
  // Declared here, as pointers in maps are treated as scalars by verifier.
  struct pt_regs* regs;
} global_ctx_t;

typedef struct call_depths_entry {
  uint32_t depth;
  // The compiler asserts probe_id fits in uint16.
  uint16_t probe_id;
  // condition_state packs up to 8 per-leaf 2-bit statuses (see
  // LEAF_STATUS_* in this file). Written by the entry-side driver via
  // SM_OP_CONDITION_LEAF_RECORD, copied here at insertion time, copied
  // back into the SM at call_depths_delete time so the return-side
  // condition program can read individual leaves via
  // SM_OP_CONDITION_LEAF_LOAD. Zero for non-split probes.
  uint16_t condition_state;
  uint64_t dict_ptr; // dictionary pointer for generic shape functions (0 if N/A)
  // Timestamp of the entry event that established this call. Returned to the
  // return probe via call_depths_delete and stamped as entry_ktime_ns on the
  // return event header so userspace can correlate entry and return for the
  // same invocation.
  uint64_t entry_ktime_ns;
} call_depths_entry_t;

#define CALL_DEPTHS_SIZE 8

// Call depths is a set of call depths at entry that are used to track
// the in-progress calls. It is unsorted. Zero-valued entries are considered
// available for insertion.
typedef struct call_depths {
  call_depths_entry_t depths[CALL_DEPTHS_SIZE];
} call_depths_t;

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 8192);
  __type(key, uint64_t); // goid
  __type(value, call_depths_t);
} in_progress_calls SEC(".maps");

static inline __attribute__((always_inline)) bool call_depths_insert(
    call_depths_t* depths, uint32_t depth, uint16_t probe_id,
    uint16_t condition_state,
    uint64_t dict_ptr, uint64_t entry_ktime_ns) {
  for (int i = 0; i < CALL_DEPTHS_SIZE; i++) {
    if (depths->depths[i].depth == 0 && depths->depths[i].probe_id == 0) {
      depths->depths[i].depth = depth;
      depths->depths[i].probe_id = probe_id;
      depths->depths[i].condition_state = condition_state;
      depths->depths[i].dict_ptr = dict_ptr;
      depths->depths[i].entry_ktime_ns = entry_ktime_ns;
      return true;
    }
  }
  return false;
}

static inline __attribute__((always_inline)) bool call_depths_delete(
    call_depths_t* depths, uint32_t depth, uint16_t probe_id,
    int* remaining, uint64_t* out_dict_ptr, uint64_t* out_entry_ktime_ns,
    uint16_t* out_condition_state) {
  bool found = false;
  for (int i = 0; i < CALL_DEPTHS_SIZE; i++) {
    if (depths->depths[i].depth == depth && depths->depths[i].probe_id == probe_id) {
      if (out_dict_ptr) {
        *out_dict_ptr = depths->depths[i].dict_ptr;
      }
      if (out_entry_ktime_ns) {
        *out_entry_ktime_ns = depths->depths[i].entry_ktime_ns;
      }
      if (out_condition_state) {
        *out_condition_state = depths->depths[i].condition_state;
      }
      depths->depths[i].depth = 0;
      depths->depths[i].probe_id = 0;
      depths->depths[i].condition_state = 0;
      depths->depths[i].dict_ptr = 0;
      depths->depths[i].entry_ktime_ns = 0;
      found = true;
    } else if (depths->depths[i].depth != 0 || depths->depths[i].probe_id != 0) {
      (*remaining)++;
    }
  }
  return found;
}

#endif // __CONTEXT_H__
