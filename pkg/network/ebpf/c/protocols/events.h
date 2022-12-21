#ifndef __USM_EVENTS_H
#define __USM_EVENTS_H

#include "events-types.h"

#define STR(x) #x

#define USM_EVENTS_INIT(name, value, batch_size)                                                  \
    _Static_assert((sizeof(value)*batch_size) <= BATCH_BUFFER_SIZE, STR(name)" batch too large"); \
                                                                                                  \
    BPF_PERCPU_ARRAY_MAP(name##_batch_state, __u32, batch_state_t, 1)                             \
    BPF_PERF_EVENT_ARRAY_MAP(name##_batch_events, __u32, 0)                                       \
    BPF_HASH_MAP(name##_batches, batch_key_t, batch_data_t, 0)                                    \
                                                                                                  \
    static __always_inline bool name##_batch_full(batch_data_t *batch) {                          \
        return batch && batch->len == batch_size;                                                 \
    }                                                                                             \
                                                                                                  \
    static __always_inline void name##_flush_batch(struct pt_regs *ctx) {                         \
        u32 zero = 0;                                                                             \
        batch_state_t *batch_state = bpf_map_lookup_elem(&name##_batch_state, &zero);             \
        if (batch_state == NULL || batch_state->idx_to_flush == batch_state->idx) {               \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        batch_key_t key = get_batch_key(batch_state->idx_to_flush);                               \
        batch_data_t *batch = bpf_map_lookup_elem(&name##_batches, &key);                         \
        if (batch == NULL) {                                                                      \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        long ret = bpf_perf_event_output(ctx, &name##_batch_events, key.cpu,                      \
                                        batch, sizeof(batch_data_t));                             \
        if (ret < 0) {                                                                            \
            log_debug(STR(name) " batch flush error: cpu: %d idx: %d err:%d \n",                  \
                      key.cpu, batch->idx, ret);                                                  \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        log_debug(STR(name) " batch flushed: cpu: %d idx: %d\n", key.cpu, batch->idx);            \
        batch->len = 0;                                                                           \
        batch_state->idx_to_flush++;                                                              \
    }                                                                                             \
                                                                                                  \
    static __always_inline void name##_batch_enqueue(value *element) {                            \
        u32 zero = 0;                                                                             \
        batch_state_t *batch_state =  bpf_map_lookup_elem(&name##_batch_state, &zero);            \
        if (batch_state == NULL) {                                                                \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        batch_key_t key = get_batch_key(batch_state->idx);                                        \
        batch_data_t *batch = bpf_map_lookup_elem(&name##_batches, &key);                         \
        if (batch == NULL) {                                                                      \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        if (name##_batch_full(batch)) {                                                           \
            log_debug(STR(name) " enqueue error: dropping element because batch is full. "        \
                      "cpu=%d batch_idx=%d\n",                                                    \
                      bpf_get_smp_processor_id(), batch->idx);                                    \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        u32 offset = batch->len*sizeof(value);                                                    \
        if (offset < 0 || offset+sizeof(value)>sizeof(batch->data)) {                             \
            return;                                                                               \
        }                                                                                         \
                                                                                                  \
        bpf_memcpy(&batch->data[offset], element, sizeof(value));                                 \
        /* cap and obj_size are used as metadata by userspace only */                             \
        batch->cap = batch_size;                                                                  \
        batch->element_size = sizeof(value);                                                      \
        batch->len++;                                                                             \
        batch->idx = batch_state->idx;                                                            \
        log_debug(STR(name) " element enqueued: cpu: %d batch_idx: %d len: %d\n",                 \
                  key.cpu, batch_state->idx, batch->len);                                         \
                                                                                                  \
        if (name##_batch_full(batch)) {                                                           \
            batch_state->idx++;                                                                   \
        }                                                                                         \
    }                                                                                             \

static __always_inline batch_key_t get_batch_key(u64 batch_idx) {
    batch_key_t key = { 0 };
    key.cpu = bpf_get_smp_processor_id();
    key.page_num = batch_idx % BATCH_PAGES_PER_CPU;
    return key;
}

#endif
