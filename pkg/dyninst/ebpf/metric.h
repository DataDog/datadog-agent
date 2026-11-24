#ifndef __METRIC_H__
#define __METRIC_H__

// Stack machine helpers for metric probes. The event handler of a metric probe
// evaluates the probe's condition and value like any other probe, and then
// records the value in the probe's metric_state_t in metrics_buf instead of
// producing an event. Userspace reads metrics_buf periodically.
//
// The helpers are global (noinline) functions so that the verifier checks them
// once, independently of the stack machine loop. Like the filter helpers, they
// re-derive their map pointers instead of taking them as arguments.
//
// Must be included from stack_machine.h, after the EXPR_STATUS_* definitions.

#include "context.h"
#include "program.h"
#include "scratch.h"
#include "types.h"

// expr_status_read reads the status of the expression with the given index
// from the packed expression status array.
static __always_inline uint8_t expr_status_read(
    scratch_buf_t* buf,
    buf_offset_t base_offset,
    uint32_t expr_idx
) {
  uint32_t bit_offset = expr_idx * EXPR_STATUS_BITS;
  buf_offset_t byte_offset = base_offset + bit_offset / 8;
  uint32_t shift = bit_offset % 8;
  if (!scratch_buf_bounds_check(&byte_offset, 1)) {
    return EXPR_STATUS_ABSENT;
  }
  return ((uint8_t)(*buf)[byte_offset] >> shift) & ((1 << EXPR_STATUS_BITS) - 1);
}

static __always_inline void metric_record_error(metric_state_t* state,
                                                uint8_t error) {
  __sync_fetch_and_add(&state->eval_errors, 1);
  state->last_error = error;
}

// sm_metric_cond_error_check_noctx runs right after a metric probe's condition.
// When the condition couldn't be evaluated, the stack machine treats it as
// passed, but a metric probe shouldn't count such a hit: this records an
// evaluation error instead and returns 1 to abort the event handler. Returns 0
// otherwise.
__attribute__((noinline)) int
sm_metric_cond_error_check_noctx(uint32_t probe_id) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  if (!sm) {
    return 1;
  }
  if (!sm->condition_eval_error) {
    return 0;
  }
  metric_state_t* state = bpf_map_lookup_elem(&metrics_buf, &probe_id);
  if (state) {
    metric_record_error(state, sm->condition_nil_deref
                                   ? METRIC_ERROR_CONDITION_NIL_DEREF
                                   : METRIC_ERROR_CONDITION);
  }
  return 1;
}

// sm_metric_update_noctx records a hit of a metric probe in its metric state.
// The value, if the probe has one, is the event's first root expression; it is
// read from the event root data at value_offset. Returns 1 if the hit couldn't
// be recorded.
__attribute__((noinline)) int
sm_metric_update_noctx(uint32_t probe_id, uint32_t kind, uint32_t value_kind,
                       uint32_t value_size, uint32_t value_offset) {
  const uint32_t zero = 0;
  stack_machine_t* sm = bpf_map_lookup_elem(&stack_machine_buf, &zero);
  scratch_buf_t* buf = bpf_map_lookup_elem(&events_scratch_buf_map, &zero);
  metric_state_t* state = bpf_map_lookup_elem(&metrics_buf, &probe_id);
  if (!sm || !buf || !state) {
    return 1;
  }
  uint64_t bits = 1; // Count probes without a value count their hits.
  if (value_kind != METRIC_VALUE_NONE) {
    uint8_t status = expr_status_read(buf, sm->expr_results_offset, 0);
    if (status != EXPR_STATUS_PRESENT) {
      uint8_t error = METRIC_ERROR_VALUE_UNAVAILABLE;
      if (status == EXPR_STATUS_NIL_DEREF) {
        error = METRIC_ERROR_VALUE_NIL_DEREF;
      } else if (status == EXPR_STATUS_OOB) {
        error = METRIC_ERROR_VALUE_OUT_OF_BOUNDS;
      }
      metric_record_error(state, error);
      return 1;
    }
    bool is_signed = value_kind == METRIC_VALUE_SIGNED;
    buf_offset_t offset = sm->expr_results_offset + value_offset;
    // Each read is preceded by its own bounds check with a constant length,
    // which is what lets the verifier bound the access.
    switch (value_size) {
    case 1: {
      if (!scratch_buf_bounds_check(&offset, 1)) {
        return 1;
      }
      uint8_t v = *(uint8_t*)(&(*buf)[offset]);
      bits = is_signed ? (uint64_t)(int64_t)(int8_t)v : v;
    } break;
    case 2: {
      if (!scratch_buf_bounds_check(&offset, 2)) {
        return 1;
      }
      uint16_t v = *(uint16_t*)(&(*buf)[offset]);
      bits = is_signed ? (uint64_t)(int64_t)(int16_t)v : v;
    } break;
    case 4: {
      if (!scratch_buf_bounds_check(&offset, 4)) {
        return 1;
      }
      uint32_t v = *(uint32_t*)(&(*buf)[offset]);
      bits = is_signed ? (uint64_t)(int64_t)(int32_t)v : v;
    } break;
    case 8: {
      if (!scratch_buf_bounds_check(&offset, 8)) {
        return 1;
      }
      bits = *(uint64_t*)(&(*buf)[offset]);
    } break;
    default:
      return 1;
    }
  }
  if (kind == METRIC_KIND_COUNT) {
    __sync_fetch_and_add(&state->sum, (int64_t)bits);
  } else {
    state->value = bits;
  }
  __sync_fetch_and_add(&state->updates, 1);
  return 0;
}

#endif // __METRIC_H__
