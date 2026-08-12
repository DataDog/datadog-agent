#ifndef _HELPERS_SPAN_H_
#define _HELPERS_SPAN_H_

#include "maps.h"

#include "process.h"

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

// --- Legacy Datadog proprietary TLS lookup ---
//
// Wire-protocol slot size for the legacy proprietary TLS array, fixed at
// 24 bytes: span_id (u64) + trace_id (__int128). This is independent of
// sizeof(struct span_context_t), which grew to 32 bytes when extra_attrs_id
// was appended for the OTel path. User-space registrants
// (dd-trace, syscall_tester) still write 24-byte slots, so the eBPF reader
// must use 24 for both the per-thread offset and the read length.
#define LEGACY_SPAN_TLS_SLOT_SIZE 24

int __attribute__((always_inline)) fill_span_context_legacy(struct span_context_t *span) {
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
    span->extra_attrs_id = 0;

    // Legacy Datadog proprietary TLS
    if (fill_span_context_legacy(span)) {
        return;
    }

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
