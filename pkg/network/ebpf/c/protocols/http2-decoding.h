#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Format" section
#define HTTP2_FRAME_HEADER_SIZE 9
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

// Struct which represent the http2 frame by its fields.

static __always_inline void http2_parse_data(char const *p, http_packet_t *packet_type, http_method_t *method) {
    // parse the http2 data over here?!
}

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

static __always_inline int http2_process(http2_transaction_t* http2_stack,  skb_info_t *skb_info,__u64 tags) {
//    char *buffer = (char *)http2_stack->request_fragment;
    http2_packet_t packet_type = HTTP2_PACKET_UNKNOWN;
    http2_method_t method = HTTP2_METHOD_UNKNOWN;
    http2_schema_t schema = HTTP2_SCHEMA_UNKNOWN;

    if (http2_stack->request_method > 0) {
        method = http2_stack->request_method;
    }

    if (http2_stack->packet_type > 0) {
        packet_type = http2_stack->packet_type;
    }

    if (http2_stack->schema > 0) {
        schema = http2_stack->schema;
    }

    if (packet_type != 0){
        log_debug("[tasik] ----------------------------------");
        log_debug("[tasik] XXXXXXXXXXXXXXXXXXXXXXXXXX The method is %d", method);
        log_debug("[tasik] XXXXXXXXXXXXXXXXXXXXXXXXXX The packet_type is %d", packet_type);
        log_debug("[tasik] XXXXXXXXXXXXXXXXXXXXXXXXXX The schema is %d", schema);
        log_debug("[tasik] ----------------------------------");
    }

    if (packet_type == 2) {
        packet_type = HTTP2_REQUEST;
             log_debug("[tasik] ------------ first char of the path bla in 0 spot is %c", http2_stack->path[0]);
             log_debug("[tasik] ------------ first char of the path bla in 1 spot is %c", http2_stack->path[1]);
             log_debug("[tasik] ------------ first char of the path bla in 2 spot is %c", http2_stack->path[2]);
             log_debug("[tasik] ------------ first char of the path bla in 3 spot is %c", http2_stack->path[3]);
             log_debug("[tasik] ------------ first char of the path bla in 4 spot is %c", http2_stack->path[4]);
             log_debug("[tasik] ------------ first char of the path bla in 5 spot is %c", http2_stack->path[5]);
             log_debug("[tasik] ----------------------------------");

             log_debug("[tasik] ------------ first char of the authority bla in 0 spot is %c", http2_stack->authority[0]);
             log_debug("[tasik] ------------ first char of the authority bla in 1 spot is %c", http2_stack->authority[1]);
             log_debug("[tasik] ------------ first char of the authority bla in 2 spot is %c", http2_stack->authority[2]);
             log_debug("[tasik] ------------ first char of the authority bla in 3 spot is %c", http2_stack->authority[3]);
             log_debug("[tasik] ------------ first char of the authority bla in 4 spot is %c", http2_stack->authority[4]);
             log_debug("[tasik] ------------ first char of the authority bla in 5 spot is %c", http2_stack->authority[5]);
    }

    http2_transaction_t *http2 = http2_fetch_state(http2_stack, packet_type);
    if (!http2 || http2_seen_before(http2, skb_info)) {
        return 0;
    }
//
//    log_debug("http2_process: type=%d method=%d\n", packet_type, method);
//    if (packet_type == HTTP_REQUEST) {
//        http_begin_request(http, method, buffer);
//        http_update_seen_before(http, skb_info);
//    } else if (packet_type == HTTP_RESPONSE) {
//        http_begin_response(http, buffer);
//        http_update_seen_before(http, skb_info);
//    }
//
//    http->tags |= tags;
//
//    if (http_responding(http)) {
//        http->response_last_seen = bpf_ktime_get_ns();
//    }
//
//    if (http_closed(http, skb_info, http_stack->owned_by_src_port)) {
//        http_enqueue(http);
//        bpf_map_delete_elem(&http_in_flight, &http_stack->tup);
//    }

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
static __always_inline __u64 read_var_int(http2_transaction_t* http2_transaction, char n){
    if (n < 1 || n > 8) {
        return -1;
    }

    if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
            return false;
    }

    __u64 index = (__u64)(*(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment));
    __u64 n2 = n;
    if (n < 8) {
        index &= (1 << n2) - 1;
    }

    if (index < (1 << n2) - 1) {
        http2_transaction->current_offset_in_request_fragment += 1;
        return index;
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
     if (value == kHTTP) {
        http2_transaction->schema = value;
        return true;
     }
     if ((value <= k500) && (value >= k200)) {
        http2_transaction->packet_type = 3; // this will be response type
        return true;
     }

     return false;
}

// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline void parse_field_indexed(http2_transaction_t* http2_transaction){
     __u64 index = read_var_int(http2_transaction, 7);
     // if the index is smaller then 61 we will be in static table.
     bool found = false;

    // we search the index in the static table
    static_table_value* static_value = bpf_map_lookup_elem(&http2_static_table, &index);
    if (static_value != NULL) {
        found = classify_static_value(http2_transaction, static_value);
    }

    // if we could not find the index in the static table
    if (!found) {
        __u64 *global_counter = bpf_map_lookup_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup);
        if (global_counter != NULL) {
            // we change the index to fit our internal dynamic table implementation index.
            // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
            __u64 new_index = *global_counter - (index - 62);
//            log_debug("[tasik] ------------");
//            log_debug("[tasik] the global_counter is %d", *global_counter);
//            log_debug("[tasik] the hpack index is %d", index);
//            log_debug("[tasik] the new_index is %d", new_index);
//            log_debug("[tasik] ------------");

            dynamic_table_index dynamic_index = {};
            dynamic_index.index = new_index;
            dynamic_index.old_tup = http2_transaction->old_tup;

            dynamic_table_value *dynamic_value_new = bpf_map_lookup_elem(&http2_dynamic_table, &dynamic_index);
            if (dynamic_value_new != NULL) {
                // index 5 represents the :path header - from dynamic table
                if ((dynamic_value_new->index == 5) && (sizeof(dynamic_value_new->value.buffer)>0)){
                    bpf_memcpy(http2_transaction->path, dynamic_value_new->value.buffer, HTTP2_MAX_PATH_LEN);
                }

                // index 1 represents the :path header - from dynamic table
                if ((dynamic_value_new->index == 1) && (sizeof(dynamic_value_new->value.buffer)>0)){
                    bpf_memcpy(http2_transaction->authority, dynamic_value_new->value.buffer, HTTP2_MAX_PATH_LEN);
                }
            }
        }
    }
}

// readString decoded string an hpack string from payload.
//
// wantStr is whether s will be used. If false, decompression and
// []byte->string garbage are skipped if s will be ignored
// anyway. This does mean that huffman decoding errors for non-indexed
// strings past the MAX_HEADER_LIST_SIZE are ignored, but the server
// is returning an error anyway, and because they're not indexed, the error
// won't affect the decoding state.
static __always_inline bool read_string(http2_transaction_t* http2_transaction, __u32 current_offset_in_request_fragment, __u64 *out_str_len, size_t payload_size){
    // need to make sure that I am right but it seems like this part is interesting for headers which are not interesting
    // for as for example te:trailers, if so we may consider not supporting this part of the code in order to avoid
    // complexity and drop each index which is not interesting for us.
    *out_str_len = read_var_int(http2_transaction, 7);
    return true;
}

static __always_inline void update_current_offset(http2_transaction_t* http2_transaction, __u64 str_len, size_t payload_size){
    bool ok = read_string(http2_transaction, 6, &str_len, payload_size);
    if (!ok && str_len <= 0){
        return;
    }

    http2_transaction->current_offset_in_request_fragment += str_len;
}

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline void parse_field_literal(http2_transaction_t* http2_transaction, bool index_type, size_t payload_size, uint8_t n){
    __u64 counter = 0;

    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup);
    if (counter_ptr != NULL) {
        counter = *counter_ptr;
    }
    counter += 1;
    // update the global counter.
    bpf_map_update_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup, &counter, BPF_ANY);

     __u64 index = read_var_int(http2_transaction, n);

    dynamic_table_value dynamic_value = {};
    dynamic_table_index dynamic_index = {};
    static_table_value *static_value = bpf_map_lookup_elem(&http2_static_table, &index);
    if (static_value != NULL) {
        if (index_type) {
            dynamic_value.index = static_value->name;
        }

        __u64 str_len = 0;
        bool ok = read_string(http2_transaction, 6, &str_len, payload_size);
        if (!ok && str_len <= 0){
            return;
        }

        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
            return ;
        }

        char *beginning = http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment;
        // TODO: use const __u64 size11 = str_len < HTTP2_MAX_PATH_LEN ? str_len : HTTP2_MAX_PATH_LEN;

        // create the new dynamic value which will be added to the internal table.
        bpf_memcpy(dynamic_value.value.buffer, beginning, HTTP2_MAX_PATH_LEN);
        dynamic_value.value.string_len = str_len;
        dynamic_value.index = index;

        // create the new dynamic index which is bashed on the counter and the conn_tup.
        dynamic_index.index = counter;
        dynamic_index.old_tup = http2_transaction->old_tup;

        bpf_map_update_elem(&http2_dynamic_table, &dynamic_index, &dynamic_value, BPF_ANY);

        http2_transaction->current_offset_in_request_fragment += str_len;

        // index 5 represents the :path header - from static table
        if ((index == 5) && (sizeof(dynamic_value.value.buffer)>0)){
            bpf_memcpy(http2_transaction->path, dynamic_value.value.buffer, HTTP2_MAX_PATH_LEN);
        }

        // index 1 represents the :authority header - from static table
        if ((index == 1) && (sizeof(dynamic_value.value.buffer)>0)){
            bpf_memcpy(http2_transaction->authority, dynamic_value.value.buffer, HTTP2_MAX_PATH_LEN);
        }
        }
    else {
        __u64 str_len = 0;
        update_current_offset(http2_transaction, str_len, payload_size);

        // Literal Header Field with Incremental Indexing - New Name, which means we need to read the body as well,
        // so we are reading again and updating the len.
        if (index == 0) {
            update_current_offset(http2_transaction, str_len, payload_size);
        }
    }
}

// parse_header_field_repr is handling the header frame by bit calculation and is storing the needed data for our
// internal hpack algorithm.
static __always_inline void parse_header_field_repr(http2_transaction_t* http2_transaction, size_t payload_size, __u8 first_char) {
    log_debug("[http2] parse_header_field_repr is in");
    log_debug("[http2] first char %d", first_char);

    if ((first_char&128) != 0) {
        // Indexed representation.
        // MSB bit set.
        // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
        log_debug("[http2]first char %d & 128 != 0; calling parse_field_indexed", first_char);
        parse_field_indexed(http2_transaction);
        }
    if ((first_char&192) == 64) {
        // 6.2.1 Literal Header Field with Incremental Indexing
        // top two bits are 10
        // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
        log_debug("[http2] first char %d & 192 == 64; calling parse_field_literal", first_char);
        parse_field_literal(http2_transaction, true, payload_size, 6);
    }
}

// This function reads the http2 headers frame.
static __always_inline bool decode_http2_headers_frame(http2_transaction_t* http2_transaction, __u32 payload_size) {
    log_debug("[http2] decode_http2_headers_frame is in");

// need to come back and understand how many times I will iterate over the current frame
//#pragma unroll
    for (int i = 0; i < HTTP2_MAX_HEADERS_COUNT; i++) {
        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
                return false;
        }
        __u8 first_char = *(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment);
        parse_header_field_repr(http2_transaction, payload_size, first_char);
    }

    return true;
}

#endif
