#ifndef __HTTP2_H
#define __HTTP2_H

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Format" section
#define HTTP2_FRAME_HEADER_SIZE 9
#define HTTP2_SETTINGS_SIZE 6

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "http2-defs.h"
#include "http-types.h"
#include "protocol-classification-defs.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "http2.h"

BPF_HASH_MAP(http2_static_table, u64, static_table_value, 20)

BPF_HASH_MAP(http2_dynamic_table, u64, dynamic_table_value, 20)

/* thread_struct id too big for allocation on stack in eBPF function, we use an array as a heap allocator */
BPF_PERCPU_ARRAY_MAP(http2_trans_alloc, __u32, http2_transaction_t, 1)

// All types of http2 frames exist in the protocol.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Type Registry" section.
typedef enum {
    kDataFrame          = 0,
    kHeadersFrame       = 1,
    kPriorityFrame      = 2,
    kRSTStreamFrame     = 3,
    kSettingsFrame      = 4,
    kPushPromiseFrame   = 5,
    kPingFrame          = 6,
    kGoAwayFrame        = 7,
    kWindowUpdateFrame  = 8,
    kContinuationFrame  = 9,
} __attribute__ ((packed)) frame_type_t;

// Struct which represent the http2 frame by its fields.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540#section-4.1 for frame format.
struct http2_frame {
    uint32_t length;
    frame_type_t type;
    uint8_t flags;
    uint32_t stream_id;
};

static __always_inline uint32_t as_uint32_t(unsigned char input) {
    return (uint32_t)input;
}

// This function checks if the http2 frame header is empty.
static __always_inline bool is_empty_frame_header(const char *frame) {
#pragma unroll
    for (uint32_t i = 0; i < HTTP2_FRAME_HEADER_SIZE; i++) {
        if (frame[i] != 0) {
            return false;
        }
    }
    return true;
}

// This function reads the http2 frame header and validate the frame.
static __always_inline bool read_http2_frame_header(const char *buf, size_t buf_size, struct http2_frame *out) {
    if (buf == NULL) {
        return false;
    }

    if (buf_size < HTTP2_FRAME_HEADER_SIZE) {
        return false;
    }

    if (is_empty_frame_header(buf)) {
        return false;
    }

// We extract the frame by its shape to fields.
// See: https://datatracker.ietf.org/doc/html/rfc7540#section-4.1
    out->length = as_uint32_t(buf[0])<<16 | as_uint32_t(buf[1])<<8 | as_uint32_t(buf[2]);
    out->type = (frame_type_t)buf[3];
    out->flags = (uint8_t)buf[4];
    out->stream_id = (as_uint32_t(buf[5]) << 24 |
                      as_uint32_t(buf[6]) << 16 |
                      as_uint32_t(buf[7]) << 8 |
                      as_uint32_t(buf[8])) & 2147483647;

    return true;
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

    // TODO: compare with original code.
    return -1;
}

// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline void parse_field_indexed(http2_transaction_t* http2_transaction){
        log_debug("[http2] parse_field_indexed in");

     __u64 index = read_var_int(http2_transaction, 7);

     log_debug("[http2] ************************ the current index at parse_field_indexed is: %d", index);

    static_table_value *static_value = bpf_map_lookup_elem(&http2_static_table, &index);
    if (static_value != NULL) {
        log_debug("[http2] the static name in parse_field_indexed is %d", static_value->name);
        log_debug("[http2] the static value in parse_field_indexed is %d", static_value->value);
    } else {
        log_debug("[http2] value is null - unable to find the index at the static table");
    }

    dynamic_table_value *dynamic_value_new = bpf_map_lookup_elem(&http2_dynamic_table, &index);

    if (dynamic_value_new != NULL) {
        log_debug("[http2] ************************* the dynamic2 index is %d", dynamic_value_new->index);
        log_debug("[http2] ************************* the dynamic value in spot 0 is %c", dynamic_value_new->value.path_buffer[0]);
        log_debug("[http2] ************************* the dynamic value in spot 3 is %c", dynamic_value_new->value.path_buffer[3]);
    } else {
        log_debug("[http2] value is null, unable to find the index in the dynamic table");
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
    bool is_huff = false;
    __u8 first_char = *(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment);
    if ((first_char&128) != 0) {
            is_huff = true;
    }

    *out_str_len = read_var_int(http2_transaction, 7);
    return true;
}

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline void parse_field_literal(http2_transaction_t* http2_transaction, bool index_type, size_t payload_size, uint8_t n){
        log_debug("[http2] parse_field_literal in");

     __u64 index = read_var_int(http2_transaction, n);
    if (index) {
        log_debug("[http2] the index is parse_field_indexed %llu", index);
    }

    dynamic_table_value dynamic_value = {};
    static_table_value *static_value = bpf_map_lookup_elem(&http2_static_table, &index);
    if (static_value != NULL) {
        if (index_type) {
            dynamic_value.index = static_value->name;
            log_debug("[http2] the dynamic index is %d", dynamic_value.index);
        }

        __u64 str_len = 0;
        bool ok = read_string(http2_transaction, 6, &str_len, payload_size);
        if (!ok && str_len <= 0){
            return;
        }

        log_debug("[http2] the string len is %llu", str_len);
        if (str_len <= 0) {
            return;
        }

        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
            return ;
        }

        if (http2_transaction->current_offset_in_request_fragment + str_len  > sizeof(http2_transaction->request_fragment)) {
            return;
        }

        char *beginning = http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment;
        // TODO: use const __u64 size11 = str_len < HTTP2_MAX_PATH_LEN ? str_len : HTTP2_MAX_PATH_LEN;
        bpf_memcpy(http2_transaction->request_fragment_bla, beginning, HTTP2_MAX_PATH_LEN);

         log_debug("[http2] ------------ first char bla in 0 spot is %c", http2_transaction->request_fragment_bla[0]);
         log_debug("[http2] ------------ first char bla in 1 spot is %c", http2_transaction->request_fragment_bla[1]);
         log_debug("[http2] ------------ first char bla in 2 spot is %c", http2_transaction->request_fragment_bla[2]);
         log_debug("[http2] ------------ first char bla in 3 spot is %c", http2_transaction->request_fragment_bla[3]);
         log_debug("[http2] ------------ first char bla in 4 spot is %c", http2_transaction->request_fragment_bla[4]);
         log_debug("[http2] ------------ first char bla in 5 spot is %c", http2_transaction->request_fragment_bla[5]);
         log_debug("[http2] ------------ first char bla in 6 spot is %c", http2_transaction->request_fragment_bla[6]);
         log_debug("[http2] ------------ first char bla in 7 spot is %c", http2_transaction->request_fragment_bla[7]);
         log_debug("[http2] ------------ first char bla in 8 spot is %c", http2_transaction->request_fragment_bla[8]);
         log_debug("[http2] ------------ first char bla in 9 spot is %c", http2_transaction->request_fragment_bla[9]);
         log_debug("[http2] ------------ first char bla in 10 spot is %c", http2_transaction->request_fragment_bla[10]);
         log_debug("[http2] ------------ first char bla in 11 spot is %c", http2_transaction->request_fragment_bla[11]);
         log_debug("[http2] ------------ first char bla in 12 spot is %c", http2_transaction->request_fragment_bla[12]);
         log_debug("[http2] ------------ first char bla in 13 spot is %c", http2_transaction->request_fragment_bla[13]);
         log_debug("[http2] ------------ first char bla in 14 spot is %c", http2_transaction->request_fragment_bla[14]);

        bpf_memcpy(dynamic_value.value.path_buffer, http2_transaction->request_fragment_bla, HTTP2_MAX_PATH_LEN);
         log_debug("[http2] ------------ first char blaaaaaaa in 0 spot is %c", dynamic_value.value.path_buffer[0]);

         // static table index starts from index 62
        __u64 index2 = (__u64)(static_value->name + 62);
        log_debug("[http2] the index2 is %d", index2);

        bpf_map_update_elem(&http2_dynamic_table, &index2, &dynamic_value, BPF_ANY);
        dynamic_table_value *dynamic_value_new = bpf_map_lookup_elem(&http2_dynamic_table, &index2);

        if (dynamic_value_new != NULL) {
            log_debug("[http2] the dynamic2 index is %d", dynamic_value_new->index);
            log_debug("[http2] the dynamic value in spot 0 is %c", dynamic_value_new->value.path_buffer[0]);
            log_debug("[http2] the dynamic value in spot 3 is %c", dynamic_value_new->value.path_buffer[3]);
        }

        http2_transaction->current_offset_in_request_fragment += str_len;

        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
            return ;
        }
        __u8 current_char = *(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment);
        log_debug("[http2] ------------ the current char is  %d", current_char);
        if (current_char > 0) {
            log_debug("[http2] blblablalbalblabllba");
        }

        }
        else {
            log_debug("[http2] unable to find the static value in map");
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
//    if ((first_char&240) == 16) {
//        // 6.2.2 Literal Header Field without Indexing
//        // top four bits are 0000
//        // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.2
//        log_debug("[http2] first char %d & 240 == 0; calling parse_field_literal", first_char);
//        parse_field_literal(http2_transaction, false, payload_size, 4);
//    }
}

// This function reads the http2 headers frame.
static __always_inline bool decode_http2_headers_frame(http2_transaction_t* http2_transaction, __u32 payload_size) {
    log_debug("[http2] decode_http2_headers_frame is in");

// need to come back and understand how many times I will iterate over the current frame
//#pragma unroll
    for (int i = 0; i < HTTP2_MAX_FRAME_LEN; i++) {
        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
                return false;
        }
        __u8 first_char = *(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment);
        parse_header_field_repr(http2_transaction, payload_size, first_char);
    }

    return true;
}


// This function filters the needed frames from the http2 session.
static __always_inline void process_http2_frames(http2_transaction_t* http2_transaction, struct __sk_buff *skb) {
    log_debug("[http2] http2 process_http2_frames");

    struct http2_frame current_frame = {};

#pragma unroll
    // Iterate till max frames to avoid high connection rate.
    for (uint32_t i = 0; i < HTTP2_MAX_FRAMES; ++i) {
        if (http2_transaction->current_offset_in_request_fragment + HTTP2_FRAME_HEADER_SIZE > skb->len) {
        log_debug("[http2] size is too big!");
          return;
        }

        // Load the current frame into http2_frame strct in order to filter the needed frames.
        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
            return;
        }


        if (!read_http2_frame_header(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment, HTTP2_FRAME_HEADER_SIZE, &current_frame)){
            return;
        }

        http2_transaction->current_offset_in_request_fragment += HTTP2_FRAME_HEADER_SIZE;

        // End of stream my apper in the data frame as well as the header frame.
        if (current_frame.type == kDataFrame && current_frame.flags == 1){
           log_debug("[http2] ********* End of stream flag was found!!! *********", current_frame.stream_id);
        }

        if (current_frame.length == 0) {
            continue;
        }

        // Filter all types of frames except header frame.
        if (current_frame.type != kHeadersFrame) {
            http2_transaction->current_offset_in_request_fragment += (__u32)current_frame.length;
            continue;
        }

        // End of stream my apper in the header frame as well.
        if (current_frame.flags == 1){
           log_debug("[http2] ********* End of stream flag was found!!! *********", current_frame.stream_id);
        }

        // Verify size of pos with max of XX not bigger then the packet.
        if (http2_transaction->current_offset_in_request_fragment + (__u32)current_frame.length > skb->len) {
            return;
        }

//        log_debug("[http2] the current frame len is: %d", current_frame.length);
//        log_debug("[http2] the current frame flags is: %d", current_frame.flags);
//        log_debug("[http2] the current frame type is: %d", current_frame.type);
//        log_debug("[http2] the current frame place: %d", http2_transaction->current_offset_in_request_fragment);

        // Load the current frame into http2_frame strct in order to filter the needed frames.
        if (!decode_http2_headers_frame(http2_transaction, current_frame.length)){
            log_debug("[http2] unable to read http2 header frame");
            return;
        }

        http2_transaction->current_offset_in_request_fragment += (__u32)current_frame.length;
    }
}

#endif
