#ifndef _HELPERS_SPAN_H_
#define _HELPERS_SPAN_H_

#include "maps.h"

#include "process.h"

#include "constants/macros.h"

// --- OTel thread local context record helpers (separate file) ---
#include "span_otel.h"

// --- Go pprof labels helpers (separate file) ---
#include "span_go.h"


// --- Unified span context fill ---
//
// fill_span_context is the single entry point every hook calls to attach a span
// context to an event. is_span_tracking_enabled() is backed by a load-time
// constant, so the verifier folds the branch below and drops both readers
// entirely when the feature is off.
void __attribute__((always_inline)) fill_span_context(struct span_context_t *span, struct go_labels_context_t *go_labels) {
    span->span_id = 0;
    span->trace_id[0] = span->trace_id[1] = 0;
    span->extra_attrs_id = 0;
    if (go_labels) {
        go_labels->id = 0;
    }

    if (!is_span_tracking_enabled()) {
        return;
    }

    if (fill_span_context_otel(span)) {
        return;
    }

    if (go_labels) {
        go_labels->id = collect_go_labels();
    }
}

void __attribute__((always_inline)) reset_span_context(struct span_context_t *span, struct go_labels_context_t *go_labels) {
    span->span_id = 0;
    span->trace_id[0] = 0;
    span->trace_id[1] = 0;
    span->extra_attrs_id = 0;
    go_labels->id = 0;
}

void __attribute__((always_inline)) unregister_span_context() {
    unregister_otel_tls();
    unregister_go_labels();
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
