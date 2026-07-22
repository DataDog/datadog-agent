#ifndef _HELPERS_SPAN_H_
#define _HELPERS_SPAN_H_

#include "maps.h"

#include "process.h"

// --- Unified span context fill ---
//
// fill_span_context is the single entry point every hook calls to attach a
// span context to an event. It currently leaves the span empty; upcoming
// APM-correlation readers will populate it here.
void __attribute__((always_inline)) fill_span_context(struct span_context_t *span) {
    // No span context available.
    span->span_id = 0;
    span->trace_id[0] = span->trace_id[1] = 0;
    span->extra_attrs_id = 0;
}

void __attribute__((always_inline)) reset_span_context(struct span_context_t *span) {
    span->span_id = 0;
    span->trace_id[0] = 0;
    span->trace_id[1] = 0;
    span->extra_attrs_id = 0;
}

void __attribute__((always_inline)) copy_span_context(struct span_context_t *src, struct span_context_t *dst) {
    dst->span_id = src->span_id;
    dst->trace_id[0] = src->trace_id[0];
    dst->trace_id[1] = src->trace_id[1];
    // extra_attrs_id must be copied too: for exec events, fill_span_context
    // runs against syscall->exec.span_context at prepare_binprm, and the
    // event-side span_context only gets populated via this helper at
    // send_exec_event.
    dst->extra_attrs_id = src->extra_attrs_id;
}

#endif
