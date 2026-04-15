#ifndef __STACK_MACHINE_H__
#define __STACK_MACHINE_H__

#include "bpf_tracing.h"
#include "compiler.h"
#include "context.h"
#include "debug.h"
#include "framing.h"
#include "scratch.h"
#include "types.h"
#include "program.h"
#include "queue.h"
#include "chased_pointers_trie.h"
#include "swiss_map_hash.h"

// Expression status values. Each expression gets EXPR_STATUS_BITS bits in the
// expression status array at the start of event root data.
#define EXPR_STATUS_BITS      2
#define EXPR_STATUS_ABSENT    0  // evaluation failed (unknown reason)
#define EXPR_STATUS_PRESENT   1  // evaluation succeeded
#define EXPR_STATUS_NIL_DEREF 2  // nil pointer dereference
#define EXPR_STATUS_OOB       3  // index out of bounds

// Sentinel value for expr_status_idx indicating no status should be written
// (used by condition expressions which report errors via condition_eval_error).
#define EXPR_STATUS_IDX_NONE 0xFFFFFFFF

// expr_status_write writes a status value into the packed expression status
// array for the given expression index.
static __always_inline void expr_status_write(
    scratch_buf_t* buf,
    buf_offset_t base_offset,
    uint32_t expr_idx,
    uint8_t status
) {
    uint32_t bit_offset = expr_idx * EXPR_STATUS_BITS;
    buf_offset_t byte_offset = base_offset + bit_offset / 8;
    uint32_t shift = bit_offset % 8;
    if (!scratch_buf_bounds_check(&byte_offset, 1)) return;
    uint8_t mask = ((1 << EXPR_STATUS_BITS) - 1) << shift;
    (*buf)[byte_offset] = ((*buf)[byte_offset] & ~mask) | ((status & ((1 << EXPR_STATUS_BITS) - 1)) << shift);
}

const int32_t defaultCollectionSizeBytesLimit = 512;

DEFINE_BINARY_SEARCH(
    lookup_type_info,
    type_t,
    type_id,
    type_ids,
    num_types);

static bool get_type_info(type_t t, const type_info_t** info_out) {
  uint32_t idx = lookup_type_info_by_type_id(t);
  *info_out = bpf_map_lookup_elem(&type_info, &idx);
  if (!*info_out) {
    return false;
  }
  return true;
}

DEFINE_BINARY_SEARCH(
    lookup_type_id,
    uint32_t,
    go_runtime_type,
    go_runtime_types,
    num_go_runtime_types);

static type_t lookup_go_interface(uint32_t go_runtime_type) {
  if (go_runtime_type == 0) {
    return 0;
  }
  uint32_t idx = lookup_type_id_by_go_runtime_type(go_runtime_type);
  uint32_t* got = bpf_map_lookup_elem(&go_runtime_types, &idx);
  if (!got || *got != go_runtime_type) {
    return 0;
  }
  uint32_t* type_id = bpf_map_lookup_elem(&go_runtime_type_ids, &idx);
  if (!type_id) {
    return 0;
  }
  return *type_id;
}

static bool chased_pointers_trie_push(chased_pointers_trie_t* chased, target_ptr_t ptr,
                                      type_t type) {
  switch (chased_pointers_trie_insert(chased, ptr, type)) {
  case CHASED_POINTERS_TRIE_INSERTED:
    return true;
  case CHASED_POINTERS_TRIE_ALREADY_EXISTS:
    break;
  case CHASED_POINTERS_TRIE_FULL:
    LOG(3, "chased_pointers_push: full %lld %d\n", ptr, type);
    break;
  case CHASED_POINTERS_TRIE_NULL:
    LOG(1, "chased_pointers_push: null %lld %d\n", ptr, type);
    break;
  case CHASED_POINTERS_TRIE_ERROR:
    LOG(1, "chased_pointers_push: error %lld %d\n", ptr, type);
    break;
  }
  return false;
}

typedef struct zero_data_ctx {
  scratch_buf_t* buf;
  buf_offset_t base_offset;
} zero_data_ctx_t;

static long zero_data_loop(unsigned long i, void* _ctx) {
  zero_data_ctx_t* ctx = (zero_data_ctx_t*)_ctx;
  buf_offset_t offset = ctx->base_offset + i;
  if (!scratch_buf_bounds_check(&offset, 1)) {
    return 1;
  }
  (*ctx->buf)[offset] = 0;
  return 0;
}

void zero_data(scratch_buf_t* buf, buf_offset_t base_offset, uint64_t len) {
  zero_data_ctx_t ctx = {
      .buf = buf,
      .base_offset = base_offset,
  };
  bpf_loop(len, zero_data_loop, &ctx, 0);
}

typedef struct copy_data_ctx {
  scratch_buf_t* buf;
  buf_offset_t src;
  buf_offset_t dst;
} copy_data_ctx_t;

static long copy_data_loop(unsigned long i, void* _ctx) {
  copy_data_ctx_t* ctx = (copy_data_ctx_t*)_ctx;
  buf_offset_t src = ctx->src + i;
  buf_offset_t dst = ctx->dst + i;
  if (!scratch_buf_bounds_check(&src, 1)) {
    return 1;
  }
  if (!scratch_buf_bounds_check(&dst, 1)) {
    return 1;
  }
  (*ctx->buf)[dst] = (*ctx->buf)[src];
  return 0;
}

void copy_data(scratch_buf_t* buf, buf_offset_t src, buf_offset_t dst,
               uint64_t len) {
  copy_data_ctx_t ctx = {
      .buf = buf,
      .src = src,
      .dst = dst,
  };
  bpf_loop(len, copy_data_loop, &ctx, 0);
}

static inline uint32_t read_uint32(const uint8_t* buf) {
  return (uint32_t)buf[0] | (uint32_t)buf[1] << 8 | (uint32_t)buf[2] << 16 |
         (uint32_t)buf[3] << 24;
}

static inline int32_t read_int32(const uint8_t* buf) {
  return (int32_t)buf[0] | (int32_t)buf[1] << 8 | (int32_t)buf[2] << 16 |
         (int32_t)buf[3] << 24;
}

static inline uint16_t read_uint16(const uint8_t* buf) {
  return (uint16_t)buf[0] | (uint16_t)buf[1] << 8;
}

static inline __attribute__((always_inline)) uint8_t
sm_read_program_uint8(stack_machine_t* sm) {
  uint32_t zero = 0;
  uint8_t* data = bpf_map_lookup_elem(&stack_machine_code, &zero);
  if (!data) {
    LOG(1, "enqueue: failed to load code\n");
    return 0;
  }
  if (sm->pc >= stack_machine_code_len) {
    LOG(1, "enqueue: code read out of bounds %d >= %d\n", sm->pc, stack_machine_code_len);
    return 0;
  }
  uint8_t param = data[sm->pc];
  sm->pc += 1;
  return param;
}

// Peek at a uint16 at an arbitrary bytecode offset without advancing PC.
static inline __attribute__((always_inline)) uint16_t
sm_read_program_uint16_at(stack_machine_t* sm, uint32_t offset) {
  uint32_t zero = 0;
  uint8_t* data = bpf_map_lookup_elem(&stack_machine_code, &zero);
  if (!data) return 0;
  if (offset >= stack_machine_code_len - 1) return 0;
  return read_uint16(&data[offset]);
}

static inline __attribute__((always_inline)) uint16_t
sm_read_program_uint16(stack_machine_t* sm) {
  uint32_t zero = 0;
  uint8_t* data = bpf_map_lookup_elem(&stack_machine_code, &zero);
  if (!data) {
    LOG(1, "enqueue: failed to load code\n");
    return 0;
  }
  if (sm->pc >= stack_machine_code_len - 1) {
    LOG(1, "enqueue: code read out of bounds %d+1 >= %d\n", sm->pc, stack_machine_code_len);
    return 0;
  }
  uint32_t param = read_uint16(&data[sm->pc]);
  sm->pc += 2;
  return param;
}

static inline __attribute__((always_inline)) uint32_t
sm_read_program_uint32(stack_machine_t* sm) {
  uint32_t zero = 0;
  uint8_t* data = bpf_map_lookup_elem(&stack_machine_code, &zero);
  if (!data) {
    LOG(1, "enqueue: failed to load code\n");
    return 0;
  }
  if (sm->pc >= stack_machine_code_len - 3) {
    LOG(1, "enqueue: code read out of bounds %d+3 >= %d\n", sm->pc, stack_machine_code_len);
    return 0;
  }
  uint32_t param = read_uint32(&data[sm->pc]);
  sm->pc += 4;
  return param;
}

static inline __attribute__((always_inline)) bool
sm_data_stack_push(stack_machine_t* sm, uint32_t value) {
  if (sm->data_stack_pointer >= ENQUEUE_STACK_DEPTH) {
    LOG(2, "enqueue: push on full data stack");
    return false;
  }
  sm->data_stack[sm->data_stack_pointer] = value;
  sm->data_stack_pointer++;
  return true;
}

static inline __attribute__((always_inline)) bool
sm_data_stack_pop(stack_machine_t* sm) {
  if (sm->data_stack_pointer == 0) {
    LOG(2, "enqueue: pop on empty data stack");
    return false;
  }
  sm->data_stack_pointer--;
  if (sm->data_stack_pointer >= ENQUEUE_STACK_DEPTH) {
    LOG(2, "enqueue: stack out of bounds %d", sm->data_stack_pointer);
    return false;
  }
  // Note that we don't need to zero out the stack, but it can help find
  // bugs.
  sm->data_stack[sm->data_stack_pointer] = 0;
  return true;
}

static inline __attribute__((always_inline)) bool sm_return(stack_machine_t* sm) {
  if (sm->pc_stack_pointer == 0) {
    return false;
  }
  sm->pc_stack_pointer--;
  if (sm->pc_stack_pointer >= ENQUEUE_STACK_DEPTH) {
    LOG(2, "enqueue: return early %d", sm->pc_stack_pointer);
    return false;
  }
  sm->pc = sm->pc_stack[sm->pc_stack_pointer];
  // Note that we don't need to zero out the stack, but it can help find
  // bugs.
  sm->pc_stack[sm->pc_stack_pointer] = 0;
  return true;
}

static inline __attribute__((always_inline)) bool
sm_chase_pointer(global_ctx_t* ctx, pointers_queue_item_t item) {
  stack_machine_t* sm = ctx->stack_machine;

  // Serialize object entry.
  const type_info_t* info;
  if (!get_type_info((type_t)item.di.type, &info)) {
    LOG(4, "chase: type info not found %d", item.di.type);
    return true;
  }
  if (info->byte_len == 0) {
    return true;
  }
  uint32_t byte_len = info->byte_len;
  switch (info->dynamic_size_class) {
  case DYNAMIC_SIZE_CLASS_STATIC:
    break;
  case DYNAMIC_SIZE_CLASS_SLICE:
    if (sm->collection_size_limit == -1) {
      byte_len = defaultCollectionSizeBytesLimit;
    } else {
      // In this case the info stores byte len of a single element.
      byte_len = sm->collection_size_limit * info->byte_len;
    }
    break;
  case DYNAMIC_SIZE_CLASS_STRING:
    if (sm->string_size_limit == -1) {
      byte_len = defaultCollectionSizeBytesLimit;
    } else {
      byte_len = sm->string_size_limit;
    }
    break;
  case DYNAMIC_SIZE_CLASS_HASHMAP:
    if (sm->collection_size_limit == -1) {
      byte_len = defaultCollectionSizeBytesLimit * 4;
    } else {
      // In this case the info stores byte len of a single element.
      byte_len = sm->collection_size_limit * info->byte_len * 4;
    }
    break;
  }
  sm->offset = scratch_buf_serialize(ctx->buf, &item.di, byte_len);
  if (!sm->offset) {
    LOG(3, "chase: failed to serialize type %d", item.di.type);
    return true;
  }

  // Recurse if there is more to capture object of this type.
  sm->pointer_chasing_ttl = item.ttl;
  sm->di_0 = item.di;
  sm->di_0.length = item.di.length;
  if (!info->enqueue_pc) {
    return false;
  }
  if (info->enqueue_pc >= stack_machine_code_len) {
    LOG(1,
        "chase: enqueue_pc out of "
        "bounds %ld >= %ld",
        info->enqueue_pc, stack_machine_code_len);
    return false;
  }
  if (sm->pc_stack_pointer >= ENQUEUE_STACK_DEPTH) {
    LOG(2, "enqueue: call stack limit reached");
    return false;
  }
  sm->pc_stack[sm->pc_stack_pointer] = sm->pc;
  sm->pc_stack_pointer++;
  sm->pc = info->enqueue_pc;
  return true;
}

// Returns false if the pointer has already been memoized.
static inline __attribute__((always_inline)) bool
sm_memoize_pointer(__maybe_unused global_ctx_t* ctx, type_t type,
                   target_ptr_t addr, uint32_t maybe_len) {
  // Check if address was already processed before.
  stack_machine_t* sm = ctx->stack_machine;
  if (maybe_len == ENQUEUE_LEN_SENTINEL) {
    // Statically sized object.
    return chased_pointers_trie_push(&sm->chased, addr, type);
  }
  // Dynamically sized object, we may try to capture same address and type
  // multiple times, but with different lengths.
  return chased_slices_push(&sm->chased_slices, addr, type, maybe_len);
}

static inline __attribute__((always_inline)) bool
sm_record_pointer(global_ctx_t* ctx, type_t type, target_ptr_t addr,
                  bool decrease_ttl,
                  uint32_t maybe_len) {
  stack_machine_t* sm = ctx->stack_machine;
  if (addr == 0) {
    return true;
  }
  if (decrease_ttl && sm->pointer_chasing_ttl == 0) {
    return true;
  }
  if (!sm_memoize_pointer(ctx, type, addr, maybe_len)) {
    return true;
  }
  pointers_queue_item_t* item;
  if (decrease_ttl) {
    item = pointers_queue_push_back(&ctx->stack_machine->pointers_queue);
  } else {
    item = pointers_queue_push_front(&ctx->stack_machine->pointers_queue);
  }
  if (item == NULL) {
    LOG(3, "sm_record_pointer: pointers queue push failed");
    return false;
  }
  *item = (pointers_queue_item_t){
      .di = {
          .type = type,
          .length = maybe_len,
          .address = addr,
      },
      .ttl = sm->pointer_chasing_ttl - (decrease_ttl ? 1 : 0),
  };
  return true;
}

// lookup_go_dict_type resolves a runtime type to its actual IR type ID
// (without the pointer-to-pointee dereferencing that lookup_go_interface does).
// Used by dict resolution where we need the type as declared, not the
// interface-adjusted pointee.
static type_t lookup_go_dict_type(uint32_t go_runtime_type) {
  if (go_runtime_type == 0) {
    return 0;
  }
  uint32_t idx = lookup_type_id_by_go_runtime_type(go_runtime_type);
  uint32_t* got = bpf_map_lookup_elem(&go_runtime_types, &idx);
  if (!got || *got != go_runtime_type) {
    return 0;
  }
  uint32_t* type_id = bpf_map_lookup_elem(&go_runtime_type_direct_ids, &idx);
  if (!type_id) {
    return 0;
  }
  return *type_id;
}

static inline __attribute__((always_inline)) bool
sm_record_go_interface_impl(global_ctx_t* global_ctx, uint64_t go_runtime_type,
                            target_ptr_t addr) {
  // Resolve implementation type.
  if (go_runtime_type == (uint64_t)(-1)) {
    // TODO: Maybe this should not short-circuit the rest of execution.
    // Note that this happens only when there's an issue reading the
    // runtime.firstmoduledata or if the type does not reside inside of
    // it.
    LOG(3, "chase: interface unknown go runtime type");
    return true;
  }
  type_t t = lookup_go_interface(go_runtime_type);
  if (t == 0) {
    LOG(4, "chase: interface type not found %llx", go_runtime_type);
    return true;
  }
  const bool decrease_ttl = true;
  return sm_record_pointer(global_ctx, t, addr, decrease_ttl, ENQUEUE_LEN_SENTINEL);
}

typedef struct typebounds {
  uint64_t types;
  uint64_t etypes;
} typebounds_t;

typedef struct moduledata {
  uint64_t addr;
  typebounds_t types;
} moduledata_t;

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, moduledata_t);
} moduledata_buf SEC(".maps");

// Translate a pointer to a type (i.e. a pointer pointing to type information
// inside moduledata) like that found inside an empty interface to an offset
// into moduledata. We commonly represent runtime type information as such an
// offset.
static inline __attribute__((always_inline)) uint64_t
go_runtime_type_from_ptr(target_ptr_t type_ptr) {
  const unsigned long zero = 0;
  moduledata_t* moduledata =
      (moduledata_t*)bpf_map_lookup_elem(&moduledata_buf, &zero);
  if (!moduledata) {
    LOG(1, "go_runtime_type_from_ptr: moduledata not found");
    return -1;
  }
  // Detect if the moduledata is up-to-date by checking if the address is
  // correct. If it is not, then we need to update the typebounds.
  if (moduledata->addr != VARIABLE_runtime_dot_firstmoduledata) {
    moduledata->addr = VARIABLE_runtime_dot_firstmoduledata;
    if (bpf_probe_read_user(&moduledata->types, sizeof(typebounds_t),
                            (void*)(VARIABLE_runtime_dot_firstmoduledata +
                                    OFFSET_runtime_dot_moduledata__types))) {
      LOG(1, "go_runtime_type_from_ptr: failed to read moduledata types %llx + %d",
          VARIABLE_runtime_dot_firstmoduledata,
          OFFSET_runtime_dot_moduledata__types);
      return -1;
    }
  }

  typebounds_t* typebounds = &moduledata->types;
  if (type_ptr >= typebounds->types && type_ptr < typebounds->etypes) {
    return type_ptr - typebounds->types;
  }
  LOG(1, "go_runtime_type_from_ptr: type_ptr %llx not in typebounds %llx-%llx",
      type_ptr, typebounds->types, typebounds->etypes);
  return -1;
}

// TODO: These could be extracted from the debug info, but for now we'll
// simplify and hardcode them. They have never changed.
#define OFFSET_runtime_dot_iface__tab 0x00
#define OFFSET_runtime_dot_iface__data 0x08
#define OFFSET_runtime_dot_itab___type 0x08
#define OFFSET_runtime_dot_eface___type 0x00
#define OFFSET_runtime_dot_eface__data 0x08

static inline __attribute__((always_inline)) bool
sm_resolve_go_empty_interface(global_ctx_t* ctx, resolved_go_interface_t* res) {
  scratch_buf_t* buf = ctx->buf;
  stack_machine_t* sm = ctx->stack_machine;
  buf_offset_t offset = sm->offset;
  res->addr = 0;
  res->go_runtime_type = 0;
  if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) * 2)) {
    return false;
  }
  if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) +
                                             OFFSET_runtime_dot_eface__data)) {
    return false;
  }
  target_ptr_t type_addr =
      *(target_ptr_t*)(&(*buf)[offset + OFFSET_runtime_dot_eface___type]);
  res->addr =
      *(target_ptr_t*)&((*buf)[offset + OFFSET_runtime_dot_eface__data]);
  if (type_addr == 0) {
    // Not an error, just literally a nil interface.
    return true;
  }
  // TODO: check the return value for error
  res->go_runtime_type = go_runtime_type_from_ptr(type_addr);
  return true;
}

// Resolves address and implementation type of a non-empty interface.
static inline __attribute__((always_inline)) bool
sm_resolve_go_interface(global_ctx_t* ctx, resolved_go_interface_t* res) {
  scratch_buf_t* buf = ctx->buf;
  stack_machine_t* sm = ctx->stack_machine;
  buf_offset_t offset = sm->offset;
  res->addr = 0;
  res->go_runtime_type = 0;
  if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) * 2)) {
    return false;
  }
  if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) +
                                             OFFSET_runtime_dot_iface__data)) {
    return false;
  }
  res->addr =
      *(target_ptr_t*)&((*buf)[offset + OFFSET_runtime_dot_iface__data]);
  target_ptr_t itab =
      *(target_ptr_t*)(&(*buf)[offset + OFFSET_runtime_dot_iface__tab]);
  if (itab == 0) {
    return true;
  }
  target_ptr_t type_addr;
  if (bpf_probe_read_user(&type_addr, sizeof(target_ptr_t),
                          (void*)(itab) +
                              (uint64_t)(OFFSET_runtime_dot_itab___type))) {
    LOG(3, "enqueue: failed interface type read %llx",
        (void*)(itab) + (uint64_t)(OFFSET_runtime_dot_itab___type));
    return true;
  }
  // TODO: check the return value for error
  res->go_runtime_type = go_runtime_type_from_ptr(type_addr);
  return true;
}

// Resolve the type of a Go any type.
//
// Note that it will only return false in the case of a bounds check failure.
static inline __attribute__((always_inline)) bool
sm_resolve_go_any_type(global_ctx_t* global_ctx, resolved_go_any_type_t* r) {
  r->i.addr = 0;
  r->i.go_runtime_type = 0;
  r->type = (type_t)0;
  r->has_info = false;
  if (!sm_resolve_go_empty_interface(global_ctx, &r->i)) {
    return false;
  }
  if (r->i.go_runtime_type == (uint64_t)(-1)) {
    return true;
  }
  r->type = lookup_go_interface(r->i.go_runtime_type);
  if (r->type == 0) {
    return true;
  }
  const type_info_t* info;
  if (!get_type_info(r->type, &info)) {
    LOG(3, "any type info not found %d", r->type);
    return true;
  }
  r->has_info = true;
  r->info = *info;
  return true;
}

// inline __attribute__((always_inline)) bool sm_record_go_context_value(
//     global_ctx_t* ctx, const go_context_value_type_t* spec,
//     const resolved_go_any_type_t* value, const type_t expected_type) {
//   scratch_buf_t* buf = ctx->buf;
//   stack_machine_t* sm = ctx->stack_machine;
//   buf_offset_t offset = sm->offset;

//   if (spec == NULL) {
//     LOG(1, "go context value spec is null");
//     return false;
//   }
//   if (spec->index < 0) {
//     // Not interesting type.
//     return true;
//   }
//   if ((sm->go_context_capture_bitmask & (1ULL << spec->index)) == 0) {
//     // Already captured value type.
//     return true;
//   }
//   sm->go_context_capture_bitmask &= ~(1ULL << spec->index);
//   buf_offset_t target_offset = sm->go_context_offset + spec->offset;
//   // Value is referenced similarily as empty interface (but using custom type).
//   if (target_offset < 0 ||
//       !scratch_buf_bounds_check(&target_offset, 2 * sizeof(uint64_t))) {
//     return false;
//   }
//   *(target_ptr_t*)(&(*buf)[target_offset + 0]) = value->i.addr;
//   if (target_offset < 0 ||
//       !scratch_buf_bounds_check(&target_offset, 2 * sizeof(uint64_t))) {
//     return false;
//   }
//   *(uint64_t*)&((*buf)[target_offset + 8]) = value->i.go_runtime_type;
//   if (expected_type != 0 && expected_type != value->type) {
//     // Type mismatch, just bail, recorded go runtime type will expose the issue
//     // upstream.
//     return true;
//   }
//   // Note the value type might be legitimately different from the spec type, if
//   // the latter has its own set of expressions to capture.
//   return sm_record_pointer(ctx, spec->type != 0 ? spec->type : value->type,
//                            value->i.addr, ENQUEUE_LEN_SENTINEL);
// }

// inline __attribute__((always_inline)) bool
// sm_resolve_go_context_value(global_ctx_t* ctx, const int32_t key_offset,
//                             const int32_t value_offset) {
//   scratch_buf_t* buf = ctx->buf;
//   stack_machine_t* sm = ctx->stack_machine;
//   buf_offset_t offset = sm->offset;

//   if (key_offset == -1 || value_offset == -1) {
//     return true;
//   }

//   sm->offset += value_offset;
//   resolved_go_any_type_t* val = &sm->resolved_1;
//   if (!sm_resolve_go_any_type(ctx, val)) {
//     return false;
//   }
//   sm->offset -= value_offset;

//   if (val->has_info &&
//       !sm_record_go_context_value(ctx, &val->info.go_context_value, val,
//                                   /*expected_type=*/TYPE_NONE)) {
//     return false;
//   }

//   sm->offset += key_offset;
//   resolved_go_any_type_t* key = &sm->resolved_0;
//   if (!sm_resolve_go_any_type(ctx, key)) {
//     return false;
//   }
//   sm->offset -= key_offset;

//   if (key->has_info && val->has_info &&
//       !sm_record_go_context_value(ctx, &key->info.go_context_key, val,
//                                   key->info.go_context_key_value_type)) {
//     return false;
//   }
//   return true;
// }

// copy_from_code_ctx_t is the context for copying bytes from the stack machine
// code array into the scratch buffer, using bpf_loop.
typedef struct copy_from_code_ctx {
  scratch_buf_t* buf;
  buf_offset_t dst;
  uint32_t src_pc;
  bool failed;
} copy_from_code_ctx_t;

static long copy_from_code_loop(unsigned long i, void* _ctx) {
  copy_from_code_ctx_t* ctx = (copy_from_code_ctx_t*)_ctx;
  // Look up the code map first.
  uint32_t zero = 0;
  uint8_t* code = bpf_map_lookup_elem(&stack_machine_code, &zero);
  if (!code) {
    ctx->failed = true;
    return 1;
  }
  // Bounds check src right before access (after map lookup, so verifier
  // state is fresh).
  uint64_t src = (uint64_t)(uint32_t)(ctx->src_pc + (uint32_t)i);
  barrier_var(src);
  if (src >= stack_machine_code_len) {
    ctx->failed = true;
    return 1;
  }
  uint8_t val = code[src];
  barrier_var(val);
  // Bounds check dst right before write.
  buf_offset_t dst = ctx->dst + i;
  if (!scratch_buf_bounds_check(&dst, 1)) {
    ctx->failed = true;
    return 1;
  }
  (*ctx->buf)[dst] = val;
  LOG(4, "copy_from_code_loop: i=%d, dst=%d, val=%d", i, dst, val);
  return 0;
}

__attribute__((noinline)) bool
sm_copy_from_code(scratch_buf_t* buf, buf_offset_t dst,
                  uint32_t src_pc, uint32_t byte_size) {
  if (!buf) {
    return false;
  }
  copy_from_code_ctx_t ctx = {
      .buf = buf,
      .dst = dst,
      .src_pc = src_pc,
      .failed = false,
  };
  bpf_loop(byte_size, copy_from_code_loop, &ctx, 0);
  return !ctx.failed;
}

// cmp_eq_bytes_ctx_t is the context for comparing two byte sequences in the
// scratch buffer, using bpf_loop.
typedef struct cmp_eq_bytes_ctx {
  scratch_buf_t* buf;
  buf_offset_t lhs;
  buf_offset_t rhs;
  bool equal;
} cmp_eq_bytes_ctx_t;

static long cmp_eq_bytes_loop(unsigned long i, void* _ctx) {
  cmp_eq_bytes_ctx_t* ctx = (cmp_eq_bytes_ctx_t*)_ctx;
  buf_offset_t lhs = ctx->lhs + i;
  buf_offset_t rhs = ctx->rhs + i;
  if (!scratch_buf_bounds_check(&lhs, 1)) {
    return 1;
  }
  if (!scratch_buf_bounds_check(&rhs, 1)) {
    return 1;
  }
  char lhs_val = (*ctx->buf)[lhs];
  char rhs_val = (*ctx->buf)[rhs];
  barrier_var(lhs_val);
  barrier_var(rhs_val);
  LOG(4, "cmp_eq_bytes_loop: i=%d, lhs=%d, rhs=%d, lhs_val=%d, rhs_val=%d", i, lhs, rhs, lhs_val, rhs_val);
  if (lhs_val != rhs_val) {
    ctx->equal = false;
    return 1;
  }
  return 0;
}

__attribute__((noinline)) bool
sm_cmp_eq_bytes(scratch_buf_t* buf, buf_offset_t lhs, buf_offset_t rhs,
                uint32_t len) {
  if (!buf) {
    return false;
  }
  cmp_eq_bytes_ctx_t ctx = {
      .buf = buf,
      .lhs = lhs,
      .rhs = rhs,
      .equal = true,
  };
  bpf_loop(len, cmp_eq_bytes_loop, &ctx, 0);
  return ctx.equal;
}

// ---------------------------------------------------------------------------
// Swiss map lookup: O(1) hash-based key lookup in Go swisstable maps.
// See pkg/dyninst/irgen/go_swiss_maps.md for the algorithm specification.
// ---------------------------------------------------------------------------

// Per-process hash secrets, memoized on first use. These never change after
// runtime.alginit() so we read them once from the traced process and cache
// them in globals for the lifetime of the BPF program.
// Swiss hash state packed into a uint64 to avoid a small variable at
// the end of BSS where the compiler's 8-byte load would exceed the
// map value size. Bit 0 = initialized, bit 1 = use_aes.
static uint64_t g_swiss_hash_flags = 0;
#define SWISS_HASH_FLAG_INITIALIZED 1
#define SWISS_HASH_FLAG_USE_AES     2
// aeskeysched: 128 bytes of AES round keys.
static uint8_t g_swiss_aeskeysched[128] = {};

// Initialize the hash secret globals by reading from the traced process.
// Returns true on success.
static __always_inline bool swiss_hash_ensure_initialized(void) {
  if (g_swiss_hash_flags & SWISS_HASH_FLAG_INITIALIZED) return true;

  if (VARIABLE_runtime_dot_useAeshash == 0) {
    LOG(2, "swiss_hash: hash secret addresses not available");
    return false;
  }

  uint8_t use_aes = 0;
  if (bpf_probe_read_user(&use_aes, 1,
                           (void*)VARIABLE_runtime_dot_useAeshash) != 0) {
    LOG(2, "swiss_hash: failed to read useAeshash");
    return false;
  }

  if (use_aes) {
    g_swiss_hash_flags |= SWISS_HASH_FLAG_USE_AES;
    if (bpf_probe_read_user(g_swiss_aeskeysched, 128,
                             (void*)VARIABLE_runtime_dot_aeskeysched) != 0) {
      LOG(2, "swiss_hash: failed to read aeskeysched");
      return false;
    }
  }
  // Note: wyhash fallback (use_aes == false) is not supported. The hash_finish
  // function will report OOB for map index expressions on such systems.

  g_swiss_hash_flags |= SWISS_HASH_FLAG_INITIALIZED;
  return true;
}

// ---------------------------------------------------------------------------
// Swiss map lookup — decomposed into sm_loop opcodes.
// See SM_OP_SWISS_MAP_SETUP through SM_OP_SWISS_MAP_CHECK_SLOT.
// ---------------------------------------------------------------------------

// swiss_map_slot_params_t is defined in context.h.

// Check a single slot for a key match. Global noinline — verified separately.
// Returns: 1 = found, 0 = no match, -1 = error.
__attribute__((noinline)) int
swiss_map_check_slot(scratch_buf_t* buf, swiss_map_slot_params_t* p) {
  if (!buf || !p) return -1;

  buf_offset_t kd_off = p->key_data_off;
  if (!scratch_buf_bounds_check(&kd_off, 516)) return -1;
  const uint8_t* lit_key = (const uint8_t*)&(*buf)[kd_off];

  if (p->is_string_key) {
    uint8_t str_header[16];
    if (bpf_probe_read_user(str_header, 16, (void*)p->key_addr) != 0) return 0;
    target_ptr_t str_ptr = *(target_ptr_t*)&str_header[0];
    uint64_t str_len = *(uint64_t*)&str_header[8];

    // Quick length check.
    uint32_t lit_len = *(uint32_t*)lit_key;
    if (str_len != lit_len) return 0;

    // Read and compare string bytes (up to 512 bytes).
    // When lit_len == 0 (empty string key), both lengths match and there
    // are no bytes to compare — skip straight to the value read below.
    if (lit_len > 0) {
      uint32_t cmp_len = lit_len;
      if (cmp_len > SWISS_MAP_MAX_STR_KEY_LEN) cmp_len = SWISS_MAP_MAX_STR_KEY_LEN;
      buf_offset_t tmp_off = kd_off + p->key_data_len;
      if (!scratch_buf_dereference(buf, tmp_off, cmp_len, str_ptr)) return 0;
      const uint8_t* lit_bytes = lit_key + 4;
      for (uint32_t i = 0; i < cmp_len && i < SWISS_MAP_MAX_STR_KEY_LEN; i++) {
        buf_offset_t off = tmp_off + i;
        if (!scratch_buf_bounds_check(&off, 1)) return 0;
        if ((*buf)[off] != lit_bytes[i]) return 0;
      }
    }
  } else {
    uint8_t slot_key[8] = {};
    uint8_t ksz = p->key_byte_size;
    if (ksz > 8) ksz = 8;
    if (bpf_probe_read_user(slot_key, ksz, (void*)p->key_addr) != 0) return 0;
    for (uint8_t i = 0; i < ksz && i < 8; i++) {
      if (slot_key[i] != lit_key[i]) return 0;
    }
  }

  // Key matched — read value.
  if (!scratch_buf_dereference(buf, p->result_offset,
                               p->val_byte_size, p->val_addr)) {
    return -1;
  }
  return 1;
}

// ---------------------------------------------------------------------------
// sm_swiss_map_setup: global noinline function for SM_OP_SWISS_MAP_SETUP.
// Reads bytecode params, copies key data, reads map header, computes hash
// (wyhash inline or AES state initialization), determines groups/length_mask,
// and stores all probe state in sm->swiss_map_state.
//
// Returns:
//   1 = success, probe state ready (AES rounds still needed if use_aes)
//   2 = success, hash done, probe state ready (wyhash path; skip AESENC+HASH_FINISH)
//   0 = key not found (nil map or null table — OOB written)
//  -1 = error
// ---------------------------------------------------------------------------
__attribute__((noinline)) int
sm_swiss_map_setup(scratch_buf_t* buf, stack_machine_t* sm) {
  if (!buf || !sm) return -1;

  // Read bytecode parameters directly into swiss_map_state to minimize
  // stack usage (BPF combined stack limit is 512 bytes).
  #define ST sm->swiss_map_state
  ST.is_string_key = sm_read_program_uint8(sm);
  ST.key_byte_size = sm_read_program_uint8(sm);
  ST.val_byte_size = sm_read_program_uint32(sm);
  uint8_t seed_offset = sm_read_program_uint8(sm);
  uint8_t dir_ptr_offset = sm_read_program_uint8(sm);
  uint8_t dir_len_offset = sm_read_program_uint8(sm);
  uint8_t global_shift_offset = sm_read_program_uint8(sm);
  ST.ctrl_offset = sm_read_program_uint8(sm);
  ST.slots_offset = sm_read_program_uint8(sm);
  ST.slot_size = sm_read_program_uint16(sm);
  ST.key_in_slot_offset = sm_read_program_uint8(sm);
  ST.val_in_slot_offset = sm_read_program_uint8(sm);
  uint8_t table_groups_field_offset = sm_read_program_uint8(sm);
  uint8_t groups_data_field_offset = sm_read_program_uint8(sm);
  uint8_t groups_len_mask_field_offset = sm_read_program_uint8(sm);
  ST.group_byte_size = sm_read_program_uint16(sm);
  ST.expr_status_idx = sm_read_program_uint32(sm);
  uint16_t key_data_len = sm_read_program_uint16(sm);

  // Max key data: 4 (length prefix) + 512 (string) = 516 for strings,
  // or 8 for base types.
  if (key_data_len > 516) key_data_len = 516;
  barrier_var(key_data_len);

  // Key data goes after the map header in the scratch buffer. The header
  // was written at sm->offset by EXPR_DEREFERENCE_PTR, which saved its
  // byte_len to sm->buf_offset_1.
  ST.key_data_off = sm->offset + sm->buf_offset_1;
  sm->pc += key_data_len;
  ST.key_data_len = key_data_len;
  ST.result_offset = sm->offset;
  ST.probe_index = 0;

  // Read map header fields from scratch buffer.
  buf_offset_t base = sm->offset;

  buf_offset_t seed_off = base + seed_offset;
  if (!scratch_buf_bounds_check(&seed_off, 8)) return -1;
  uint64_t seed = *(uint64_t*)&(*buf)[seed_off];

  buf_offset_t dir_ptr_off = base + dir_ptr_offset;
  if (!scratch_buf_bounds_check(&dir_ptr_off, 8)) return -1;
  target_ptr_t dir_ptr = *(target_ptr_t*)&(*buf)[dir_ptr_off];

  buf_offset_t dir_len_off = base + dir_len_offset;
  if (!scratch_buf_bounds_check(&dir_len_off, 8)) return -1;
  int64_t dir_len = *(int64_t*)&(*buf)[dir_len_off];

  // Nil map check.
  if (dir_ptr == 0) {
    LOG(4, "swiss_map_setup: nil map");
    if (ST.expr_status_idx != EXPR_STATUS_IDX_NONE) {
      expr_status_write(buf, sm->expr_results_offset, ST.expr_status_idx,
                        EXPR_STATUS_OOB);
    }
    scratch_buf_set_len(buf, sm->expr_results_end_offset);
    return 0;
  }

  // Ensure hash secrets are initialized.
  if (!swiss_hash_ensure_initialized()) {
    scratch_buf_set_len(buf, sm->expr_results_end_offset);
    return -1;
  }
  ST.use_aes = (g_swiss_hash_flags & SWISS_HASH_FLAG_USE_AES) ? 1 : 0;

  // Hash computation is deferred to HASH_FINISH (PHASE_INIT) so that
  // the caller can copy key data to scratch buf first (the key data
  // destination must not overlap the map header at sm->offset).
  // Store seed in hash_scratch.state[0:8] for HASH_FINISH to use.
  *(uint64_t*)&sm->hash_scratch.state[0] = seed;


  ST.groups_data_ptr = dir_ptr;
  ST.length_mask = (uint64_t)dir_len;
  ST.hash_phase = SWISS_HASH_PHASE_INIT;
  ST.aes_rounds_left = 0;
  #undef ST

  // Pack field offsets for HASH_FINISH's table lookup.
  sm->value_0 = ((uint64_t)global_shift_offset) |
                ((uint64_t)table_groups_field_offset << 8) |
                ((uint64_t)groups_data_field_offset << 16) |
                ((uint64_t)groups_len_mask_field_offset << 24);
  sm->buf_offset_0 = base;

  return 1; // proceed to AESENC (which will be a no-op) then HASH_FINISH
}

// ---------------------------------------------------------------------------
// sm_swiss_map_hash_finish: global noinline function for SM_OP_SWISS_MAP_HASH_FINISH.
// Handles AES hash phase transitions and final hash extraction.
//
// Returns:
//   2 = need more AESENC rounds (caller should sm->pc -= 2)
//   0 = hash done, probe state set
//  -1 = error
//  -2 = key not found (null table; OOB already written)
// ---------------------------------------------------------------------------
__attribute__((noinline)) int
sm_swiss_map_hash_finish(scratch_buf_t* buf, stack_machine_t* sm) {
  if (!buf || !sm) return -1;

  uint8_t* state = sm->hash_scratch.state;
  uint8_t phase = sm->swiss_map_state.hash_phase;

  if (phase == SWISS_HASH_PHASE_INIT) {
    // Key data has been copied to scratch. Now read it and init hash.
    uint64_t seed = *(uint64_t*)&state[0]; // stored by SETUP

    buf_offset_t kd_off = sm->swiss_map_state.key_data_off;
    if (!scratch_buf_bounds_check(&kd_off, 516)) return -1;
    const uint8_t* key_data = (const uint8_t*)&(*buf)[kd_off];

    const uint8_t* hash_key_data = key_data;
    uint32_t hash_key_len = (uint32_t)sm->swiss_map_state.key_byte_size;
    if (sm->swiss_map_state.is_string_key) {
      hash_key_len = *(uint32_t*)key_data;
      hash_key_data = key_data + 4;
    }
    if (hash_key_len > SWISS_MAP_MAX_STR_KEY_LEN)
      hash_key_len = SWISS_MAP_MAX_STR_KEY_LEN;
    barrier_var(hash_key_len);
    sm->swiss_map_state.hash_key_len_full = (uint16_t)hash_key_len;

    if (!sm->swiss_map_state.use_aes) {
      // The Go runtime falls back to wyhash on systems without AES hardware
      // support. We do not implement the wyhash path because we have no way
      // to test it — virtually all amd64 and arm64 production hardware has
      // AES. Report the expression as OOB so the caller sees a clean
      // "unavailable" rather than a wrong value.
      LOG(2, "swiss_map_hash: wyhash not supported, reporting OOB");
      if (sm->swiss_map_state.expr_status_idx != EXPR_STATUS_IDX_NONE) {
        expr_status_write(buf, sm->expr_results_offset,
                          sm->swiss_map_state.expr_status_idx, EXPR_STATUS_OOB);
      }
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      return -1;

    } else {
      // AES: set up initial state based on key type.
      uint8_t* unscrambled = sm->hash_scratch.unscrambled;
      if (!sm->swiss_map_state.is_string_key &&
          sm->swiss_map_state.key_byte_size == 4) {
        *(uint64_t*)&state[0] = seed;
        *(uint32_t*)&state[8] = *(uint32_t*)hash_key_data;
        *(uint32_t*)&state[12] = 0;
        sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_DIRECT_DONE;
        sm->swiss_map_state.aes_rounds_left = 3;
        sm->swiss_map_state.aes_self_keyed = 0;
        sm->swiss_map_state.aes_rk_offset = 0;
        // arm64: same round key repeated (no advance), final round no MC.
        sm->swiss_map_state.aes_rk_no_advance = is_arm64 ? 1 : 0;
        sm->swiss_map_state.aes_final_skip_mc = is_arm64 ? 1 : 0;
        return 2;
      } else if (!sm->swiss_map_state.is_string_key &&
                 sm->swiss_map_state.key_byte_size == 8) {
        *(uint64_t*)&state[0] = seed;
        *(uint64_t*)&state[8] = *(uint64_t*)hash_key_data;
        sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_DIRECT_DONE;
        sm->swiss_map_state.aes_rounds_left = 3;
        sm->swiss_map_state.aes_self_keyed = 0;
        sm->swiss_map_state.aes_rk_offset = 0;
        sm->swiss_map_state.aes_rk_no_advance = is_arm64 ? 1 : 0;
        sm->swiss_map_state.aes_final_skip_mc = is_arm64 ? 1 : 0;
        return 2;
      } else {
        // General memhash / strhash: seed scramble first.
        *(uint64_t*)&state[0] = seed;
        if (is_arm64) {
          // arm64: V30 = [seed | len_as_uint64]. The length is stored
          // as a single 64-bit value in the high lane (VMOV R2, V30.D[1]).
          *(uint64_t*)&state[8] = (uint64_t)hash_key_len;
        } else {
          // x86: PINSRW $4 + PSHUFHW $0 replicates the 16-bit length
          // across the high 64 bits.
          uint16_t len16 = (uint16_t)hash_key_len;
          *(uint16_t*)&state[8] = len16;
          *(uint16_t*)&state[10] = len16;
          *(uint16_t*)&state[12] = len16;
          *(uint16_t*)&state[14] = len16;
        }
        copy16(unscrambled, state);
        if (is_arm64) {
          // arm64: AESE(state, keysched[0:16]) + AESMC. The AESE function
          // XORs keysched into state before SubBytes. Don't pre-XOR here.
          sm->swiss_map_state.aes_self_keyed = 0;
          sm->swiss_map_state.aes_rk_no_advance = 0;
          sm->swiss_map_state.aes_final_skip_mc = 0;
        } else {
          // x86: pre-XOR with keysched, then self-keyed AESENC.
          xor16(state, g_swiss_aeskeysched);
          sm->swiss_map_state.aes_self_keyed = 1;
        }
        sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_SEED_SCRAMBLE_DONE;
        sm->swiss_map_state.aes_rounds_left = 1;
        sm->swiss_map_state.aes_rk_offset = 0;
        return 2;
      }
    }
    // For wyhash: state[0:8] has the hash. Fall through to finalize below.

  } else if (phase == SWISS_HASH_PHASE_SEED_SCRAMBLE_DONE) {
    // After 1-round seed scramble. Dispatch based on key length tier.
    uint32_t len = (uint32_t)sm->swiss_map_state.hash_key_len_full;
    if (len > SWISS_MAP_MAX_STR_KEY_LEN) len = SWISS_MAP_MAX_STR_KEY_LEN;
    barrier_var(len);

    if (len == 0) {
      sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_FINAL_EXTRA;
      if (is_arm64) {
        // arm64 len==0: return scrambled seed directly (no extra round).
        // AESENC will be a no-op (rounds_left=0), then HASH_FINISH re-enters
        // with FINAL_EXTRA phase which falls through to finalize.
        sm->swiss_map_state.aes_rounds_left = 0;
      } else {
        sm->swiss_map_state.aes_rounds_left = 1;
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      return 2;
    }

    // Get key data offset (buf-relative to avoid pointer arithmetic issues).
    buf_offset_t kd_off = sm->swiss_map_state.key_data_off;
    if (!scratch_buf_bounds_check(&kd_off, 516)) return -1;
    buf_offset_t hkd_off = kd_off + (sm->swiss_map_state.is_string_key ? 4 : 0);
    if (!scratch_buf_bounds_check(&hkd_off, 16)) return -1;
    const uint8_t* hkd = (const uint8_t*)&(*buf)[hkd_off];

    // Load first 16 bytes into lane0 (needed by all tiers).
    uint8_t* lane0 = sm->hash_scratch.lanes[0];
    if (len >= 16) {
      copy16(lane0, hkd);
    } else if (is_arm64) {
      // arm64 aes0to15: scattered loading via bit-tests on length.
      // Data is placed at specific vector positions matching the Go runtime's
      // VLD1 into V2.D[0], V2.S[2], V2.H[6], V2.B[14] pattern.
      zero16(lane0);
      uint32_t src = 0;
      uint32_t alen = len & 0xf;
      if (alen & 8) { // TBZ $3
        for (uint32_t i = 0; i < 8; i++) lane0[i] = hkd[src + i];
        src += 8;
      }
      if (alen & 4) { // TBZ $2 → V2.S[2] = bytes 8-11
        for (uint32_t i = 0; i < 4; i++) lane0[8 + i] = hkd[src + i];
        src += 4;
      }
      if (alen & 2) { // TBZ $1 → V2.H[6] = bytes 12-13
        lane0[12] = hkd[src];
        lane0[13] = hkd[src + 1];
        src += 2;
      }
      if (alen & 1) { // TBZ $0 → V2.B[14] = byte 14
        lane0[14] = hkd[src];
      }
    } else {
      zero16(lane0);
      for (uint32_t i = 0; i < len && i < 16; i++) lane0[i] = hkd[i];
    }

    if (len <= 16) {
      if (is_arm64) {
        // arm64: scrambled seed is the round key, data is the state.
        // All 3 rounds include AESMC (no skip for the 0-16 byte tier).
        copy16(sm->hash_scratch.unscrambled, state); // scrambled seed → rk
        copy16(state, lane0);                         // data → state
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 0;
      } else {
        // x86: XOR data with seed, then self-keyed AESENC.
        xor16(lane0, state);
        copy16(state, lane0);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_SINGLE_LANE_DONE;
      sm->swiss_map_state.aes_rounds_left = 3;
      return 2;
    }

    if (len <= 32) {
      // 2-lane: load overlapping 16-byte chunks.
      // Both arm64 and x86: lane0 = first 16 bytes, lane1 = last 16 bytes.
      // (arm64 VLD1.P post-indexes R0 so the first load reads from the start.)
      buf_offset_t lane1_off = kd_off + (sm->swiss_map_state.is_string_key ? 4 : 0) + len - 16;
      if (!scratch_buf_bounds_check(&lane1_off, 16)) return -1;
      copy16(sm->hash_scratch.lanes[1], (const uint8_t*)&(*buf)[lane1_off]);
      if (is_arm64) {
        // arm64: scrambled seed (state) is the round key for lane0 data.
        // Save original seed_vec (in unscrambled) to seeds[1] for seed1
        // derivation later in LANE0_DONE.
        copy16(sm->hash_scratch.seeds[1], sm->hash_scratch.unscrambled);
        copy16(sm->hash_scratch.unscrambled, state); // scrambled seed as rk
        copy16(state, lane0);                         // start data → state
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 1;
      } else {
        xor16(lane0, state);
        copy16(state, lane0);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_LANE0_DONE;
      sm->swiss_map_state.aes_rounds_left = 3;
      return 2;
    }

    // --- Multi-lane: 4 lanes (33-64) or 8 lanes (65+) ---
    uint8_t num_lanes = (len <= 64) ? 4 : 8;
    sm->swiss_map_state.num_lanes = num_lanes;
    sm->swiss_map_state.current_lane = 1;

    // Save scrambled seed as seeds[0].
    copy16(sm->hash_scratch.seeds[0], state);

    // Begin seed preparation for seed 1.
    if (is_arm64) {
      // arm64: AESE(keysched[16], original_seed_vec). keysched slot is state,
      // original seed_vec (V30, saved in unscrambled) is the round key.
      copy16(state, g_swiss_aeskeysched + 16);
      // unscrambled already holds the original seed_vec from above.
      sm->swiss_map_state.aes_self_keyed = 2;
      sm->swiss_map_state.aes_final_skip_mc = 0;
    } else {
      // x86: unscrambled ^ keysched[16], then self-keyed AESENC.
      copy16(state, sm->hash_scratch.unscrambled);
      xor16(state, g_swiss_aeskeysched + 16);
      sm->swiss_map_state.aes_self_keyed = 1;
    }
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_MULTI_SEED_PREP;
    sm->swiss_map_state.aes_rounds_left = 1;
    return 2;

  } else if (phase == SWISS_HASH_PHASE_MULTI_SEED_PREP) {
    // Just finished 1 self-keyed round on current seed. Save it.
    uint8_t lane = sm->swiss_map_state.current_lane;
    if (lane >= SWISS_MAP_MAX_AES_LANES) return -1;
    SEED_WRITE(sm->hash_scratch, lane, state);
    sm->swiss_map_state.current_lane = lane + 1;

    if (lane + 1 < sm->swiss_map_state.num_lanes) {
      // Prepare next seed.
      uint32_t ks_off = (uint32_t)(lane + 1) * 16;
      if (ks_off > 112) ks_off = 112;
      if (is_arm64) {
        // arm64: AESE(keysched[offset], original_seed_vec). keysched is state,
        // original seed_vec (in unscrambled, preserved from PHASE_INIT) is rk.
        copy16(state, g_swiss_aeskeysched + ks_off);
        // unscrambled already holds the original seed_vec — don't overwrite it.
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 0;
      } else {
        copy16(state, sm->hash_scratch.unscrambled);
        xor16(state, g_swiss_aeskeysched + ks_off);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.aes_rounds_left = 1;
      return 2;
    }

    // All seeds prepared. Load data into lanes and XOR with seeds.
    uint32_t len = (uint32_t)sm->swiss_map_state.hash_key_len_full;
    if (len > SWISS_MAP_MAX_STR_KEY_LEN) len = SWISS_MAP_MAX_STR_KEY_LEN;
    barrier_var(len);
    uint8_t nl = sm->swiss_map_state.num_lanes;

    buf_offset_t kd_off = sm->swiss_map_state.key_data_off;
    if (!scratch_buf_bounds_check(&kd_off, 516)) return -1;
    // Base offset of the actual key bytes (past the 4-byte length prefix for strings).
    buf_offset_t hkd_off = kd_off + (sm->swiss_map_state.is_string_key ? 4 : 0);

    if (len <= 128) {
      // Non-looping: first N/2 lanes from start, last N/2 from end (overlapping).
      // Both arm64 and x86 use the same lane ordering: VLD1.P post-indexes R0
      // on arm64, so the first load reads from the start, the second from the end.
      uint8_t half = nl / 2;
      // First half: lanes[i] = key_data[16*i]
      for (uint8_t i = 0; i < half && i < SWISS_MAP_MAX_AES_LANES; i++) {
        buf_offset_t off = hkd_off + 16 * (uint32_t)i;
        if (!scratch_buf_bounds_check(&off, 16)) return -1;
        LANE_WRITE(sm->hash_scratch, i, (const uint8_t*)&(*buf)[off]);
      }
      // Second half: overlapping from end
      for (uint8_t i = 0; i < half && (half + i) < SWISS_MAP_MAX_AES_LANES; i++) {
        buf_offset_t off = hkd_off + len - (uint32_t)(half - i) * 16;
        if (!scratch_buf_bounds_check(&off, 16)) return -1;
        LANE_WRITE(sm->hash_scratch, half + i, (const uint8_t*)&(*buf)[off]);
      }
    } else {
      // 129+ looping: initial data is the LAST 128 bytes.
      for (uint8_t i = 0; i < 8 && i < SWISS_MAP_MAX_AES_LANES; i++) {
        buf_offset_t off = hkd_off + len - 128 + 16 * (uint32_t)i;
        if (!scratch_buf_bounds_check(&off, 16)) return -1;
        LANE_WRITE(sm->hash_scratch, i, (const uint8_t*)&(*buf)[off]);
      }
      // Set up block loop: process from start in 128-byte chunks.
      sm->swiss_map_state.block_offset = 0;
      sm->swiss_map_state.blocks_remaining = (len - 1) / 128; // number of full 128-byte blocks
    }

    if (!is_arm64) {
      // x86: XOR each lane with its seed. arm64 skips this — AESE will
      // XOR the per-lane seed as the round key internally.
      for (uint8_t i = 0; i < nl && i < SWISS_MAP_MAX_AES_LANES; i++) {
        SEED_READ(sm->hash_scratch, i, sm->hash_scratch.tmp);
        LANE_XOR(sm->hash_scratch, i, sm->hash_scratch.tmp);
      }
    }

    if (len > 128) {
      // 129+ path: start block loop.
      sm->swiss_map_state.current_lane = 0;
      if (is_arm64) {
        // arm64: accumulators are seeds (V0-V7). Load seed[0] into state,
        // lane[0] (old tail data) is the round key.
        SEED_READ(sm->hash_scratch, 0, state);
        LANE_READ(sm->hash_scratch, 0, sm->hash_scratch.unscrambled);
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 0;
      } else {
        copy16(state, sm->hash_scratch.lanes[0]);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_BLOCK_SELF_ROUNDS;
      sm->swiss_map_state.aes_rounds_left = 1;
      return 2;
    }

    // Non-looping: go straight to 3 data rounds per lane.
    sm->swiss_map_state.current_lane = 0;
    if (is_arm64) {
      // arm64: data is in lanes[], seed is the round key.
      LANE_READ(sm->hash_scratch, 0, state);
      SEED_READ(sm->hash_scratch, 0, sm->hash_scratch.unscrambled);
      sm->swiss_map_state.aes_self_keyed = 2;
      sm->swiss_map_state.aes_final_skip_mc = 1;
    } else {
      copy16(state, sm->hash_scratch.lanes[0]);
      sm->swiss_map_state.aes_self_keyed = 1;
    }
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_MULTI_DATA_ROUNDS;
    sm->swiss_map_state.aes_rounds_left = 3;
    return 2;

  } else if (phase == SWISS_HASH_PHASE_MULTI_DATA_ROUNDS) {
    // Just finished 3 rounds on current lane. Save result.
    uint8_t lane = sm->swiss_map_state.current_lane;
    if (lane >= SWISS_MAP_MAX_AES_LANES) return -1;
    LANE_WRITE(sm->hash_scratch, lane, state);

    uint8_t next = lane + 1;
    if (next < sm->swiss_map_state.num_lanes) {
      // Load next lane and do 3 rounds.
      sm->swiss_map_state.current_lane = next;
      if (is_arm64) {
        LANE_READ(sm->hash_scratch, next, state);
        SEED_READ(sm->hash_scratch, next, sm->hash_scratch.unscrambled);
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 1;
      } else {
        LANE_READ(sm->hash_scratch, next, state);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.aes_rounds_left = 3;
      return 2;
    }

    // All lanes done. Fall through to MULTI_DONE.
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_MULTI_DONE;
    // Fall through (no return 2 — process MULTI_DONE immediately).

  }
  if (phase == SWISS_HASH_PHASE_MULTI_DONE ||
      sm->swiss_map_state.hash_phase == SWISS_HASH_PHASE_MULTI_DONE) {
    // XOR-fold all lanes down to lanes[0], matching Go runtime:
    // 8 lanes: [0]^=[4],[1]^=[5],[2]^=[6],[3]^=[7],[0]^=[2],[1]^=[3],[0]^=[1]
    // 4 lanes: [0]^=[2],[1]^=[3],[0]^=[1]
    uint8_t nl = sm->swiss_map_state.num_lanes;
    if (nl == 8) {
      for (uint8_t i = 0; i < 4; i++)
        { LANE_READ(sm->hash_scratch, i + 4, sm->hash_scratch.tmp); LANE_XOR(sm->hash_scratch, i, sm->hash_scratch.tmp); }
    }
    if (nl >= 4) {
      xor16(sm->hash_scratch.lanes[0], sm->hash_scratch.lanes[2]);
      xor16(sm->hash_scratch.lanes[1], sm->hash_scratch.lanes[3]);
    }
    xor16(sm->hash_scratch.lanes[0], sm->hash_scratch.lanes[1]);
    copy16(state, sm->hash_scratch.lanes[0]);
    // Fall through to finalize.

  } else if (phase == SWISS_HASH_PHASE_BLOCK_SELF_ROUNDS) {
    // 129+ path: just did 1 round on current lane. Save & advance.
    uint8_t lane = sm->swiss_map_state.current_lane;
    if (lane >= SWISS_MAP_MAX_AES_LANES) return -1;
    if (is_arm64) {
      // arm64: accumulator is seed, save result back to seeds.
      SEED_WRITE(sm->hash_scratch, lane, state);
    } else {
      LANE_WRITE(sm->hash_scratch, lane, state);
    }

    uint8_t next = lane + 1;
    if (next < 8) {
      sm->swiss_map_state.current_lane = next;
      if (is_arm64) {
        // arm64: AESE(seed[next], lane[next]). seed is state, lane data is rk.
        SEED_READ(sm->hash_scratch, next, state);
        LANE_READ(sm->hash_scratch, next, sm->hash_scratch.unscrambled);
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 0;
      } else {
        LANE_READ(sm->hash_scratch, next, state);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.aes_rounds_left = 1;
      return 2;
    }

    // All 8 rounds done. Start per-lane data-keyed round.
    // Load data chunk for lane 0 into unscrambled (custom round key).
    {
      buf_offset_t kd_off = sm->swiss_map_state.key_data_off;
      if (!scratch_buf_bounds_check(&kd_off, 516)) return -1;
      buf_offset_t hkd_off = kd_off + (sm->swiss_map_state.is_string_key ? 4 : 0);
      buf_offset_t data_off = hkd_off + (uint32_t)sm->swiss_map_state.block_offset;
      if (!scratch_buf_bounds_check(&data_off, 16)) return -1;
      copy16(sm->hash_scratch.unscrambled, (const uint8_t*)&(*buf)[data_off]);
    }
    sm->swiss_map_state.current_lane = 0;
    if (is_arm64) {
      // arm64: AESE(seed[0], new_data). seed is the accumulator.
      SEED_READ(sm->hash_scratch, 0, state);
    } else {
      LANE_READ(sm->hash_scratch, 0, state);
    }
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_BLOCK_DATA_ROUND;
    sm->swiss_map_state.aes_rounds_left = 1;
    sm->swiss_map_state.aes_self_keyed = 2;
    sm->swiss_map_state.aes_final_skip_mc = 0;
    return 2;

  } else if (phase == SWISS_HASH_PHASE_BLOCK_DATA_ROUND) {
    // Just finished 1 data-keyed round on current lane. Save & advance.
    uint8_t lane = sm->swiss_map_state.current_lane;
    if (is_arm64) {
      SEED_WRITE(sm->hash_scratch, lane, state);
    } else {
      LANE_WRITE(sm->hash_scratch, lane, state);
    }

    uint8_t next = lane + 1;
    if (next < 8) {
      // Load next lane and its data chunk as round key.
      sm->swiss_map_state.current_lane = next;
      if (is_arm64) {
        SEED_READ(sm->hash_scratch, next, state);
      } else {
        LANE_READ(sm->hash_scratch, next, state);
      }

      buf_offset_t kd_off2 = sm->swiss_map_state.key_data_off;
      if (!scratch_buf_bounds_check(&kd_off2, 516)) return -1;
      buf_offset_t hkd_off2 = kd_off2 + (sm->swiss_map_state.is_string_key ? 4 : 0);
      buf_offset_t data_off2 = hkd_off2 + (uint32_t)sm->swiss_map_state.block_offset +
                               16 * (uint32_t)next;
      if (!scratch_buf_bounds_check(&data_off2, 16)) return -1;
      copy16(sm->hash_scratch.unscrambled, (const uint8_t*)&(*buf)[data_off2]);

      sm->swiss_map_state.aes_rounds_left = 1;
      sm->swiss_map_state.aes_self_keyed = 2;
      return 2;
    }

    // All 8 data-keyed rounds done. Advance block.
    sm->swiss_map_state.block_offset += 128;
    sm->swiss_map_state.blocks_remaining--;

    if (is_arm64) {
      // arm64: save current block data to lanes[] so next iteration's
      // BLOCK_SELF_ROUNDS has the "old data" as round keys.
      buf_offset_t kd_off3 = sm->swiss_map_state.key_data_off;
      if (scratch_buf_bounds_check(&kd_off3, 516)) {
        buf_offset_t hkd_off3 = kd_off3 + (sm->swiss_map_state.is_string_key ? 4 : 0);
        buf_offset_t blk_off = hkd_off3 + (uint32_t)sm->swiss_map_state.block_offset - 128;
        for (uint8_t i = 0; i < 8 && i < SWISS_MAP_MAX_AES_LANES; i++) {
          buf_offset_t d_off = blk_off + 16 * (uint32_t)i;
          if (scratch_buf_bounds_check(&d_off, 16))
            LANE_WRITE(sm->hash_scratch, i, (const uint8_t*)&(*buf)[d_off]);
        }
      }
    }

    if (sm->swiss_map_state.blocks_remaining > 0) {
      // More blocks: go back to self/old-data-keyed rounds.
      sm->swiss_map_state.current_lane = 0;
      if (is_arm64) {
        SEED_READ(sm->hash_scratch, 0, state);
        LANE_READ(sm->hash_scratch, 0, sm->hash_scratch.unscrambled);
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 0;
      } else {
        copy16(state, sm->hash_scratch.lanes[0]);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_BLOCK_SELF_ROUNDS;
      sm->swiss_map_state.aes_rounds_left = 1;
      return 2;
    }

    // No more blocks. Do 3 final rounds per lane.
    sm->swiss_map_state.current_lane = 0;
    if (is_arm64) {
      // arm64: AESE(seed, last_data). last data is in lanes[] (saved above).
      SEED_READ(sm->hash_scratch, 0, state);
      LANE_READ(sm->hash_scratch, 0, sm->hash_scratch.unscrambled);
      sm->swiss_map_state.aes_self_keyed = 2;
      sm->swiss_map_state.aes_final_skip_mc = 1;
    } else {
      copy16(state, sm->hash_scratch.lanes[0]);
      sm->swiss_map_state.aes_self_keyed = 1;
    }
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_BLOCK_FINAL_ROUNDS;
    sm->swiss_map_state.aes_rounds_left = 3;
    return 2;

  } else if (phase == SWISS_HASH_PHASE_BLOCK_FINAL_ROUNDS) {
    // Just finished 3 final rounds on current lane. Save & advance.
    uint8_t lane = sm->swiss_map_state.current_lane;
    if (lane >= SWISS_MAP_MAX_AES_LANES) return -1;
    if (is_arm64) {
      SEED_WRITE(sm->hash_scratch, lane, state);
    } else {
      LANE_WRITE(sm->hash_scratch, lane, state);
    }

    uint8_t next = lane + 1;
    if (next < 8) {
      sm->swiss_map_state.current_lane = next;
      if (is_arm64) {
        SEED_READ(sm->hash_scratch, next, state);
        LANE_READ(sm->hash_scratch, next, sm->hash_scratch.unscrambled);
        sm->swiss_map_state.aes_self_keyed = 2;
        sm->swiss_map_state.aes_final_skip_mc = 1;
      } else {
        LANE_READ(sm->hash_scratch, next, state);
        sm->swiss_map_state.aes_self_keyed = 1;
      }
      sm->swiss_map_state.aes_rounds_left = 3;
      return 2;
    }
    if (is_arm64) {
      // arm64 129+: XOR-fold seeds[0..7] (accumulators are seeds, not lanes).
      for (uint8_t i = 0; i < 4; i++)
        { SEED_READ(sm->hash_scratch, i + 4, sm->hash_scratch.tmp);
          xor16(_seed_ptr(sm->hash_scratch.seeds, i), sm->hash_scratch.tmp); }
      xor16(sm->hash_scratch.seeds[0], sm->hash_scratch.seeds[2]);
      xor16(sm->hash_scratch.seeds[1], sm->hash_scratch.seeds[3]);
      xor16(sm->hash_scratch.seeds[0], sm->hash_scratch.seeds[1]);
      copy16(state, sm->hash_scratch.seeds[0]);
    } else {
      // x86: XOR-fold lanes[0..7].
      for (uint8_t i = 0; i < 4; i++)
        { LANE_READ(sm->hash_scratch, i + 4, sm->hash_scratch.tmp); LANE_XOR(sm->hash_scratch, i, sm->hash_scratch.tmp); }
      xor16(sm->hash_scratch.lanes[0], sm->hash_scratch.lanes[2]);
      xor16(sm->hash_scratch.lanes[1], sm->hash_scratch.lanes[3]);
      xor16(sm->hash_scratch.lanes[0], sm->hash_scratch.lanes[1]);
      copy16(state, sm->hash_scratch.lanes[0]);
    }
    // Fall through to finalize.

  } else if (phase == SWISS_HASH_PHASE_LANE0_DONE) {
    // Save lane0 AES result (in state) to lanes[0].
    copy16(sm->hash_scratch.lanes[0], state);

    // Reconstruct data1 = overlapping "last 16 bytes" window.
    buf_offset_t kd_off2 = sm->swiss_map_state.key_data_off;
    if (!scratch_buf_bounds_check(&kd_off2, 516)) return -1;
    const uint8_t* key_data2 = (const uint8_t*)&(*buf)[kd_off2];
    uint32_t len2;
    const uint8_t* hash_key_data2;
    if (sm->swiss_map_state.is_string_key) {
      len2 = *(uint32_t*)key_data2;
      hash_key_data2 = key_data2 + 4;
    } else {
      len2 = (uint32_t)sm->swiss_map_state.key_byte_size;
      hash_key_data2 = key_data2;
    }
    if (len2 > SWISS_MAP_MAX_STR_KEY_LEN) len2 = SWISS_MAP_MAX_STR_KEY_LEN;
    barrier_var(len2);

    // data1 = last 16 bytes of key data = lane1 (already loaded in
    // SEED_SCRAMBLE_DONE from hkd + len - 16). Just copy it.
    copy16(sm->hash_scratch.seeds[0], sm->hash_scratch.lanes[1]);

    // Prepare seed1: derive from keysched[16:32].
    uint8_t* seed1 = sm->hash_scratch.seeds[1];
    if (is_arm64) {
      // arm64: AESE(keysched[16:32], original_seed_vec). keysched is state,
      // original seed_vec (saved to seeds[1] in SEED_SCRAMBLE_DONE) is the rk.
      copy16(state, g_swiss_aeskeysched + 16);
      copy16(sm->hash_scratch.unscrambled, sm->hash_scratch.seeds[1]);
      sm->swiss_map_state.aes_self_keyed = 2;
      sm->swiss_map_state.aes_final_skip_mc = 0;
    } else {
      copy16(seed1, sm->hash_scratch.unscrambled);
      xor16(seed1, g_swiss_aeskeysched + 16);
      copy16(state, seed1);
      sm->swiss_map_state.aes_self_keyed = 1;
    }
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_SEED1_DONE;
    sm->swiss_map_state.aes_rounds_left = 1;
    return 2;

  } else if (phase == SWISS_HASH_PHASE_SEED1_DONE) {
    if (is_arm64) {
      // arm64: state has derived seed1. Use it as round key for lane1 data.
      // lane1 data is in seeds[0] (saved earlier from lanes[1]).
      copy16(sm->hash_scratch.unscrambled, state); // seed1 as custom rk
      copy16(state, sm->hash_scratch.seeds[0]);    // lane1 data as state
      sm->swiss_map_state.aes_self_keyed = 2;
      sm->swiss_map_state.aes_final_skip_mc = 1;
    } else {
      uint8_t* data1 = sm->hash_scratch.seeds[0];
      xor16(data1, state);
      copy16(state, data1);
      sm->swiss_map_state.aes_self_keyed = 1;
    }
    sm->swiss_map_state.hash_phase = SWISS_HASH_PHASE_LANE1_DONE;
    sm->swiss_map_state.aes_rounds_left = 3;
    return 2;

  } else if (phase == SWISS_HASH_PHASE_LANE1_DONE) {
    xor16(state, sm->hash_scratch.lanes[0]);
    // Fall through to finalize.

  } else if (phase == SWISS_HASH_PHASE_FINAL_EXTRA ||
             phase == SWISS_HASH_PHASE_SINGLE_LANE_DONE ||
             phase == SWISS_HASH_PHASE_DIRECT_DONE) {
    // state[0:8] is the hash. Fall through to finalize.

  } else {
    LOG(2, "swiss_map_hash_finish: unknown phase %d", phase);
    return -1;
  }

  // --- Finalize: extract hash, compute h1/h2, determine groups ---
  uint64_t hash = *(uint64_t*)&state[0];
  uint64_t h1 = hash >> 7;
  sm->swiss_map_state.h2 = (uint8_t)(hash & 0x7F);
  LOG(4, "swiss_map_hash_finish: hash=0x%llx h2=0x%x", hash, sm->swiss_map_state.h2);

  // Recover dir_ptr, dir_len, and field offsets stored by SETUP.
  target_ptr_t dir_ptr = sm->swiss_map_state.groups_data_ptr;
  int64_t dir_len = (int64_t)sm->swiss_map_state.length_mask;
  uint8_t global_shift_offset = (uint8_t)(sm->value_0);
  uint8_t table_groups_field_offset = (uint8_t)(sm->value_0 >> 8);
  uint8_t groups_data_field_offset = (uint8_t)(sm->value_0 >> 16);
  uint8_t groups_len_mask_field_offset = (uint8_t)(sm->value_0 >> 24);

  if (dir_len == 0) {
    sm->swiss_map_state.groups_data_ptr = dir_ptr;
    sm->swiss_map_state.length_mask = 0;
    sm->swiss_map_state.probe_offset = 0;
  } else {
    buf_offset_t gs_off = sm->buf_offset_0 + global_shift_offset;
    if (!scratch_buf_bounds_check(&gs_off, 1)) return -1;
    uint8_t global_shift = (*buf)[gs_off];
    uint64_t table_idx = hash >> global_shift;
    target_ptr_t table_ptr;
    if (bpf_probe_read_user(&table_ptr, 8,
                             (void*)(dir_ptr + table_idx * 8)) != 0) {
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      return -1;
    }
    if (table_ptr == 0) {
      if (sm->swiss_map_state.expr_status_idx != EXPR_STATUS_IDX_NONE) {
        expr_status_write(buf, sm->expr_results_offset,
                          sm->swiss_map_state.expr_status_idx, EXPR_STATUS_OOB);
      }
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      return -2;
    }
    target_ptr_t groups_ref_addr = table_ptr + table_groups_field_offset;
    if (bpf_probe_read_user(&sm->swiss_map_state.groups_data_ptr, 8,
                             (void*)(groups_ref_addr +
                                     groups_data_field_offset)) != 0) {
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      return -1;
    }
    if (bpf_probe_read_user(&sm->swiss_map_state.length_mask, 8,
                             (void*)(groups_ref_addr +
                                     groups_len_mask_field_offset)) != 0) {
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      return -1;
    }
    sm->swiss_map_state.probe_offset = h1 & sm->swiss_map_state.length_mask;
  }
  sm->swiss_map_state.probe_index = 0;
  LOG(4, "swiss_map_hash_finish: groups=0x%llx lm=%llu po=%llu",
      sm->swiss_map_state.groups_data_ptr,
      sm->swiss_map_state.length_mask,
      sm->swiss_map_state.probe_offset);
  return 0; // hash done, probe state set
}

// ---------------------------------------------------------------------------
// sm_swiss_map_aesenc: global noinline function for one AESENC round.
// Operates on sm->hash_scratch.state using sm->hash_scratch.tmp as scratch.
// Isolates the AESENC stack frame from sm_loop.
// ---------------------------------------------------------------------------
__attribute__((noinline)) int
sm_swiss_map_aesenc(stack_machine_t* sm) {
  if (!sm) return -1;
  uint8_t* state = sm->hash_scratch.state;
  uint8_t* tmp = sm->hash_scratch.tmp;

  // SubBytes + ShiftRows.
  tmp[0]  = aes_sbox[state[0]];
  tmp[1]  = aes_sbox[state[5]];
  tmp[2]  = aes_sbox[state[10]];
  tmp[3]  = aes_sbox[state[15]];
  tmp[4]  = aes_sbox[state[4]];
  tmp[5]  = aes_sbox[state[9]];
  tmp[6]  = aes_sbox[state[14]];
  tmp[7]  = aes_sbox[state[3]];
  tmp[8]  = aes_sbox[state[8]];
  tmp[9]  = aes_sbox[state[13]];
  tmp[10] = aes_sbox[state[2]];
  tmp[11] = aes_sbox[state[7]];
  tmp[12] = aes_sbox[state[12]];
  tmp[13] = aes_sbox[state[1]];
  tmp[14] = aes_sbox[state[6]];
  tmp[15] = aes_sbox[state[11]];

  // Determine round key.
  // aes_self_keyed: 0=keysched, 1=self-keyed (state), 2=custom (unscrambled)
  const uint8_t* rk;
  if (sm->swiss_map_state.aes_self_keyed == 1) {
    rk = state;
  } else if (sm->swiss_map_state.aes_self_keyed == 2) {
    // Custom round key stored in hash_scratch.unscrambled (used for data-keyed
    // AESENC in the 129+ block loop).
    rk = sm->hash_scratch.unscrambled;
  } else {
    uint32_t off = sm->swiss_map_state.aes_rk_offset & 0x7f;
    if (off > 112) off = 112;
    rk = &g_swiss_aeskeysched[off];
    sm->swiss_map_state.aes_rk_offset = off + 16;
  }

  // MixColumns + AddRoundKey.
  uint8_t a0, a1, a2, a3, x0, x1, x2, x3;

  a0 = tmp[0]; a1 = tmp[1]; a2 = tmp[2]; a3 = tmp[3];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[0] = x0 ^ x1 ^ a1 ^ a2 ^ a3 ^ rk[0];
  state[1] = a0 ^ x1 ^ x2 ^ a2 ^ a3 ^ rk[1];
  state[2] = a0 ^ a1 ^ x2 ^ x3 ^ a3 ^ rk[2];
  state[3] = x0 ^ a0 ^ a1 ^ a2 ^ x3 ^ rk[3];

  a0 = tmp[4]; a1 = tmp[5]; a2 = tmp[6]; a3 = tmp[7];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[4] = x0 ^ x1 ^ a1 ^ a2 ^ a3 ^ rk[4];
  state[5] = a0 ^ x1 ^ x2 ^ a2 ^ a3 ^ rk[5];
  state[6] = a0 ^ a1 ^ x2 ^ x3 ^ a3 ^ rk[6];
  state[7] = x0 ^ a0 ^ a1 ^ a2 ^ x3 ^ rk[7];

  a0 = tmp[8]; a1 = tmp[9]; a2 = tmp[10]; a3 = tmp[11];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[8]  = x0 ^ x1 ^ a1 ^ a2 ^ a3 ^ rk[8];
  state[9]  = a0 ^ x1 ^ x2 ^ a2 ^ a3 ^ rk[9];
  state[10] = a0 ^ a1 ^ x2 ^ x3 ^ a3 ^ rk[10];
  state[11] = x0 ^ a0 ^ a1 ^ a2 ^ x3 ^ rk[11];

  a0 = tmp[12]; a1 = tmp[13]; a2 = tmp[14]; a3 = tmp[15];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[12] = x0 ^ x1 ^ a1 ^ a2 ^ a3 ^ rk[12];
  state[13] = a0 ^ x1 ^ x2 ^ a2 ^ a3 ^ rk[13];
  state[14] = a0 ^ a1 ^ x2 ^ x3 ^ a3 ^ rk[14];
  state[15] = x0 ^ a0 ^ a1 ^ a2 ^ x3 ^ rk[15];
  return 0;
}

// ---------------------------------------------------------------------------
// sm_swiss_map_aese: global noinline function for one arm64 AESE+AESMC round.
// ARM64 AESE(state, rk) = ShiftRows(SubBytes(state XOR rk))
// ARM64 AESMC(state) = MixColumns(state)
// The key difference from x86 AESENC: round key is XORed BEFORE SubBytes,
// and MixColumns has no AddRoundKey step.
// When aes_skip_mc is set, MixColumns is omitted (arm64 final rounds).
// ---------------------------------------------------------------------------
__attribute__((noinline)) int
sm_swiss_map_aese(stack_machine_t* sm) {
  if (!sm) return -1;
  uint8_t* state = sm->hash_scratch.state;
  uint8_t* tmp = sm->hash_scratch.tmp;

  // Determine round key (same selection logic as sm_swiss_map_aesenc).
  const uint8_t* rk;
  if (sm->swiss_map_state.aes_self_keyed == 1) {
    rk = state;
  } else if (sm->swiss_map_state.aes_self_keyed == 2) {
    rk = sm->hash_scratch.unscrambled;
  } else {
    uint32_t off = sm->swiss_map_state.aes_rk_offset & 0x7f;
    if (off > 112) off = 112;
    rk = &g_swiss_aeskeysched[off];
    if (!sm->swiss_map_state.aes_rk_no_advance)
      sm->swiss_map_state.aes_rk_offset = off + 16;
  }

  // AESE: XOR round key into state, then SubBytes + ShiftRows.
  // XOR in-place to avoid stack allocation.
  state[0]  ^= rk[0];  state[1]  ^= rk[1];  state[2]  ^= rk[2];  state[3]  ^= rk[3];
  state[4]  ^= rk[4];  state[5]  ^= rk[5];  state[6]  ^= rk[6];  state[7]  ^= rk[7];
  state[8]  ^= rk[8];  state[9]  ^= rk[9];  state[10] ^= rk[10]; state[11] ^= rk[11];
  state[12] ^= rk[12]; state[13] ^= rk[13]; state[14] ^= rk[14]; state[15] ^= rk[15];

  // SubBytes + ShiftRows (same permutation as x86).
  tmp[0]  = aes_sbox[state[0]];
  tmp[1]  = aes_sbox[state[5]];
  tmp[2]  = aes_sbox[state[10]];
  tmp[3]  = aes_sbox[state[15]];
  tmp[4]  = aes_sbox[state[4]];
  tmp[5]  = aes_sbox[state[9]];
  tmp[6]  = aes_sbox[state[14]];
  tmp[7]  = aes_sbox[state[3]];
  tmp[8]  = aes_sbox[state[8]];
  tmp[9]  = aes_sbox[state[13]];
  tmp[10] = aes_sbox[state[2]];
  tmp[11] = aes_sbox[state[7]];
  tmp[12] = aes_sbox[state[12]];
  tmp[13] = aes_sbox[state[1]];
  tmp[14] = aes_sbox[state[6]];
  tmp[15] = aes_sbox[state[11]];

  if (sm->swiss_map_state.aes_skip_mc) {
    // AESE only (no AESMC) — arm64 final round.
    copy16(state, tmp);
    return 0;
  }

  // AESMC: MixColumns only (NO AddRoundKey — that was done before SubBytes).
  uint8_t a0, a1, a2, a3, x0, x1, x2, x3;

  a0 = tmp[0]; a1 = tmp[1]; a2 = tmp[2]; a3 = tmp[3];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[0] = x0 ^ x1 ^ a1 ^ a2 ^ a3;
  state[1] = a0 ^ x1 ^ x2 ^ a2 ^ a3;
  state[2] = a0 ^ a1 ^ x2 ^ x3 ^ a3;
  state[3] = x0 ^ a0 ^ a1 ^ a2 ^ x3;

  a0 = tmp[4]; a1 = tmp[5]; a2 = tmp[6]; a3 = tmp[7];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[4] = x0 ^ x1 ^ a1 ^ a2 ^ a3;
  state[5] = a0 ^ x1 ^ x2 ^ a2 ^ a3;
  state[6] = a0 ^ a1 ^ x2 ^ x3 ^ a3;
  state[7] = x0 ^ a0 ^ a1 ^ a2 ^ x3;

  a0 = tmp[8]; a1 = tmp[9]; a2 = tmp[10]; a3 = tmp[11];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[8]  = x0 ^ x1 ^ a1 ^ a2 ^ a3;
  state[9]  = a0 ^ x1 ^ x2 ^ a2 ^ a3;
  state[10] = a0 ^ a1 ^ x2 ^ x3 ^ a3;
  state[11] = x0 ^ a0 ^ a1 ^ a2 ^ x3;

  a0 = tmp[12]; a1 = tmp[13]; a2 = tmp[14]; a3 = tmp[15];
  x0 = xtime(a0); x1 = xtime(a1); x2 = xtime(a2); x3 = xtime(a3);
  state[12] = x0 ^ x1 ^ a1 ^ a2 ^ a3;
  state[13] = a0 ^ x1 ^ x2 ^ a2 ^ a3;
  state[14] = a0 ^ a1 ^ x2 ^ x3 ^ a3;
  state[15] = x0 ^ a0 ^ a1 ^ a2 ^ x3;
  return 0;
}

static long sm_loop(__maybe_unused unsigned long i, void* _ctx) {
  global_ctx_t* ctx = (global_ctx_t*)_ctx;
  scratch_buf_t* buf = ctx->buf;
  stack_machine_t* sm = ctx->stack_machine;
  if (sm == NULL) {
    return 1;
  }

  // Note that we add max length padding to the end of the ops buffer so
  // that the body of the below function doesn't need to worry about any
  // bounds checks.
  if (sm->pc >= stack_machine_code_len - stack_machine_code_max_op) {
    LOG(1, "enqueue: pc %d out of bounds", sm->pc);
    return 1;
  }
  const sm_opcode_t op = (sm_opcode_t)sm_read_program_uint8(sm);
  LOG(4, "%6llx %s %s", (uint64_t)(sm->pc - 1), padding(sm->pc_stack_pointer), op_code_name(op));
  if (sm->pc >= stack_machine_code_len - stack_machine_code_max_op + 1) {
    return 1;
  }
  barrier_var(sm->pc);
  switch (op) {
  case SM_OP_ILLEGAL: {
    LOG(1, "enqueue: illegal instruction");
    return 1;
  } break;

  case SM_OP_CALL: {
    uint32_t next_pc = sm_read_program_uint32(sm);
    if (sm->pc_stack_pointer >= ENQUEUE_STACK_DEPTH) {
      LOG(2, "enqueue: call stack limit reached");
      return 1;
    }
    sm->pc_stack[sm->pc_stack_pointer] = sm->pc;
    sm->pc_stack_pointer++;
    sm->pc = next_pc;
  } break;

  case SM_OP_RETURN: {
    if (!sm_return(sm)) {
      return 1;
    }
  } break;

  case SM_OP_INCREMENT_OUTPUT_OFFSET: {
    sm->offset += sm_read_program_uint32(sm);
  } break;

  case SM_OP_EXPR_PREPARE: {
    sm->expr_results_end_offset = scratch_buf_len(buf);
    sm->offset = sm->expr_results_end_offset;
    if (sm->expr_type == POINTER) {
      if (!scratch_buf_bounds_check(&sm->offset, 8)) {
        return 1;
      }
      *(uint64_t*)(&(*buf)[sm->offset]) = sm->root_addr;
    }
  } break;

  case SM_OP_EXPR_SAVE: {
    uint32_t result_offset = sm_read_program_uint32(sm);
    uint32_t byte_len = sm_read_program_uint32(sm);
    uint32_t expr_idx = sm_read_program_uint32(sm);

    // Save the result.
    copy_data(buf, sm->offset, sm->expr_results_offset + result_offset,
              byte_len);

    LOG(4, "copy data 0x%llx->0x%llx !%u", sm->offset, sm->expr_results_offset + result_offset, byte_len);

    // Write expression status = present.
    expr_status_write(buf, sm->expr_results_offset, expr_idx, EXPR_STATUS_PRESENT);

    // Set the offset at the result data, for potential following process type functions.
    sm->offset = sm->expr_results_offset + result_offset;
    // Truncate scratch buffer, removing temporary processing data past the frame.
    // We do it here, as result may be used for following enqueue function that
    // may want to store data items that we need to preserve.
    scratch_buf_set_len(buf, sm->expr_results_end_offset);
  } break;

  case SM_OP_EXPR_DEREFERENCE_CFA: {
    int32_t cfa_offset = sm_read_program_uint32(sm);
    uint32_t data_len = sm_read_program_uint32(sm);
    uint32_t output_offset = sm_read_program_uint32(sm);
    target_ptr_t addr = (target_ptr_t)((int64_t)(sm->frame_data.cfa) + cfa_offset);
    if (!scratch_buf_dereference(buf, sm->offset + output_offset, data_len, addr)) {
      return 1;
    }
  } break;

  case SM_OP_EXPR_READ_REGISTER: {
    uint8_t regnum = sm_read_program_uint8(sm);
    uint8_t byte_size = sm_read_program_uint8(sm);
    buf_offset_t output_offset = sm->offset + sm_read_program_uint32(sm);
    struct pt_regs* regs = ctx->regs;
    if (!regs) {
      LOG(2, "enqueue: missing regs");
      // Zero the data and move along. In the future when we track availability
      // of data, we'll want to mark here that we don't have the data.
      // By writing a zero, we ensure that we don't end up chasing any
      // garbage pointers in any subsequent enqueue logic (because we don't
      // chase zero values).
    } else {
      switch (regnum) {
      // We need to switch over the regnum, as DWARF_REGISTER macro for amd64 requires
      // paramter to be a literal number.
      case 0:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(0);
        break;
      case 1:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(1);
        break;
      case 2:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(2);
        break;
      case 3:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(3);
        break;
      case 4:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(4);
        break;
      case 5:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(5);
        break;
      case 6:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(6);
        break;
      case 7:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(7);
        break;
      case 8:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(8);
        break;
      case 9:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(9);
        break;
      case 10:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(10);
        break;
      case 11:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(11);
        break;
      case 12:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(12);
        break;
      case 13:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(13);
        break;
      case 14:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(14);
        break;
      case 15:
        *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(15);
        break;
      default:
        LOG(2, "unknown register: %d", regnum);
        return 1;
      }
    }
    switch (byte_size) {
    case 1:
      if (!scratch_buf_bounds_check(&output_offset, 1)) {
        return 1;
      }
      *(uint8_t*)(&(*buf)[output_offset]) = sm->value_0;
      break;
    case 2:
      if (!scratch_buf_bounds_check(&output_offset, 2)) {
        return 1;
      }
      *(uint16_t*)(&(*buf)[output_offset]) = sm->value_0;
      break;
    case 4:
      if (!scratch_buf_bounds_check(&output_offset, 4)) {
        return 1;
      }
      *(uint32_t*)(&(*buf)[output_offset]) = sm->value_0;
      break;
    case 8:
      if (!scratch_buf_bounds_check(&output_offset, 8)) {
        return 1;
      }
      *(uint64_t*)(&(*buf)[output_offset]) = sm->value_0;
      LOG(4, "read %llx", sm->value_0);
      break;
    default:
      LOG(1, "unexpected copy register byte size %d", (int)byte_size);
      return 1;
    }
    LOG(5, "recorded scratch@0x%llx < [register expr]", sm->offset);
  } break;

  case SM_OP_EXPR_DEREFERENCE_PTR: {
    LOG(4, "EXPR_DEREFERENCE_PTR: starting");
    uint32_t bias = sm_read_program_uint32(sm);
    uint32_t byte_len = sm_read_program_uint32(sm);
    uint32_t expr_status_idx = sm_read_program_uint32(sm);
    buf_offset_t value_offset = sm->offset;
    if (!scratch_buf_bounds_check(&value_offset, sizeof(target_ptr_t))) {
      return 1;
    }
    target_ptr_t addr = *(target_ptr_t*)&((*buf)[value_offset]);
    if (addr == 0) {
      // NULL pointer: write nil-deref status.
      if (expr_status_idx != EXPR_STATUS_IDX_NONE) {
        expr_status_write(buf, sm->expr_results_offset, expr_status_idx, EXPR_STATUS_NIL_DEREF);
      }
      sm->condition_nil_deref = true;
      // Abort expression evaluation.
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) {
        return 1;
      }
      return 0;
    }
    addr += bias;
    if (!scratch_buf_dereference(buf, sm->offset, byte_len, addr)) {
      // Dereference failed: abort expression evaluation.
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) {
        return 1;
      }
      return 0;
    }
    // Save the dereference byte_len so subsequent ops (like SWISS_MAP_SETUP)
    // know how much data was written at sm->offset.
    sm->buf_offset_1 = byte_len;
  } break;

  case SM_OP_EXPR_SLICE_BOUNDS_CHECK: {
    LOG(4, "EXPR_SLICE_BOUNDS_CHECK: starting");
    uint32_t index = sm_read_program_uint32(sm);
    uint32_t expr_status_idx = sm_read_program_uint32(sm);

    // The slice header is [data_ptr (8 bytes), len (8 bytes)] at sm->offset.
    // The len field is at a fixed offset of 8 (we only support 64-bit targets).
    buf_offset_t len_off = sm->offset + 8;
    if (!scratch_buf_bounds_check(&len_off, 8)) {
      return 1;
    }
    int64_t slice_len = *(int64_t*)&((*buf)[len_off]);

    if ((int64_t)index >= slice_len || slice_len < 0) {
      LOG(3, "EXPR_SLICE_BOUNDS_CHECK: index %u >= len %lld, aborting", index, slice_len);
      if (expr_status_idx != EXPR_STATUS_IDX_NONE) {
        expr_status_write(buf, sm->expr_results_offset, expr_status_idx, EXPR_STATUS_OOB);
      }
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) {
        return 1;
      }
      return 0;
    }
    // Bounds check passed — data pointer is at sm->offset.
  } break;

  case SM_OP_PROCESS_POINTER: {
    type_t elem_type = (type_t)sm_read_program_uint32(sm);
    if (elem_type == 0) {
      LOG(1, "enqueue: unknown pointer type %d", elem_type)
      return 1;
    }
    if (!scratch_buf_bounds_check(&sm->offset, sizeof(target_ptr_t))) {
      return 1;
    }
    target_ptr_t addr = *(target_ptr_t*)&((*buf)[sm->offset]);

    if (!sm_record_pointer(ctx, elem_type, addr, /*decrease_ttl=*/true, ENQUEUE_LEN_SENTINEL)) {
      LOG(3, "enqueue: failed pointer chase");
    }
  } break;

  case SM_OP_PROCESS_SLICE: {
    type_t slice_data_type = (type_t)sm_read_program_uint32(sm);
    uint32_t elem_byte_len = sm_read_program_uint32(sm);
    if (!scratch_buf_bounds_check(&sm->offset, 16)) {
      return 1;
    }
    // Note that this hard-codes the offsets of the array and len fields of a
    // slice header.
    target_ptr_t addr = *(target_ptr_t*)&((*buf)[sm->offset]);
    int64_t len = *(int64_t*)(&(*buf)[sm->offset + 8]);
    if (len > 0) {
      if (!sm_record_pointer(ctx, slice_data_type, addr, /*decrease_ttl=*/false, len * elem_byte_len)) {
        LOG(3, "enqueue: failed slice chase");
      }
    }
    LOG(4, "enqueue: slice len %d", len)
  } break;

  case SM_OP_PROCESS_ARRAY_DATA_PREP: {
    uint32_t array_len = sm_read_program_uint32(sm);
    // We need to iterate over the slice data, push the length on the data stack to control the loop.
    sm_data_stack_push(sm, sm->offset + array_len);
    LOG(4, "array data prep: %d (offset: %d)", array_len, sm->offset);
  } break;

  case SM_OP_PROCESS_SLICE_DATA_PREP: {
    if (sm->di_0.length == 0) {
      // Nothing to do for an empty slice.
      sm_return(sm);
      break;
    }

    // We need to iterate over the slice data, push the length on the data stack to control the loop.
    sm_data_stack_push(sm, sm->offset + sm->di_0.length);
  } break;

  case SM_OP_PROCESS_SLICE_DATA_REPEAT: {
    uint32_t buffer_advancement = sm_read_program_uint32(sm);
    sm->offset += buffer_advancement;
    LOG(4, "offset after increment: %d", sm->offset);
    uint32_t sp = *(volatile uint32_t*)&sm->data_stack_pointer;
    uint32_t stack_idx = sp - 1;
    if (stack_idx >= ENQUEUE_STACK_DEPTH) {
      if (stack_idx + 1 == 0) {
        LOG(2, "unexpected empty data stack during slice iteration");
      } else {
        LOG(2, "unexpected full data stack during slice iteration");
      }
      return 1;
    }
    if (sm->offset >= sm->data_stack[stack_idx]) {
      // End of the slice.
      sm_data_stack_pop(sm);
      break;
    }
    // Jump back to a call instruction that directly preceedes this one.
    sm->pc -= 5 + 5;
  } break;

  case SM_OP_PROCESS_STRING: {
    type_t string_data_type = (type_t)sm_read_program_uint32(sm);
    LOG(4, "processing string @0x%llx", sm->offset);
    if (!scratch_buf_bounds_check(&sm->offset, 16)) {
      return false;
    }
    // Note that this hard-codes the offsets of the pointer and len fields of a
    // slice header.
    target_ptr_t addr = *(target_ptr_t*)&((*buf)[sm->offset]);
    int64_t len = *(int64_t*)(&(*buf)[sm->offset + 8]);
    if (len > 0) {
      if (!sm_record_pointer(ctx, string_data_type, addr, /*decrease_ttl=*/false, len)) {
        LOG(3, "enqueue: failed string chase");
      }
    }
    LOG(4, "enqueue: string len @%llx !%lld (offset: %d)", addr, len, sm->offset);
  } break;

    // case SM_OP_PREPARE_POINTEE_DATA: {
    //   LOG(4, "prepare pointee data %u %u %llx", sm->di_0.type,
    //       sm->di_0.length, sm->di_0.address);
    //   sm->buf_offset_0 = scratch_buf_reserve(buf, &sm->di_0);
    //   if (!sm->buf_offset_0) {
    //     LOG(1, "enqueue: failed to serialize pointee data root");
    //     return 1;
    //   }
    //   sm->expr_type = POINTER;
    //   sm->expr_results_offset = sm->buf_offset_0;
    //   sm->root_addr = sm->di_0.address;
    //   sm->offset = sm->buf_offset_0;
    //   zero_data(buf, sm->offset, sm->di_0.length);
    // } break;

  case SM_OP_CHASE_POINTERS: {
    pointers_queue_item_t* item = pointers_queue_pop_front(&sm->pointers_queue);
    if (item != NULL) {
      // Loop as long as there are more pointers to chase.
      LOG(4, "chasing pointer @%llx", item->di.address);
      sm->pc--;
      sm_chase_pointer(ctx, *item);
    }
  } break;

  case SM_OP_PREPARE_EVENT_ROOT: {
    type_t typ = (type_t)sm_read_program_uint32(sm);
    uint32_t data_len = sm_read_program_uint32(sm);
    // This is needed to prevent the reordering of the bounds check underneath
    // scratch_buf_reserve and the above read_uint32 calls. On
    // older verifiers, spilling can hide the fact that there was bounds
    // checking from the verifier.
    barrier_var(typ);
    barrier_var(data_len);

    sm->di_0.type = typ;
    sm->di_0.length = data_len;
    sm->di_0.address = 0;
    sm->buf_offset_0 = scratch_buf_reserve(buf, &sm->di_0);
    if (!sm->buf_offset_0) {
      LOG(1, "enqueue: failed to serialize event data root");
      return 1;
    }
    sm->expr_results_offset = sm->buf_offset_0;
    sm->expr_type = FRAME;
    sm->offset = sm->buf_offset_0;
    zero_data(buf, sm->offset, data_len);
  } break;

  case SM_OP_PROCESS_GO_EMPTY_INTERFACE: {
    resolved_go_interface_t r;
    if (!sm_resolve_go_empty_interface(ctx, &r)) {
      return 1;
    }
    LOG(4, "resolved_go_interface_t: %llx %llx", r.addr, r.go_runtime_type)
    // Overwrite the type_addr with the go_runtime_type.
    if (!scratch_buf_bounds_check(&sm->offset, sizeof(target_ptr_t))) {
      return 1;
    }
    *(uint64_t*)(&(*buf)[sm->offset + OFFSET_runtime_dot_eface___type]) =
        r.go_runtime_type;
    if (!sm_record_go_interface_impl(ctx, r.go_runtime_type, r.addr)) {
      LOG(3, "enqueue: failed empty interface chase");
    }
  } break;

  case SM_OP_PROCESS_GO_INTERFACE: {
    resolved_go_interface_t r;
    if (!sm_resolve_go_interface(ctx, &r)) {
      return 1;
    }
    if (r.go_runtime_type == 0) {
      break;
    }
    // Overwrite the type_addr with the go_runtime_type.
    if (!scratch_buf_bounds_check(&sm->offset, sizeof(target_ptr_t))) {
      return 1;
    }
    *(uint64_t*)(&(*buf)[sm->offset + OFFSET_runtime_dot_iface__tab]) =
        r.go_runtime_type;
    if (!sm_record_go_interface_impl(ctx, r.go_runtime_type, r.addr)) {
      LOG(3, "enqueue: failed interface chase");
    }
  } break;

  case SM_OP_PROCESS_GO_DICT_TYPE: {
    // Resolve a generic shape type parameter to its concrete type by
    // reading the runtime dictionary. For entry probes, the dict pointer
    // is read from a CPU register (captured via PT_REGS). For return
    // probes, bit 7 of dict_register is set and the dict pointer is
    // read from sm->saved_dict_ptr (restored from call context).
    uint32_t dict_index = sm_read_program_uint32(sm);
    uint8_t dict_register = sm_read_program_uint8(sm);
    uint32_t output_offset = sm_read_program_uint32(sm);

    // Compute absolute output position: event root start + offset.
    // Use barrier_var to help the verifier track bounds.
    buf_offset_t write_pos = sm->buf_offset_0;
    barrier_var(write_pos);
    write_pos += output_offset;
    barrier_var(write_pos);

    // Read the dict pointer: from saved state (return) or register (entry).
    uint64_t dict_ptr = 0;
    if (dict_register & 0x80) {
      // Return probe: dict pointer was saved at entry time and restored
      // from call context into sm->saved_dict_ptr by event.c.
      dict_ptr = sm->saved_dict_ptr;
    } else {
      // Entry probe: read from the register value saved in PT_REGS.
      struct pt_regs* regs = ctx->regs;
      if (regs) {
        switch (dict_register) {
        case 0: dict_ptr = regs->DWARF_REGISTER(0); break;
        case 1: dict_ptr = regs->DWARF_REGISTER(1); break;
        case 2: dict_ptr = regs->DWARF_REGISTER(2); break;
        case 3: dict_ptr = regs->DWARF_REGISTER(3); break;
        case 4: dict_ptr = regs->DWARF_REGISTER(4); break;
        case 5: dict_ptr = regs->DWARF_REGISTER(5); break;
        case 6: dict_ptr = regs->DWARF_REGISTER(6); break;
        case 7: dict_ptr = regs->DWARF_REGISTER(7); break;
        case 8: dict_ptr = regs->DWARF_REGISTER(8); break;
        case 9: dict_ptr = regs->DWARF_REGISTER(9); break;
        case 10: dict_ptr = regs->DWARF_REGISTER(10); break;
        case 11: dict_ptr = regs->DWARF_REGISTER(11); break;
        case 12: dict_ptr = regs->DWARF_REGISTER(12); break;
        case 13: dict_ptr = regs->DWARF_REGISTER(13); break;
        case 14: dict_ptr = regs->DWARF_REGISTER(14); break;
        case 15: dict_ptr = regs->DWARF_REGISTER(15); break;
        default: break;
        }
      }
    }
    // Always stash for entry path: event.c reads this after the stack
    // machine runs and stores it in the call context for return probes.
    sm->saved_dict_ptr = dict_ptr;
    LOG(4, "dict: reg=%d idx=%d ptr=%llx", dict_register, dict_index, dict_ptr);
    if (dict_ptr == 0) {
      LOG(3, "dict: null dict pointer from register %d", dict_register);
      if (scratch_buf_bounds_check(&write_pos, sizeof(uint64_t))) {
        *(uint64_t*)(&(*buf)[write_pos]) = 0;
      }
      break;
    }

    // Read dict[dict_index] from user memory.
    uint64_t type_ptr = 0;
    if (bpf_probe_read_user(&type_ptr, sizeof(uint64_t),
                            (void*)(dict_ptr + (uint64_t)dict_index * sizeof(uint64_t)))) {
      LOG(3, "dict: failed to read dict[%d] at %llx", dict_index, dict_ptr);
      if (scratch_buf_bounds_check(&write_pos, sizeof(uint64_t))) {
        *(uint64_t*)(&(*buf)[write_pos]) = 0;
      }
      break;
    }

    // Convert to runtime type offset.
    uint64_t runtime_type = 0;
    if (type_ptr != 0) {
      runtime_type = go_runtime_type_from_ptr(type_ptr);
    }
    LOG(4, "dict: type_ptr=%llx runtime_type=%llx", type_ptr, runtime_type);

    // Write the resolved runtime type offset at the designated position
    // in the event root data.
    if (scratch_buf_bounds_check(&write_pos, sizeof(uint64_t))) {
      *(uint64_t*)(&(*buf)[write_pos]) = runtime_type;
    }
  } break;

  case SM_OP_CALL_DICT_RESOLVED: {
    // Dynamically dispatch to the concrete type's ProcessType function
    // based on the dict-resolved runtime type. Falls back to the shape
    // type's ProcessType if resolution fails.
    uint32_t output_offset = sm_read_program_uint32(sm);
    uint32_t fallback_pc = sm_read_program_uint32(sm);

    // Read the resolved runtime type from the event root data.
    // Use expr_results_offset as the base, not buf_offset_0, because
    // ExprSave formerly clobbered buf_offset_0 for status tracking.
    // expr_results_offset is set by PrepareEventRoot and not modified.
    buf_offset_t read_pos = sm->expr_results_offset;
    barrier_var(read_pos);
    read_pos += output_offset;
    barrier_var(read_pos);

    uint32_t target_pc = fallback_pc;
    if (scratch_buf_bounds_check(&read_pos, sizeof(uint64_t))) {
      uint64_t runtime_type = *(uint64_t*)(&(*buf)[read_pos]);
      if (runtime_type != 0 && runtime_type != (uint64_t)(-1)) {
        type_t concrete_type = lookup_go_dict_type(runtime_type);
        if (concrete_type != 0) {
          const type_info_t* info;
          if (get_type_info(concrete_type, &info) && info->enqueue_pc != 0) {
            target_pc = info->enqueue_pc;
          }
        }
      }
    }

    // Call: push return address and jump.
    if (sm->pc_stack_pointer >= ENQUEUE_STACK_DEPTH) {
      LOG(2, "dict_call: call stack limit reached");
      return 1;
    }
    sm->pc_stack[sm->pc_stack_pointer] = sm->pc;
    sm->pc_stack_pointer++;
    sm->pc = target_pc;
  } break;

  case SM_OP_PROCESS_GO_HMAP: {
    // https://github.com/golang/go/blob/8d04110c/src/runtime/map.go#L105
    const uint8_t same_size_grow = 8;

    type_t buckets_array_type = (type_t)sm_read_program_uint32(sm);
    uint32_t bucket_byte_len = sm_read_program_uint32(sm);

    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
      return 1;
    }
    uint8_t flags = *(uint8_t*)&((*buf)[sm->buf_offset_0]);

    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
      return 1;
    }
    uint8_t b = *(uint8_t*)&((*buf)[sm->buf_offset_0]);

    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
      return 1;
    }
    target_ptr_t buckets_addr = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);

    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
      return 1;
    }
    target_ptr_t oldbuckets_addr = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);

    if (buckets_addr != 0) {
      uint32_t num_buckets = 1 << b;
      uint32_t buckets_size = num_buckets * bucket_byte_len;
      if (!sm_record_pointer(ctx, buckets_array_type, buckets_addr, false,
                             buckets_size)) {
        LOG(3, "enqueue: failed map chase (new buckets)");
      }
    }

    if (oldbuckets_addr != 0) {
      uint32_t num_buckets = 1 << b;
      if ((flags & same_size_grow) == 0) {
        num_buckets >>= 1;
      }
      uint32_t buckets_size = num_buckets * bucket_byte_len;
      if (!sm_record_pointer(ctx, buckets_array_type, oldbuckets_addr, false,
                             buckets_size)) {
        LOG(3, "enqueue: failed map chase (old buckets)");
      }
    }
  } break;

  case SM_OP_PROCESS_GO_SWISS_MAP: {
    type_t table_ptr_slice_type = (type_t)sm_read_program_uint32(sm);
    type_t group_type = (type_t)sm_read_program_uint32(sm);
    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
    sm->buf_offset_1 = sm->offset + sm_read_program_uint8(sm);
    LOG(4, "offset: %d", sm->buf_offset_1 - sm->offset);

    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
      return 1;
    }
    target_ptr_t dir_ptr = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);
    if (!scratch_buf_bounds_check(&sm->buf_offset_1, sizeof(int64_t))) {
      return 1;
    }
    int64_t dir_len = *(int64_t*)&((*buf)[sm->buf_offset_1]);
    LOG(4, "type: %d, dir_ptr: 0x%llx, dir_len: %lld", group_type, dir_ptr, dir_len)

    if (dir_len > 0) {
      if (!sm_record_pointer(ctx, table_ptr_slice_type, dir_ptr, /*decrease_ttl=*/false, 8 * dir_len)) {
        LOG(3, "enqueue: failed swiss map record (full)");
      }
    } else {
      if (!sm_record_pointer(ctx, group_type, dir_ptr, /*decrease_ttl=*/false, ENQUEUE_LEN_SENTINEL)) {
        LOG(3, "enqueue: failed swiss map record (inline)");
      }
    }
  } break;

  case SM_OP_PROCESS_GO_SWISS_MAP_GROUPS: {
    type_t group_slice_type = (type_t)sm_read_program_uint32(sm);
    uint32_t group_byte_len = sm_read_program_uint32(sm);

    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);

    LOG(5, "Offset diff: %d", sm->buf_offset_0 - sm->offset);
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
      return 1;
    }
    target_ptr_t data = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);

    sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(int64_t))) {
      return 1;
    }
    uint64_t length_mask = *(uint64_t*)&((*buf)[sm->buf_offset_0]);
    LOG(4, "group_slice_type: %d, data: 0x%llx, length_mask: %llu", group_slice_type, data, length_mask);
    if (!sm_record_pointer(ctx, group_slice_type, data, /*decrease_ttl=*/false,
                           group_byte_len * (length_mask + 1))) {
      LOG(3, "enqueue: failed swiss map groups record");
    }
  } break;

    // case SM_OP_ENQUEUE_GO_SUBROUTINE: {
    //   if (!scratch_buf_bounds_check(&sm->offset, sizeof(target_ptr_t))) {
    //     return 1;
    //   }
    //   uint32_t orig_buf_len = scratch_buf_len(buf);
    //   // First serialize as "unknown subroutine" that just captures the entry pc.
    //   target_ptr_t addr = *(target_ptr_t*)&((*buf)[sm->offset]);
    //   if (addr == 0) {
    //     break;
    //   }
    //   sm->di_0.type = unresolved_go_subroutine_type;
    //   sm->di_0.length = 8;
    //   sm->di_0.address = addr;
    //   sm->buf_offset_0 = scratch_buf_serialize(buf, &sm->di_0, 8);
    //   if (!sm->buf_offset_0) {
    //     LOG(3, "enqueue: failed to serialize subroutine");
    //     break;
    //   }
    //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(uint64_t))) {
    //     return 1;
    //   }
    //   uint64_t entry_pc = *(uint64_t*)&((*buf)[sm->buf_offset_0]);
    //   type_t type = lookup_go_subroutine(entry_pc);
    //   if (type != TYPE_NONE) {
    //     // We know the actual subroutine type. Drop the previously serialized
    //     // message.
    //     scratch_buf_set_len(buf, orig_buf_len);
    //     if (!sm_record_pointer(ctx, type, addr, ENQUEUE_LEN_SENTINEL)) {
    //       LOG(3, "enqueue: failed subroutine record");
    //     }
    //   }
    // } break;

    // case SM_OP_PREPARE_GO_CONTEXT: {
    //   uint32_t data_len = sm_read_program_uint32(sm);
    //   type_t typ = (type_t)sm_read_program_uint32(sm);
    //   uint8_t capture_count = sm_read_program_uint8(sm);
    //   sm->buf_offset_0 = sm->offset;
    //   if (!scratch_buf_bounds_check(&sm->buf_offset_0,
    //                                 sizeof(target_ptr_t) +
    //                                     OFFSET_runtime_dot_iface__data)) {
    //     return false;
    //   }
    //   // Synthetic type expects the address of the synthetic go context structure
    //   // to be the same as the address for the original context interface
    //   // implementation.
    //   sm->di_0.type = typ;
    //   sm->di_0.length = data_len;
    //   sm->di_0.address = *(target_ptr_t*)&(
    //       (*buf)[sm->buf_offset_0 + OFFSET_runtime_dot_iface__data]);
    //   if (sm->di_0.address == 0 ||
    //       !sm_memoize_pointer(ctx, (type_t)sm->di_0.type, sm->di_0.address)) {
    //     // Already processed, bail out.
    //     sm->pc += 2;
    //     break;
    //   }
    //   // Prepare for traversal.
    //   sm->go_context_offset =
    //       scratch_buf_serialize(ctx->buf, &sm->di_0, data_len);
    //   if (!sm->go_context_offset) {
    //     LOG(3, "enqueue: failed to serialize go context interface");
    //     // Bail out.
    //     sm->pc += 2;
    //     break;
    //   }
    //   sm->go_context_capture_bitmask = (1ULL << capture_count) - 1;
    //   zero_data(buf, sm->go_context_offset, data_len);
    //   if (!sm_data_stack_push(sm, scratch_buf_len(buf))) {
    //     return 1;
    //   }
    // } break;

    // case SM_OP_TRAVERSE_GO_CONTEXT: {
    //   if (sm->go_context_capture_bitmask == 0) {
    //     // All valute types have been captured.
    //     break;
    //   }
    //   resolved_go_interface_t r;
    //   if (!sm_resolve_go_interface(ctx, &r)) {
    //     return 1;
    //   }
    //   if (r.go_runtime_type == 0) {
    //     LOG(1, "go context runtime type is nil");
    //     break;
    //   }
    //   if (r.go_runtime_type == (uint64_t)-1) {
    //     LOG(1, "go context runtime type is unknown");
    //     break;
    //   }
    //   type_t context_type = lookup_go_interface(r.go_runtime_type);
    //   if (context_type == TYPE_NONE) {
    //     LOG(1, "go context runtime type not found %lld", r.go_runtime_type);
    //     break;
    //   }
    //   const type_info_t* context_info;
    //   if (!get_type_info(context_type, &context_info)) {
    //     LOG(1, "go context implementation type info not found %d", context_type);
    //     break;
    //   }
    //   if (context_info->byte_len == 0) {
    //     break;
    //   }
    //   sm->di_0.type = context_type;
    //   sm->di_0.length = ENQUEUE_LEN_SENTINEL;
    //   sm->di_0.address = r.addr;
    //   sm->offset =
    //       scratch_buf_serialize(ctx->buf, &sm->di_0, context_info->byte_len);
    //   if (!sm->offset) {
    //     LOG(3, "enqueue: failed to serialize go context impl");
    //     break;
    //   }
    //   if (!sm_resolve_go_context_value(
    //           ctx, context_info->go_context_impl.key_offset,
    //           context_info->go_context_impl.value_offset)) {
    //     return 1;
    //   }
    //   if (context_info->go_context_impl.context_offset == -1) {
    //     // We reached the bottom context.
    //     break;
    //   }
    //   sm->offset += context_info->go_context_impl.context_offset;
    //   // Loop.
    //   sm->pc--;
    // } break;

    // case SM_OP_CONCLUDE_GO_CONTEXT: {
    //   uint32_t stack_top = sm->data_stack_pointer - 1;
    //   if (stack_top >= ENQUEUE_STACK_DEPTH) {
    //     LOG(2, "enqueue: stack out of bounds %d", stack_top);
    //     return 1;
    //   }
    //   scratch_buf_set_len(buf, sm->data_stack[stack_top]);
    //   if (!sm_data_stack_pop(sm)) {
    //     return 1;
    //   }
    //   sm->go_context_offset = 0;
    //   sm->go_context_capture_bitmask = 0;
    // } break;

  case SM_OP_EXPR_PUSH_OFFSET: {
    uint32_t byte_size = sm_read_program_uint32(sm);
    if (!sm_data_stack_push(sm, sm->offset)) {
      return 1;
    }
    sm->offset += byte_size;
  } break;

  case SM_OP_EXPR_LOAD_LITERAL: {
    uint16_t byte_size = sm_read_program_uint16(sm);
    if (byte_size > 255 || byte_size == 0) {
      LOG(1, "enqueue: load_literal: invalid byte_size %d", byte_size);
      return 1;
    }
    if (!sm_copy_from_code(buf, sm->offset, sm->pc, (uint32_t)byte_size)) {
      return 1;
    }
    sm->pc += byte_size;
  } break;

  case SM_OP_EXPR_READ_STRING: {
    uint16_t max_len = sm_read_program_uint16(sm);
    if (max_len > 255) {
      max_len = 255;
    }
    // Read Go string header at sm->offset: ptr (8 bytes) + len (8 bytes).
    if (!scratch_buf_bounds_check(&sm->offset, 16)) {
      return 1;
    }
    uint64_t str_ptr = *(uint64_t*)(&(*buf)[sm->offset]);
    uint64_t str_len = *(uint64_t*)(&(*buf)[sm->offset + 8]);

    // Push current offset onto data stack (bookmark).
    if (!sm_data_stack_push(sm, sm->offset)) {
      return 1;
    }

    // Cap the length.
    uint32_t capped_len = str_len;
    if (capped_len > max_len) {
      capped_len = max_len;
    }

    // Overwrite in-place: [u32 len][bytes...]
    // Use constant 259 (4 + max 255) so the verifier sees a compile-time bound.
    if (!scratch_buf_bounds_check(&sm->offset, 259)) {
      return 1;
    }
    *(uint32_t*)(&(*buf)[sm->offset]) = capped_len;

    // Read string data from userspace.
    if (capped_len > 0 && str_ptr != 0) {
      buf_offset_t data_offset = sm->offset + 4;
      bpf_probe_read_user(&(*buf)[data_offset], capped_len & 0xFF, (void*)str_ptr);
    }

    // Advance offset past materialized data.
    sm->offset += 4 + capped_len;
  } break;

  case SM_OP_EXPR_CMP_EQ_BASE: {
    uint8_t byte_size = sm_read_program_uint8(sm);
    if (byte_size > 8 || byte_size == 0) {
      LOG(1, "enqueue: cmp_eq_base: invalid byte_size %d", byte_size);
      return 1;
    }
    // Pop LHS offset from data stack.
    if (sm->data_stack_pointer == 0) {
      LOG(1, "enqueue: cmp_eq_base: empty data stack");
      return 1;
    }
    sm->data_stack_pointer--;
    if (sm->data_stack_pointer >= ENQUEUE_STACK_DEPTH) {
      return 1;
    }
    uint32_t lhs_offset = sm->data_stack[sm->data_stack_pointer];
    sm->data_stack[sm->data_stack_pointer] = 0;

    buf_offset_t lhs_off = lhs_offset;
    buf_offset_t rhs_off = sm->offset;
    if (!scratch_buf_bounds_check(&lhs_off, 8) ||
        !scratch_buf_bounds_check(&rhs_off, 8)) {
      return 1;
    }

    bool eq = sm_cmp_eq_bytes(buf, lhs_off, rhs_off, (uint32_t)byte_size);

    // Write bool result at sm->offset.
    if (!scratch_buf_bounds_check(&sm->offset, 1)) {
      return 1;
    }
    (*buf)[sm->offset] = eq ? 1 : 0;
  } break;

  case SM_OP_EXPR_CMP_EQ_STRING: {
    // Pop LHS offset from data stack.
    if (sm->data_stack_pointer == 0) {
      LOG(1, "enqueue: cmp_eq_string: empty data stack");
      return 1;
    }
    sm->data_stack_pointer--;
    if (sm->data_stack_pointer >= ENQUEUE_STACK_DEPTH) {
      return 1;
    }
    uint32_t lhs_offset = sm->data_stack[sm->data_stack_pointer];
    sm->data_stack[sm->data_stack_pointer] = 0;

    buf_offset_t lhs_off = lhs_offset;
    buf_offset_t rhs_off = sm->offset;
    // Both have format: [u32 len][bytes...]
    if (!scratch_buf_bounds_check(&lhs_off, 4) ||
        !scratch_buf_bounds_check(&rhs_off, 4)) {
      return 1;
    }
    uint32_t lhs_len = *(uint32_t*)(&(*buf)[lhs_off]);
    uint32_t rhs_len = *(uint32_t*)(&(*buf)[rhs_off]);

    uint8_t result = 0;
    if (lhs_len == rhs_len) {
      uint32_t cmp_len = lhs_len;
      if (cmp_len > 256) {
        cmp_len = 256;
      }
      result = sm_cmp_eq_bytes(buf, lhs_off + 4, rhs_off + 4, cmp_len) ? 1 : 0;
    }

    // Write result at sm->offset.
    if (!scratch_buf_bounds_check(&sm->offset, 1)) {
      return 1;
    }
    (*buf)[sm->offset] = result;
  } break;

  case SM_OP_CONDITION_BEGIN: {
    sm->condition_eval_error = true;
  } break;

  case SM_OP_CONDITION_CHECK: {
    sm->condition_eval_error = false;
    if (!scratch_buf_bounds_check(&sm->offset, 1)) {
      return 1;
    }
    uint8_t val = (*buf)[sm->offset];
    if (val == 0) {
      sm->condition_failed = true;
      LOG(1, "condition check failed");
      return 1; // Abort stack machine.
    }
  } break;

  // ---------------------------------------------------------------------------
  // Swiss map lookup opcodes. The compiler emits these 5 in sequence:
  //   SETUP [params] → AESENC → HASH_FINISH → PROBE → CHECK_SLOT
  // AESENC/HASH_FINISH may loop via PC replay for multiple AES rounds.
  // PROBE/CHECK_SLOT may loop via PC replay for quadratic probing.
  // ---------------------------------------------------------------------------

  case SM_OP_SWISS_MAP_SETUP: {
    LOG(4, "SWISS_MAP_SETUP: starting");
    int rc = sm_swiss_map_setup(buf, sm);
    if (rc <= 0) {
      // nil map (0) or error (-1). OOB already written.
      if (!sm_return(sm)) return 1;
      break;
    }
    // Copy key data from bytecode to scratch buffer.
    // SETUP set key_data_off past the map header and advanced PC past key data.
    // We do the copy here (sm_loop frame) to avoid stack depth from
    // sm_copy_from_code inside the global function.
    // Copy key data. Use swiss_map_state fields as temps to avoid
    // clobbering buf_offset_0 (used by HASH_FINISH for the header base).
    {
      uint32_t kdl = sm->swiss_map_state.key_data_len;
      if (kdl > 516) kdl = 516;
      barrier_var(kdl);
      if (!sm_copy_from_code(buf, sm->swiss_map_state.key_data_off,
                             sm->pc - kdl, kdl)) {
        if (!sm_return(sm)) return 1;
        break;
      }
    }
    // AESENC follows (no-op on first pass since rounds_left=0),
    // then HASH_FINISH handles PHASE_INIT.
  } break;

  case SM_OP_SWISS_MAP_AESENC: {
    if (sm->swiss_map_state.aes_rounds_left == 0) {
      // No-op: PHASE_INIT hasn't set up rounds yet. Fall through to HASH_FINISH.
      break;
    }
    if (is_arm64) {
      // On arm64, the final round of each batch skips MixColumns (AESE only).
      sm->swiss_map_state.aes_skip_mc =
          (sm->swiss_map_state.aes_final_skip_mc &&
           sm->swiss_map_state.aes_rounds_left == 1) ? 1 : 0;
      sm_swiss_map_aese(sm);
    } else {
      sm_swiss_map_aesenc(sm);
    }
    sm->swiss_map_state.aes_rounds_left--;
    if (sm->swiss_map_state.aes_rounds_left > 0) {
      sm->pc -= 1; // replay AESENC
    }
  } break;

  case SM_OP_SWISS_MAP_HASH_FINISH: {
    int hf_rc = sm_swiss_map_hash_finish(buf, sm);
    if (hf_rc == 2) {
      sm->pc -= 2; // back to AESENC for more rounds
    } else if (hf_rc == 0) {
      // Hash done, probe state set. Fall through to PROBE.
    } else if (hf_rc == -2) {
      // Key not found (null table). OOB already written.
      if (!sm_return(sm)) return 1;
    } else {
      // Error.
      if (!sm_return(sm)) return 1;
    }
  } break;

  case SM_OP_SWISS_MAP_PROBE: {
    // Read control word at current probe group.
    // Store group_addr and ctrl_word in struct fields to avoid stack locals.
    sm->swiss_map_state.group_addr =
        sm->swiss_map_state.groups_data_ptr +
        sm->swiss_map_state.probe_offset * sm->swiss_map_state.group_byte_size;

    if (bpf_probe_read_user(&sm->value_0, 8,
                             (void*)(sm->swiss_map_state.group_addr +
                                     sm->swiss_map_state.ctrl_offset)) != 0) {
      LOG(3, "swiss_map_probe: failed to read ctrl");
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) return 1;
      break;
    }

    // Compute H2 match bitset and empty bitset.
    // sm->value_0 holds the ctrl_word.
    // Compute H2 match/empty bitsets. value_0 holds ctrl_word; reuse it for v.
    sm->swiss_map_state.empty_matches =
        (sm->value_0 & ~(sm->value_0 << 6)) & 0x8080808080808080ULL;
    sm->value_0 ^= (0x0101010101010101ULL * (uint64_t)sm->swiss_map_state.h2);
    sm->swiss_map_state.h2_matches =
        ((sm->value_0 - 0x0101010101010101ULL) & ~sm->value_0) & 0x8080808080808080ULL;

    if (sm->swiss_map_state.h2_matches) {
      // H2 matches found — fall through to CHECK_SLOT.
      break;
    }
    if (sm->swiss_map_state.empty_matches) {
      // No H2 match and empty slot → key not in map.
      LOG(4, "swiss_map_probe: key not found (empty slot)");
      if (sm->swiss_map_state.expr_status_idx != EXPR_STATUS_IDX_NONE) {
        expr_status_write(buf, sm->expr_results_offset,
                          sm->swiss_map_state.expr_status_idx, EXPR_STATUS_OOB);
      }
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) return 1;
      break;
    }
    // No H2 match, no empty slot — advance probe.
    sm->swiss_map_state.probe_index++;
    sm->swiss_map_state.probe_offset =
        (sm->swiss_map_state.probe_offset +
         sm->swiss_map_state.probe_index) &
        sm->swiss_map_state.length_mask;
    sm->pc -= 1; // replay PROBE
  } break;

  case SM_OP_SWISS_MAP_CHECK_SLOT: {
    // Find first H2-matching slot. Use value_0 as temp for m.
    sm->value_0 = sm->swiss_map_state.h2_matches;
    {
      int slot = 0;
      uint64_t m = sm->value_0;
      if (!(m & 0x80))               { m >>= 8; slot = 1; }
      if (slot == 1 && !(m & 0x80))  { m >>= 8; slot = 2; }
      if (slot == 2 && !(m & 0x80))  { m >>= 8; slot = 3; }
      if (slot == 3 && !(m & 0x80))  { m >>= 8; slot = 4; }
      if (slot == 4 && !(m & 0x80))  { m >>= 8; slot = 5; }
      if (slot == 5 && !(m & 0x80))  { m >>= 8; slot = 6; }
      if (slot == 6 && !(m & 0x80))  { slot = 7; }

      sm->swiss_map_state.h2_matches &= ~((uint64_t)0xFF << (slot * 8));

      sm->value_0 = sm->swiss_map_state.group_addr +
          sm->swiss_map_state.slots_offset +
          (uint64_t)slot * sm->swiss_map_state.slot_size;
    }
    sm->swiss_map_state.slot_params.key_addr = sm->value_0 + sm->swiss_map_state.key_in_slot_offset;
    sm->swiss_map_state.slot_params.val_addr = sm->value_0 + sm->swiss_map_state.val_in_slot_offset;
    sm->swiss_map_state.slot_params.key_data_off = sm->swiss_map_state.key_data_off;
    sm->swiss_map_state.slot_params.result_offset = sm->swiss_map_state.result_offset;
    sm->swiss_map_state.slot_params.val_byte_size = sm->swiss_map_state.val_byte_size;
    sm->swiss_map_state.slot_params.key_data_len = sm->swiss_map_state.key_data_len;
    sm->swiss_map_state.slot_params.key_byte_size = sm->swiss_map_state.key_byte_size;
    sm->swiss_map_state.slot_params.is_string_key = sm->swiss_map_state.is_string_key;
    int result = swiss_map_check_slot(buf, &sm->swiss_map_state.slot_params);
    if (result == 1) {
      LOG(4, "swiss_map_check_slot: key found");
      break; // Value written at result_offset. Done.
    }
    if (result < 0) {
      LOG(3, "swiss_map_check_slot: error");
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) return 1;
      break;
    }
    // No match for this slot.
    if (sm->swiss_map_state.h2_matches) {
      sm->pc -= 1; // replay CHECK_SLOT for next match
    } else if (sm->swiss_map_state.empty_matches) {
      // No more H2 matches + empty slot → key not in map.
      LOG(4, "swiss_map_check_slot: key not found (exhausted h2 matches)");
      if (sm->swiss_map_state.expr_status_idx != EXPR_STATUS_IDX_NONE) {
        expr_status_write(buf, sm->expr_results_offset,
                          sm->swiss_map_state.expr_status_idx, EXPR_STATUS_OOB);
      }
      scratch_buf_set_len(buf, sm->expr_results_end_offset);
      if (!sm_return(sm)) return 1;
    } else {
      // Advance probe and go back to PROBE opcode.
      sm->swiss_map_state.probe_index++;
      sm->swiss_map_state.probe_offset =
          (sm->swiss_map_state.probe_offset +
           sm->swiss_map_state.probe_index) &
          sm->swiss_map_state.length_mask;
      sm->pc -= 2; // back to PROBE (1 byte CHECK_SLOT + 1 byte PROBE)
    }
  } break;

  default:
    LOG(1, "enqueue: @0x%x unknown instruction %d\n", sm->pc - 1, op);
    return 1;
  }

  return 0;
}

__attribute__((always_inline)) int sm_run(global_ctx_t* ctx) {
  // TODO: Use a tighter bound on the number of iterations. The current choice
  // is arbitrary.
  const int limit = 512 << 10;
  int n = bpf_loop(limit, sm_loop, ctx, 0);
  if (n == limit) {
    LOG(2, "stack machine loop hit limit of %d steps", n)
    return -1;
  }
  LOG(4, "stack machine loop finished in %d steps", n)
  return n;
}

__attribute__((always_inline)) int
stack_machine_process_frame(global_ctx_t* ctx, frame_data_t* frame_data,
                            uint32_t entrypoint) {
  ctx->stack_machine->pc = entrypoint;
  ctx->stack_machine->offset = scratch_buf_len(ctx->buf);
  ctx->stack_machine->frame_data = *frame_data;
  ctx->stack_machine->condition_failed = false;
  ctx->stack_machine->condition_eval_error = false;
  ctx->stack_machine->condition_nil_deref = false;
  return sm_run(ctx);
}

__attribute__((always_inline)) int
stack_machine_chase_pointers(global_ctx_t* ctx) {
  ctx->stack_machine->pc = chase_pointers_entrypoint;
  ctx->stack_machine->offset = scratch_buf_len(ctx->buf);
  ctx->stack_machine->condition_failed = false;
  ctx->stack_machine->condition_eval_error = false;
  ctx->stack_machine->condition_nil_deref = false;
  return sm_run(ctx);
}

#endif // __STACK_MACHINE_H__
