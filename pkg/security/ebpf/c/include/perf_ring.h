#ifndef _PERF_RING_H_
#define _PERF_RING_H_

#include "map-defs.h"

#include "structs/all.h"
#include "constants/custom.h"

struct perf_map_stats_t {
    u64 bytes;
    u64 count;
    u64 lost;
};

struct ring_buffer_stats_t {
    u64 usage;
};

BPF_PERF_EVENT_ARRAY_MAP(events, u32)
BPF_HASH_MAP(events_stats, u32, struct perf_map_stats_t, EVENT_MAX)

#if USE_RING_BUFFER == 1
BPF_ARRAY_MAP(events_ringbuf_stats, u64, 1)

void __attribute__((always_inline)) store_ring_buffer_stats() {
    // check needed for code elimination
    u64 use_ring_buffer;
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);
    if (use_ring_buffer) {
        int zero = 0;
        struct ring_buffer_stats_t *stats = bpf_map_lookup_elem(&events_ringbuf_stats, &zero);
        if (stats)
            stats->usage = bpf_ringbuf_query(&events, 0);
    }
}
#endif

void __attribute__((always_inline)) send_event_with_size_ptr(void *ctx, u64 event_type, void *kernel_event, u64 kernel_event_size) {
    struct kevent_t *header = kernel_event;
    header->type = event_type;
    header->cpu = bpf_get_smp_processor_id();
    header->timestamp = bpf_ktime_get_ns();

#if USE_RING_BUFFER == 1
    u64 use_ring_buffer;
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);
    int perf_ret;
    if (use_ring_buffer) {
        perf_ret = bpf_ringbuf_output(&events, kernel_event, kernel_event_size, 0);
    } else {
        perf_ret = bpf_perf_event_output(ctx, &events, header->cpu, kernel_event, kernel_event_size);
    }
#else
    int perf_ret = bpf_perf_event_output(ctx, &events, header->cpu, kernel_event, kernel_event_size);
#endif

    if (event_type < EVENT_MAX) {
        struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &event_type);
        if (stats != NULL) {
            if (!perf_ret) {
                __sync_fetch_and_add(&stats->bytes, kernel_event_size + 4);
                __sync_fetch_and_add(&stats->count, 1);
            } else {
                __sync_fetch_and_add(&stats->lost, 1);
            }
        }
    }
}

#define send_event(ctx, event_type, kernel_event) ({                  \
    u64 size = sizeof(kernel_event);                                  \
    send_event_with_size_ptr(ctx, event_type, &kernel_event, size); \
})

#define send_event_ptr(ctx, event_type, kernel_event) ({              \
    u64 size = sizeof(*kernel_event);                                 \
    send_event_with_size_ptr(ctx, event_type, kernel_event, size); \
})

#endif
