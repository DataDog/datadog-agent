#ifndef __HTTP2_MAPS_DEFS_CLASSIFY_H
#define __HTTP2_MAPS_DEFS_CLASSIFY_H

#include "http2-decoding-defs.h"

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_LRU_MAP(http2_in_flight, conn_tuple_t, http2_transaction_t, 0)

typedef struct {
    __u32 offset;
    __u8 iteration;
} __attribute__ ((packed)) http2_tail_call_state_t;

/* thread_struct id too big for allocation on stack in eBPF function, we use an array as a heap allocator */
BPF_LRU_MAP(http2_iterations, conn_tuple_t, http2_tail_call_state_t, 1024)

BPF_PERCPU_ARRAY_MAP(http2_trans_alloc, __u32, http2_transaction_t, 1)
BPF_PERCPU_ARRAY_MAP(http_trans_alloc, __u32, http_transaction_t, 1)

BPF_PERCPU_ARRAY_MAP(http2_heap_buffer, __u32, heap_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_headers_to_process, __u32, http2_headers_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_frames_to_process, __u32, http2_frames_t, 1)
#endif
