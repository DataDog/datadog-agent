#ifndef __EVENT_H__
#define __EVENT_H__

#include "bpf_helpers.h"
#include "compiler.h"
#include "context.h"
#include "framing.h"
#include "murmur2.h"
#include "walk_stack.h"
#include "scratch.h"

char _license[] SEC("license") = "GPL";

SEC("uprobe") int probe_run_with_cookie(struct pt_regs* regs) {
  uint64_t start = bpf_ktime_get_ns();

  const uint64_t cookie = bpf_get_attach_cookie(regs);
  if (cookie >= num_probe_params) {
    return 0;
  }
  const probe_params_t* params = &probe_params[cookie];

  global_ctx_t global_ctx;
  global_ctx.stack_machine = stack_machine_ctx_load();
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

  event_header_t* header = events_scratch_buf_init(&global_ctx.buf);
  if (!header) {
    return 0;
  }
  *header = (event_header_t){
      .data_byte_len = sizeof(event_header_t),
      .stack_byte_len = 0, // set this if we collect stacks
      .ktime_ns = start,
  };

  __maybe_unused int process_steps = 0;
  __maybe_unused int chase_steps = 0;
  uint64_t stack_hash = 0;
  global_ctx.stack_walk->regs = *regs;
  global_ctx.stack_walk->stack.pcs.pcs[0] = regs->ip;
  global_ctx.stack_walk->stack.fps[0] = regs->bp;
  if (params->frameless) {
    if (bpf_probe_read_user(&global_ctx.stack_walk->stack.pcs.pcs[1],
                            sizeof(global_ctx.stack_walk->stack.pcs.pcs[1]),
                            (void*)(regs->sp))) {
      return 1;
    }
    global_ctx.stack_walk->stack.fps[1] = regs->bp;
    global_ctx.stack_walk->idx_shift = 1;
  }
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
      .fp = global_ctx.regs->bp,
      .stack_idx = 0,
  };
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
