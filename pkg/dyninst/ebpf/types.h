#ifndef __TYPES_H__
#define __TYPES_H__

#include "ktypes.h"
#include "binary_search.h"

// Common types.

typedef struct type_info {
  uint32_t byte_len;
  uint32_t enqueue_pc;
} type_info_t;

typedef struct probe_params {
  uint32_t stack_machine_pc;
  uint32_t stream_id;
  bool frameless;
} probe_params_t;

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
  SM_OP_PROCESS_ARRAY_PREP = 11,
  SM_OP_PROCESS_ARRAY_REPEAT = 12,
  SM_OP_PROCESS_SLICE = 13,
  SM_OP_PROCESS_SLICE_DATA_PREP = 14,
  SM_OP_PROCESS_SLICE_DATA_REPEAT = 15,
  SM_OP_PROCESS_STRING = 16,
  SM_OP_PROCESS_GO_EMPTY_INTERFACE = 17,
  SM_OP_PROCESS_GO_INTERFACE = 18,
  SM_OP_PROCESS_GO_HMAP = 19,
  SM_OP_PROCESS_GO_SWISS_MAP = 20,
  SM_OP_PROCESS_GO_SWISS_MAP_GROUPS = 21,
  // Top level ops.
  SM_OP_CHASE_POINTERS = 22,
  SM_OP_PREPARE_EVENT_ROOT = 23,

  // Legacy ops, to be adopted.
  SM_OP_ENQUEUE_POINTER = 26,
  SM_OP_ENQUEUE_SLICE_HEADER = 27,
  SM_OP_ENQUEUE_STRING_HEADER = 28,
  SM_OP_ENQUEUE_GO_EMPTY_INTERFACE = 29,
  SM_OP_ENQUEUE_GO_INTERFACE = 30,
  SM_OP_ENQUEUE_GO_HMAP_HEADER = 31,
  SM_OP_ENQUEUE_GO_SWISS_MAP = 32,
  SM_OP_ENQUEUE_GO_SWISS_MAP_GROUPS = 33,
  SM_OP_ENQUEUE_GO_SUBROUTINE = 34,
  SM_OP_DEREFERENCE_CFA_OFFSET = 35,
  SM_OP_COPY_FROM_REGISTER = 36,
  SM_OP_PREPARE_EXPR_EVAL = 37,
  SM_OP_SAVE_EXPR_RESULT = 38,
  SM_OP_DEREFERENCE_PTR = 39,
  SM_OP_ZERO_FILL = 40,
  SM_OP_SET_PRESENCE_BIT = 41,
  SM_OP_PREPARE_POINTEE_DATA = 42,
  SM_OP_PREPARE_EVENT_DATA = 43,
} sm_opcode_t;


#ifdef DYNINST_DEBUG
static const char* op_code_name(sm_opcode_t op_code) {
  switch (op_code) {
  case SM_OP_INVALID: return "INVALID";
  case SM_OP_CALL: return "CALL";
  case SM_OP_RETURN: return "RETURN";
  case SM_OP_ILLEGAL: return "ILLEGAL";
  case SM_OP_INCREMENT_OUTPUT_OFFSET: return "INCREMENT_OUTPUT_OFFSET";
  case SM_OP_EXPR_PREPARE: return "EXPR_PREPARE";
  case SM_OP_EXPR_SAVE: return "EXPR_SAVE";
  case SM_OP_EXPR_DEREFERENCE_CFA: return "EXPR_DEREFERENCE_CFA";
  case SM_OP_EXPR_READ_REGISTER: return "EXPR_READ_REGISTER";
  case SM_OP_EXPR_DEREFERENCE_PTR: return "EXPR_DEREFERENCE_PTR";
  case SM_OP_PROCESS_POINTER: return "PROCESS_POINTER";
  case SM_OP_PROCESS_ARRAY_PREP: return "PROCESS_ARRAY_PREP";
  case SM_OP_PROCESS_ARRAY_REPEAT: return "PROCESS_ARRAY_REPEAT";
  case SM_OP_PROCESS_SLICE: return "PROCESS_SLICE";
  case SM_OP_PROCESS_SLICE_DATA_PREP: return "PROCESS_SLICE_DATA_PREP";
  case SM_OP_PROCESS_SLICE_DATA_REPEAT: return "PROCESS_SLICE_DATA_REPEAT";
  case SM_OP_PROCESS_STRING: return "PROCESS_STRING";
  case SM_OP_PROCESS_GO_EMPTY_INTERFACE: return "PROCESS_GO_EMPTY_INTERFACE";
  case SM_OP_PROCESS_GO_INTERFACE: return "PROCESS_GO_INTERFACE";
  case SM_OP_PROCESS_GO_HMAP: return "PROCESS_GO_HMAP";
  case SM_OP_PROCESS_GO_SWISS_MAP: return "PROCESS_GO_SWISS_MAP";
  case SM_OP_PROCESS_GO_SWISS_MAP_GROUPS: return "PROCESS_GO_SWISS_MAP_GROUPS";
  case SM_OP_CHASE_POINTERS: return "CHASE_POINTERS";
  case SM_OP_PREPARE_EVENT_ROOT: return "PREPARE_EVENT_ROOT";
  case SM_OP_ENQUEUE_POINTER: return "ENQUEUE_POINTER";
  case SM_OP_ENQUEUE_SLICE_HEADER: return "ENQUEUE_SLICE_HEADER";
  case SM_OP_ENQUEUE_STRING_HEADER: return "ENQUEUE_STRING_HEADER";
  case SM_OP_ENQUEUE_GO_EMPTY_INTERFACE: return "ENQUEUE_GO_EMPTY_INTERFACE";
  case SM_OP_ENQUEUE_GO_INTERFACE: return "ENQUEUE_GO_INTERFACE";
  case SM_OP_ENQUEUE_GO_HMAP_HEADER: return "ENQUEUE_GO_HMAP_HEADER";
  case SM_OP_ENQUEUE_GO_SWISS_MAP: return "ENQUEUE_GO_SWISS_MAP";
  case SM_OP_ENQUEUE_GO_SWISS_MAP_GROUPS: return "ENQUEUE_GO_SWISS_MAP_GROUPS";
  case SM_OP_ENQUEUE_GO_SUBROUTINE: return "ENQUEUE_GO_SUBROUTINE";
  case SM_OP_DEREFERENCE_CFA_OFFSET: return "DEREFERENCE_CFA_OFFSET";
  case SM_OP_COPY_FROM_REGISTER: return "COPY_FROM_REGISTER";
  case SM_OP_PREPARE_EXPR_EVAL: return "PREPARE_EXPR_EVAL";
  case SM_OP_SAVE_EXPR_RESULT: return "SAVE_EXPR_RESULT";
  case SM_OP_DEREFERENCE_PTR: return "DEREFERENCE_PTR";
  case SM_OP_ZERO_FILL: return "ZERO_FILL";
  case SM_OP_SET_PRESENCE_BIT: return "SET_PRESENCE_BIT";
  case SM_OP_PREPARE_POINTEE_DATA: return "PREPARE_POINTEE_DATA";
  case SM_OP_PREPARE_EVENT_DATA: return "PREPARE_EVENT_DATA";
  default: break;
  }
  return "UNKNOWN";
}
#endif

#ifdef DYNINST_GENERATED_CODE

{{ . }}

#else

typedef enum type { TYPE_NONE = 0 } type_t;
const type_info_t type_info[] = {};
const uint32_t type_ids[] = {};
const uint32_t num_types = 0;

#endif

DEFINE_BINARY_SEARCH(
  lookup_type_info,
  type_t,
  type_id,
  type_ids,
  num_types
);

static bool get_type_info(type_t t, const type_info_t** info_out) {
  uint32_t idx = lookup_type_info_by_type_id(t);
  if (idx >= num_types || type_ids[idx] != t) {
    return false;
  }
  *info_out = &type_info[idx];
  return true;
}

// Note that this cannot just be uintptr_t because the BPF target has 32-bit
// pointers.
typedef uint64_t target_ptr_t;

typedef struct frame_data {
  uint16_t stack_idx;
  uint64_t fp;
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

#endif // __TYPES_H__
