#ifndef __HTTP2_MAPS_DEFS_H
#define __HTTP2_MAPS_DEFS_H

// http2_static_table is the map that holding the supported static values by index and its static value.
BPF_HASH_MAP(http2_static_table, u8, static_table_entry_t, 20)

// http2_dynamic_table is the map that holding the supported dynamic values - the index is the static index and the
// tcp_con and it is value is the buffer which contains the dynamic string.
BPF_LRU_MAP(http2_dynamic_table, dynamic_table_index_t, dynamic_table_entry_t, 1024)

// http2_dynamic_counter_table is a map that holding the current dynamic values amount, in order to use for the
// internal calculation of the internal index in the http2_dynamic_table, it is hold by conn_tup to support different
// clients and the value is the current counter.
BPF_LRU_MAP(http2_dynamic_counter_table, conn_tuple_t, u64, 1024)


/* This map is used to keep track of in-flight HTTP transactions for each TCP connection */
BPF_LRU_MAP(http2_in_flight, http2_stream_key_t, http2_stream_t, 0)

/* thread_struct id too big for allocation on stack in eBPF function, we use an array as a heap allocator */
BPF_LRU_MAP(http2_iterations, http2_iterations_key_t, http2_tail_call_state_t, 1024)

BPF_PERCPU_ARRAY_MAP(http2_heap_buffer, __u32, heap_buffer_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_headers_to_process, __u32, http2_headers_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_stream_heap, __u32, http2_stream_t, 1)
BPF_PERCPU_ARRAY_MAP(http2_ctx_heap, __u32, http2_ctx_t, 1)

#endif
