#ifndef _HOOKS_SPAN_FILL_H_
#define _HOOKS_SPAN_FILL_H_

#include "perf_ring.h"
#include "helpers/span.h"
#include "helpers/span_fill.h"

// Combine calling fill_span_context and send_event. Used as a tail-call.
static int __attribute__((always_inline)) fill_span_and_send_impl(void *ctx) {
    u32 zero = 0;
    struct span_fill_slot_t *slot = bpf_map_lookup_elem(&span_fill_event, &zero);
    if (!slot) {
        return 0;
    }

    // Fill on the stack, so fill_span_context() -- and the span-context readers
    // it inlines -- only ever touch fixed offsets.
    struct span_context_t span = {0};
    fill_span_context(&span);

    // Then copy it into the payload at the offset the caller recorded. The offset
    // is read and bounded here, after the call, so that the bound stays adjacent
    // to the map_value arithmetic it guards (this is done to please the verifier)
    u32 span_off = slot->span_off;
    if (span_off > sizeof(slot->data) - sizeof(span)) {
        return 0;
    }
    *(struct span_context_t *)((char *)&slot->data + span_off) = span;

    u64 size = slot->size;

    // The size is always set by SPAN_FILL_EVENT so the following is only here to
    // please the verifier.
    if (size > sizeof(slot->data)) {
        size = sizeof(slot->data);
    } else if (size < sizeof(struct kevent_t)) {
        size = sizeof(struct kevent_t);
    }

    send_event_with_size_ptr(ctx, slot->event_type, &slot->data, size);
    return 0;
}

TAIL_CALL_FNC(fill_span_and_send, void *ctx) {
    return fill_span_and_send_impl(ctx);
}

TAIL_CALL_TRACEPOINT_FNC(fill_span_and_send, void *ctx) {
    return fill_span_and_send_impl(ctx);
}

// setsockopt has its own tail-call target because it uses a dedicated setsockopt_event map.
static int __attribute__((always_inline)) fill_span_and_send_setsockopt_impl(void *ctx) {
    u32 zero = 0;
    struct setsockopt_event_t *event = bpf_map_lookup_elem(&setsockopt_event, &zero);
    if (!event) {
        return 0;
    }

    fill_span_context(&event->span);

    u64 sent_size = event->sent_size;
    if (sent_size > MAX_BPF_FILTER_SIZE) {
        sent_size = MAX_BPF_FILTER_SIZE;
    }
    send_event_with_size_ptr(ctx, EVENT_SETSOCKOPT, event,
                             offsetof(struct setsockopt_event_t, bpf_filters_buffer) + sent_size);
    return 0;
}

TAIL_CALL_FNC(fill_span_and_send_setsockopt, void *ctx) {
    return fill_span_and_send_setsockopt_impl(ctx);
}

TAIL_CALL_TRACEPOINT_FNC(fill_span_and_send_setsockopt, void *ctx) {
    return fill_span_and_send_setsockopt_impl(ctx);
}

#endif
