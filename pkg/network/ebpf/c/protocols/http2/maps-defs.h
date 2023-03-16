#ifndef __HTTP2_MAPS_DEFS_H
#define __HTTP2_MAPS_DEFS_H

// http2_static_table is the map that holding the supported static values by index and its static value.
BPF_HASH_MAP(http2_static_table, u8, static_table_entry_t, 20)

/* http2_dynamic_table is the map that holding the supported dynamic values - the index is the static index and the
   tcp_con and it is value is the buffer which contains the dynamic string. */
BPF_LRU_MAP(http2_dynamic_table, dynamic_table_index_t, dynamic_table_entry_t, 1024)

/* http2_dynamic_counter_table is a map that holding the current dynamic values amount, in order to use for the
   internal calculation of the internal index in the http2_dynamic_table, it is hold by conn_tup to support different
   clients and the value is the current counter. */
BPF_LRU_MAP(http2_dynamic_counter_table, conn_tuple_t, u64, 1024)

/* This map is used to keep track of in-flight HTTP2 transactions for each TCP connection */
BPF_LRU_MAP(http2_in_flight, http2_stream_key_t, http2_stream_t, 0)

/* This map serves the purpose of maintaining the current state of tail calls for each frame,
   identified by a tuple consisting of con_tup and skb_info.
   It allows retrieval of both the current offset and the number of iterations that have already been executed. */
BPF_LRU_MAP(http2_iterations, dispatcher_arguments_t, http2_tail_call_state_t, 1024)

/* Allocating a buffer on the heap, the buffer represents the frame payload. */
BPF_PERCPU_ARRAY_MAP(http2_heap_buffer, __u32, heap_buffer_t, 1)

/* Allocating an array of headers, to hold all interesting headers from the frame. */
BPF_PERCPU_ARRAY_MAP(http2_headers_to_process, __u32, http2_headers_t, 1)

/* Allocating a stream on the heap, the stream is used to save the current stream info. */
BPF_PERCPU_ARRAY_MAP(http2_stream_heap, __u32, http2_stream_t, 1)

/* Allocating a ctx on the heap, in order to save the ctx between the current stream. */
BPF_PERCPU_ARRAY_MAP(http2_ctx_heap, __u32, http2_ctx_t, 1)

#endif
