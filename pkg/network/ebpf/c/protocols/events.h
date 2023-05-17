#ifndef __USM_EVENTS_H
#define __USM_EVENTS_H

#include "bpf_telemetry.h"

#include "protocols/events-types.h"

/* USM_EVENTS_INIT defines two functions used for the purposes of buffering and sending
   data to userspace:
   1) <name>_batch_enqueue
   2) <name>_batch_flush
   For more information of this please refer to
   pkg/networks/protocols/events/README.md */
#define USM_EVENTS_INIT(name, value, batch_size)                                        \
    _Static_assert((sizeof(value)*batch_size) <= BATCH_BUFFER_SIZE,                     \
                   _STR(name)" batch is too large");                                    \
                                                                                        \
    BPF_PERCPU_ARRAY_MAP(name##_batch_state, __u32, batch_state_t, 1)                   \
    BPF_PERF_EVENT_ARRAY_MAP(name##_batch_events, __u32)                                \
    BPF_HASH_MAP(name##_batches, batch_key_t, batch_data_t, 1)                          \
                                                                                        \
    static __always_inline bool name##_batch_full(batch_data_t *batch) {                \
        return batch && batch->len == batch_size;                                       \
    }                                                                                   \
                                                                                        \
    static __always_inline bool is_##name##_monitoring_enabled() {                      \
        __u64 val = 0;                                                                  \
        LOAD_CONSTANT(_STR(name##_monitoring_enabled), val);                            \
        return val > 0;                                                                 \
    }                                                                                   \
                                                                                        \
    static __always_inline void name##_batch_flush(struct pt_regs *ctx) {               \
        if (!is_##name##_monitoring_enabled()) {                                        \
            return;                                                                     \
        }                                                                               \
        u32 zero = 0;                                                                   \
        batch_state_t *batch_state = bpf_map_lookup_elem(&name##_batch_state, &zero);   \
        if (!batch_state) {                                                             \
            /* batch is not ready to be flushed */                                      \
            return;                                                                     \
        }                                                                               \
        _Pragma( STR(unroll(BATCH_PAGES_PER_CPU)) )                                     \
            for (int i = 0; i < BATCH_PAGES_PER_CPU; i++) {                             \
                if (batch_state->idx_to_flush == batch_state->idx) return;              \
                                                                                        \
                batch_key_t key = get_batch_key(batch_state->idx_to_flush);             \
                batch_data_t *batch = bpf_map_lookup_elem(&name##_batches, &key);       \
                if (!batch) {                                                           \
                    return;                                                             \
                }                                                                       \
                                                                                        \
                long ret = bpf_perf_event_output_with_telemetry(ctx,                    \
                                                                &name##_batch_events,   \
                                                                key.cpu,                \
                                                                batch,                  \
                                                                sizeof(batch_data_t));  \
                if (ret < 0) {                                                          \
                    _LOG(name, "batch flush error: cpu: %d idx: %d err:%d",             \
                         key.cpu, batch->idx, ret);                                     \
                    return;                                                             \
                }                                                                       \
                                                                                        \
                _LOG(name, "batch flushed: cpu: %d idx: %d", key.cpu, batch->idx);      \
                batch->dropped_events = 0;                                              \
                batch->len = 0;                                                         \
                batch_state->idx_to_flush++;                                            \
            }                                                                           \
    }                                                                                   \
                                                                                        \
    static __always_inline void name##_batch_enqueue(value *event) {                    \
        u32 zero = 0;                                                                   \
        batch_state_t *batch_state =  bpf_map_lookup_elem(&name##_batch_state, &zero);  \
        if (batch_state == NULL) {                                                      \
            return;                                                                     \
        }                                                                               \
                                                                                        \
        batch_key_t key = get_batch_key(batch_state->idx);                              \
        batch_data_t *batch = bpf_map_lookup_elem(&name##_batches, &key);               \
        if (batch == NULL) {                                                            \
            return;                                                                     \
        }                                                                               \
                                                                                        \
        /* if this happens it indicates that <protocol>_batch_flush is not
        executing often enough and/or that BATCH_PAGES_PER_CPU is not large
        enough */                                                                       \
        if (name##_batch_full(batch)) {                                                 \
            batch->dropped_events++;                                                    \
            _LOG(name, "enqueue error: dropping event because batch is full.",          \
                 bpf_get_smp_processor_id(), batch->idx);                               \
            return;                                                                     \
        }                                                                               \
                                                                                        \
        /* this will copy the given event into an eBPF map entry representing the
           current active batch */                                                      \
        if (!__enqueue_event((void *)batch, event, sizeof(value)))                      \
            return;                                                                     \
                                                                                        \
        /* annotate batch with metadata used by userspace */                            \
        batch->cap = batch_size;                                                        \
        batch->event_size = sizeof(value);                                              \
        batch->idx = batch_state->idx;                                                  \
                                                                                        \
        _LOG(name, "event enqueued: cpu: %d batch_idx: %d len: %d",                     \
             key.cpu, batch_state->idx, batch->len);                                    \
        /* if we have filled up the batch we move to the next one.
           notice the batch will be sent "asynchronously" to userspace during the
           next call of <protocol>_batch_flush */                                       \
        if (name##_batch_full(batch)) {                                                 \
            batch_state->idx++;                                                         \
        }                                                                               \
    }                                                                                   \

static __always_inline batch_key_t get_batch_key(u64 batch_idx) {
    batch_key_t key = { 0 };
    key.cpu = bpf_get_smp_processor_id();
    key.page_num = batch_idx % BATCH_PAGES_PER_CPU;
    return key;
}

static __always_inline bool __enqueue_event(batch_data_t *batch, void *event, size_t event_size) {
    /* bounds check to make eBPF verifier happy */
    u32 offset = batch->len*event_size;
    if (offset < 0 || offset+event_size>sizeof(batch->data)) {
        return false;
    }

    bpf_memcpy(&batch->data[offset], event, event_size);
    batch->len++;
    return true;
}

#define _STR(x) #x
#define _LOG(protocol, message, args...) \
    log_debug(_STR(protocol) " " message "\n", args);

#endif
