#ifndef __HTTP2_MAPS_DEFS_H
#define __HTTP2_MAPS_DEFS_H

// http2_remainder maps a connection tuple to the remainder from the previous packet.
// It is possible for frames to be split to multiple tcp packets, so we need to associate the remainder from the previous
// packet, to the current one.
BPF_HASH_MAP(http2_remainder, conn_tuple_t, frame_header_remainder_t, 0)

/* http2_dynamic_table is the map that holding the supported dynamic values - the index is the static index and the
   conn tuple and it is value is the buffer which contains the dynamic string. */
BPF_HASH_MAP(http2_dynamic_table, dynamic_table_index_t, dynamic_table_entry_t, 0)

// A map between a stream (connection and a stream id) to the current global dynamic counter.
// The value also a field called "previous" which is used to cache the last index we've cleaned during our cleanup
// tail calls.
BPF_HASH_MAP(http2_dynamic_counter_table, conn_tuple_t, dynamic_counter_t, 0)

/* This map is used to keep track of in-flight HTTP2 transactions for each TCP connection */
BPF_HASH_MAP(http2_in_flight, http2_stream_key_t, http2_stream_t, 0)

/* This map serves the purpose of maintaining the current state of tail calls for each frame,
   identified by a tuple consisting of con_tup and skb_info.
   It allows retrieval of both the current offset and the number of iterations that have already been executed. */
BPF_HASH_MAP(http2_iterations, dispatcher_arguments_t, http2_tail_call_state_t, 0)

/* This map serves the purpose of maintaining the current state of tail calls for each frame.
   It allows retrieval of both the current offset and the number of iterations that have already been executed. */
BPF_HASH_MAP(tls_http2_iterations, tls_dispatcher_arguments_t, http2_tail_call_state_t, 0)

/* Allocating an array of headers, to hold all interesting headers from the frame. */
BPF_PERCPU_ARRAY_MAP(http2_headers_to_process, http2_header_t[HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING], 1)

/* Allocating an array of frame, to hold all interesting frames from the packet. */
BPF_PERCPU_ARRAY_MAP(http2_frames_to_process, http2_tail_call_state_t, 1)

/* Allocating a stream on the heap, the stream is used to save the current stream info. */
BPF_PERCPU_ARRAY_MAP(http2_stream_heap, http2_stream_t, 1)

/* This map acts as a scratch buffer for "preparing" http2_event_t objects before they're
   enqueued. The primary motivation here is to save eBPF stack memory. */
BPF_PERCPU_ARRAY_MAP(http2_scratch_buffer, http2_event_t, 1)

/* Allocating a ctx on the heap, in order to save the ctx between the current stream. */
BPF_PERCPU_ARRAY_MAP(http2_ctx_heap, http2_ctx_t, 1)

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a http2 telemetry object
 */
BPF_ARRAY_MAP(http2_telemetry, http2_telemetry_t, 1)
BPF_ARRAY_MAP(tls_http2_telemetry, http2_telemetry_t, 1)

#endif
