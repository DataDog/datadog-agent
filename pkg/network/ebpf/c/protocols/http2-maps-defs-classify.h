#ifndef __HTTP2_MAPS_DEFS_CLASSIFY_H
#define __HTTP2_MAPS_DEFS_CLASSIFY_H

#include "http2-decoding-defs.h"

/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_LRU_MAP(http2_in_flight, http2_stream_key_t, http2_stream_t, 0)

/* thread_struct id too big for allocation on stack in eBPF function, we use an array as a heap allocator */
BPF_LRU_MAP(http2_iterations, http2_iterations_key_t, http2_tail_call_state_t, 1024)

BPF_PERCPU_ARRAY_MAP(http2_heap_buffer, __u32, heap_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_headers_to_process, __u32, http2_headers_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_stream_heap, __u32, http2_stream_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_ctx_heap, __u32, http2_ctx_t, 1)
#endif
