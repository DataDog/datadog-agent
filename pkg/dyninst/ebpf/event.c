#ifndef __EVENT_H__
#define __EVENT_H__

#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "compiler.h"
#include "context.h"
#include "framing.h"
#include "murmur2.h"
#include "walk_stack.h"
#include "scratch.h"
#include "throttler.h"

char _license[] SEC("license") = "GPL";

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, uint64_t);
} throttled_events SEC(".maps");

SEC("uprobe") int probe_run_with_cookie(struct pt_regs* regs) {
  uint64_t start_ns = bpf_ktime_get_ns();

  const uint64_t cookie = bpf_get_attach_cookie(regs);
  if (cookie >= num_probe_params) {
    return 0;
  }
  const probe_params_t* params = bpf_map_lookup_elem(&probe_params, &cookie);
  if (!params) {
    return 0;
  }

  if (should_throttle(params->throttler_idx, start_ns)) {
    uint32_t zero = 0;
    uint64_t* cnt = bpf_map_lookup_elem(&throttled_events, &zero);
    if (cnt) {
      ++*cnt;
    }
    return 0;
  }
  global_ctx_t global_ctx;
  global_ctx.stack_machine = stack_machine_ctx_load(params->pointer_chasing_limit);
  if (!global_ctx.stack_machine) {
    return 0;
  }
  global_ctx.stack_walk = stack_walk_ctx_load();
  if (!global_ctx.stack_walk) {
    return 0;
  }
  global_ctx.regs = NULL;

  const int64_t out_ringbuf_avail_data =
      bpf_ringbuf_query(&out_ringbuf, BPF_RB_AVAIL_DATA);
  const int64_t out_ringbuf_avail_space =
      (int64_t)(RINGBUF_CAPACITY)-out_ringbuf_avail_data;
  if (out_ringbuf_avail_space < (int64_t)SCRATCH_BUF_LEN) {
    // TODO: Report dropped events metric.
    return 0;
  }

  di_event_header_t* header = events_scratch_buf_init(&global_ctx.buf);
  if (!header) {
    return 0;
  }
  *header = (di_event_header_t){
      .data_byte_len = sizeof(di_event_header_t),
      .stack_byte_len = 0, // set this if we collect stacks
      .ktime_ns = start_ns,
      .prog_id = prog_id,
  };

  __maybe_unused int process_steps = 0;
  __maybe_unused int chase_steps = 0;
  uint64_t stack_hash = 0;
  global_ctx.stack_walk->regs = *regs;
  global_ctx.stack_walk->stack.pcs.pcs[0] = regs->DWARF_PC_REG;
#if defined(bpf_target_x86)
  bool frameless = *(volatile bool *)&params->frameless;
  global_ctx.stack_walk->stack.fps[0] = regs->DWARF_BP_REG;
  if (frameless) {
    if (bpf_probe_read_user(&global_ctx.stack_walk->stack.pcs.pcs[1],
                            sizeof(global_ctx.stack_walk->stack.pcs.pcs[1]),
                            (void*)(regs->sp))) {
      return 1;
    }
    global_ctx.stack_walk->stack.fps[1] = regs->DWARF_BP_REG;
    global_ctx.stack_walk->idx_shift = 1;
  }
#elif defined(bpf_target_arm64)
  // Use the link register to populate the next frame's pc.
  global_ctx.stack_walk->stack.pcs.pcs[1] = regs->DWARF_REGISTER(30);
  // For reasons explained below when setting up the cfa, the BP register is
  // pointing to the caller's frame pointer at this point.
  global_ctx.stack_walk->stack.fps[1] = regs->DWARF_BP_REG;
  global_ctx.stack_walk->idx_shift = 1;
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
// Stack layout is slightly different in Go between arm64 and x86_64.
// Established based on following documentation and machine code reads:
// https://tip.golang.org/src/cmd/compile/abi-internal#architecture-specifics
//
// There's some trickery to get our hands on the CFA in Go based on whether or
// not the leaf function is frameless. If it is frameless, then we need to
// assume that the frame pointer is actually set up for the caller's frame.
#if defined(bpf_target_arm64)
// On arm, if the function is framless, the base pointer will be pointing to at
// the lowest entry of our caller's frame which is almost our CFA. If it's not
// frameless, then the base pointer will be pointing to the lowest entry of our
// frame and it needs to be dereferenced to get to the CFA. Fortunately, we
// already did that dereferencing in the stack walk.
//
// Or at least you might think that'd be the situation. Unfortunately, or
// perhaps fortunately, Go is marking the beginning of the prologue stack
// adjustment as the end of the prologue. So, when we use the prologue_end
// marker set by Go in DWARF to find the prologue end, we're actually getting
// the beginning of the prologue adjustment and can assume the registers are
// still set up for the caller's frame.
//
// See https://github.com/golang/go/issues/74357.
    *(volatile uint64_t*)(&frame_data.cfa) = global_ctx.regs->DWARF_BP_REG + 8;
#elif defined(bpf_target_x86)
// On x86, if the function is frameless, the stack pointer is pointing to the
// return pc, so one word above that is our CFA. If it's not frameless, then the
// base pointer is pointing to our frame pointer which is 16 bytes less than our
// CFA.
    if (frameless) {
      *(volatile uint64_t*)(&frame_data.cfa) = global_ctx.regs->DWARF_SP_REG + 8;
    } else {
      *(volatile uint64_t*)(&frame_data.cfa) = global_ctx.regs->DWARF_BP_REG + 16;
    }
#else
    #error "Unsupported architecture"
#endif

  if (params->stack_machine_pc != 0) {
    process_steps = stack_machine_process_frame(&global_ctx, &frame_data,
                                                params->stack_machine_pc);
  }
  chase_steps = stack_machine_chase_pointers(&global_ctx);
  if (!events_scratch_buf_submit(global_ctx.buf)) {
    // TODO: Report dropped events metric.
  }
  if (stack_hash != 0) {
    upsert_stack_hash(stack_hash);
  }
  LOG(1, "probe_run done: %d steps", process_steps + chase_steps);
  return 0;
}

#endif // __EVENT_H__
