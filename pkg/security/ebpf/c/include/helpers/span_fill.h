#ifndef _HELPERS_SPAN_FILL_H_
#define _HELPERS_SPAN_FILL_H_

#include "maps.h"

// --- Deferred span-context fill + send (tail-called) ---

// span_fill_prepare stashes the slot header for the deferred fill+send tail call.
static void *__attribute__((always_inline)) span_fill_prepare(u64 event_type, u32 size, u32 span_off) {
    u32 zero = 0;
    struct span_fill_slot_t *slot = bpf_map_lookup_elem(&span_fill_event, &zero);
    if (!slot) {
        return NULL;
    }
    slot->event_type = event_type;
    slot->size = size;
    slot->span_off = span_off;
    return &slot->data;
}

// SPAN_FILL_EVENT returns a zeroed, correctly-typed pointer for the new event to be filled by the caller.
#define SPAN_FILL_EVENT(type, event_type) ({                                                     \
    type *__evt = (type *)span_fill_prepare((event_type), sizeof(type), offsetof(type, span));    \
    if (__evt) {                                                                                 \
        __builtin_memset(__evt, 0, sizeof(type));                                                \
    }                                                                                            \
    __evt;                                                                                       \
})

// Indices into span_fill_progs / span_fill_tp_progs (see maps.h).
enum SPAN_FILL_KEY {
    SPAN_FILL_KEY_GENERIC = 0,    // generic program over the shared span_fill_event slot
    SPAN_FILL_KEY_SETSOCKOPT = 1, // setsockopt-specific program over setsockopt_event
};

// span_fill_tail_call_key dispatches to the span-fill target program at `key`,
// matching the caller's BPF program type (bpf_tail_call requires the target type
// to match).
static void __attribute__((always_inline)) span_fill_tail_call_key(void *ctx, enum TAIL_CALL_PROG_TYPE prog_type, u32 key) {
    switch (prog_type) {
    case KPROBE_OR_FENTRY_TYPE:
        bpf_tail_call_compat(ctx, &span_fill_progs, key);
        break;
    case TRACEPOINT_TYPE:
        bpf_tail_call_compat(ctx, &span_fill_tp_progs, key);
        break;
    }
}

// span_fill_tail_call is the common case: the generic program over the shared slot.
static void __attribute__((always_inline)) span_fill_tail_call(void *ctx, enum TAIL_CALL_PROG_TYPE prog_type) {
    span_fill_tail_call_key(ctx, prog_type, SPAN_FILL_KEY_GENERIC);
}

#endif
