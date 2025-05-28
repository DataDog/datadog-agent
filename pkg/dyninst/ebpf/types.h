#ifndef __TYPES_H__
#define __TYPES_H__

#include "ktypes.h"

// Common types.

// TODO: ifdef-control between including generated codes and stubs to avoid compilation errors in IDE.

// Note that this cannot just be uintptr_t because the BPF target has 32-bit
// pointers.
typedef uint64_t target_ptr_t;

// A stub for the address of key runtime variables and offsets into their data
// structures. In the generated code these variables and offsets will have the
// correct values.
const target_ptr_t VARIABLE_runtime_dot_firstmoduledata = 0;
const uint64_t OFFSET_runtime_dot_moduledata__types = 0;
const uint64_t OFFSET_runtime_dot_eface___type = 0;
const uint64_t OFFSET_runtime_dot_eface__data = 0;
const uint64_t OFFSET_runtime_dot_iface__data = 0;
const uint64_t OFFSET_runtime_dot_iface__tab = 0;
const uint64_t OFFSET_runtime_dot_itab___type = 0;

#define RUNTIME_DOT_G_PREFIX_BYTES 0

// A stub enum corresponding to a generated type.
typedef enum type { TYPE_NONE = 0 } type_t;

// A stub for the enum corresponding to the data of a string.
const type_t string_data_type = TYPE_NONE;

// A stub enum corresponding to a generated subprogram of interest.
typedef enum prog { PROG_NONE = 0 } prog_t;

// Location for stack machine program to chase pointers.
static const uint32_t chase_pointers_entrypoint = 0;

typedef struct probe_params {
  uint32_t stack_machine_pc;
  uint32_t stream_id;
  bool frameless;
  bool return_event;
  bool capture_stack;
} probe_params_t;

extern const probe_params_t probe_params[];
extern const uint64_t num_probe_params;

typedef struct frame_data {
  uint16_t stack_idx;
  uint64_t fp;
} frame_data_t;

typedef struct go_context_impl {
  // Offset of the wrapped go context, or -1 for leaf impl.
  int32_t context_offset;
  // Offsets of stored key and value, or -1.
  int32_t key_offset;
  int32_t value_offset;
} go_context_impl_t;

typedef struct go_context_value_type {
  // Position of the capture type within overall go context capture structure,
  // or -1 if the type is not interesting to capture.
  int32_t index;
  int32_t offset;
  // Type used to capture the go context value, or 0 if the original type should
  // be used.
  type_t type;
} go_context_value_type_t;

typedef struct type_info {
  uint32_t byte_len;
  uint32_t enqueue_pc;

  // When chasing pointer to this type, whether to serialize the data behind the pointer,
  // before running enqueue function.
  // TODO: fold data serialization into the enqueue function to avoid this knob.
  bool serialize_before_enqueue;

  // Details of type implementing go context.
  go_context_impl_t go_context_impl;

  // Go context value spec identified by this type being the key.
  go_context_value_type_t go_context_key;
  // Expected type of the value, or 0 if type is unrestricted.
  type_t go_context_key_value_type;

  // Go context value spec identified by this type being the value.
  go_context_value_type_t go_context_value;
} type_info_t;

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

static bool get_type_info(type_t t, const type_info_t** info_out) {
  return false;
}

static type_t lookup_go_subroutine(uint64_t pc) {
  return TYPE_NONE;
}

#endif // __TYPES_H__
