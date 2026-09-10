#ifndef _HELPERS_SPAN_CTX_EVENT_H_
#define _HELPERS_SPAN_CTX_EVENT_H_

#include "maps.h"

// monitor_span_ctx_event counts one per-event span context fill failure into
// span_ctx_stats.
static void __attribute__((always_inline)) monitor_span_ctx_event(u32 reader, u32 status) {
    if (status < SPAN_CTX_EVENT_FIRST_ERROR) {
        return;
    }
    u32 key = reader * SPAN_CTX_EVENT_STATUS_LAST + status;
    struct span_ctx_event_stats_t *stats = bpf_map_lookup_elem(&span_ctx_stats, &key);
    if (!stats) {
        return;
    }
    __sync_fetch_and_add(&stats->count, 1);
}

#endif
