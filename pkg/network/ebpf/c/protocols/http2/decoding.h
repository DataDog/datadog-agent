#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "map-defs.h"
#include "ip.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/maps-defs.h"
#include "protocols/http/types.h"
#include "protocols/classification/defs.h"
#include "protocols/events.h"

USM_EVENTS_INIT(http2, http2_stream_t, HTTP2_BATCH_SIZE);

// returns true if the given index is one of the relevant headers we care for in the static table.
// The full table can be found in the user mode code `createStaticTable`.
static __always_inline bool is_interesting_static_entry(__u64 index) {
    return (1 < index && index < 6) || (7 < index && index < 15);
}

// returns true if the given index is below MAX_STATIC_TABLE_INDEX.
static __always_inline bool is_static_table_entry(__u64 index) {
    return index <= MAX_STATIC_TABLE_INDEX;
}

// http2_fetch_stream returns the current http2 in flight stream.
static __always_inline http2_stream_t *http2_fetch_stream(http2_stream_key_t *http2_stream_key) {
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
    bpf_map_update_with_telemetry(http2_in_flight, http2_stream_key, http2_stream_ptr, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_in_flight, http2_stream_key);
}

// read_var_int reads an unsigned variable length integer off the
// beginning of p. n is the parameter as described in
// https://httpwg.org/specs/rfc7541.html#rfc.section.5.1.
//
// n must always be between 1 and 8.
//
// The returned remain buffer is either a smaller suffix of p, or err != nil.
static __always_inline bool read_var_int_2(struct __sk_buff *skb, skb_info_t *skb_info, __u8 first, __u8 bits, __u8 *out){
    __u8 current_char_as_number = first;

    const u8 max_number_for_bits = (1 << bits) - 1;
    current_char_as_number &= max_number_for_bits;

    if (current_char_as_number < max_number_for_bits) {
        *out = current_char_as_number;
        return true;
    }

    if (skb_info->data_off > skb->len) {
        return false;
    }

    __u8 b = 0;
    bpf_skb_load_bytes(skb, skb_info->data_off, &b, sizeof(b));
    current_char_as_number += b & 127;
    if ((b & 128 ) == 0) {
        skb_info->data_off++;
        *out = current_char_as_number;
        return true;
    }

    return false;
}

static __always_inline bool read_var_int(struct __sk_buff *skb, skb_info_t *skb_info, __u8 bits, __u8 *out){
    if (skb_info->data_off > skb->len) {
        return false;
    }

    __u8 current_char_as_number = 0;
    bpf_skb_load_bytes(skb, skb_info->data_off, &current_char_as_number, sizeof(current_char_as_number));
    skb_info->data_off++;

    return read_var_int_2(skb, skb_info, current_char_as_number, bits, out);
}

//get_dynamic_counter returns the current dynamic counter by the conn tup.
static __always_inline __u64 get_dynamic_counter(conn_tuple_t *tup) {
    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
    if (counter_ptr != NULL) {
        return *counter_ptr;
    }
    return 0;
}

// set_dynamic_counter is updating the current dynamic counter of the given tup.
static __always_inline void set_dynamic_counter(conn_tuple_t *tup, __u64 counter) {
    bpf_map_update_elem(&http2_dynamic_counter_table, tup, &counter, BPF_ANY);
}

// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline parse_result_t parse_field_indexed(struct __sk_buff *skb, skb_info_t *skb_info, __u64 global_counter, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u8 first){
    __u8 index = 0;
    if (!read_var_int_2(skb, skb_info, first, 7, &index)) {
        return HEADER_ERROR;
    }

    // TODO: can improve by declaring MAX_INTERESTING_STATIC_TABLE_INDEX
    if (is_static_table_entry(index)) {
        if (!is_interesting_static_entry(index)) {
            return HEADER_NOT_INTERESTING;
        }
        if (headers_to_process != NULL) {
            headers_to_process->index = index;
            headers_to_process->type = kStaticHeader;
        }
        return HEADER_INTERESTING;
    }

    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    http2_ctx->dynamic_index.index = global_counter - (index - MAX_STATIC_TABLE_INDEX);

    if (bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index) == NULL) {
        return HEADER_NOT_INTERESTING;
    }
    if (headers_to_process != NULL) {
        headers_to_process->index = http2_ctx->dynamic_index.index;
        headers_to_process->type = kDynamicHeader;
    }
    return HEADER_INTERESTING;
}

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline parse_result_t parse_field_literal(struct __sk_buff *skb, skb_info_t *skb_info, __u64 counter, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u8 first){
    __u8 index = 0;
    if (!read_var_int_2(skb, skb_info, first, 6, &index)) {
        return HEADER_ERROR;
    }

    // Read the value
    __u8 str_len = 0;
    if (!read_var_int(skb, skb_info, 6, &str_len)) {
        return HEADER_ERROR;
    }

    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        if (!read_var_int(skb, skb_info, 6, &str_len)) {
            return HEADER_ERROR;
        }
        skb_info->data_off += str_len;
        return HEADER_NOT_INTERESTING;
    } else if (is_static_table_entry(index) && is_interesting_static_entry(index)) {
        // if the index does not appear in our static table, then it is not relevant for us
        skb_info->data_off += str_len;
        return HEADER_NOT_INTERESTING;
    } else if (str_len >= HTTP2_MAX_PATH_LEN || index != kIndexPath){
        // if the index is not path or the len of string is bigger then we support, we continue.
        skb_info->data_off += str_len;
        return HEADER_NOT_INTERESTING;
    }

//    const __u16 offset = heap_buffer->offset < HTTP2_BUFFER_SIZE - 1 ? heap_buffer->offset : HTTP2_BUFFER_SIZE - 1;
//    skb_info->data_off += str_len;
//
//    if (offset >= HTTP2_BUFFER_SIZE - HTTP2_MAX_PATH_LEN) {
//        return HEADER_NOT_INTERESTING;
//    }
//    dynamic_table_entry_t dynamic_value = {};
//    dynamic_value.string_len = str_len;

    // create the new dynamic value which will be added to the internal table.
//    bpf_memcpy(dynamic_value.buffer, &heap_buffer->fragment[offset % HTTP2_BUFFER_SIZE], HTTP2_MAX_PATH_LEN);

//    http2_ctx->dynamic_index.index = counter - 1;
//    bpf_map_update_elem(&http2_dynamic_table, &http2_ctx->dynamic_index, &dynamic_value, BPF_ANY);

    if (headers_to_process != NULL) {
        headers_to_process->index = counter - 1;
        headers_to_process->type = kDynamicHeader;
    }
    return HEADER_INTERESTING;
}

// This function reads the http2 headers frame.
static __always_inline __u8 filter_relevant_headers(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup,
    http2_ctx_t *http2_ctx, http2_header_t *headers_to_process) {
    char current_ch;
    __u8 interesting_headers = 0;
    parse_result_t res;
    http2_header_t *current_header;

    __u64 counter = get_dynamic_counter(tup);

#pragma unroll (HTTP2_MAX_HEADERS_COUNT)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT; ++headers_index) {
        if (skb_info->data_off > skb->len) {
            break;
        }
        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off++;

        current_header = NULL;
        if (interesting_headers < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING) {
            current_header = &headers_to_process[interesting_headers];
        }

        if ((current_ch&128) != 0) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            res = parse_field_indexed(skb, skb_info, counter, http2_ctx, current_header, current_ch);
        } else if ((current_ch&192) == 64) {
            counter++;
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 11
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            res = parse_field_literal(skb, skb_info, counter, http2_ctx, current_header, current_ch);
        } else {
            continue;
        }

        if (res == HEADER_ERROR) {
            break;
        } else if (res == HEADER_INTERESTING) {
            interesting_headers++;
        }
    }

    set_dynamic_counter(tup, counter);
    return interesting_headers;
}

static __always_inline void process_headers(http2_ctx_t *http2_ctx, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers) {
    http2_header_t *current_header;

#pragma unroll (HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING; ++iteration) {
        if (iteration >= interesting_headers) {
            break;
        }

        current_header = &headers_to_process[iteration];

        if (current_header->type == kStaticHeader) {
            // fetch static value
            static_table_entry_t* static_value = bpf_map_lookup_elem(&http2_static_table, &current_header->index);
            if (static_value == NULL) {
                break;
            }

            if (static_value->key == kMethod){
                current_stream->request_started = bpf_ktime_get_ns();
                // TODO: Can be done in the user mode.
                switch(static_value->value) {
                case kGET:
                    current_stream->request_method = HTTP_GET;
                    break;
                case kPOST:
                    current_stream->request_method = HTTP_POST;
                    break;
                default:
                    break;
                }
            } else if (static_value->key == kStatus) {
                // TODO: Can be done in the user mode.
                switch(static_value->value) {
                case k200:
                    current_stream->response_status_code = 200;
                    break;
                case k204:
                    current_stream->response_status_code = 204;
                    break;
                case k206:
                    current_stream->response_status_code = 206;
                    break;
                case k400:
                    current_stream->response_status_code = 400;
                    break;
                case k500:
                    current_stream->response_status_code = 500;
                    break;
                default:
                    break;
                }
            }
        } else if (current_header->type == kDynamicHeader) {
            http2_ctx->dynamic_index.index = current_header->index;
            dynamic_table_entry_t* dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index);
            if (dynamic_value == NULL) {
                break;
            }
            // TODO: reuse same struct
            current_stream->path_size = dynamic_value->string_len;
            bpf_memcpy(current_stream->request_path, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
        }
    }
}

static __always_inline void handle_end_of_stream(http2_stream_key_t *http2_stream_key_template, http2_stream_t *current_stream) {
    if (!current_stream->request_end_of_stream) {
        current_stream->request_end_of_stream = true;
        return;
    }

    // response end of stream;
    current_stream->response_last_seen = bpf_ktime_get_ns();
    current_stream->tup = http2_stream_key_template->tup;

    // enqueue
    http2_batch_enqueue(current_stream);
    bpf_map_delete_elem(&http2_in_flight, http2_stream_key_template);
}

static __always_inline void process_headers_frame(struct __sk_buff *skb, http2_stream_t *current_stream, http2_iterations_key_t *iterations_key, http2_ctx_t *http2_ctx, struct http2_frame *current_frame_header) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = filter_relevant_headers(skb, &iterations_key->skb_info, &iterations_key->tup, http2_ctx, headers_to_process);
    if (interesting_headers > 0) {
        process_headers(http2_ctx, current_stream, headers_to_process, interesting_headers);
    }
}

static __always_inline __u32 http2_entrypoint(struct __sk_buff *skb, http2_iterations_key_t *iterations_key, http2_ctx_t *http2_ctx) {
    __u32 offset = iterations_key->skb_info.data_off;
    // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
    if (offset + HTTP2_FRAME_HEADER_SIZE > skb->len) {
        // TODO: fix return code
        return -1;
    }

    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    bpf_memset((char*)frame_buf, 0, sizeof(frame_buf));

    // read frame.
    bpf_skb_load_bytes_with_telemetry(skb, offset, frame_buf, HTTP2_FRAME_HEADER_SIZE);
    offset += HTTP2_FRAME_HEADER_SIZE;

    struct http2_frame current_frame = {};
    if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &current_frame)){
        log_debug("[http2] unable to read_http2_frame_header offset %lu\n", offset);
        // TODO: fix return code
        return -1;
    }

    bool is_end_of_stream = (current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
    bool is_data_end_of_stream = current_frame.type == kDataFrame && is_end_of_stream;
    if (current_frame.type != kHeadersFrame && !is_data_end_of_stream) {
        // Should not process the frame.
        return HTTP2_FRAME_HEADER_SIZE + current_frame.length;
    }

    http2_ctx->http2_stream_key.stream_id = current_frame.stream_id;
    http2_stream_t *current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
    if (current_stream == NULL) {
        return HTTP2_FRAME_HEADER_SIZE + current_frame.length;
    }

    if (current_frame.type == kHeadersFrame) {
        process_headers_frame(skb, current_stream, iterations_key, http2_ctx, &current_frame);
    }

    if (is_end_of_stream) {
        handle_end_of_stream(&http2_ctx->http2_stream_key, current_stream);
    }

    return HTTP2_FRAME_HEADER_SIZE + current_frame.length;
}

#endif
