#ifndef __COORDINATED_SAMPLE_H__
#define __COORDINATED_SAMPLE_H__

#include "dwarf_regs.h"
#include "throttler.h"
#include "program.h"
#include "types.h"

// Coordinated sampling: make one keep/drop decision per trace and apply it to
// every probe in that trace. See pkg/dyninst/docs/coordinated-sampling.md.
//
// Working state lives in a per-CPU map instead of on the stack, which the
// verifier limits to 512 bytes across a whole call chain.

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

// Everything the sampling path needs for one probe hit. It is filled in at the
// top of the uprobe and read again at the keep/drop decision.
typedef struct coord_scratch {
  // The probe's registers, copied out of the uprobe context.
  struct pt_regs regs;
  // The span the walk found, if it found one.
  trace_context_t trace;
  // The link the context walk is currently on.
  resolved_go_interface_t cur;
  // Trace this hit belongs to. Only meaningful when present is 1.
  uint64_t trace_id;
  // 1 if trace_id was found, 0 if there was no context in scope or no span in
  // it. 0 means fall back to the probe's own throttler.
  uint8_t present;
  uint8_t __pad[7];
  // The record to insert when this hit is the first one of its trace. Built
  // here because it does not fit on the stack.
  trace_decision_t new_decision;
} coord_scratch_t;

struct {
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
  __uint(max_entries, 1);
  __type(key, uint32_t);
  __type(value, coord_scratch_t);
} coord_scratch_buf SEC(".maps");

// Take one unit from the session budget without checking it first. Used by a
// probe that follows a keep decision made earlier in the same trace: it always
// emits, so the budget can go negative and hold back later traces until the
// period resets.
static inline __attribute__((always_inline)) void
session_throttler_consume(void) {
  uint32_t idx = session_throttler_idx;
  throttler_t* t = (throttler_t*)bpf_map_lookup_elem(&throttler_buf, &idx);
  if (t) {
    __sync_fetch_and_sub(&t->budget, 1);
  }
}

// Decide whether to drop this hit, given the trace it belongs to.
//
// The first hit of a trace asks the session throttler and stores the answer;
// every later hit of the same trace reuses that answer, so a trace is kept
// whole or not at all. Within a kept trace each probe only gets to emit once,
// which is what the emitted bitset below is for: it stops a probe inside a loop
// from flooding us.
//
// Returns true to drop. Kept noinline so its frame is not charged to the
// uprobe's, which is already close to the verifier's stack limit.
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
    __builtin_memset(&s->new_decision, 0, sizeof(s->new_decision));
    s->new_decision.decision = should_throttle(session_throttler_idx, start_ns)
                                   ? COORD_DECISION_DROP
                                   : COORD_DECISION_EMIT;
    long r = bpf_map_update_elem(&decision_by_trace, &trace_id,
                                 &s->new_decision, BPF_NOEXIST);
    d = (trace_decision_t*)bpf_map_lookup_elem(&decision_by_trace, &trace_id);
    if (!d) {
      return true;
    }
    is_entry = (r == 0);
  }

  if (d->decision != COORD_DECISION_EMIT) {
    return true;
  }

  // One bit per probe. Indexed through a switch because the verifier only
  // tracks bounds for constant offsets into a map value. probe_id is below
  // COORD_MAX_PROBES here, so word is below COORD_BITSET_WORDS.
  if (probe_id < COORD_MAX_PROBES) {
    uint32_t word = probe_id >> 6;
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

// How many links of the context chain this walk follows before giving up. It is
// shorter than MAX_GO_CONTEXT_DEPTH, used by the walk that runs during capture,
// because this one runs on every probe hit, including the hits we are about to
// drop. A span buried deeper than this reads as "no trace", and the probe falls
// back to its own throttler.
#define COORD_MAX_CONTEXT_DEPTH 16

// Read argument register regnum out of a copy of pt_regs. Same shape as
// SM_OP_EXPR_READ_REGISTER in stack_machine.h; both spell out every register
// because the verifier needs each load to sit at a constant offset.
static inline __attribute__((always_inline)) uint64_t
coord_read_arg_reg(const struct pt_regs* regs, uint8_t regnum) {
  uint64_t v = 0;
#define COORD_READ_ARG_REG(n)                                                  \
  case n: v = regs->DWARF_REGISTER(n); break;
  switch (regnum) {
    DWARF_ARG_REGS(COORD_READ_ARG_REG)
  default: break;
  }
#undef COORD_READ_ARG_REG
  return v;
}

// Copy the registers the walk reads into scratch, one field at a time. This has
// to happen here, in the uprobe itself: the verifier rejects a copy of the
// whole struct (it reaches past the end of the context it lets us read), and
// copying here also keeps the context pointer out of the helpers below.
static inline __attribute__((always_inline)) void
coord_copy_regs(const struct pt_regs* regs) {
  uint32_t zero = 0;
  coord_scratch_t* s =
      (coord_scratch_t*)bpf_map_lookup_elem(&coord_scratch_buf, &zero);
  if (!s || !regs) {
    return;
  }
  __builtin_memset(&s->regs, 0, sizeof(s->regs));
#define COORD_COPY_REG(n) s->regs.DWARF_REGISTER(n) = regs->DWARF_REGISTER(n);
  DWARF_ARG_REGS(COORD_COPY_REG)
#undef COORD_COPY_REG
  s->regs.DWARF_BP_REG = regs->DWARF_BP_REG;
  s->regs.DWARF_SP_REG = regs->DWARF_SP_REG;
}

// Publish "no trace in scope" so the probe falls back to the per-probe throttler.
static inline __attribute__((always_inline)) void coord_clear_trace_id(void) {
  uint32_t zero = 0;
  coord_scratch_t* s =
      (coord_scratch_t*)bpf_map_lookup_elem(&coord_scratch_buf, &zero);
  if (s) {
    s->present = 0;
    s->trace_id = 0;
  }
}

// Walk the context.Context that is in scope at this probe (params->ctx_loc_*
// says where it lives) until we reach the dd-trace span, and put its trace_id
// in scratch. Reads the registers from scratch, so coord_copy_regs has to run
// first.
//
// The walk mirrors sm_go_context_chain_hop, which does the same hops during
// capture. They stay separate because that one is driven one hop per stack
// machine instruction and writes the span into the event buffer, while this is
// a plain loop that runs before we even know whether we will capture anything.
//
// Kept noinline so its frame is not charged to the uprobe's, which is already
// close to the verifier's stack limit.
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
  if (params->ctx_loc_kind != CTX_LOC_REGISTERS &&
      params->ctx_loc_kind != CTX_LOC_STACK) {
    return 0;
  }
  if (params->ctx_loc_kind == CTX_LOC_REGISTERS) {
    itab = coord_read_arg_reg(&s->regs, params->ctx_reg_tab);
    impl_addr = coord_read_arg_reg(&s->regs, params->ctx_reg_data);
  } else {
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
  for (int i = 0; i < COORD_MAX_CONTEXT_DEPTH; i++) {
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

// Both throttle points call this to decide whether to drop an event. It uses
// the per-trace decision when the walk found a trace, and the probe's own
// throttler when it did not. Returns true to drop.
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
