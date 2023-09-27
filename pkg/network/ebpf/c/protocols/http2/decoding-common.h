#ifndef DECODING_COMMON_H_
#define DECODING_COMMON_H_

#include "protocols/http2/helpers.h"
#include "protocols/http2/maps-defs.h"

// returns true if the given index is one of the relevant headers we care for in the static table.
// The full table can be found in the user mode code `createStaticTable`.
static __always_inline bool is_interesting_static_entry(const __u64 index) {
    return (1 < index && index < 6) || (7 < index && index < 15);
}

// returns true if the given index is below MAX_STATIC_TABLE_INDEX.
static __always_inline bool is_static_table_entry(const __u64 index) {
    return index <= MAX_STATIC_TABLE_INDEX;
}

// A similar implementation of read_http2_frame_header, but instead of getting both a char array and an out parameter,
// we get only the out parameter (equals to struct http2_frame * representation of the char array) and we perform the
// field adjustments we have in read_http2_frame_header.
static __always_inline bool format_http2_frame_header(struct http2_frame *out) {
    if (is_empty_frame_header((char *)out)) {
        return false;
    }

    // We extract the frame by its shape to fields.
    // See: https://datatracker.ietf.org/doc/html/rfc7540#section-4.1
    out->length = bpf_ntohl(out->length << 8);
    out->stream_id = bpf_ntohl(out->stream_id << 1);

    log_debug("[grpctls] length: %d, type: %d", out->length, out->type);

    return out->type <= kContinuationFrame;
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

// get_dynamic_counter returns the current dynamic counter by the conn tup.
static __always_inline __u64 *get_dynamic_counter(conn_tuple_t *tup) {
    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
    if (counter_ptr != NULL) {
        return counter_ptr;
    }
    __u64 counter = 0;
    bpf_map_update_elem(&http2_dynamic_counter_table, tup, &counter, BPF_ANY);
    return bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
}

static __always_inline void parse_field_indexed(dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter) {
    if (headers_to_process == NULL) {
        return;
    }
    // TODO: can improve by declaring MAX_INTERESTING_STATIC_TABLE_INDEX
    if (is_interesting_static_entry(index)) {
        headers_to_process->index = index;
        headers_to_process->type = kStaticHeader;
        (*interesting_headers_counter)++;
        return;
    }
    if (is_static_table_entry(index)) {
        return;
    }

    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    dynamic_index->index = global_dynamic_counter - (index - MAX_STATIC_TABLE_INDEX);

    if (bpf_map_lookup_elem(&http2_dynamic_table, dynamic_index) == NULL) {
        return;
    }

    headers_to_process->index = dynamic_index->index;
    headers_to_process->type = kExistingDynamicHeader;
    (*interesting_headers_counter)++;
    return;
}

static __always_inline void handle_end_of_stream(http2_stream_t *current_stream, http2_stream_key_t *http2_stream_key_template) {
    if (!current_stream->request_end_of_stream) {
        current_stream->request_end_of_stream = true;
        return;
    }

    log_debug("Got EndOfStream event");

    // response end of stream;
    current_stream->response_last_seen = bpf_ktime_get_ns();
    current_stream->tup = http2_stream_key_template->tup;

    // enqueue
    http2_batch_enqueue(current_stream);
    bpf_map_delete_elem(&http2_in_flight, http2_stream_key_template);
}

#endif // DECODING_COMMON_H_
