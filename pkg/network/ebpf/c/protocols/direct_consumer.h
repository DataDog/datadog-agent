#ifndef __USM_DIRECT_CONSUMER_H
#define __USM_DIRECT_CONSUMER_H

#include "bpf_telemetry.h"
#include "bpf_builtins.h"

// Stringify helper for building per-stream LOAD_CONSTANT names (e.g.
// "http2_ringbuffer_wakeup_size"). Mirrors events.h; guarded so it does not
// clash when both headers are included in the same translation unit.
#ifndef _STR
#define _STR(x) #x
#endif

/* USM_DIRECT_CONSUMER_INIT defines utility functions for DirectConsumer pattern.
   DirectConsumer is used for kernel >= 5.8 where events are sent directly via
   bpf_perf_event_output/bpf_ringbuf_output instead of map-based batching.

   This macro generates two protocol-specific functions:
   1) <name>_get_ringbuf_flags - determines wakeup flags for ring buffers
   2) <name>_output_event - outputs event to perf/ring buffer with telemetry

   Parameters:
   - name: protocol name prefix (e.g., "http", "kafka")
   - event_type: type of the event structure
   - map_name: name of the perf/ring buffer map
*/
#define USM_DIRECT_CONSUMER_INIT(name, event_type, map_name)                                                \
    static __always_inline __u64 name##_get_ringbuf_flags(size_t data_size) {                               \
        __u64 ringbuffer_wakeup_size = 0;                                                                   \
        /* Per-stream constant: protocols with two simultaneous streams (e.g. http2 +     \
           terminated_http2) need independent wakeup thresholds, so the name is namespaced \
           by `name` to match the Go side (NewDirectConsumer installs <proto>_ringbuffer_wakeup_size). */ \
        LOAD_CONSTANT(_STR(name##_ringbuffer_wakeup_size), ringbuffer_wakeup_size);                                    \
        if (ringbuffer_wakeup_size == 0) {                                                                  \
            return 0;                                                                                       \
        }                                                                                                   \
        /* Query the amount of data waiting to be consumed in the ring buffer */                            \
        __u64 pending_data = bpf_ringbuf_query(&map_name, DD_BPF_RB_AVAIL_DATA);                            \
        return (pending_data + data_size) >= ringbuffer_wakeup_size ?                                       \
               DD_BPF_RB_FORCE_WAKEUP : DD_BPF_RB_NO_WAKEUP;                                                \
    }                                                                                                       \
                                                                                                            \
    static __always_inline void name##_output_event(void *ctx, event_type *event) {                         \
        __u64 ringbuffers_enabled = 0;                                                                      \
        LOAD_CONSTANT("ringbuffers_enabled", ringbuffers_enabled);                                          \
                                                                                                            \
        if (ringbuffers_enabled) {                                                                          \
            bpf_ringbuf_output_with_telemetry(&map_name, event, sizeof(event_type),                         \
                                               name##_get_ringbuf_flags(sizeof(event_type)));               \
        } else {                                                                                            \
            u32 cpu = bpf_get_smp_processor_id();                                                           \
            bpf_perf_event_output_with_telemetry(ctx, &map_name, cpu, event, sizeof(event_type));           \
        }                                                                                                   \
    }

#endif // __USM_DIRECT_CONSUMER_H
