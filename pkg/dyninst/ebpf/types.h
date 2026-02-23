#ifndef __TYPES_H__
#define __TYPES_H__

// Types used to program the stack machine and event processing.

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
  char __padding[3];
} probe_params_t;

typedef struct throttler_params {
  uint64_t period_ns;
  int64_t budget;
} throttler_params_t;

typedef struct stats {
  uint64_t cpu_ns;
  uint64_t hit_cnt;
  uint64_t throttled_cnt;
} stats_t;

typedef enum dynamic_size_class {
  DYNAMIC_SIZE_CLASS_STATIC = 0,
  DYNAMIC_SIZE_CLASS_SLICE = 1,
  DYNAMIC_SIZE_CLASS_STRING = 2,
  DYNAMIC_SIZE_CLASS_HASHMAP = 3,
} dynamic_size_class_t;

typedef struct type_info {
  dynamic_size_class_t dynamic_size_class;
  uint32_t byte_len;
  uint32_t enqueue_pc;
  uint32_t __padding;
} type_info_t;

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
  SM_OP_PROCESS_GO_HMAP = 18,
  SM_OP_PROCESS_GO_SWISS_MAP = 19,
  SM_OP_PROCESS_GO_SWISS_MAP_GROUPS = 20,
  // Top level ops.
  SM_OP_CHASE_POINTERS = 21,
  SM_OP_PREPARE_EVENT_ROOT = 22,
} sm_opcode_t;

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
  default:
    break;
  }
  return "UNKNOWN";
}
#endif

#endif // __TYPES_H__
