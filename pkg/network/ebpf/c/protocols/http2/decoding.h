#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "ip.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/maps-defs.h"
#include "protocols/http2/read_into_buffer.h"
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
    bpf_map_update_elem(&http2_in_flight, http2_stream_key, http2_stream_ptr, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_in_flight, http2_stream_key);
}

// read_var_int reads an unsigned variable length integer off the
// beginning of p. n is the parameter as described in
// https://httpwg.org/specs/rfc7541.html#rfc.section.5.1.
//
// n must always be between 1 and 8.
//
// The returned remain buffer is either a smaller suffix of p, or err != nil.
static __always_inline bool read_var_int_2(struct __sk_buff *skb, skb_info_t *skb_info, __u8 current_char_as_number, __u8 max_number_for_bits, __u8 *out){
    current_char_as_number &= max_number_for_bits;

    if (current_char_as_number < max_number_for_bits) {
        *out = current_char_as_number;
        return true;
    }

    if (skb_info->data_off <= skb->len) {
        __u8 b = 0;
        bpf_skb_load_bytes(skb, skb_info->data_off, &b, sizeof(b));
        if ((b & 128 ) == 0) {
            skb_info->data_off++;
            *out = current_char_as_number + b & 127;
            return true;
        }
    }

    return false;
}

static __always_inline bool read_var_int(struct __sk_buff *skb, skb_info_t *skb_info, __u8 max_number_for_bits, __u8 *out){
    if (skb_info->data_off > skb->len) {
        return false;
    }
    __u8 current_char_as_number = 0;
    bpf_skb_load_bytes(skb, skb_info->data_off, &current_char_as_number, sizeof(current_char_as_number));
    skb_info->data_off++;

    return read_var_int_2(skb, skb_info, current_char_as_number, max_number_for_bits, out);
}

//get_dynamic_counter returns the current dynamic counter by the conn tup.
static __always_inline __u64* get_dynamic_counter(conn_tuple_t *tup) {
    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
    if (counter_ptr != NULL) {
        return counter_ptr;
    }
    __u64 counter = 0;
    bpf_map_update_elem(&http2_dynamic_counter_table, tup, &counter, BPF_ANY);
    return bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
}

// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline parse_result_t parse_field_indexed(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter){
    if (headers_to_process == NULL) {
        return HEADER_NOT_INTERESTING;
    }
    // TODO: can improve by declaring MAX_INTERESTING_STATIC_TABLE_INDEX
    if (is_interesting_static_entry(index)) {
        headers_to_process->index = index;
        headers_to_process->type = kStaticHeader;
        return HEADER_INTERESTING;
    }
    if (is_static_table_entry(index)) {
        return HEADER_NOT_INTERESTING;
    }

    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    http2_ctx->dynamic_index.index = global_dynamic_counter - (index - MAX_STATIC_TABLE_INDEX);

    if (bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index) == NULL) {
        return HEADER_NOT_INTERESTING;
    }

    headers_to_process->index = http2_ctx->dynamic_index.index;
    headers_to_process->type = kExistingDynamicHeader;
    return HEADER_INTERESTING;
}

READ_INTO_BUFFER(path, HTTP2_MAX_PATH_LEN, BLK_SIZE)

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline parse_result_t parse_field_literal(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter){
    __u8 str_len = 0;
    if (!read_var_int(skb, skb_info, MAX_6_BITS, &str_len)) {
        return HEADER_ERROR;
    }
    // The key is new and inserted into the dynamic table. So we are skipping the new value.

    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        if (!read_var_int(skb, skb_info, MAX_6_BITS, &str_len)) {
            return HEADER_ERROR;
        }
        goto not_interesting;
    }
    if (str_len >= HTTP2_MAX_PATH_LEN || index != kIndexPath || headers_to_process == NULL){
        goto not_interesting;
    }

    __u32 final_size = str_len < HTTP2_MAX_PATH_LEN ? str_len : HTTP2_MAX_PATH_LEN;
    if (skb_info->data_off + final_size > skb->len) {
        goto not_interesting;
    }

    headers_to_process->index = global_dynamic_counter - 1;
    headers_to_process->type = kNewDynamicHeader;
    headers_to_process->new_dynamic_value_offset = skb_info->data_off;
    headers_to_process->new_dynamic_value_size = str_len;
    skb_info->data_off += str_len;
    return HEADER_INTERESTING;
not_interesting:
    skb_info->data_off += str_len;
    return HEADER_NOT_INTERESTING;
}

// This function reads the http2 headers frame.
static __always_inline __u8 filter_relevant_headers(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 frame_length) {
    __u8 current_ch;
    __u8 interesting_headers = 0;
    parse_result_t res;
    http2_header_t *current_header;
    const __u32 frame_end = skb_info->data_off + frame_length;
    const __u32 end = frame_end < skb->len + 1 ? frame_end : skb->len + 1;
    bool is_literal = false;
    bool is_indexed = false;
    __u8 max_bits = 0;
    __u8 index = 0;

    __u64 *global_dynamic_counter = get_dynamic_counter(tup);
    if (global_dynamic_counter == NULL) {
        return 0;
    }

#pragma unroll (HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (skb_info->data_off >= end) {
            break;
        }
        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off++;

        is_indexed = (current_ch&128) != 0;
        is_literal = (current_ch&192) == 64;

        if (is_indexed) {
            max_bits = MAX_7_BITS;
        } else if (is_literal) {
            max_bits = MAX_6_BITS;
        } else {
            continue;
        }

        index = 0;
        if (!read_var_int_2(skb, skb_info, current_ch, max_bits, &index)) {
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
            res = parse_field_indexed(skb, skb_info, tup, http2_ctx, current_header, index, *global_dynamic_counter);
        } else {
            (*global_dynamic_counter)++;
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 11
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            res = parse_field_literal(skb, skb_info, tup, http2_ctx, current_header, index, *global_dynamic_counter);
        }

        if (res == HEADER_ERROR) {
            break;
        }
        interesting_headers += res == HEADER_INTERESTING;
    }

    return interesting_headers;
}

static __always_inline void process_headers(struct __sk_buff *skb, http2_ctx_t *http2_ctx, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers) {
    http2_header_t *current_header;
    dynamic_table_entry_t dynamic_value = {};

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
                // TODO: mark request
                current_stream->request_started = bpf_ktime_get_ns();
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
        } else if (current_header->type == kExistingDynamicHeader) {
            http2_ctx->dynamic_index.index = current_header->index;
            dynamic_table_entry_t* dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index);
            if (dynamic_value == NULL) {
                break;
            }
            current_stream->path_size = dynamic_value->string_len;
            bpf_memcpy(current_stream->request_path, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
        } else if (current_header->type == kNewDynamicHeader) {
            dynamic_value.string_len = current_header->new_dynamic_value_size;

            // create the new dynamic value which will be added to the internal table.
            read_into_buffer_path(dynamic_value.buffer, skb, current_header->new_dynamic_value_offset);
            http2_ctx->dynamic_index.index = current_header->index;
            bpf_map_update_elem(&http2_dynamic_table, &http2_ctx->dynamic_index, &dynamic_value, BPF_ANY);
            current_stream->path_size = current_header->new_dynamic_value_size;
            bpf_memcpy(current_stream->request_path, dynamic_value.buffer, HTTP2_MAX_PATH_LEN);
        }
    }
}

static __always_inline void handle_end_of_stream(http2_stream_t *current_stream, http2_stream_key_t *http2_stream_key_template) {
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

static __always_inline void process_headers_frame(struct __sk_buff *skb, http2_stream_t *current_stream, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx, struct http2_frame *current_frame_header) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = filter_relevant_headers(skb, skb_info, tup, http2_ctx, headers_to_process, current_frame_header->length);
    if (interesting_headers > 0) {
        process_headers(skb, http2_ctx, current_stream, headers_to_process, interesting_headers);
    }
}

static __always_inline bool http2_entrypoint(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx) {
    // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
    if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb->len) {
        return false;
    }

    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    bpf_memset((char*)frame_buf, 0, sizeof(frame_buf));

    // read frame.
    bpf_skb_load_bytes(skb, skb_info->data_off, frame_buf, HTTP2_FRAME_HEADER_SIZE);
    skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;

    struct http2_frame current_frame = {};
    if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &current_frame)){
        log_debug("[http2] unable to read_http2_frame_header offset %lu\n", skb_info->data_off);
        return false;
    }

    bool is_headers_frame = current_frame.type == kHeadersFrame;
    bool is_end_of_stream = (current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
    bool is_data_end_of_stream = current_frame.type == kDataFrame && is_end_of_stream;
    if (!is_headers_frame && !is_data_end_of_stream) {
        // Should not process the frame.
        skb_info->data_off += current_frame.length;
        return true;
    }

    http2_ctx->http2_stream_key.stream_id = current_frame.stream_id;
    http2_stream_t *current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
    if (current_stream == NULL) {
        skb_info->data_off += current_frame.length;
        return true;
    }

    if (is_headers_frame) {
        process_headers_frame(skb, current_stream, skb_info, tup, http2_ctx, &current_frame);
    } else {
        skb_info->data_off += current_frame.length;
    }

    if (is_end_of_stream) {
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key);
    }

    return true;
}

SEC("socket/http2_filter")
int socket__http2_filter(struct __sk_buff *skb) {
    const __u32 zero = 0;
    dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
    if (args == NULL) {
        log_debug("http2_filter failed to fetch arguments for tail call\n");
        return 0;
    }
    dispatcher_arguments_t iterations_key;
    bpf_memcpy(&iterations_key, args, sizeof(dispatcher_arguments_t));

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&http2_iterations, &iterations_key);
    if (tail_call_state == NULL) {
        http2_tail_call_state_t iteration_value = {};
        iteration_value.offset = iterations_key.skb_info.data_off;
        bpf_map_update_elem(&http2_iterations, &iterations_key, &iteration_value, BPF_NOEXIST);
        tail_call_state = bpf_map_lookup_elem(&http2_iterations, &iterations_key);
        if (tail_call_state == NULL) {
            return 0;
        }
    }

    // If we detected a tcp termination we should stop processing the packet, and clear its dynamic table by deleting the counter.
    if (is_tcp_termination(&iterations_key.skb_info)) {
        bpf_map_delete_elem(&http2_dynamic_counter_table, &iterations_key.tup);
        goto delete_iteration;
    }

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }

    // create the http2 ctx for the current http2 frame.
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = iterations_key.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);
    http2_ctx->dynamic_index.tup = iterations_key.tup;
    iterations_key.skb_info.data_off = tail_call_state->offset;

    // perform the http2 decoding part.
    if (!http2_entrypoint(skb, &iterations_key.skb_info, &iterations_key.tup, http2_ctx)) {
        goto delete_iteration;
    }
    if (iterations_key.skb_info.data_off >= skb->len) {
        goto delete_iteration;
    }

    // update the tail calls state when the http2 decoding part was completed successfully.
    tail_call_state->iteration += 1;
    tail_call_state->offset = iterations_key.skb_info.data_off;
    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS) {
        bpf_tail_call_compat(skb, &protocols_progs, PROTOCOL_HTTP2);
    }

delete_iteration:
    bpf_map_delete_elem(&http2_iterations, &iterations_key);

    return 0;
}

#endif
