#ifndef _HELPERS_SPAN_H_
#define _HELPERS_SPAN_H_

#include "maps.h"

#include "process.h"

#include "constants/macros.h"

// Load-time gate for the legacy Datadog proprietary TLS APM-correlation path.
// The manager patches this constant at probe load based on
// runtime_security_config.apm_correlation.legacy_enabled (default: false).
// When the constant is 0, the verifier sees the gated block as dead code.
static __attribute__((always_inline)) int legacy_apm_correlation_enabled(void) {
    u64 enabled;
    LOAD_CONSTANT("legacy_apm_correlation_enabled", enabled);
    return enabled != 0;
}

// --- Datadog proprietary span TLS (existing mechanism) ---

int __attribute__((always_inline)) handle_register_span_memory(void *data) {
    struct span_tls_t tls = {};
    bpf_probe_read(&tls, sizeof(tls), data);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    bpf_map_update_elem(&span_tls, &tgid, &tls, BPF_NOEXIST);

    return 0;
}

int __attribute__((always_inline)) unregister_span_memory() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    bpf_map_delete_elem(&span_tls, &tgid);

    return 0;
}

// --- OTel Thread Local Context Record helpers (separate file) ---
#include "span_otel.h"

// --- Go pprof labels helpers (separate file) ---
#include "span_go.h"

// --- Legacy Datadog proprietary TLS lookup ---
//
// Marked __noinline__ so it gets its own 512-byte BPF stack frame and does not
// contribute to the caller's stack budget. This mirrors the technique used for
// fill_span_context_go in span_go.h (see comment there), and is the only way
// to keep module_load → trace_init_module_ret under the 512-byte verifier
// limit once init_module_event_t (event + name + args buffers) is on the
// caller's stack.
//
// All locals in this body — the pointer returned by bpf_map_lookup_elem, the
// resolved namespace PID, and the inlined helper's scratch — live in this
// subprog's own frame and disappear after the call returns.
//
// Returns 1 when the proprietary TLS lookup produced a non-zero span context
// (caller should stop and return), 0 otherwise.

// Wire-protocol slot size for the legacy proprietary TLS array, fixed at
// 24 bytes: span_id (u64) + trace_id (__int128). This is independent of
// sizeof(struct span_context_t), which grew to 32 bytes when has_extra_attrs
// + _pad[7] were appended for the OTel path. User-space registrants
// (dd-trace, syscall_tester) still write 24-byte slots, so the eBPF reader
// must use 24 for both the per-thread offset and the read length.
#define LEGACY_SPAN_TLS_SLOT_SIZE 24

int __attribute__((__noinline__)) fill_span_context_legacy(struct span_context_t *span) {
    if (!span) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct span_tls_t *tls = bpf_map_lookup_elem(&span_tls, &tgid);
    if (!tls) {
        return 0;
    }

    u32 tid = get_namespace_nr_from_task_struct((struct task_struct *)bpf_get_current_task());
    if (!tid) {
        tid = (u32)pid_tgid;
    }

    if (bpf_probe_read_user(span, LEGACY_SPAN_TLS_SLOT_SIZE,
                            tls->base + (tid % tls->max_threads) * LEGACY_SPAN_TLS_SLOT_SIZE) < 0) {
        return 0;
    }
    return span->span_id != 0 || span->trace_id[0] != 0 || span->trace_id[1] != 0;
}

// --- Unified span context fill ---

void __attribute__((always_inline)) fill_span_context(struct span_context_t *span) {
    // Legacy Datadog proprietary TLS — opt-in via load-time constant. The
    // helper is __noinline__ so its locals never contribute to this caller's
    // stack frame, regardless of whether the gate is on or off.
    if (legacy_apm_correlation_enabled() && fill_span_context_legacy(span)) {
        return;
    }

    // Fall back to OTel Thread Local Context Record (native applications only).
    if (fill_span_context_otel(span)) {
        return;
    }

    // Fall back to Go pprof labels (dd-trace-go sets "span id" / "local root span id").
    if (fill_span_context_go(span)) {
        return;
    }

    // No span context available.
    span->span_id = 0;
    span->trace_id[0] = span->trace_id[1] = 0;
    span->has_extra_attrs = 0;
}

void __attribute__((always_inline)) reset_span_context(struct span_context_t *span) {
    span->span_id = 0;
    span->trace_id[0] = 0;
    span->trace_id[1] = 0;
    span->has_extra_attrs = 0;
}

void __attribute__((always_inline)) copy_span_context(struct span_context_t *src, struct span_context_t *dst) {
    dst->span_id = src->span_id;
    dst->trace_id[0] = src->trace_id[0];
    dst->trace_id[1] = src->trace_id[1];
}

#endif
