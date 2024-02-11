#ifndef __HTTP2_DECODING_COMMON_H
#define __HTTP2_DECODING_COMMON_H

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "decoding-defs.h"
#include "map-defs.h"
#include "ip.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/maps-defs.h"
#include "protocols/classification/defs.h"

// Returns true if the given index represents a path index.
static __always_inline bool is_path_index(const __u64 index) {
    return index == kEmptyPath || index == kIndexPath;
}

// Returns true is the given index represents a method index.
static __always_inline bool is_method_index(const __u64 index) {
    return index == kGET || index == kPOST;
}

// Returns true if the given index represents a status index.
static __always_inline bool is_status_index(const __u64 index) {
    return k200 <= index && index <= k500;
}

// returns true if the given index is one of the relevant headers we care for in the static table.
// The full table can be found in the user mode code `createStaticTable`.
static __always_inline bool is_interesting_static_entry(const __u64 index) {
    return (1 < index && index < 6) || (7 < index && index < 15);
}

// returns true if the given index is below MAX_STATIC_TABLE_INDEX.
static __always_inline bool is_static_table_entry(const __u64 index) {
    return index <= MAX_STATIC_TABLE_INDEX;
}

// http2_fetch_stream returns the current http2 in flight stream.
static __always_inline http2_stream_t *http2_fetch_stream(const http2_stream_key_t *http2_stream_key) {
    http2_stream_t *http2_stream_ptr = bpf_map_lookup_elem(&http2_in_flight, http2_stream_key);
    if (http2_stream_ptr != NULL) {
        return http2_stream_ptr;
    }

    const __u32 zero = 0;
    http2_stream_ptr = bpf_map_lookup_elem(&http2_stream_heap, &zero);
    if (http2_stream_ptr == NULL) {
        return NULL;
    }
    bpf_memset(http2_stream_ptr, 0, sizeof(http2_stream_t));
    bpf_map_update_elem(&http2_in_flight, http2_stream_key, http2_stream_ptr, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_in_flight, http2_stream_key);
}

// get_dynamic_counter returns the current dynamic counter by the conn tuple.
static __always_inline __u64 *get_dynamic_counter(conn_tuple_t *tup) {
    __u64 counter = 0;
    bpf_map_update_elem(&http2_dynamic_counter_table, tup, &counter, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
}

// parse_field_indexed parses fully-indexed headers.
static __always_inline void parse_field_indexed(http2_stream_t *current_stream, dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter, http2_telemetry_t *http2_tel) {
    if (headers_to_process == NULL) {
        return;
    }

    if (is_static_table_entry(index)) {
        if (is_method_index(index)) {
           current_stream->request_started = bpf_ktime_get_ns();
           current_stream->request_method.static_table_entry = index;
           current_stream->request_method.finalized = true;
            __sync_fetch_and_add(&http2_tel->request_seen, 1);
        } else if (is_status_index(index)) {
            current_stream->status_code.static_table_entry = index;
            current_stream->status_code.finalized = true;
            __sync_fetch_and_add(&http2_tel->response_seen, 1);
        } else if (is_path_index(index)) {
            current_stream->path.static_table_entry = index;
            current_stream->path.finalized = true;
        }
        return;
    }

    // We change the index to match our internal dynamic table implementation index.
    // Our internal indexes start from 1, so we subtract 61 in order to match the given index.
    dynamic_index->index = global_dynamic_counter - (index - MAX_STATIC_TABLE_INDEX);
    dynamic_table_entry_t *dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, dynamic_index);
    if (dynamic_value == NULL) {
        // If the entry is missing, it means the index does not represents an interesting header and we should abort.
        return;
    }
    if (is_path_index(dynamic_value->original_index)) {
        current_stream->path.length = dynamic_value->string_len;
        current_stream->path.is_huffman_encoded = dynamic_value->is_huffman_encoded;
        current_stream->path.finalized = true;
        bpf_memcpy(current_stream->path.raw_buffer, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
    } else if (is_status_index(dynamic_value->original_index)) {
        bpf_memcpy(current_stream->status_code.raw_buffer, dynamic_value->buffer, HTTP2_STATUS_CODE_MAX_LEN);
        current_stream->status_code.is_huffman_encoded = dynamic_value->is_huffman_encoded;
        current_stream->status_code.finalized = true;
    } else if (is_method_index(dynamic_value->original_index)) {
        current_stream->request_started = bpf_ktime_get_ns();
        bpf_memcpy(current_stream->request_method.raw_buffer, dynamic_value->buffer, HTTP2_METHOD_MAX_LEN);
        current_stream->request_method.is_huffman_encoded = dynamic_value->is_huffman_encoded;
        current_stream->request_method.length = dynamic_value->string_len;
        current_stream->request_method.finalized = true;
    }
    return;
}

// update_path_size_telemetry updates the path size telemetry.
static __always_inline void update_path_size_telemetry(http2_telemetry_t *http2_tel, __u64 size) {
    // This line can be considered as a step function of the difference multiplied by difference.
    // step function of the difference is 0 if the difference is negative, and 1 if the difference is positive.
    // Thus, if the difference is negative, we will get 0, and if the difference is positive, we will get the difference.
    size = size < HTTP2_TELEMETRY_MAX_PATH_LEN ? 0 : size - HTTP2_TELEMETRY_MAX_PATH_LEN;
    // This line acts as a ceil function, which means that if the size is not a multiple of the bucket size, we will
    // round it up to the next bucket. Since we don't have float numbers in eBPF, we are adding the (bucket size - 1)
    // to the size, and then dividing it by the bucket size. This will give us the ceil function.
#define CEIL_FUNCTION_FACTOR (HTTP2_TELEMETRY_PATH_BUCKETS_SIZE - 1)
    __u8 bucket_idx = (size + CEIL_FUNCTION_FACTOR) / HTTP2_TELEMETRY_PATH_BUCKETS_SIZE;

    // This line guarantees that the bucket index is between 0 and HTTP2_TELEMETRY_PATH_BUCKETS.
    // Although, it is not needed, we keep this function to please the verifier, and to have an explicit lower bound.
    bucket_idx = bucket_idx < 0 ? 0 : bucket_idx;
    // This line guarantees that the bucket index is between 0 and HTTP2_TELEMETRY_PATH_BUCKETS, and we cannot
    // exceed the upper bound.
    bucket_idx = bucket_idx > HTTP2_TELEMETRY_PATH_BUCKETS ? HTTP2_TELEMETRY_PATH_BUCKETS : bucket_idx;

    __sync_fetch_and_add(&http2_tel->path_size_bucket[bucket_idx], 1);
}

// handle_end_of_stream is called when we see a HTTP2 END_STREAM (EOS) flag in
// a frame. When a stream is considered as ended, we can enqueue the stream's
// in-flight data for batch processing.
//
// For a given stream to be considered as ended, both the client and server
// sides must send an EOS, so this function should be called twice for each
// stream, before it actually enqueues the stream's stats.
//
// See RFC 7540 section 5.1: https://datatracker.ietf.org/doc/html/rfc7540#section-5.1
static __always_inline void handle_end_of_stream(http2_stream_t *current_stream, http2_stream_key_t *http2_stream_key_template, http2_telemetry_t *http2_tel) {
    // We want to see the EOS twice for a given stream: one for the client, one for the server.
    if (!current_stream->request_end_of_stream) {
        current_stream->request_end_of_stream = true;
        return;
    }

    // response end of stream;
    current_stream->response_last_seen = bpf_ktime_get_ns();

    const __u32 zero = 0;
    http2_event_t *event = bpf_map_lookup_elem(&http2_scratch_buffer, &zero);
    if (event) {
        bpf_memcpy(&event->tuple, &http2_stream_key_template->tup, sizeof(conn_tuple_t));
        bpf_memcpy(&event->stream, current_stream, sizeof(http2_stream_t));
        // enqueue
        http2_batch_enqueue(event);
    }

    bpf_map_delete_elem(&http2_in_flight, http2_stream_key_template);
}

// A similar implementation of read_http2_frame_header, but instead of getting both a char array and an out parameter,
// we get only the out parameter (equals to http2_frame_t* representation of the char array) and we perform the
// field adjustments we have in read_http2_frame_header.
static __always_inline bool format_http2_frame_header(http2_frame_t *out) {
    if (is_empty_frame_header((char *)out)) {
        return false;
    }

    // We extract the frame by its shape to fields.
    // See: https://datatracker.ietf.org/doc/html/rfc7540#section-4.1
    out->length = bpf_ntohl(out->length << 8);
    out->stream_id = bpf_ntohl(out->stream_id << 1);

    return out->type <= kContinuationFrame && out->length <= MAX_FRAME_SIZE && (out->stream_id == 0 || (out->stream_id % 2 == 1));
}

static __always_inline void reset_frame(http2_frame_t *out) {
    *out = (http2_frame_t){ 0 };
}

#endif
