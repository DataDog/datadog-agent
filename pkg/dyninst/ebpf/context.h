#ifndef __CONTEXT_H__
#define __CONTEXT_H__

#include "bpf_tracing.h"
#include "types.h"
#include "queue.h"
#include "scratch.h"

typedef uint32_t type_t;

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

#define MAX_CHASED_POINTERS 128
typedef struct chased_pointers {
  uint32_t n;
  target_ptr_t ptrs[MAX_CHASED_POINTERS];
  type_t types[MAX_CHASED_POINTERS];
} chased_pointers_t;

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
  chased_pointers_t chased;
  // Remaining pointer chasing limit, given currently processed data item.
  // Maybe 0, in which case data might still be processed (i.e. interface type rewrite),
  // but no further pointers will be chased.
  uint32_t pointer_chasing_ttl;

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

  // Temporary data, stored here to save on stack space.
  uint64_t value_0;
  resolved_go_any_type_t resolved_0, resolved_1;
  buf_offset_t buf_offset_0, buf_offset_1;
  di_data_item_header_t di_0;
} stack_machine_t;

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, stack_machine_t);
} stack_machine_buf SEC(".maps");

static stack_machine_t* stack_machine_ctx_load(uint32_t pointer_chasing_limit) {
  const unsigned long zero = 0;
  stack_machine_t* stack_machine =
      (stack_machine_t*)bpf_map_lookup_elem(&stack_machine_buf, &zero);
  if (!stack_machine) {
    return (stack_machine_t*)NULL;
  }
  stack_machine->pc_stack_pointer = 0;
  stack_machine->data_stack_pointer = 0;
  stack_machine->chased.n = 0;
  stack_machine->pointer_chasing_ttl = pointer_chasing_limit;
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

#endif // __CONTEXT_H__
