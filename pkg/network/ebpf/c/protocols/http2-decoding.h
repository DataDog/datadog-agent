#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Format" section
#define HTTP2_SETTINGS_SIZE 6

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "http2-defs.h"
#include "http2-maps-defs.h"
#include "http2-maps-defs-classify.h"
#include "http-types.h"
#include "protocol-classification-defs.h"
#include "bpf_telemetry.h"
#include "ip.h"

/* thread_struct id too big for allocation on stack in eBPF function, we use an array as a heap allocator */
BPF_PERCPU_ARRAY_MAP(http2_trans_alloc, __u32, http2_transaction_t, 1)
BPF_PERCPU_ARRAY_MAP(http_trans_alloc, __u32, http_transaction_t, 1)

static __always_inline http2_transaction_t *http2_fetch_state(http2_transaction_t *http2, http2_packet_t packet_type) {
    if (packet_type == HTTP_PACKET_UNKNOWN) {
        return bpf_map_lookup_elem(&http2_in_flight, &http2->tup);
    }

    // We detected either a request or a response
    // In this case we initialize (or fetch) state associated to this tuple
    bpf_map_update_with_telemetry(http2_in_flight, &http2->tup, http2, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_in_flight, &http2->tup);
}

static __always_inline bool http2_seen_before(http2_transaction_t *http2, skb_info_t *skb_info) {
    if (!skb_info || !skb_info->tcp_seq) {
        return false;
    }

    // check if we've seen this TCP segment before. this can happen in the
    // context of localhost traffic where the same TCP segment can be seen
    // multiple times coming in and out from different interfaces
    return http2->tcp_seq == skb_info->tcp_seq;
}

static __always_inline void http2_update_seen_before(http2_transaction_t *http2, skb_info_t *skb_info) {
    if (!skb_info || !skb_info->tcp_seq) {
        return;
    }

    http2->tcp_seq = skb_info->tcp_seq;
}

static __always_inline void http2_begin_request(http2_transaction_t *http2, http2_method_t method, char *buffer) {
//    http2->request_method = method;
    http2->request_started = bpf_ktime_get_ns();
    http2->response_last_seen = 0;
    bpf_memcpy(&http2->request_fragment, buffer, HTTP2_BUFFER_SIZE);
}

static __always_inline int http2_responding(http2_transaction_t *http2) {
    return (http2 != NULL && http2->response_status_code != 0);
}

static __always_inline int http2_process(http2_transaction_t* http2_stack,  skb_info_t *skb_info,__u64 tags) {
    http2_packet_t packet_type = HTTP2_PACKET_UNKNOWN;
    http2_method_t method = HTTP2_METHOD_UNKNOWN;
    __u64 response_code;

    if (http2_stack->packet_type > 0) {
        packet_type = http2_stack->packet_type;
    }

    if (packet_type != 0){
        log_debug("[http2] ----------------------------------\n");
        log_debug("[http2] The method is %d\n", method);
        log_debug("[http2] The packet_type is %d\n", packet_type);
        log_debug("[http2] the response status code is %d\n", http2_stack->response_status_code);
        log_debug("[http2] the end of stream is %d\n", http2_stack->end_of_stream);
        log_debug("[http2] ----------------------------------\n");
    }

    if (packet_type == 3) {
        packet_type = HTTP2_RESPONSE;
    } else if (packet_type == 2) {
        packet_type = HTTP2_REQUEST;
    }

    http2_transaction_t *http2 = http2_fetch_state(http2_stack, packet_type);
    if (!http2 || http2_seen_before(http2, skb_info)) {
        log_debug("[http2] the http2 has been seen before!\n");
        return 0;
    }

    if (packet_type == HTTP2_REQUEST) {
        log_debug("[http2] http2_process request: type=%d method=%d\n", packet_type, method);
        http2_begin_request(http2, method, (char *)http2_stack->request_fragment);
        http2_update_seen_before(http2, skb_info);
    } else if (packet_type == HTTP2_RESPONSE) {
        log_debug("[http2] http2_begin_response: htx=%llx status=%d\n", http2, http2->response_status_code);
        http2_update_seen_before(http2, skb_info);
    }

    if (http2_stack->response_status_code > 0) {
        http2_transaction_t *trans = bpf_map_lookup_elem(&http2_in_flight, &http2->old_tup);
        if (trans != NULL) {
            const __u32 zero = 0;
            http_transaction_t *http = bpf_map_lookup_elem(&http_trans_alloc, &zero);
            if (http == NULL) {
                return 0;
            }
            bpf_memset(http, 0, sizeof(http_transaction_t));
            bpf_memcpy(&http->tup, &trans->tup, sizeof(conn_tuple_t));

            http->request_fragment[0] = 'z';
            http->request_fragment[1] = http2->path_size;
            bpf_memcpy(&http->request_fragment[8], trans->path, HTTP2_MAX_PATH_LEN);

            // todo: take it out to a function?!
            if (trans->request_method == 2) {
                log_debug("[slavin] found http2 get");
                method = HTTP2_GET;
            } else if (trans->request_method == 3) {
                log_debug("[slavin] found http2 post");
                method = HTTP2_POST;
            }

            // todo: take it out to a function and add all the other options as well.
            switch(http2_stack->response_status_code) {
            case k200: response_code = 200;
            case k204: response_code = 204;
            case k206: response_code = 206;
            case k400: response_code = 400;
            case k500: response_code = 500;
            }

            http->response_status_code = response_code;
            http->request_started = trans->request_started;
            http->request_method = method;
            http->response_last_seen = bpf_ktime_get_ns();
            http->owned_by_src_port = trans->owned_by_src_port;
            http->tcp_seq = trans->tcp_seq;
            http->tags = trans->tags;

            http_enqueue(http);
//            bpf_map_delete_elem(&http2_in_flight, &http2_stack->tup);
        }
    }

    return 0;
}

// read_var_int reads an unsigned variable length integer off the
// beginning of p. n is the parameter as described in
// https://httpwg.org/specs/rfc7541.html#rfc.section.5.1.
//
// n must always be between 1 and 8.
//
// The returned remain buffer is either a smaller suffix of p, or err != nil.
// The error is errNeedMore if p doesn't contain a complete integer.
static __always_inline __u64 read_var_int(http2_transaction_t* http2_transaction, __u64 factor, __u64 current_char_as_number){
    if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
        return false;
    }

    current_char_as_number &= (1 << factor) - 1;

    if (current_char_as_number < (1 << factor) - 1) {
        http2_transaction->current_offset_in_request_fragment += 1;
        return current_char_as_number;
    }

    // TODO: compare with original code if needed.
    return -1;
}

static __always_inline bool classify_static_value(http2_transaction_t* http2_transaction, static_table_value* static_value){
     header_value value = static_value->value;
     header_key name = static_value->name;

     if ((name == kMethod) && (value == kPOST)){
        http2_transaction->request_method = value;
        http2_transaction->packet_type = 2; // this will be request and we need to make it better
        return true;
     }
     if ((value <= k500) && (value >= k200)) {
        http2_transaction->packet_type = 3; // this will be response type
        http2_transaction->response_status_code = value;
        return true;
     }

     return false;
}

// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline void parse_field_indexed(http2_transaction_t* http2_transaction, __u64 current_char_as_number){
    __u64 index = read_var_int(http2_transaction, 7, current_char_as_number);
    if (index <= 61) {
        static_table_value* static_value = bpf_map_lookup_elem(&http2_static_table, &index);
        if (static_value != NULL) {
            classify_static_value(http2_transaction, static_value);
        }
        return;
    }

    __u64 *global_counter = bpf_map_lookup_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup);
    if (global_counter == NULL) {
        return;
    }
    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    __u64 new_index = *global_counter - (index - 62);
    dynamic_table_index dynamic_index = {};
    dynamic_index.index = new_index;
    dynamic_index.old_tup = http2_transaction->old_tup;

    dynamic_table_value *dynamic_value_new = bpf_map_lookup_elem(&http2_dynamic_table, &dynamic_index);
    if (dynamic_value_new == NULL) {
        return;
    }

    // index 5 represents the :path header - from dynamic table
    if ((dynamic_value_new->index == 5) && (sizeof(dynamic_value_new->value.buffer)>0)){
        bpf_memcpy(http2_transaction->path, dynamic_value_new->value.buffer, HTTP2_MAX_PATH_LEN);
        http2_transaction->path_size = dynamic_value_new->value.string_len;
    }
}

static __always_inline bool update_current_offset(http2_transaction_t* http2_transaction, __u64 current_char_as_number){
    __u64 str_len = read_var_int(http2_transaction, 6, current_char_as_number);
    if (str_len == -1) {
        return false;
    }
    http2_transaction->current_offset_in_request_fragment += (__u32)str_len;
    return true;
}

//// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
//// which will be stored in the dynamic table.
static __always_inline void parse_field_literal(http2_transaction_t* http2_transaction, bool index_type, __u64 current_char_as_number){
    __u64 counter = 0;

    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup);
    if (counter_ptr != NULL) {
        counter = *counter_ptr;
    }
    counter += 1;
    // update the global counter.
    bpf_map_update_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup, &counter, BPF_ANY);

     __u64 index = read_var_int(http2_transaction, 6, current_char_as_number);

    dynamic_table_value dynamic_value = {};
    dynamic_table_index dynamic_index = {};
    static_table_value *static_value = bpf_map_lookup_elem(&http2_static_table, &index);
    if (static_value == NULL) {
        update_current_offset(http2_transaction, current_char_as_number);

        // Literal Header Field with Incremental Indexing - New Name, which means we need to read the body as well,
        // so we are reading again and updating the len.
        // TODO: Better document.
        if (index == 0) {
            update_current_offset(http2_transaction, current_char_as_number);
        }
        return;
    }

    if (index_type) {
        dynamic_value.index = static_value->name;
    }

    __u64 str_len = read_var_int(http2_transaction, 7, current_char_as_number);
    if (str_len == -1 || str_len == 0){
        return;
    }

    if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
        return;
    }

    char *beginning = http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment;
    // create the new dynamic value which will be added to the internal table.
    bpf_memcpy(dynamic_value.value.buffer, beginning, HTTP2_MAX_PATH_LEN);


    dynamic_value.value.string_len = str_len;
    dynamic_value.index = index;

    // create the new dynamic index which is bashed on the counter and the conn_tup.
    dynamic_index.index = counter;
    dynamic_index.old_tup = http2_transaction->old_tup;

    bpf_map_update_elem(&http2_dynamic_table, &dynamic_index, &dynamic_value, BPF_ANY);

    http2_transaction->current_offset_in_request_fragment += (__u32)str_len;

    // index 5 represents the :path header - from static table
    if (index == 5){
        bpf_memcpy(http2_transaction->path, dynamic_value.value.buffer, HTTP2_MAX_PATH_LEN);
        http2_transaction->path_size = str_len;
    }
}

// This function reads the http2 headers frame.
static __always_inline bool process_headers(http2_transaction_t* http2_transaction) {
    __s64 remaining_length = 0;
    char current_ch;

#pragma unroll
    for (unsigned headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT; headers_index++) {
        remaining_length = (__s64)sizeof(http2_transaction->request_fragment) - (__s64)http2_transaction->current_offset_in_request_fragment;
        // TODO: if remaining_length == 0, just break and return true.
        if (remaining_length <= 0) {
            return false;
        }
        current_ch = http2_transaction->request_fragment[http2_transaction->current_offset_in_request_fragment];
        if ((current_ch&128) != 0) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            log_debug("[http2] first char %d & 128 != 0; calling parse_field_indexed", current_ch);
            parse_field_indexed(http2_transaction, current_ch);
        } else if ((current_ch&192) == 64) {
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 10
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            log_debug("[http2] first char %d & 192 == 64; calling parse_field_literal", current_ch);
            parse_field_literal(http2_transaction, true, current_ch);
        }
    }

    return true;
}

#define HTTP2_END_OF_STREAM 0x1

static __always_inline void process_frames(http2_transaction_t* http2_transaction, struct __sk_buff *skb) {
    struct http2_frame current_frame = {};
    bool is_end_of_stream;
    bool is_supported_frame;
    __s64 remaining_length = 0;

#pragma unroll
    for (uint32_t frame_index = 0; frame_index < HTTP2_MAX_FRAMES; frame_index++) {
        remaining_length = (__s64)sizeof(http2_transaction->request_fragment) - (__s64)http2_transaction->current_offset_in_request_fragment;
        // We have left less than frame header, nothing to read.
        if (HTTP2_FRAME_HEADER_SIZE > remaining_length) {
            return;
        }
        // Reading the header.
        if (!read_http2_frame_header(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment, HTTP2_FRAME_HEADER_SIZE, &current_frame)){
            return;
        }
        // Modifying the offset.
        http2_transaction->current_offset_in_request_fragment += HTTP2_FRAME_HEADER_SIZE;
        // Modifying the remaining length.
        remaining_length -= HTTP2_FRAME_HEADER_SIZE;

        log_debug("[http2] ----------\n");
        log_debug("[http2] length is %lu; type is %d\n", current_frame.length, current_frame.type);
        log_debug("[http2] flags are %d; stream id is %lu\n", current_frame.flags, current_frame.stream_id);
        log_debug("[http2] ----------\n");

        is_end_of_stream = (current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
        is_supported_frame = current_frame.type == kDataFrame || current_frame.type == kHeadersFrame;
        if (is_supported_frame && is_end_of_stream){
            log_debug("[http2] found end of stream %d\n", current_frame.stream_id);
            //TODO: handle_end_of_stream();
            http2_transaction->end_of_stream = true;
        }

        if (current_frame.type != kHeadersFrame) {
            log_debug("[http2] frame is not headers, thus skipping it\n");
            // Skipping the frame payload.
            http2_transaction->current_offset_in_request_fragment += (__u32)current_frame.length;
            continue;
        }

        // Checking we can process the entire frame.
        if (remaining_length < (__s64)current_frame.length) {
            log_debug("[http2] we have %lld remaining bytes in the buffer, while the frame's length is %d\n", remaining_length, current_frame.length);
            return;
        }
        // Process headers.
        process_headers(http2_transaction);
        // TODO: Remove when process_headers is completed.
        http2_transaction->current_offset_in_request_fragment += (__u32)current_frame.length;
    }
}

#endif
