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

DEFINE_BINARY_SEARCH(
  lookup_type_info,
  type_t,
  type_id,
  type_ids,
  num_types
);

static bool get_type_info(type_t t, const type_info_t** info_out) {
  uint32_t idx = lookup_type_info_by_type_id(t);
  *info_out = bpf_map_lookup_elem(&type_info, &idx);
  if (!*info_out) {
    return false;
  }
  return true;
}

__attribute__((noinline)) bool chased_pointer_contains(chased_pointers_t* chased, target_ptr_t ptr, type_t type) {
  if (!chased) {
    return false;
  }
  uint32_t max = chased->n;
  if (max >= MAX_CHASED_POINTERS) {
    return false;
  }
  // Iterating backwards results in simpler code that passes the verifier.
  for (int32_t i = max-1; i >= 0; i--) {
    if (chased->ptrs[i] == ptr && chased->types[i] == type) {
      return true;
    }
  }
  return false;
}

static bool chased_pointers_push(chased_pointers_t* chased, target_ptr_t ptr,
                                 type_t type) {
  if (chased_pointer_contains(chased, ptr, type)) {
    return false;
  }
  uint32_t i = chased->n;
  if (i >= MAX_CHASED_POINTERS) { // to please the verifier
    return false;
  }
  chased->ptrs[i] = ptr;
  chased->types[i] = type;
  chased->n++;
  return true;
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

static inline __attribute__((always_inline)) uint16_t
sm_read_program_uint16(stack_machine_t* sm) {
  uint32_t zero = 0;
  uint8_t* data = bpf_map_lookup_elem(&stack_machine_code, &zero);
  if (!data) {
    LOG(1, "enqueue: failed to load code\n");
    return 0;
  }
  if (sm->pc >= stack_machine_code_len-1) {
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
  if (sm->pc >= stack_machine_code_len-3) {
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
  sm->offset = scratch_buf_serialize(ctx->buf, &item.di, info->byte_len);
  if (!sm->offset) {
    LOG(3, "chase: failed to serialize type %d", item.di.type);
    return true;
  }

  // Recurse if there is more to capture object of this type.
  sm->pointer_chasing_ttl = item.ttl;
  sm->di_0 = item.di;
  sm->di_0.length = info->byte_len;
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
                   target_ptr_t addr) {
  // Check if address was already processed before.
  stack_machine_t* sm = ctx->stack_machine;
  return chased_pointers_push(&sm->chased, addr, type);
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
  if (!sm_memoize_pointer(ctx, type, addr)) {
    return true;
  }
  pointers_queue_item_t* item;
  if (decrease_ttl) {
    item = pointers_queue_push_back(&ctx->stack_machine->pointers_queue);
  } else {
    item = pointers_queue_push_front(&ctx->stack_machine->pointers_queue);
  }
  if (item == NULL) {
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

// inline __attribute__((always_inline)) bool
// sm_record_go_interface_impl(global_ctx_t* global_ctx, uint64_t go_runtime_type,
//                          target_ptr_t addr) {
//   // Resolve implementation type.
//   if (go_runtime_type == (uint64_t)(-1)) {
//     // TODO: Maybe this should not short-circuit the rest of execution.
//     // Note that this happens only when there's an issue reading the
//     // runtime.firstmoduledata or if the type does not reside inside of
//     // it.
//     LOG(3, "chase: interface unknown go runtime type");
//     return true;
//   }
//   type_t t = lookup_go_interface(go_runtime_type);
//   if (t == TYPE_NONE) {
//     LOG(4, "chase: interface type not found %lld", go_runtime_type);
//     return true;
//   }
//   return sm_record_pointer(global_ctx, t, addr, ENQUEUE_LEN_SENTINEL);
// }

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

// // Translate a pointer to a type (i.e. a pointer pointing to type information
// // inside moduledata) like that found inside an empty interface to an offset
// // into moduledata. We commonly represent runtime type information as such an
// // offset.
// inline __attribute__((always_inline)) uint64_t
// go_runtime_type_from_ptr(target_ptr_t type_ptr) {
//   const unsigned long zero = 0;
//   moduledata_t* moduledata =
//       (moduledata_t*)bpf_map_lookup_elem(&moduledata_buf, &zero);
//   if (!moduledata) {
//     return -1;
//   }
//   // Detect if the moduledata is up-to-date by checking if the address is
//   // correct. If it is not, then we need to update the typebounds.
//   if (moduledata->addr != VARIABLE_runtime_dot_firstmoduledata) {
//     moduledata->addr = VARIABLE_runtime_dot_firstmoduledata;
//     if (bpf_probe_read_user(&moduledata->types, sizeof(typebounds_t),
//                             (void*)(VARIABLE_runtime_dot_firstmoduledata +
//                                     OFFSET_runtime_dot_moduledata__types))) {
//       return -1;
//     }
//   }

//   typebounds_t* typebounds = &moduledata->types;
//   if (type_ptr >= typebounds->types && type_ptr < typebounds->etypes) {
//     return type_ptr - typebounds->types;
//   }
//   return -1;
// }

// inline __attribute__((always_inline)) bool
// sm_resolve_go_empty_interface(global_ctx_t* ctx, resolved_go_interface_t* res) {
//   scratch_buf_t* buf = ctx->buf;
//   stack_machine_t* sm = ctx->stack_machine;
//   buf_offset_t offset = sm->offset;
//   res->addr = 0;
//   res->go_runtime_type = 0;
//   if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) * 2)) {
//     return false;
//   }
//   if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) +
//                                              OFFSET_runtime_dot_eface__data)) {
//     return false;
//   }
//   target_ptr_t type_addr =
//       *(target_ptr_t*)(&(*buf)[offset + OFFSET_runtime_dot_eface___type]);
//   res->addr =
//       *(target_ptr_t*)&((*buf)[offset + OFFSET_runtime_dot_eface__data]);
//   if (type_addr == 0) {
//     // Not an error, just literally a nil interface.
//     return true;
//   }
//   // TODO: check the return value for error
//   res->go_runtime_type = go_runtime_type_from_ptr(type_addr);
//   return true;
// }

// // Resolves address and implementation type of a non-empty interface.
// inline __attribute__((always_inline)) bool
// sm_resolve_go_interface(global_ctx_t* ctx, resolved_go_interface_t* res) {
//   scratch_buf_t* buf = ctx->buf;
//   stack_machine_t* sm = ctx->stack_machine;
//   buf_offset_t offset = sm->offset;
//   res->addr = 0;
//   res->go_runtime_type = 0;
//   if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) * 2)) {
//     return false;
//   }
//   if (!scratch_buf_bounds_check(&offset, sizeof(target_ptr_t) +
//                                              OFFSET_runtime_dot_iface__data)) {
//     return false;
//   }
//   res->addr =
//       *(target_ptr_t*)&((*buf)[offset + OFFSET_runtime_dot_iface__data]);
//   target_ptr_t itab =
//       *(target_ptr_t*)(&(*buf)[offset + OFFSET_runtime_dot_iface__tab]);
//   if (itab == 0) {
//     return true;
//   }
//   target_ptr_t type_addr;
//   if (bpf_probe_read_user(&type_addr, sizeof(target_ptr_t),
//                           (void*)(itab) +
//                               (uint64_t)(OFFSET_runtime_dot_itab___type))) {
//     LOG(3, "enqueue: failed interface type read %llx",
//         (void*)(itab) + (uint64_t)(OFFSET_runtime_dot_itab___type));
//     return true;
//   }
//   // TODO: check the return value for error
//   res->go_runtime_type = go_runtime_type_from_ptr(type_addr);
//   return true;
// }

// inline __attribute__((always_inline)) bool
// sm_resolve_go_any_type(global_ctx_t* global_ctx, resolved_go_any_type_t* r) {
//   r->i.addr = 0;
//   r->i.go_runtime_type = 0;
//   r->type = (type_t)0;
//   r->has_info = false;
//   if (!sm_resolve_go_empty_interface(global_ctx, &r->i)) {
//     return false;
//   }
//   if (r->i.go_runtime_type == (uint64_t)(-1)) {
//     return true;
//   }
//   r->type = lookup_go_interface(r->i.go_runtime_type);
//   if (r->type == TYPE_NONE) {
//     return true;
//   }
//   const type_info_t* info;
//   if (!get_type_info(r->type, &info)) {
//     LOG(3, "any type info not found %d", r->type);
//     return true;
//   }
//   r->has_info = true;
//   r->info = *info;
//   return true;
// }

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
  LOG(4, "%6x %s %s", sm->pc - 1, padding(sm->pc_stack_pointer), op_code_name(op));
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
    uint32_t bit_offset = sm_read_program_uint32(sm);

    // Save the result.
    copy_data(buf, sm->offset, sm->expr_results_offset + result_offset,
              byte_len);

    LOG(4, "copy data 0x%llx->0x%llx !%u", sm->offset, sm->expr_results_offset + result_offset, byte_len);

    // Set the presence bit.
    sm->buf_offset_0 = sm->expr_results_offset + bit_offset / 8;
    bit_offset %= 8;
    if (!scratch_buf_bounds_check(&sm->buf_offset_0, 1)) {
      return 1;
    }
    (*buf)[sm->buf_offset_0] |= (1 << bit_offset);

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
      case 0: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(0); break;
      case 1: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(1); break;
      case 2: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(2); break;
      case 3: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(3); break;
      case 4: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(4); break;
      case 5: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(5); break;
      case 6: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(6); break;
      case 7: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(7); break;
      case 8: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(8); break;
      case 9: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(9); break;
      case 10: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(10); break;
      case 11: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(11); break;
      case 12: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(12); break;
      case 13: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(13); break;
      case 14: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(14); break;
      case 15: *(volatile uint64_t*)(&sm->value_0) = regs->DWARF_REGISTER(15); break;
      default: LOG(2, "unknown register: %d", regnum); return 1;
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

  // case SM_OP_EXPR_DEREFERENCE_PTR: {
  //   uint32_t bias = sm_read_program_uint32(sm);
  //   uint32_t byte_len = sm_read_program_uint32(sm);
  //   buf_offset_t value_offset = sm->offset;
  //   if (!scratch_buf_bounds_check(&value_offset, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   data_item_header_t di = {
  //       .type = 0,
  //       .length = byte_len,
  //       .address = *(target_ptr_t*)&((*buf)[value_offset]) + bias};
  //   if (di.address == 0) {
  //     sm->offset = 0;
  //   } else {
  //     sm->offset = scratch_buf_serialize(buf, &di, byte_len);
  //   }
  //   if (!sm->offset) {
  //     // Abort expression evaluation by returning early.
  //     scratch_buf_set_len(buf, sm->expr_results_end_offset);
  //     if (!sm_return(sm)) {
  //       return 1;
  //     }
  //   }
  // } break;

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
    sm_data_stack_push(sm, array_len);
    LOG(4, "array data prep: %d", array_len);
  } break;

  case SM_OP_PROCESS_SLICE_DATA_PREP: {
    if (sm->di_0.length == 0) {
      // Nothing to do for an empty slice.
      sm_return(sm);
      break;
    }

    // We need to iterate over the slice data, push the length on the data stack to control the loop.
    sm_data_stack_push(sm, sm->di_0.length);
  } break;

  case SM_OP_PROCESS_SLICE_DATA_REPEAT: {
    uint32_t elem_byte_len = sm_read_program_uint32(sm);
    sm->offset += elem_byte_len;
    uint32_t sp = *(volatile uint32_t *)&sm->data_stack_pointer;
    uint32_t stack_idx = sp - 1;
    if (stack_idx >= ENQUEUE_STACK_DEPTH) {
      if (stack_idx + 1 == 0) {
        LOG(2, "unexpected empty data stack during slice iteration");
      } else {
        LOG(2, "unexpected full data stack during slice iteration");
      }
      return 1;
    }
    uint32_t* remaining =  &sm->data_stack[stack_idx];
    LOG(4, "remaining: %d", *remaining);
    if (*remaining <= elem_byte_len) {
      // End of the slice.
      sm_data_stack_pop(sm);
      break;
    }
    *remaining -= elem_byte_len;
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
    LOG(4, "enqueue: string len @%llx !%lld", addr, len)
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

  // case SM_OP_ENQUEUE_GO_EMPTY_INTERFACE: {
  //   resolved_go_interface_t r;
  //   if (!sm_resolve_go_empty_interface(ctx, &r)) {
  //     return 1;
  //   }
  //   // Overwrite the type_addr with the go_runtime_type.
  //   if (!scratch_buf_bounds_check(&sm->offset, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   *(uint64_t*)(&(*buf)[sm->offset + OFFSET_runtime_dot_eface___type]) =
  //       r.go_runtime_type;
  //   if (!sm_record_go_interface_impl(ctx, r.go_runtime_type, r.addr)) {
  //     LOG(3, "enqueue: failed empty interface chase");
  //   }
  // } break;

  // case SM_OP_ENQUEUE_GO_INTERFACE: {
  //   resolved_go_interface_t r;
  //   if (!sm_resolve_go_interface(ctx, &r)) {
  //     return 1;
  //   }
  //   if (r.go_runtime_type == 0) {
  //     break;
  //   }
  //   // Overwrite the type_addr with the go_runtime_type.
  //   if (!scratch_buf_bounds_check(&sm->offset, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   *(uint64_t*)(&(*buf)[sm->offset + OFFSET_runtime_dot_iface__tab]) =
  //       r.go_runtime_type;
  //   if (!sm_record_go_interface_impl(ctx, r.go_runtime_type, r.addr)) {
  //     LOG(3, "enqueue: failed interface chase");
  //   }
  // } break;

  // case SM_OP_ENQUEUE_GO_HMAP_HEADER: {
  //   // https://github.com/golang/go/blob/8d04110c/src/runtime/map.go#L105
  //   const uint8_t same_size_grow = 8;

  //   type_t buckets_array_type = (type_t)sm_read_program_uint32(sm);
  //   uint32_t bucket_byte_len = sm_read_program_uint32(sm);

  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   uint8_t flags = *(uint8_t*)&((*buf)[sm->buf_offset_0]);

  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   uint8_t b = *(uint8_t*)&((*buf)[sm->buf_offset_0]);

  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   target_ptr_t buckets_addr = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);

  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   target_ptr_t oldbuckets_addr = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);

  //   // We might have to chase two sets of buckets. Stack variable controls
  //   // jumping to repeat this op.
  //   uint32_t stack_top = sm->data_stack_pointer - 1;
  //   if (stack_top >= ENQUEUE_STACK_DEPTH) {
  //     LOG(2, "enqueue: stack out of bounds %d", stack_top);
  //     return 1;
  //   }
  //   uint32_t* stage = &sm->data_stack[stack_top];

  //   if (*stage == 2) {
  //     // This is first iteration.
  //     *stage = 1;
  //     if (buckets_addr != 0) {
  //       uint32_t num_buckets = 1 << b;
  //       uint32_t buckets_size = num_buckets * bucket_byte_len;
  //       if (!sm_record_pointer(ctx, buckets_array_type, buckets_addr,
  //                              buckets_size)) {
  //         LOG(3, "enqueue: failed map chase (new buckets)");
  //       }
  //       break;
  //     }
  //   }

  //   // This is second iteration, or there were no new buckets.
  //   *stage = 0;
  //   if (oldbuckets_addr != 0) {
  //     uint32_t num_buckets = 1 << b;
  //     if ((flags & same_size_grow) == 0) {
  //       num_buckets >>= 1;
  //     }
  //     uint32_t buckets_size = num_buckets * bucket_byte_len;
  //     if (!sm_record_pointer(ctx, buckets_array_type, oldbuckets_addr,
  //                            buckets_size)) {
  //       LOG(3, "enqueue: failed map chase (old buckets)");
  //     }
  //   }
  // } break;

  // case SM_OP_ENQUEUE_GO_SWISS_MAP: {
  //   type_t table_ptr_slice_type = (type_t)sm_read_program_uint32(sm);
  //   type_t group_type = (type_t)sm_read_program_uint32(sm);
  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   sm->buf_offset_1 = sm->offset + sm_read_program_uint8(sm);

  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   target_ptr_t dir_ptr = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_1, sizeof(int64_t))) {
  //     return 1;
  //   }
  //   int64_t dir_len = *(int64_t*)&((*buf)[sm->buf_offset_1]);

  //   if (dir_len > 0) {
  //     if (!sm_record_pointer(ctx, table_ptr_slice_type, dir_ptr, 8 * dir_len)) {
  //       LOG(3, "enqueue: failed swiss map record (full)");
  //     }
  //   } else {
  //     if (!sm_record_pointer(ctx, group_type, dir_ptr, ENQUEUE_LEN_SENTINEL)) {
  //       LOG(3, "enqueue: failed swiss map record (inline)");
  //     }
  //   }
  // } break;

  // case SM_OP_ENQUEUE_GO_SWISS_MAP_GROUPS: {
  //   type_t group_slice_type = (type_t)sm_read_program_uint32(sm);
  //   uint32_t group_byte_len = sm_read_program_uint32(sm);

  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(target_ptr_t))) {
  //     return 1;
  //   }
  //   target_ptr_t data = *(target_ptr_t*)&((*buf)[sm->buf_offset_0]);

  //   sm->buf_offset_0 = sm->offset + sm_read_program_uint8(sm);
  //   if (!scratch_buf_bounds_check(&sm->buf_offset_0, sizeof(int64_t))) {
  //     return 1;
  //   }
  //   uint64_t length_mask = *(uint64_t*)&((*buf)[sm->buf_offset_0]);

  //   if (!sm_record_pointer(ctx, group_slice_type, data,
  //                          group_byte_len * (length_mask + 1))) {
  //     LOG(3, "enqueue: failed swiss map groups record");
  //   }
  // } break;

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

  default: LOG(1, "enqueue: @0x%x unknown instruction %d\n", sm->pc-1, op); return 1;
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
  return sm_run(ctx);
}

__attribute__((always_inline)) int
stack_machine_chase_pointers(global_ctx_t* ctx) {
  ctx->stack_machine->pc = chase_pointers_entrypoint;
  ctx->stack_machine->offset = scratch_buf_len(ctx->buf);
  return sm_run(ctx);
}

#endif // __STACK_MACHINE_H__
