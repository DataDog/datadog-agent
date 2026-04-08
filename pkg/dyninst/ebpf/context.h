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
  // Set to true by ConditionBegin, cleared by ConditionCheck. If a condition
  // exits early (e.g. nil pointer dereference), this flag remains set to
  // signal that the condition could not be fully evaluated.
  bool condition_eval_error;
  // Set to true when a nil pointer dereference causes an expression or
  // condition evaluation to abort. Used together with condition_eval_error
  // to distinguish nil-caused failures from other evaluation errors.
  bool condition_nil_deref;

  // Dictionary pointer for generic shape functions. Set by
  // SM_OP_PROCESS_GO_DICT_TYPE on entry, propagated through call context
  // for return probes.
  uint64_t saved_dict_ptr;

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
    uint8_t val_in_slot_offset;
    uint8_t key_byte_size;
    uint8_t is_string_key;

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
} stack_machine_t;

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
  chased_pointers_trie_init(&stack_machine->chased);
  chased_slices_init(&stack_machine->chased_slices);
  stack_machine->pointer_chasing_ttl = probe_params->pointer_chasing_limit;
  stack_machine->collection_size_limit = probe_params->collection_size_limit;
  stack_machine->string_size_limit = probe_params->string_size_limit;
  stack_machine->pointers_queue.len = 0;
  stack_machine->saved_dict_ptr = 0;
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
  uint32_t probe_id;
  uint64_t dict_ptr; // dictionary pointer for generic shape functions (0 if N/A)
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
    call_depths_t* depths, uint32_t depth, uint32_t probe_id, uint64_t dict_ptr) {
  for (int i = 0; i < CALL_DEPTHS_SIZE; i++) {
    if (depths->depths[i].depth == 0 && depths->depths[i].probe_id == 0) {
      depths->depths[i].depth = depth;
      depths->depths[i].probe_id = probe_id;
      depths->depths[i].dict_ptr = dict_ptr;
      return true;
    }
  }
  return false;
}

static inline __attribute__((always_inline)) bool call_depths_delete(
    call_depths_t* depths, uint32_t depth, uint32_t probe_id,
    int* remaining, uint64_t* out_dict_ptr) {
  bool found = false;
  for (int i = 0; i < CALL_DEPTHS_SIZE; i++) {
    if (depths->depths[i].depth == depth && depths->depths[i].probe_id == probe_id) {
      if (out_dict_ptr) {
        *out_dict_ptr = depths->depths[i].dict_ptr;
      }
      depths->depths[i].depth = 0;
      depths->depths[i].probe_id = 0;
      depths->depths[i].dict_ptr = 0;
      found = true;
    } else if (depths->depths[i].depth != 0 || depths->depths[i].probe_id != 0) {
      (*remaining)++;
    }
  }
  return found;
}

#endif // __CONTEXT_H__
