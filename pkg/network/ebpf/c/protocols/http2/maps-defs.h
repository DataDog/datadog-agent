#ifndef __HTTP2_MAPS_DEFS_H
#define __HTTP2_MAPS_DEFS_H

// http2_remainder maps a connection tuple to the remainder from the previous packet.
// It is possible for frames to be split to multiple tcp packets, so we need to associate the remainder from the previous
// packet, to the current one.
BPF_HASH_MAP(http2_remainder, conn_tuple_t, frame_header_remainder_t, 2048)

// The map acts as a set, to indicate if a given dynamic index (conn tuple + index) is interesting.
// If a key exists - the index is interesting, otherwise it is not.
BPF_HASH_MAP(http2_interesting_dynamic_table_set, dynamic_table_index_t, bool, 0)

/* http2_dynamic_counter_table is a map that holding the current dynamic values amount, in order to use for the
   internal calculation of the internal index in the http2_interesting_dynamic_table_set, it is hold by conn_tup to
   support different clients and the value is the current counter. */
BPF_HASH_MAP(http2_dynamic_counter_table, conn_tuple_t, u64, 0)

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

/* This map acts as a scratch buffer for "preparing" http2_event_t objects before they're
   enqueued. The primary motivation here is to save eBPF stack memory. */
BPF_PERCPU_ARRAY_MAP(http2_scratch_buffer, http2_event_t, 1)

// Allocating a stream_key on the heap to reduce stack pressure.
BPF_PERCPU_ARRAY_MAP(http2_stream_key_heap, http2_stream_key_t, 1)

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a http2 telemetry object
 */
BPF_ARRAY_MAP(http2_telemetry, http2_telemetry_t, 1)

// A perf buffer to send http2 paths to the user mode LRU datastore.
BPF_PERF_EVENT_ARRAY_MAP(http2_dynamic_table_perf_buffer, __u32)

// This map acts as a heap for dynamic table values to be sent on the perf buffer.
BPF_PERCPU_ARRAY_MAP(http2_dynamic_table_heap, dynamic_table_value_t, 1)

#endif
