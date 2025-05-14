#ifndef _PERF_RING_H_
#define _PERF_RING_H_

#include "map-defs.h"

#include "structs/all.h"
#include "constants/custom.h"

struct perf_map_stats_t {
    u64 bytes;
    u64 count;
    u64 lost;
    u64 discarded;
};

struct ring_buffer_stats_t {
    u64 usage;
};

#if USE_RING_BUFFER == 1
void __attribute__((always_inline)) store_ring_buffer_stats() {
    // check needed for code elimination
    u64 use_ring_buffer;
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);
    if (use_ring_buffer) {
        int zero = 0;
        struct ring_buffer_stats_t *stats = bpf_map_lookup_elem(&events_ringbuf_stats, &zero);
        if (stats) {
            stats->usage = bpf_ringbuf_query(&events, 0);
        }
    }
}
#endif

#define IS_CRITICAL_EVENT_TYPE(event_type) (\
    event_type == EVENT_EXEC || \
    event_type == EVENT_EXIT || \
    event_type == EVENT_FORK || \
    event_type == EVENT_ARGS_ENVS || \
    event_type == EVENT_CGROUP_TRACING || \
    event_type == EVENT_VETH_PAIR || \
    event_type == EVENT_NET_DEVICE || \
    event_type == EVENT_UNSHARE_MNTNS || \
    event_type == EVENT_CGROUP_WRITE || \
    event_type == EVENT_MOUNT_RELEASED || \
    event_type == EVENT_MOUNT || \
    event_type == EVENT_UMOUNT)

int __attribute__((always_inline)) check_ring_buffer_size(u64 event_type) {
    u64 ring_buffer_threshold = 0;
    LOAD_CONSTANT("ring_buffer_threshold", ring_buffer_threshold);  

    u64 usage = bpf_ringbuf_query(&events, BPF_RB_AVAIL_DATA);
    return usage <= ring_buffer_threshold || IS_CRITICAL_EVENT_TYPE(event_type);
}

void __attribute__((always_inline)) send_event_with_size_ptr(void *ctx, u64 event_type, void *kernel_event, u64 kernel_event_size) {
    struct kevent_t *header = kernel_event;
    header->type = event_type;
    header->timestamp = bpf_ktime_get_ns();

    u64 cpu = bpf_get_smp_processor_id();

    int perf_ret = 0;

#if USE_RING_BUFFER == 1
    u64 use_ring_buffer;
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);
    if (use_ring_buffer) {
        if (!check_ring_buffer_size(event_type)) {
            struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &event_type);
            if (!stats) {
                return;
            }
            __sync_fetch_and_add(&stats->discarded, 1);
            return;
        }

        perf_ret = bpf_ringbuf_output(&events, kernel_event, kernel_event_size, 0);
    } else {
        perf_ret = bpf_perf_event_output(ctx, &events, cpu, kernel_event, kernel_event_size);
    }
#else
    perf_ret = bpf_perf_event_output(ctx, &events, cpu, kernel_event, kernel_event_size);
#endif

    struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &event_type);
    if (!stats) {
        return;
    }

    if (!perf_ret) {
        __sync_fetch_and_add(&stats->bytes, kernel_event_size + 4);
        __sync_fetch_and_add(&stats->count, 1);
    } else {
        __sync_fetch_and_add(&stats->lost, 1);
    }
}

#define send_event(ctx, event_type, kernel_event) ({                \
    u64 size = sizeof(kernel_event);                                \
    send_event_with_size_ptr(ctx, event_type, &kernel_event, size); \
})

#define send_event_ptr(ctx, event_type, kernel_event) ({           \
    u64 size = sizeof(*kernel_event);                              \
    send_event_with_size_ptr(ctx, event_type, kernel_event, size); \
})

#endif
