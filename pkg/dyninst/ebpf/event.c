#ifndef __EVENT_H__
#define __EVENT_H__

#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "cfa.h"
#include "compiler.h"
#include "context.h"
#include "framing.h"
#include "murmur2.h"
#include "walk_stack.h"
#include "scratch.h"
#include "throttler.h"

char _license[] SEC("license") = "GPL";

const uint32_t zero_uint32 = 0;

static inline __attribute__((always_inline)) void
read_g_fields(uint64_t g_ptr, uint64_t stack_ptr, uint64_t* goid, uint32_t* stack_byte_depth) {
  if (OFFSET_runtime_dot_g__goid == 0 && OFFSET_runtime_dot_g__m == 0) {
    return;
  }
  if (bpf_probe_read_user(
          goid, sizeof(*goid),
          (void*)(g_ptr + OFFSET_runtime_dot_g__goid))) {
    LOG(2, "failed to read goid %llx", g_ptr);
    return;
  }
  if (*goid == 0) {
    // This is pseudo-g. Extract g_ptr->m->curg->goid.
    uint64_t m_ptr;
    if (bpf_probe_read_user(
            &m_ptr, sizeof(m_ptr),
            (void*)(g_ptr + OFFSET_runtime_dot_g__m))) {
      LOG(2, "failed to read m %llx", g_ptr);
      return;
    }
    if (bpf_probe_read_user(
            &g_ptr, sizeof(g_ptr),
            (void*)(m_ptr + OFFSET_runtime_dot_m__curg))) {
      LOG(2, "failed to read curg %llx", m_ptr);
      return;
    }
    if (bpf_probe_read_user(
            goid, sizeof(*goid),
            (void*)(g_ptr + OFFSET_runtime_dot_g__goid))) {
      LOG(2, "failed to read goid %llx", g_ptr);
      return;
    }
  }
  uint64_t stack_hi;
  if (bpf_probe_read_user(
          &stack_hi, sizeof(stack_hi),
          (void*)(g_ptr + OFFSET_runtime_dot_g__stack + OFFSET_runtime_dot_stack__hi))) {
    LOG(2, "failed to read stack.lo %llx", g_ptr);
    return;
  }
  *stack_byte_depth = (stack_hi - stack_ptr);
  return;
}

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, call_depths_t);
} in_progress_calls_buf SEC(".maps");

static inline __attribute__((always_inline)) void
probe_run(uint64_t start_ns, const probe_params_t* params, struct pt_regs* regs) {
  LOG(4, "probe_run: %d %d %llx", params->probe_id, params->stack_machine_pc);
  global_ctx_t global_ctx;
  global_ctx.stack_machine = stack_machine_ctx_load(params);
  if (!global_ctx.stack_machine) {
    return;
  }
  global_ctx.stack_walk = stack_walk_ctx_load();
  if (!global_ctx.stack_walk) {
    return;
  }
  global_ctx.regs = NULL;

  // TODO: Move this check to after we've interacted with the call state.
  const int64_t out_ringbuf_avail_data =
      bpf_ringbuf_query(&out_ringbuf, BPF_RB_AVAIL_DATA);
  const int64_t out_ringbuf_avail_space =
      (int64_t)(RINGBUF_CAPACITY)-out_ringbuf_avail_data;
  if (out_ringbuf_avail_space < (int64_t)SCRATCH_BUF_LEN) {
    LOG(1, "probe_run: out_ringbuf_avail_space < SCRATCH_BUF_LEN: %lld < %d",
        out_ringbuf_avail_space, SCRATCH_BUF_LEN);
    // TODO: Report dropped events metric.
    return;
  }

  di_event_header_t* header = events_scratch_buf_init(&global_ctx.buf);
  if (!header) {
    return;
  }
  *header = (di_event_header_t){
      .data_byte_len = sizeof(di_event_header_t),
      .stack_byte_len = 0, // set this if we collect stacks
      .ktime_ns = start_ns,
      .prog_id = prog_id,
      .probe_id = params->probe_id,
  };
#if defined(bpf_target_x86)
  if (params->frameless) {
    read_g_fields(regs->DWARF_REGISTER_14, regs->DWARF_SP_REG, &header->goid, &header->stack_byte_depth);
  } else {
    read_g_fields(regs->DWARF_REGISTER_14, regs->DWARF_BP_REG, &header->goid, &header->stack_byte_depth);
  }
#elif defined(bpf_target_arm64)
  read_g_fields(regs->DWARF_REGISTER(28), regs->DWARF_SP_REG, &header->goid, &header->stack_byte_depth);
#else
#error "Unsupported architecture"
#endif

  if (params->kind == EVENT_KIND_RETURN) {
    // There was no corresponding call, so the deletion failed.
    call_depths_t* depths = bpf_map_lookup_elem(&in_progress_calls, &header->goid);
    if (!depths) {
      // Common case where the associated call was not found.
      LOG(4, "failed to lookup in_progress_calls %lld (%lld): %d",
          header->goid, header->stack_byte_depth, params->probe_id);
      return;
    }
    int remaining;
    if (!call_depths_delete(depths, header->stack_byte_depth, params->probe_id, &remaining)) {
      // Somewhat common case where the goroutine has open calls, but it's not
      // this one.
      LOG(4, "failed to delete in_progress_calls %lld (%lld): %d",
          header->goid, header->stack_byte_depth, params->probe_id);
      return;
    }
    // If we're the last call for this goid, delete the entry.
    if (remaining == 0) {
      int ret = bpf_map_delete_elem(&in_progress_calls, &header->goid);
      if (ret != 0) {
        // No clue why this would happen.
        LOG(1, "failed to delete in_progress_calls %lld (%lld) %d: %d",
            header->goid, header->stack_byte_depth, params->probe_id, ret);
      }
    }
    header->event_pairing_expectation = EVENT_PAIRING_ENTRY_PAIRING_EXPECTED;
  } else if (params->kind == EVENT_KIND_ENTRY && params->has_associated_return) {
    header->event_pairing_expectation = EVENT_PAIRING_RETURN_PAIRING_EXPECTED;
    // Optimistically assume this is the first call for this goid by just
    // attempting to insert with a single entry.
    //
    // In order to do an update, we need a value to write. We keep a per-cpu
    // array (in_progress_calls_buf) with a single element in the first slot
    // that we can use to update the in_progress_calls map.
    call_depths_t* depths = bpf_map_lookup_elem(&in_progress_calls_buf, &zero_uint32);
    if (!depths) {
      // This should never happen.
      LOG(1, "failed to get in_progress_calls_buf for %lld", header->goid);
      return;
    }
    depths->depths[0].depth = header->stack_byte_depth;
    depths->depths[0].probe_id = params->probe_id;
    int ret = bpf_map_update_elem(&in_progress_calls, &header->goid, depths, BPF_NOEXIST);
    if (ret != 0) {
      if (ret == -E2BIG) {
        // If the map is full, we can't insert any more calls so make sure we
        // tell userspace not to expect a return event.
        header->event_pairing_expectation = EVENT_PAIRING_EXPECTATION_CALL_MAP_FULL;
      } else if (ret == -EEXIST) {
        // If there are outstanding calls for this goid, we need to add this one
        // to the set.
        depths = bpf_map_lookup_elem(&in_progress_calls, &header->goid);
        if (!depths) {
          LOG(1, "failed to lookup in_progress_calls for goid %lld after failing to insert", header->goid);
          return;
        }
        // If we can't insert this call, we need to tell userspace not to expect
        // a return event.
        if (!call_depths_insert(depths, header->stack_byte_depth, params->probe_id)) {
          header->event_pairing_expectation = EVENT_PAIRING_EXPECTATION_CALL_COUNT_EXCEEDED;
        }
      }
    }
  } else {
    switch (params->no_return_reason) {
    case NO_RETURN_REASON_INLINED:
      LOG(4, "no return reason: inlined for goid %lld stack byte depth %d probe id %d", header->goid, header->stack_byte_depth, params->probe_id);
      header->event_pairing_expectation = EVENT_PAIRING_EXPECTATION_NONE_INLINED;
      break;
    case NO_RETURN_REASON_NO_BODY:
      LOG(4, "no return body for goid %lld stack byte depth %d probe id %d", header->goid, header->stack_byte_depth, params->probe_id);
      header->event_pairing_expectation = EVENT_PAIRING_EXPECTATION_NONE_NO_BODY;
      break;
    default:
      header->event_pairing_expectation = EVENT_PAIRING_EXPECTATION_NONE;
      break;
    }
  }
  __maybe_unused int process_steps = 0;
  __maybe_unused int chase_steps = 0;
  uint64_t stack_hash = 0;
  global_ctx.stack_walk->regs = *regs;
  global_ctx.stack_walk->stack.pcs.pcs[0] = regs->DWARF_PC_REG + params->top_pc_offset;
  LOG(5, "wrote event pairing expectation %d %lld %d %d %llx",
      header->event_pairing_expectation, header->goid, header->stack_byte_depth,
      params->probe_id, global_ctx.stack_walk->stack.pcs.pcs[0]);
#if defined(bpf_target_x86)
  global_ctx.stack_walk->stack.fps[0] = regs->DWARF_BP_REG;
  if (params->frameless) {
    // Call instruction saves return address on the stack.
    if (bpf_probe_read_user(&global_ctx.stack_walk->stack.pcs.pcs[1],
                            sizeof(global_ctx.stack_walk->stack.pcs.pcs[1]),
                            (void*)(regs->sp))) {
      return;
    }
    global_ctx.stack_walk->stack.fps[1] = regs->DWARF_BP_REG;
    global_ctx.stack_walk->idx_shift = 1;
  }
#elif defined(bpf_target_arm64)
  global_ctx.stack_walk->stack.fps[0] = regs->DWARF_SP_REG - 8;
  if (params->frameless) {
    // Call instruction saves return address in the link register.
    global_ctx.stack_walk->stack.pcs.pcs[1] = regs->DWARF_REGISTER(30);
    global_ctx.stack_walk->stack.fps[1] = regs->DWARF_SP_REG - 8;
    global_ctx.stack_walk->idx_shift = 1;
  }
#else
#error "Unsupported architecture"
#endif
  global_ctx.stack_walk->stack.pcs.len =
      bpf_loop(STACK_DEPTH, populate_stack_frame, &global_ctx.stack_walk, 0) +
      1;
  global_ctx.stack_walk->stack.pcs.len += global_ctx.stack_walk->idx_shift;
  stack_hash = hash_stack(&global_ctx.stack_walk->stack.pcs, 0 /* seed */);
  header->stack_hash = stack_hash;
  bool should_submit = !check_stack_hash(stack_hash);
  if (should_submit) {
    header->stack_byte_len =
        global_ctx.stack_walk->stack.pcs.len * sizeof(target_ptr_t);
    copy_stack_loop_ctx_t copy_stack_ctx = {
        .stack = &global_ctx.stack_walk->stack.pcs,
        .buf = global_ctx.buf,
    };
    bpf_loop(global_ctx.stack_walk->stack.pcs.len, copy_stack_loop,
             &copy_stack_ctx, 0);
    scratch_buf_increment_len(global_ctx.buf, header->stack_byte_len);
  } else {
    stack_hash = 0;
  }
  global_ctx.regs = &global_ctx.stack_walk->regs;
  frame_data_t frame_data = {
      .stack_idx = 0,
  };
  frame_data.cfa = calculate_cfa(global_ctx.regs, params->frameless);
  LOG(5, "cfa: %llx %d %llx %llx", frame_data.cfa, params->frameless, regs->DWARF_BP_REG, regs->DWARF_SP_REG);
  if (params->stack_machine_pc != 0) {
    process_steps = stack_machine_process_frame(&global_ctx, &frame_data,
                                                params->stack_machine_pc);
  }
  chase_steps = stack_machine_chase_pointers(&global_ctx);
  if (!events_scratch_buf_submit(global_ctx.buf)) {
    // TODO: Report dropped events metric.
    LOG(1, "probe_run output dropped");
  } else if (stack_hash != 0) {
    upsert_stack_hash(stack_hash);
  }
  LOG(1, "probe_run done: %d steps", process_steps + chase_steps);
  return;
}

// Cumulative stats aggregated throughout probe lifetime.
struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, stats_t);
} stats_buf SEC(".maps");

SEC("uprobe")
int probe_run_with_cookie(struct pt_regs* regs) {
  uint64_t start_ns = bpf_ktime_get_ns();

  stats_t* stats = bpf_map_lookup_elem(&stats_buf, &zero_uint32);
  if (!stats) {
    return 0;
  }
  stats->hit_cnt++;

  const uint64_t cookie = bpf_get_attach_cookie(regs);
  if (cookie >= num_probe_params) {
    return 0;
  }
  const probe_params_t* params = bpf_map_lookup_elem(&probe_params, &cookie);
  if (!params) {
    return 0;
  }

  if (params->kind != EVENT_KIND_RETURN && should_throttle(params->throttler_idx, start_ns)) {
    stats->throttled_cnt++;
  } else {
    probe_run(start_ns, params, regs);
  }

  stats->cpu_ns += bpf_ktime_get_ns() - start_ns;
  return 0;
}

#endif // __EVENT_H__
