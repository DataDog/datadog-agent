#ifndef _PROCESS_H_
#define _PROCESS_H_

#include <linux/sched.h>
#include "bpf_helpers.h"

#include "container.h"
#include "process-types.h"

struct bpf_map_def SEC("maps/events") events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 0,
    .pinning = 0,
    .namespace = "",
};

#define send_event_with_size_ptr(ctx, event_type, kernel_event, kernel_event_size)                                     \
    kernel_event->event.type = event_type;                                                                             \
    kernel_event->event.cpu = bpf_get_smp_processor_id();                                                              \
    kernel_event->event.timestamp = bpf_ktime_get_ns();                                                                \
                                                                                                                       \
    bpf_perf_event_output(ctx, &events, kernel_event->event.cpu, kernel_event, kernel_event_size);                     \

#define send_event_with_size(ctx, event_type, kernel_event, kernel_event_size)                                         \
    kernel_event.event.type = event_type;                                                                              \
    kernel_event.event.cpu = bpf_get_smp_processor_id();                                                               \
    kernel_event.event.timestamp = bpf_ktime_get_ns();                                                                 \
                                                                                                                       \
    bpf_perf_event_output(ctx, &events, kernel_event.event.cpu, &kernel_event, kernel_event_size);                     \

#define send_event(ctx, event_type, kernel_event)                                                                      \
    u64 size = sizeof(kernel_event);                                                                                   \
    send_event_with_size(ctx, event_type, kernel_event, size)                                                          \

#define send_event_ptr(ctx, event_type, kernel_event)                                                                  \
    u64 size = sizeof(*kernel_event);                                                                                  \
    send_event_with_size_ptr(ctx, event_type, kernel_event, size)                                                      \

void __attribute__((always_inline)) copy_proc_cache_except_comm(proc_cache_t* src, proc_cache_t* dst) {
    copy_container_id(src->container.container_id, dst->container.container_id);
    dst->exec_timestamp = src->exec_timestamp;
}

void __attribute__((always_inline)) copy_proc_cache(proc_cache_t *src, proc_cache_t *dst) {
    copy_proc_cache_except_comm(src, dst);
}

struct bpf_map_def SEC("maps/proc_cache") proc_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(proc_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

static void __attribute__((always_inline)) fill_container_context(proc_cache_t *entry, container_context_t *context) {
    if (entry) {
        copy_container_id(entry->container.container_id, context->container_id);
    }
}

void __attribute__((always_inline)) copy_pid_cache_except_exit_ts(pid_cache_t* src, pid_cache_t* dst) {
    dst->cookie = src->cookie;
    dst->ppid = src->ppid;
    dst->fork_timestamp = src->fork_timestamp;
}

struct bpf_map_def SEC("maps/pid_cache") pid_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(pid_cache_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

// defined in exec.h
proc_cache_t *get_proc_from_cookie(u32 cookie);

proc_cache_t * __attribute__((always_inline)) get_proc_cache(u32 tgid) {
    proc_cache_t *entry = NULL;

    pid_cache_t *pid_entry = (pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (pid_entry) {
        // Select the cache entry
        u32 cookie = pid_entry->cookie;
        entry = get_proc_from_cookie(cookie);
    }
    return entry;
}

static proc_cache_t * __attribute__((always_inline)) fill_process_context_with_pid_tgid(process_context_t *data, u64 pid_tgid) {
    u32 tgid = pid_tgid >> 32;

    // https://github.com/iovisor/bcc/blob/master/docs/reference_guide.md#4-bpf_get_current_pid_tgid
    data->pid = tgid;
    data->tid = pid_tgid;

    return get_proc_cache(tgid);
}

static proc_cache_t * __attribute__((always_inline)) fill_process_context(process_context_t *data) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    return fill_process_context_with_pid_tgid(data, pid_tgid);
}

#endif
