#ifndef _HELPERS_SPAN_H_
#define _HELPERS_SPAN_H_

#include "maps.h"

#include "process.h"

#include "constants/macros.h"

// --- Go pprof labels helpers (separate file) ---
#include "span_go.h"


// --- Unified span context fill ---
//
// fill_span_context is the single entry point every hook calls to attach a span
// context to an event. It leaves span_context_t empty (reserved for another
// APM-correlation reader, e.g. OTEP 4947) and snapshots the current goroutine's
// Go pprof labels into the go_labels_ctx ring via collect_go_labels(). Only the
// resulting id is stored (in go_labels); user space resolves it.
void __attribute__((always_inline)) fill_span_context(struct span_context_t *span, struct go_labels_context_t *go_labels) {
    // No span context available yet from this path.
    span->span_id = 0;
    span->trace_id[0] = span->trace_id[1] = 0;
    span->extra_attrs_id = 0;

    if (go_labels) {
        go_labels->id = collect_go_labels();
    }
}

void __attribute__((always_inline)) reset_span_context(struct span_context_t *span) {
    span->span_id = 0;
    span->trace_id[0] = 0;
    span->trace_id[1] = 0;
    span->extra_attrs_id = 0;
}

void __attribute__((always_inline)) copy_span_context(
    struct span_context_t *src, struct span_context_t *dst,
    struct go_labels_context_t *go_labels_src, struct go_labels_context_t *go_labels_dst)
{
    dst->span_id = src->span_id;
    dst->trace_id[0] = src->trace_id[0];
    dst->trace_id[1] = src->trace_id[1];
    // extra_attrs_id must be copied too: for exec events, fill_span_context
    // runs against syscall->exec.span_context at prepare_binprm, and the
    // event-side span_context only gets populated via this helper at
    // send_exec_event.
    dst->extra_attrs_id = src->extra_attrs_id;

    go_labels_dst->id = go_labels_src->id;
}

#endif
