#ifndef __COORDINATED_SAMPLE_H__
#define __COORDINATED_SAMPLE_H__

#include "throttler.h"
#include "program.h"
#include "types.h"

// Coordinated sampling: make one keep/drop decision per trace and apply it to
// every probe in that trace. See pkg/dyninst/docs/coordinated-sampling-plan.md.
//
// Note on structure: much of this file is shaped by BPF verifier / 512-byte
// combined-stack limits rather than by the logic. Working state lives in
// per-CPU scratch instead of the stack, and coord_extract_trace_id /
// coordinated_should_drop are noinline, because they inline into
// probe_run_with_cookie (which also inlines probe_run) and would otherwise
// overflow the stack.

// Probe ids at or above this bound skip the per-trace per-probe cap.
#define COORD_MAX_PROBES 256
#define COORD_BITSET_WORDS (COORD_MAX_PROBES / 64)

enum coord_decision {
  COORD_DECISION_UNSET = 0,
  COORD_DECISION_DROP = 1,
  COORD_DECISION_EMIT = 2,
};

typedef struct trace_decision {
  uint8_t decision; // enum coord_decision
  uint8_t __padding[7];
  // Bit i set once probe i has emitted in this trace (per-probe cap).
  uint64_t emitted[COORD_BITSET_WORDS];
} trace_decision_t;

// LRU eviction bounds memory; an evicted in-flight trace just re-decides.
struct {
  __uint(type, BPF_MAP_TYPE_LRU_HASH);
  __uint(max_entries, 16384);
  __type(key, uint64_t);
  __type(value, trace_decision_t);
} decision_by_trace SEC(".maps");

typedef struct coord_scratch {
  struct pt_regs regs;
  trace_context_t trace;
  resolved_go_interface_t cur;
  uint64_t trace_id;
  uint8_t present;
  uint8_t __pad[7];
  trace_decision_t nd; // BPF_NOEXIST insert value
} coord_scratch_t;

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, coord_scratch_t);
} coord_scratch_buf SEC(".maps");

// Draw one unit from the session-global budget without gating. Used for
// inherited emits; may go negative, which suppresses future traces' entry
// decisions until should_throttle() refreshes the period.
static inline __attribute__((always_inline)) void
session_throttler_consume(void) {
  uint32_t idx = session_throttler_idx;
  throttler_t* t = (throttler_t*)bpf_map_lookup_elem(&throttler_buf, &idx);
  if (t) {
    __sync_fetch_and_sub(&t->budget, 1);
  }
}

// The first event of a trace decides via the session-global throttler and
// stores the result; later events inherit it. On EMIT, each probe may emit at
// most once per trace. Returns true to drop.
static __attribute__((noinline)) bool
coordinated_should_drop(const probe_params_t* params, uint64_t start_ns,
                        uint64_t trace_id) {
  if (!params) {
    return true;
  }
  uint32_t probe_id = params->probe_id;
  bool is_entry = false;

  trace_decision_t* d =
      (trace_decision_t*)bpf_map_lookup_elem(&decision_by_trace, &trace_id);
  if (!d) {
    // First event for this trace. Concurrent first-firers race on the insert;
    // the loser falls through to the shared record.
    uint32_t zero = 0;
    coord_scratch_t* s =
        (coord_scratch_t*)bpf_map_lookup_elem(&coord_scratch_buf, &zero);
    if (!s) {
      return true;
    }
    __builtin_memset(&s->nd, 0, sizeof(s->nd));
    s->nd.decision = should_throttle(session_throttler_idx, start_ns)
                         ? COORD_DECISION_DROP
                         : COORD_DECISION_EMIT;
    long r =
        bpf_map_update_elem(&decision_by_trace, &trace_id, &s->nd, BPF_NOEXIST);
    d = (trace_decision_t*)bpf_map_lookup_elem(&decision_by_trace, &trace_id);
    if (!d) {
      return true;
    }
    is_entry = (r == 0);
  }

  if (d->decision != COORD_DECISION_EMIT) {
    return true;
  }

  // Per-probe cap. Index via a switch so the verifier sees constant offsets
  // into the map value; a computed index defeats its bounds tracking.
  if (probe_id < COORD_MAX_PROBES) {
    uint32_t word = (probe_id >> 6) & (COORD_BITSET_WORDS - 1);
    uint64_t mask = 1ULL << (probe_id & 63);
    uint64_t prev = 0;
    switch (word) {
    case 0: prev = __sync_fetch_and_or(&d->emitted[0], mask); break;
    case 1: prev = __sync_fetch_and_or(&d->emitted[1], mask); break;
    case 2: prev = __sync_fetch_and_or(&d->emitted[2], mask); break;
    case 3: prev = __sync_fetch_and_or(&d->emitted[3], mask); break;
    default: break;
    }
    if (prev & mask) {
      return true; // this probe already emitted in this trace
    }
  }

  // The entry event already consumed its unit via should_throttle().
  if (!is_entry) {
    session_throttler_consume();
  }
  return false;
}

#define COORD_PREGATE_MAX_DEPTH 16

// Read DWARF integer register regnum (0-15, the Go register-ABI arg registers).
// regs must point to a copy of pt_regs, not the raw uprobe context: the
// verifier rejects dereferencing the context pointer at a computed offset. The
// switch keeps each case a constant-offset load (DWARF_REGISTER needs a
// constant index).
static inline __attribute__((always_inline)) uint64_t
coord_read_arg_reg(const struct pt_regs* regs, uint8_t regnum) {
  uint64_t v = 0;
  switch (regnum) {
  case 0: v = regs->DWARF_REGISTER(0); break;
  case 1: v = regs->DWARF_REGISTER(1); break;
  case 2: v = regs->DWARF_REGISTER(2); break;
  case 3: v = regs->DWARF_REGISTER(3); break;
  case 4: v = regs->DWARF_REGISTER(4); break;
  case 5: v = regs->DWARF_REGISTER(5); break;
  case 6: v = regs->DWARF_REGISTER(6); break;
  case 7: v = regs->DWARF_REGISTER(7); break;
  case 8: v = regs->DWARF_REGISTER(8); break;
  case 9: v = regs->DWARF_REGISTER(9); break;
  case 10: v = regs->DWARF_REGISTER(10); break;
  case 11: v = regs->DWARF_REGISTER(11); break;
  case 12: v = regs->DWARF_REGISTER(12); break;
  case 13: v = regs->DWARF_REGISTER(13); break;
  case 14: v = regs->DWARF_REGISTER(14); break;
  case 15: v = regs->DWARF_REGISTER(15); break;
  default: break;
  }
  return v;
}

// Copy the pt_regs fields the trace_id path reads (DWARF arg registers 0-15
// plus the frame and stack pointers) out of the raw uprobe context into the
// per-CPU scratch.
//
// This must run in the caller, on the real context pointer, and copy one field
// at a time. Copying the whole struct instead (s->regs = *regs) is rejected by
// the verifier: arm64 pt_regs is 336 bytes, wider than the context window the
// verifier allows, so the tail of the copy fails with "invalid bpf_context
// access off=335". Constant-offset field reads stay in range, and handing the
// scratch copy onward keeps the context pointer out of the noinline
// subprograms entirely.
static inline __attribute__((always_inline)) void
coord_copy_regs(const struct pt_regs* regs) {
  uint32_t zero = 0;
  coord_scratch_t* s =
      (coord_scratch_t*)bpf_map_lookup_elem(&coord_scratch_buf, &zero);
  if (!s || !regs) {
    return;
  }
  __builtin_memset(&s->regs, 0, sizeof(s->regs));
#define COORD_COPY_REG(n) s->regs.DWARF_REGISTER(n) = regs->DWARF_REGISTER(n)
  COORD_COPY_REG(0);
  COORD_COPY_REG(1);
  COORD_COPY_REG(2);
  COORD_COPY_REG(3);
  COORD_COPY_REG(4);
  COORD_COPY_REG(5);
  COORD_COPY_REG(6);
  COORD_COPY_REG(7);
  COORD_COPY_REG(8);
  COORD_COPY_REG(9);
  COORD_COPY_REG(10);
  COORD_COPY_REG(11);
  COORD_COPY_REG(12);
  COORD_COPY_REG(13);
  COORD_COPY_REG(14);
  COORD_COPY_REG(15);
#undef COORD_COPY_REG
  s->regs.DWARF_BP_REG = regs->DWARF_BP_REG;
  s->regs.DWARF_SP_REG = regs->DWARF_SP_REG;
}

// Walk the in-scope context.Context (located via params->ctx_loc_*) to the
// active dd-trace span and publish its trace_id into coord scratch
// (s->trace_id / s->present). Runs before the gate, reusing the capture-time
// span-extraction / interface-resolution helpers from stack_machine.h. Only
// included by event.c, after walk_stack.h has pulled in stack_machine.h.
//
// Reads the registers from scratch, which coord_copy_regs must have populated
// first; the raw context pointer deliberately does not cross into here.
static __attribute__((noinline)) int
coord_extract_trace_id(const probe_params_t* params) {
  uint32_t zero = 0;
  coord_scratch_t* s =
      (coord_scratch_t*)bpf_map_lookup_elem(&coord_scratch_buf, &zero);
  if (!s) {
    return 0;
  }
  s->trace_id = 0;
  s->present = 0;
  if (!params) {
    return 0;
  }
  target_ptr_t impl_addr = 0;
  uint64_t itab = 0;
  if (params->ctx_loc_kind != 1 && params->ctx_loc_kind != 2) {
    return 0; // no ctx in scope
  }
  if (params->ctx_loc_kind == 1 /* registers */) {
    itab = coord_read_arg_reg(&s->regs, params->ctx_reg_tab);
    impl_addr = coord_read_arg_reg(&s->regs, params->ctx_reg_data);
  } else /* stack */ {
    uint64_t cfa = calculate_cfa(&s->regs, params->frameless);
    uint64_t words[2] = {};
    if (bpf_probe_read_user(words, sizeof(words),
                            (void*)(cfa + (int64_t)params->ctx_stack_offset))) {
      return 0;
    }
    itab = words[0];
    impl_addr = words[1];
  }
  if (itab == 0 || impl_addr == 0) {
    return 0;
  }
  target_ptr_t type_addr = 0;
  if (bpf_probe_read_user(&type_addr, sizeof(type_addr),
                          (void*)(itab + OFFSET_runtime_dot_itab___type))) {
    return 0;
  }
  __builtin_memset(&s->trace, 0, sizeof(s->trace));
  s->cur.addr = impl_addr;
  s->cur.go_runtime_type = go_runtime_type_from_ptr(type_addr);
  for (int i = 0; i < COORD_PREGATE_MAX_DEPTH; i++) {
    if (s->cur.addr == 0 || s->cur.go_runtime_type == 0 ||
        s->cur.go_runtime_type == (uint64_t)(-1)) {
      break;
    }
    type_t ir_type = lookup_go_interface(s->cur.go_runtime_type);
    if (ir_type == 0) {
      break;
    }
    const type_info_t* info = NULL;
    if (!get_type_info(ir_type, &info) || info->go_context_is_context == 0) {
      break;
    }
    if (sm_maybe_extract_ddtrace_span_from_value_ctx(s->cur.addr, info,
                                                     &s->trace)) {
      break;
    }
    if (info->go_context_context_offset < 0) {
      break; // no parent context to walk
    }
    if (!sm_resolve_go_interface_at(
            s->cur.addr + info->go_context_context_offset, &s->cur)) {
      break;
    }
  }
  if (!s->trace.valid) {
    return 0;
  }
  s->trace_id = s->trace.trace_id_lower;
  s->present = 1;
  return 0;
}

// Single sampling-decision seam for both throttle gates. Uses the per-trace
// decision when coord_extract_trace_id found a trace, else the per-probe
// throttler. Returns true to drop.
static inline __attribute__((always_inline)) bool
should_drop_event(const probe_params_t* params, uint64_t start_ns) {
  uint32_t zero = 0;
  coord_scratch_t* s =
      (coord_scratch_t*)bpf_map_lookup_elem(&coord_scratch_buf, &zero);
  if (s && s->present) {
    return coordinated_should_drop(params, start_ns, s->trace_id);
  }
  return should_throttle(params->throttler_idx, start_ns);
}

#endif // __COORDINATED_SAMPLE_H__
