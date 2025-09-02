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

void __attribute__((always_inline)) fill_span_context(struct span_context_t *span) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct span_tls_t *tls = bpf_map_lookup_elem(&span_tls, &tgid);
    if (tls) {
        u32 tid = pid_tgid;

        struct task_struct *current_ptr = (struct task_struct *)bpf_get_current_task();
        u32 pid = get_namespace_nr_from_task_struct(current_ptr);
        if (pid) {
            tid = pid;
        }

        int offset = (tid % tls->max_threads) * sizeof(struct span_context_t);
        int ret = bpf_probe_read_user(span, sizeof(struct span_context_t), tls->base + offset);
        if (ret < 0) {
            span->span_id = 0;
            span->trace_id[0] = span->trace_id[1] = 0;
        }
    }
}

void __attribute__((always_inline)) reset_span_context(struct span_context_t *span) {
    span->span_id = 0;
    span->trace_id[0] = 0;
    span->trace_id[1] = 0;
}

void __attribute__((always_inline)) copy_span_context(struct span_context_t *src, struct span_context_t *dst) {
    dst->span_id = src->span_id;
    dst->trace_id[0] = src->trace_id[0];
    dst->trace_id[1] = src->trace_id[1];
}

#endif
