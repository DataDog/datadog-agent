#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "decoding-defs.h"
#include "map-defs.h"
#include "ip.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/maps-defs.h"
#include "protocols/http2/usm-events.h"
#include "protocols/http/types.h"
#include "protocols/classification/defs.h"

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

// Similar to read_hpack_int, but with a small optimization of getting the
// current character as input argument.
static __always_inline bool read_hpack_int_with_given_current_char(struct __sk_buff *skb, skb_info_t *skb_info, __u64 current_char_as_number, __u64 max_number_for_bits, __u64 *out) {
    current_char_as_number &= max_number_for_bits;

    // In HPACK, if the number is too big to be stored in max_number_for_bits
    // bits, then those bits are all set to one, and the rest of the number must
    // be read from subsequent bytes.
    if (current_char_as_number < max_number_for_bits) {
        *out = current_char_as_number;
        return true;
    }

    // Read the next byte, and check if it is the last byte of the number.
    // While HPACK does support arbitrary sized numbers, we are limited by the
    // number of instructions we can use in a single eBPF program, so we only
    // parse one additional byte. The max value that can be parsed is
    // `(2^max_number_for_bits - 1) + 127`.
    __u64 next_char = 0;
    if (bpf_skb_load_bytes(skb, skb_info->data_off, &next_char, 1) >= 0 && (next_char & 128) == 0) {
        skb_info->data_off++;
        *out = current_char_as_number + (next_char & 127);
        return true;
    }

    return false;
}

// read_hpack_int reads an unsigned variable length integer as specified in the
// HPACK specification, from an skb.
//
// See https://httpwg.org/specs/rfc7541.html#rfc.section.5.1 for more details on
// how numbers are represented in HPACK.
//
// max_number_for_bits represents the number of bits in the first byte that are
// used to represent the MSB of number. It must always be between 1 and 8.
//
// The parsed number is stored in out.
//
// read_hpack_int returns true if the integer was successfully parsed, and false
// otherwise.
static __always_inline bool read_hpack_int(struct __sk_buff *skb, skb_info_t *skb_info, __u64 max_number_for_bits, __u64 *out) {
    __u64 current_char_as_number = 0;
    if (bpf_skb_load_bytes(skb, skb_info->data_off, &current_char_as_number, 1) < 0) {
        return false;
    }
    skb_info->data_off++;

    return read_hpack_int_with_given_current_char(skb, skb_info, current_char_as_number, max_number_for_bits, out);
}

// get_dynamic_counter returns the current dynamic counter by the conn tuple.
static __always_inline __u64 *get_dynamic_counter(conn_tuple_t *tup) {
    __u64 counter = 0;
    bpf_map_update_elem(&http2_dynamic_counter_table, tup, &counter, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
}

// parse_field_indexed parses fully-indexed headers.
static __always_inline void parse_field_indexed(dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter) {
    if (headers_to_process == NULL) {
        return;
    }

    // TODO: can improve by declaring MAX_INTERESTING_STATIC_TABLE_INDEX
    if (is_static_table_entry(index)) {
        headers_to_process->index = index;
        headers_to_process->type = kStaticHeader;
        *interesting_headers_counter += is_interesting_static_entry(index);
        return;
    }

    // We change the index to match our internal dynamic table implementation index.
    // Our internal indexes start from 1, so we subtract 61 in order to match the given index.
    dynamic_index->index = global_dynamic_counter - (index - MAX_STATIC_TABLE_INDEX);

    headers_to_process->index = dynamic_index->index;
    headers_to_process->type = kExistingDynamicHeader;
    // If the entry exists, increase the counter. If the entry is missing, then we won't increase the counter.
    // This is a simple trick to spare if-clause, to reduce pressure on the complexity of the program.
    *interesting_headers_counter += bpf_map_lookup_elem(&http2_dynamic_table, dynamic_index) != NULL;
    return;
}

READ_INTO_BUFFER(path, HTTP2_MAX_PATH_LEN, BLK_SIZE)

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


// parse_field_literal parses a header with a literal value.
//
// We are only interested in path headers, that we will store in our internal
// dynamic table, and will skip headers that are not path headers.
static __always_inline bool parse_field_literal(struct __sk_buff *skb, skb_info_t *skb_info, http2_header_t *headers_to_process, __u64 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter, http2_telemetry_t *http2_tel) {
    __u64 str_len = 0;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len)) {
            return false;
        }
        goto end;
    }

    if (index == kIndexPath) {
        update_path_size_telemetry(http2_tel, str_len);
    } else {
        goto end;
    }

    // We skip if:
    // - The string is too big
    // - This is not a path
    // - We won't be able to store the header info
    if (headers_to_process == NULL) {
        goto end;
    }

    if (skb_info->data_off + str_len > skb_info->data_end) {
        __sync_fetch_and_add(&http2_tel->path_exceeds_frame, 1);
        goto end;
    }

    headers_to_process->index = global_dynamic_counter - 1;
    headers_to_process->type = kNewDynamicHeader;
    headers_to_process->new_dynamic_value_offset = skb_info->data_off;
    headers_to_process->new_dynamic_value_size = str_len;
    // If the string len (`str_len`) is in the range of [0, HTTP2_MAX_PATH_LEN], and we don't exceed packet boundaries
    // (skb_info->data_off + str_len <= skb_info->data_end) and the index is kIndexPath, then we have a path header,
    // and we're increasing the counter. In any other case, we're not increasing the counter.
    *interesting_headers_counter += (str_len > 0 && str_len <= HTTP2_MAX_PATH_LEN);
end:
    skb_info->data_off += str_len;
    return true;
}

// filter_relevant_headers parses the http2 headers frame, and filters headers
// that are relevant for us, to be processed later on.
// The return value is the number of relevant headers that were found and inserted
// in the `headers_to_process` table.
static __always_inline __u8 filter_relevant_headers(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u32 frame_length, http2_telemetry_t *http2_tel) {
    __u8 current_ch;
    __u8 interesting_headers = 0;
    http2_header_t *current_header;
    const __u32 frame_end = skb_info->data_off + frame_length;
    const __u32 end = frame_end < skb_info->data_end + 1 ? frame_end : skb_info->data_end + 1;
    bool is_literal = false;
    bool is_indexed = false;
    __u64 max_bits = 0;
    __u64 index = 0;

    __u64 *global_dynamic_counter = get_dynamic_counter(tup);
    if (global_dynamic_counter == NULL) {
        return 0;
    }

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (skb_info->data_off >= end) {
            break;
        }
        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off++;

        is_indexed = (current_ch & 128) != 0;
        is_literal = (current_ch & 192) == 64;

        if (is_indexed) {
            max_bits = MAX_7_BITS;
        } else if (is_literal) {
            max_bits = MAX_6_BITS;
        } else {
            continue;
        }

        index = 0;
        if (!read_hpack_int_with_given_current_char(skb, skb_info, current_ch, max_bits, &index)) {
            break;
        }

        current_header = NULL;
        if (interesting_headers < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING) {
            current_header = &headers_to_process[interesting_headers];
        }

        if (is_indexed) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            parse_field_indexed(dynamic_index, current_header, index, *global_dynamic_counter, &interesting_headers);
        } else {
            __sync_fetch_and_add(global_dynamic_counter, 1);
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 11
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            if (!parse_field_literal(skb, skb_info, current_header, index, *global_dynamic_counter, &interesting_headers, http2_tel)) {
                break;
            }
        }
    }

    return interesting_headers;
}

// process_headers processes the headers that were filtered in filter_relevant_headers,
// looking for requests path, status code, and method.
static __always_inline void process_headers(struct __sk_buff *skb, dynamic_table_index_t *dynamic_index, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers,  http2_telemetry_t *http2_tel) {
    http2_header_t *current_header;
    dynamic_table_entry_t dynamic_value = {};

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING; ++iteration) {
        if (iteration >= interesting_headers) {
            break;
        }

        current_header = &headers_to_process[iteration];

        if (current_header->type == kStaticHeader) {
            static_table_value_t *static_value = bpf_map_lookup_elem(&http2_static_table, &current_header->index);
            if (static_value == NULL) {
                break;
            }

            if (current_header->index == kPOST || current_header->index == kGET) {
                // TODO: mark request
                current_stream->request_started = bpf_ktime_get_ns();
                current_stream->request_method = *static_value;
                __sync_fetch_and_add(&http2_tel->request_seen, 1);
            } else if (current_header->index >= k200 && current_header->index <= k500) {
                current_stream->response_status_code = *static_value;
                __sync_fetch_and_add(&http2_tel->response_seen, 1);
            } else if (current_header->index == kEmptyPath) {
                current_stream->path_size = HTTP_ROOT_PATH_LEN;
                bpf_memcpy(current_stream->request_path, HTTP_ROOT_PATH, HTTP_ROOT_PATH_LEN);
            } else if (current_header->index == kIndexPath) {
                current_stream->path_size = HTTP_INDEX_PATH_LEN;
                bpf_memcpy(current_stream->request_path, HTTP_INDEX_PATH, HTTP_INDEX_PATH_LEN);
            }
            continue;
        }

        dynamic_index->index = current_header->index;
        if (current_header->type == kExistingDynamicHeader) {
            dynamic_table_entry_t *dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, dynamic_index);
            if (dynamic_value == NULL) {
                break;
            }
            current_stream->path_size = dynamic_value->string_len;
            bpf_memcpy(current_stream->request_path, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
        } else {
            dynamic_value.string_len = current_header->new_dynamic_value_size;

            // create the new dynamic value which will be added to the internal table.
            read_into_buffer_path(dynamic_value.buffer, skb, current_header->new_dynamic_value_offset);
            bpf_map_update_elem(&http2_dynamic_table, dynamic_index, &dynamic_value, BPF_ANY);
            current_stream->path_size = current_header->new_dynamic_value_size;
            bpf_memcpy(current_stream->request_path, dynamic_value.buffer, HTTP2_MAX_PATH_LEN);
        }
    }
}

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

static __always_inline void process_headers_frame(struct __sk_buff *skb, http2_stream_t *current_stream, skb_info_t *skb_info, conn_tuple_t *tup, dynamic_table_index_t *dynamic_index, struct http2_frame *current_frame_header, http2_telemetry_t *http2_tel) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = filter_relevant_headers(skb, skb_info, tup, dynamic_index, headers_to_process, current_frame_header->length, http2_tel);
    process_headers(skb, dynamic_index, current_stream, headers_to_process, interesting_headers, http2_tel);
}

static __always_inline void parse_frame(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx, struct http2_frame *current_frame, http2_telemetry_t *http2_tel) {
    http2_ctx->http2_stream_key.stream_id = current_frame->stream_id;
    http2_stream_t *current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
    if (current_stream == NULL) {
        return;
    }

    if (current_frame->type == kHeadersFrame) {
        process_headers_frame(skb, current_stream, skb_info, tup, &http2_ctx->dynamic_index, current_frame, http2_tel);
    }

    // When we accept an RST, it means that the current stream is terminated.
    // See: https://datatracker.ietf.org/doc/html/rfc7540#section-6.4
    bool is_rst = current_frame->type == kRSTStreamFrame;
    // If rst, and stream is empty (no status code, or no response) then delete from inflight
    if (is_rst && (current_stream->response_status_code == 0 || current_stream->request_started == 0)) {
        bpf_map_delete_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
        return;
    }

    bool should_handle_end_of_stream = false;
    if (is_rst) {
        __sync_fetch_and_add(&http2_tel->end_of_stream_rst, 1);
        should_handle_end_of_stream = true;
    } else if ((current_frame->flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
        __sync_fetch_and_add(&http2_tel->end_of_stream, 1);
        should_handle_end_of_stream = true;
    }

    if (should_handle_end_of_stream) {
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key, http2_tel);
    }

    return;
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

    return out->type <= kContinuationFrame && out->length <= MAX_FRAME_SIZE && (out->stream_id == 0 || (out->stream_id % 2 == 1));
}

// skip_preface is a helper function to check for the HTTP2 magic sent at the beginning
// of an HTTP2 connection, and skip it if present.
static __always_inline void skip_preface(struct __sk_buff *skb, skb_info_t *skb_info) {
    char preface[HTTP2_MARKER_SIZE];
    bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
    bpf_skb_load_bytes(skb, skb_info->data_off, preface, HTTP2_MARKER_SIZE);
    if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
        skb_info->data_off += HTTP2_MARKER_SIZE;
    }
}

// The function is trying to read the remaining of a split frame header. We have the first part in
// `frame_state->buf` (from the previous packet), and now we're trying to read the remaining (`frame_state->remainder`
// bytes from the current packet).
static __always_inline void fix_header_frame(struct __sk_buff *skb, skb_info_t *skb_info, char *out, frame_header_remainder_t *frame_state) {
    bpf_memcpy(out, frame_state->buf, HTTP2_FRAME_HEADER_SIZE);
    // Verifier is unhappy with a single call to `bpf_skb_load_bytes` with a variable length (although checking boundaries)
    switch (frame_state->remainder) {
    case 1:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 1, 1);
        break;
    case 2:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 2, 2);
        break;
    case 3:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 3, 3);
        break;
    case 4:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 4, 4);
        break;
    case 5:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 5, 5);
        break;
    case 6:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 6, 6);
        break;
    case 7:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 7, 7);
        break;
    case 8:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 8, 8);
        break;
    }
    return;
}

static __always_inline void reset_frame(struct http2_frame *out) {
    *out = (struct http2_frame){ 0 };
}

static __always_inline bool get_first_frame(struct __sk_buff *skb, skb_info_t *skb_info, frame_header_remainder_t *frame_state, struct http2_frame *current_frame, http2_telemetry_t *http2_tel) {
    // No state, try reading a frame.
    if (frame_state == NULL) {
        // Checking we have enough bytes in the packet to read a frame header.
        if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb_info->data_end) {
            // Not enough bytes, cannot read frame, so we have 0 interesting frames in that packet.
            return false;
        }

        // Reading frame, and ensuring the frame is valid.
        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)current_frame, HTTP2_FRAME_HEADER_SIZE);
        skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
        if (!format_http2_frame_header(current_frame)) {
            // Frame is not valid, so we have 0 interesting frames in that packet.
            return false;
        }
        return true;
    }

    // Getting here means we have a frame state from the previous packets.
    // Scenarios in order:
    //  1. Check if we have a frame-header remainder - if so, we must try and read the rest of the frame header.
    //     In case of a failure, we abort.
    //  2. If we don't have a frame-header remainder, then we're trying to read a valid frame.
    //     HTTP2 can send valid frames (like SETTINGS and PING) during a split DATA frame. If such a frame exists,
    //     then we won't have the rest of the split frame in the same packet.
    //  3. If we reached here, and we have a remainder, then we're consuming the remainder and checking we can read the
    //     next frame header.
    //  4. We failed reading any frame. Aborting.

    // Frame-header-remainder.
    if (frame_state->header_length > 0) {
        fix_header_frame(skb, skb_info, (char*)current_frame, frame_state);
        if (format_http2_frame_header(current_frame)) {
            skb_info->data_off += frame_state->remainder;
            frame_state->remainder = 0;
            return true;
        }

        // We couldn't read frame header using the remainder.
        return false;
    }

    // Checking if we can read a frame header.
    if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE <= skb_info->data_end) {
        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)current_frame, HTTP2_FRAME_HEADER_SIZE);
        if (format_http2_frame_header(current_frame)) {
            // We successfully read a valid frame.
            skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
            return true;
        }
    }

    // We failed to read a frame, if we have a remainder trying to consume it and read the following frame.
    if (frame_state->remainder > 0) {
        skb_info->data_off += frame_state->remainder;
        // The remainders "ends" the current packet. No interesting frames were found.
        if (skb_info->data_off == skb_info->data_end) {
            frame_state->remainder = 0;
            return false;
        }
        reset_frame(current_frame);
        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)current_frame, HTTP2_FRAME_HEADER_SIZE);
        if (format_http2_frame_header(current_frame)) {
            frame_state->remainder = 0;
            skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
            return true;
        }
    }
    // still not valid / does not have a remainder - abort.
    return false;
}

// find_relevant_frames iterates over the packet and finds frames that are
// relevant for us. The frames info and location are stored in the `frames_array` array,
// and the number of frames found is returned.
//
// We consider frames as relevant if they are either:
// - HEADERS frames
// - RST_STREAM frames
// - DATA frames with the END_STREAM flag set
static __always_inline __u8 find_relevant_frames(struct __sk_buff *skb, skb_info_t *skb_info, http2_frame_with_offset *frames_array, __u8 original_index, http2_telemetry_t *http2_tel) {
    bool is_headers_or_rst_frame, is_data_end_of_stream;
    struct http2_frame current_frame = {};

    // We may have found a relevant frame already in http2_handle_first_frame,
    // so we need to adjust the index accordingly. We do not set
    // interesting_frame_index to original_index directly, as this will confuse
    // the verifier, leading it into thinking the index could have an arbitrary
    // value.
    __u8 interesting_frame_index = original_index == 1;

    __u32 iteration = 0;
#pragma unroll(HTTP2_MAX_FRAMES_TO_FILTER)
    for (; iteration < HTTP2_MAX_FRAMES_TO_FILTER; ++iteration) {
        // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
        if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb_info->data_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)&current_frame, HTTP2_FRAME_HEADER_SIZE);
        skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
        if (!format_http2_frame_header(&current_frame)) {
            break;
        }

        // END_STREAM can appear only in Headers and Data frames.
        // Check out https://datatracker.ietf.org/doc/html/rfc7540#section-6.1 for data frame, and
        // https://datatracker.ietf.org/doc/html/rfc7540#section-6.2 for headers frame.
        is_headers_or_rst_frame = current_frame.type == kHeadersFrame || current_frame.type == kRSTStreamFrame;
        is_data_end_of_stream = ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) && (current_frame.type == kDataFrame);
        if (interesting_frame_index < HTTP2_MAX_FRAMES_ITERATIONS && (is_headers_or_rst_frame || is_data_end_of_stream)) {
            frames_array[interesting_frame_index].frame = current_frame;
            frames_array[interesting_frame_index].offset = skb_info->data_off;
            interesting_frame_index++;
        }
        skb_info->data_off += current_frame.length;
    }

    // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb - if we can, update telemetry to indicate we have
    if ((iteration == HTTP2_MAX_FRAMES_TO_FILTER) && (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE <= skb_info->data_end)) {
        __sync_fetch_and_add(&http2_tel->exceeding_max_frames_to_filter, 1);
    }

    if (interesting_frame_index == HTTP2_MAX_FRAMES_ITERATIONS) {
        __sync_fetch_and_add(&http2_tel->exceeding_max_interesting_frames, 1);
    }

    return interesting_frame_index;
}

SEC("socket/http2_handle_first_frame")
int socket__http2_handle_first_frame(struct __sk_buff *skb) {
    const __u32 zero = 0;
    struct http2_frame current_frame = {};

    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    // We're not calling fetch_dispatching_arguments as, we need to modify the `data_off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    bpf_memcpy(&dispatcher_args_copy.tup, &args->tup, sizeof(conn_tuple_t));
    bpf_memcpy(&dispatcher_args_copy.skb_info, &args->skb_info, sizeof(skb_info_t));

    // If we detected a tcp termination we should stop processing the packet, and clear its dynamic table by deleting the counter.
    if (is_tcp_termination(&dispatcher_args_copy.skb_info)) {
        // Deleting the entry for the original tuple.
        bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
        terminated_http2_batch_enqueue(&dispatcher_args_copy.tup);
        // In case of local host, the protocol will be deleted for both (client->server) and (server->client),
        // so we won't reach for that path again in the code, so we're deleting the opposite side as well.
        flip_tuple(&dispatcher_args_copy.tup);
        bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
        return 0;
    }

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *iteration_value = bpf_map_lookup_elem(&http2_frames_to_process, &zero);
    if (iteration_value == NULL) {
        return 0;
    }
    iteration_value->frames_count = 0;
    iteration_value->iteration = 0;

    // skip HTTP2 magic, if present
    skip_preface(skb, &dispatcher_args_copy.skb_info);

    frame_header_remainder_t *frame_state = bpf_map_lookup_elem(&http2_remainder, &dispatcher_args_copy.tup);

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        return 0;
    }

    if (!get_first_frame(skb, &dispatcher_args_copy.skb_info, frame_state, &current_frame, http2_tel)) {
        return 0;
    }

    // If we have a state and we consumed it, then delete it.
    if (frame_state != NULL && frame_state->remainder == 0) {
        bpf_map_delete_elem(&http2_remainder, &dispatcher_args_copy.tup);
    }

    bool is_headers_or_rst_frame = current_frame.type == kHeadersFrame || current_frame.type == kRSTStreamFrame;
    bool is_data_end_of_stream = ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) && (current_frame.type == kDataFrame);
    if (is_headers_or_rst_frame || is_data_end_of_stream) {
        iteration_value->frames_array[0].frame = current_frame;
        iteration_value->frames_array[0].offset = dispatcher_args_copy.skb_info.data_off;
        iteration_value->frames_count = 1;
    }
    dispatcher_args_copy.skb_info.data_off += current_frame.length;
    // Overriding the data_off field of the cached skb_info. The next prog will start from the offset of the next valid
    // frame.
    args->skb_info.data_off = dispatcher_args_copy.skb_info.data_off;

    bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_FILTER);
    return 0;
}

SEC("socket/http2_filter")
int socket__http2_filter(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        return 0;
    }

    const __u32 zero = 0;

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *iteration_value = bpf_map_lookup_elem(&http2_frames_to_process, &zero);
    if (iteration_value == NULL) {
        return 0;
    }

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        return 0;
    }

    // Some functions might change and override fields in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, having a local copy of skb_info.
    skb_info_t local_skb_info = dispatcher_args_copy.skb_info;

    // The verifier cannot tell if `iteration_value->frames_count` is 0 or 1, so we have to help it. The value is
    // 1 if we have found an interesting frame in `socket__http2_handle_first_frame`, otherwise it is 0.
    // filter frames
    iteration_value->frames_count = find_relevant_frames(skb, &local_skb_info, iteration_value->frames_array, iteration_value->frames_count, http2_tel);

    frame_header_remainder_t new_frame_state = { 0 };
    if (local_skb_info.data_off > local_skb_info.data_end) {
        // We have a remainder
        new_frame_state.remainder = local_skb_info.data_off - local_skb_info.data_end;
        bpf_map_update_elem(&http2_remainder, &dispatcher_args_copy.tup, &new_frame_state, BPF_ANY);
    }

    if (local_skb_info.data_off < local_skb_info.data_end && local_skb_info.data_off + HTTP2_FRAME_HEADER_SIZE > local_skb_info.data_end) {
        // We have a frame header remainder
        new_frame_state.remainder = HTTP2_FRAME_HEADER_SIZE - (local_skb_info.data_end - local_skb_info.data_off);
        bpf_memset(new_frame_state.buf, 0, HTTP2_FRAME_HEADER_SIZE);
    #pragma unroll(HTTP2_FRAME_HEADER_SIZE)
        for (__u32 iteration = 0; iteration < HTTP2_FRAME_HEADER_SIZE && new_frame_state.remainder + iteration < HTTP2_FRAME_HEADER_SIZE; ++iteration) {
            bpf_skb_load_bytes(skb, local_skb_info.data_off + iteration, new_frame_state.buf + iteration, 1);
        }
        new_frame_state.header_length = HTTP2_FRAME_HEADER_SIZE - new_frame_state.remainder;
        bpf_map_update_elem(&http2_remainder, &dispatcher_args_copy.tup, &new_frame_state, BPF_ANY);
    }

    if (iteration_value->frames_count == 0) {
        return 0;
    }

    // We have couple of interesting headers, launching tail calls to handle them.
    if (bpf_map_update_elem(&http2_iterations, &dispatcher_args_copy, iteration_value, BPF_NOEXIST) >= 0) {
        // We managed to cache the iteration_value in the http2_iterations map.
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_PARSER);
    }

    return 0;
}

SEC("socket/http2_frames_parser")
int socket__http2_frames_parser(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        return 0;
    }

    // Some functions might change and override data_off field in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, storing the original value of the offset.
    __u32 original_off = dispatcher_args_copy.skb_info.data_off;

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&http2_iterations, &dispatcher_args_copy);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    const __u32 zero = 0;
    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        goto delete_iteration;
    }

    http2_frame_with_offset *frames_array = tail_call_state->frames_array;
    http2_frame_with_offset current_frame;

    // create the http2 ctx for the current http2 frame.
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);
    http2_ctx->dynamic_index.tup = dispatcher_args_copy.tup;

    #pragma unroll(HTTP2_FRAMES_PER_TAIL_CALL)
    for (__u8 index = 0; index < HTTP2_FRAMES_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }

        current_frame = frames_array[tail_call_state->iteration];
        // Having this condition after assignment and not before is due to a verifier issue.
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        tail_call_state->iteration += 1;

        dispatcher_args_copy.skb_info.data_off = current_frame.offset;

        parse_frame(skb, &dispatcher_args_copy.skb_info, &dispatcher_args_copy.tup, http2_ctx, &current_frame.frame, http2_tel);
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS && tail_call_state->iteration < tail_call_state->frames_count) {
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_PARSER);
    }

delete_iteration:
    // restoring the original value.
    dispatcher_args_copy.skb_info.data_off = original_off;
    bpf_map_delete_elem(&http2_iterations, &dispatcher_args_copy);

    return 0;
}

#endif
