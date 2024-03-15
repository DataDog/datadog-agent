#ifndef __USM_EVENTS_TYPES_H
#define __USM_EVENTS_TYPES_H

#include "ktypes.h"

#define BATCH_BUFFER_SIZE (4*1024)
#define BATCH_PAGES_PER_CPU 8

typedef struct {
    // idx is a monotonic counter used for uniquely determining a batch within a CPU core
    // this is useful for detecting race conditions that result in a batch being overwritten
    // before it gets consumed from userspace
    __u64 idx;
    // idx_to_flush is used to track which batches were flushed to userspace
    // * if idx_to_flush == idx, the current index is still being appended to;
    // * if idx_to_flush < idx, the batch at idx_to_flush needs to be sent to userspace;
    // (note that idx will never be less than idx_to_flush);
    __u64 idx_to_flush;
} batch_state_t;

// this struct is used in the map lookup that returns the active batch for a certain CPU core
typedef struct {
    __u16 cpu;
    // page_num can be obtained from (batch_state_t->idx % BATCHES_PER_CPU)
    __u16 page_num;
} batch_key_t;

typedef struct {
    __u64 idx;
    __u16 cpu;
    __u16 len;
    __u16 cap;
    __u16 event_size;
    __u32 dropped_events;
    __u32 failed_flushes;
    char data[BATCH_BUFFER_SIZE];
} batch_data_t;

#endif
