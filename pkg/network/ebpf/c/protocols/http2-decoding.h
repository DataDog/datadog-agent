#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "http2-decoding-defs.h"
#include "http2-maps-defs.h"
#include "http2-maps-defs-classify.h"
#include "http-types.h"
#include "protocol-classification-defs.h"
#include "bpf_telemetry.h"
#include "ip.h"

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
            case k200:
                response_code = 200;
                break;
            case k204:
                response_code = 204;
                break;
            case k206:
                response_code = 206;
                break;
            case k400:
                response_code = 400;
                break;
            case k500:
                response_code = 500;
                break;
            }

            http->response_status_code = response_code;
            http->request_started = trans->request_started;
            http->request_method = method;
            http->response_last_seen = bpf_ktime_get_ns();
            http->owned_by_src_port = trans->owned_by_src_port;
            http->tcp_seq = trans->tcp_seq;
            http->tags = trans->tags;

            http_batch_enqueue(http);
            bpf_map_delete_elem(&http2_in_flight, &http2_stack->tup);
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
static __always_inline __u64 read_var_int(heap_buffer_t *heap_buffer, __u64 factor){
//    const char *data_head = heap_buffer->fragment + heap_buffer->offset;
//    const char *data_end = heap_buffer->fragment + heap_buffer->size;
    const __s64 remaining_size = (__s64)heap_buffer->size - (__s64)heap_buffer->offset;

    if (remaining_size <= 0) {
        return -1;
    }

    // verifier is happy now
    if (heap_buffer->offset >= HTTP2_BUFFER_SIZE) {
        return -1;
    }

    __u64 current_char_as_number = heap_buffer->fragment[heap_buffer->offset];
    current_char_as_number &= (1 << factor) - 1;

    if (current_char_as_number < (1 << factor) - 1) {
        heap_buffer->offset += 1;
        return current_char_as_number;
    }

    // TODO: compare with original code if needed.
    return -1;
}

static __always_inline void classify_static_value(http2_stream_t *http2_stream, static_table_entry_t* static_value){
     if ((static_value->key == kMethod) && (static_value->value == kPOST)){
        http2_stream->request_method = static_value->value;
     } else if ((static_value->value <= k500) && (static_value->value >= k200)) {
        http2_stream->response_status_code = static_value->value;
     }

     return;
}

// TODO: Fix documentation
// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline void parse_field_indexed(http2_ctx_t *http2_ctx, http2_stream_t *http2_stream, heap_buffer_t *heap_buffer){
    __u64 index = read_var_int(heap_buffer, 7);
    if (index <= MAX_STATIC_TABLE_INDEX) {
        static_table_entry_t* static_value = bpf_map_lookup_elem(&http2_static_table, &index);
        if (static_value != NULL) {
            classify_static_value(http2_stream, static_value);
        }
        return;
    }

    __u64 *global_counter = bpf_map_lookup_elem(&http2_dynamic_counter_table, &http2_ctx->tup);
    if (global_counter == NULL) {
        return;
    }
    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    __u64 new_index = *global_counter - (index - (MAX_STATIC_TABLE_INDEX + 1));
    dynamic_table_index_t dynamic_index = {};
    dynamic_index.index = new_index;
    // TODO: can be out of the loop
    dynamic_index.old_tup = http2_ctx->tup;

    dynamic_table_entry_t *dynamic_value_new = bpf_map_lookup_elem(&http2_dynamic_table, &dynamic_index);
    if (dynamic_value_new == NULL) {
        return;
    }

    // index 5 represents the :path header - from dynamic table
    if (dynamic_value_new->index == 5){
        bpf_memcpy(http2_stream->path, dynamic_value_new->value.buffer, HTTP2_MAX_PATH_LEN);
        http2_stream->path_size = dynamic_value_new->value.string_len;
    }
}

//static __always_inline bool update_current_offset(http2_transaction_t* http2_transaction){
//    __u64 str_len = read_var_int(http2_transaction, 6);
//    if (str_len == -1) {
//        return false;
//    }
//    http2_transaction->current_offset_in_request_fragment += (__u32)str_len;
//    return true;
//}
//
////// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
////// which will be stored in the dynamic table.
//static __always_inline void parse_field_literal(http2_transaction_t* http2_transaction, bool index_type){
//    __u64 counter = 0;
//
//    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
//    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup);
//    if (counter_ptr != NULL) {
//        counter = *counter_ptr;
//    }
//    counter += 1;
//    // update the global counter.
//    bpf_map_update_elem(&http2_dynamic_counter_table, &http2_transaction->old_tup, &counter, BPF_ANY);
//
//    __u64 index = read_var_int(http2_transaction, 6);
//
//    dynamic_table_entry_t dynamic_value = {};
//    dynamic_table_index_t dynamic_index = {};
//    static_table_entry_t *static_value = bpf_map_lookup_elem(&http2_static_table, &index);
//    if (static_value == NULL) {
//        update_current_offset(http2_transaction);
//
//        // Literal Header Field with Incremental Indexing - New Name, which means we need to read the body as well,
//        // so we are reading again and updating the len.
//        // TODO: Better document.
//        if (index == 0) {
//            update_current_offset(http2_transaction);
//        }
//        return;
//    }
//
//    if (index_type) {
//        dynamic_value.index = static_value->key;
//    }
//
//    __u64 str_len = read_var_int(http2_transaction, 6);
//    if (str_len == -1 || str_len == 0){
//        return;
//    }
//
//    if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
//        return;
//    }
//
//    char *beginning = http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment;
//    // create the new dynamic value which will be added to the internal table.
//    bpf_memcpy(dynamic_value.value.buffer, beginning, HTTP2_MAX_PATH_LEN);
//
//    dynamic_value.value.string_len = str_len;
//    dynamic_value.index = index;
//
//    // create the new dynamic index which is bashed on the counter and the conn_tup.
//    dynamic_index.index = counter;
//    dynamic_index.old_tup = http2_transaction->old_tup;
//
//    bpf_map_update_elem(&http2_dynamic_table, &dynamic_index, &dynamic_value, BPF_ANY);
//
//    http2_transaction->current_offset_in_request_fragment += (__u32)str_len;
//
//    // index 5 represents the :path header - from static table
//    if (index == 5){
//        bpf_memcpy(http2_transaction->path, dynamic_value.value.buffer, HTTP2_MAX_PATH_LEN);
//        http2_transaction->path_size = str_len;
//    }
//}

// This function reads the http2 headers frame.
static __always_inline bool process_headers(http2_ctx_t *http2_ctx, http2_stream_t *http2_stream, heap_buffer_t *heap_buffer) {
    char current_ch;
    char *data_head = heap_buffer->fragment;
    const char *data_end = heap_buffer->fragment + heap_buffer->size;

#pragma unroll
    for (unsigned headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT; headers_index++) {
        if (data_end <= data_head) {
            return false;
        }
        current_ch = *data_head;
        if ((current_ch&128) != 0) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            log_debug("[http2] first char %d & 128 != 0; calling parse_field_indexed", current_ch);
            parse_field_indexed(http2_ctx, http2_stream, heap_buffer);
        } else if ((current_ch&192) == 64) {
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 10
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            log_debug("[http2] first char %d & 192 == 64; calling parse_field_literal", current_ch);
//            parse_field_literal(http2_transaction, true);
        }
    }

    return true;
}

static __always_inline __s8 filter_http2_frames(struct __sk_buff *skb, http2_ctx_t *http2_ctx, http2_frame_t *frames_to_process) {
    __u32 offset = http2_ctx->skb_info.data_off;
    __s64 remaining_payload_length = 0;

    // length cannot be 9
    char frame_buf[16];
    bpf_memset((char*)frame_buf, 0, 16);

    __s8 frame_index = 0;
    bool is_end_of_stream = false;
    bool is_headers_frame = false;
    bool is_data_frame_end_of_stream = false;
    frame_type_t frame_type;
    __u8 frame_flags;

#pragma unroll (HTTP2_MAX_FRAMES)
    for (int iteration = 0; iteration < HTTP2_MAX_FRAMES; iteration++) {
        remaining_payload_length = skb->len - offset;
        if (remaining_payload_length < HTTP2_FRAME_HEADER_SIZE) {
            break;
        }

        // read frame.
        bpf_skb_load_bytes_with_telemetry(skb, offset, frame_buf, HTTP2_FRAME_HEADER_SIZE);
        offset += HTTP2_FRAME_HEADER_SIZE;

        if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &frames_to_process[frame_index].header)){
            log_debug("[http2] unable to read_http2_frame_header");
            break;
        }
        frames_to_process[frame_index].offset = offset;
        offset += frames_to_process[frame_index].header.length;

        frame_type = frames_to_process[frame_index].header.type;
        frame_flags = frames_to_process[frame_index].header.flags;

        // filter frame
        is_end_of_stream = (frame_flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
        is_data_frame_end_of_stream = is_end_of_stream && (frame_type == kDataFrame);
        is_headers_frame = frame_type == kHeadersFrame;
        if (!is_data_frame_end_of_stream && !is_headers_frame) {
            log_debug("[http2] %p frame is not headers or data EOS, thus skipping it\n", skb);
            // Skipping the frame payload.
            continue;
        }

        frame_index++;
    }

    return frame_index-1;
}

static __always_inline void process_relevant_http2_frames(struct __sk_buff *skb, http2_ctx_t *http2_ctx, http2_frame_t *frames_to_process, __u8 number_of_frames) {
    const __u32 zero = 0;
    heap_buffer_t *heap_buffer = bpf_map_lookup_elem(&http2_heap_buffer, &zero);
    if (heap_buffer == NULL) {
        return;
    }
    bpf_memset(heap_buffer, 0, sizeof(heap_buffer_t));
    struct http2_frame *current_frame_header;

    http2_stream_t http2_stream = {};
    bpf_memset(&http2_stream, 0, sizeof(http2_stream_t));

#pragma unroll (HTTP2_MAX_FRAMES)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_FRAMES; iteration++) {
        if (iteration > number_of_frames) {
            break;
        }

        current_frame_header = &frames_to_process[iteration].header;
        log_debug("[http2]%d found an interesting frame length %lu; type %d", iteration, current_frame_header->length, current_frame_header->type);
        log_debug("[http2]%d found an interesting frame flags %d; stream_id %lu", iteration, current_frame_header->flags, current_frame_header->stream_id);
        log_debug("[http2]%d offset is %lu", iteration, frames_to_process[iteration].offset);

        if (current_frame_header->type == kDataFrame) {
            // TODO: handle end of stream.
            continue;
        }

        // headers frame
        heap_buffer->size = HTTP2_BUFFER_SIZE < current_frame_header->length ? HTTP2_BUFFER_SIZE : current_frame_header->length;
        // check length is not too long
        if (current_frame_header->length > heap_buffer->size) {
            //log_debug("[http2] frame is too long (%lu)", frames_to_process[iteration].header.length);
            break;
        }

        // read headers payload
        // TODO: use heap_buffer->size instead of HTTP2_BUFFER_SIZE, and bypass verifier
        bpf_skb_load_bytes_with_telemetry(skb, frames_to_process[iteration].offset, heap_buffer->fragment, HTTP2_BUFFER_SIZE);

        // process headers
        process_headers(http2_ctx, &http2_stream, heap_buffer);
        // if end of stream, process end of stream
    }
}

static __always_inline void http2_entrypoint(struct __sk_buff *skb, http2_ctx_t *http2_ctx) {
    http2_frame_t frames_to_process[HTTP2_MAX_FRAMES];
    bpf_memset(frames_to_process, 0, HTTP2_MAX_FRAMES * sizeof(http2_frame_t));

    __s8 interesting_frames = filter_http2_frames(skb, http2_ctx, frames_to_process);
    if (interesting_frames > 0) {
        process_relevant_http2_frames(skb, http2_ctx, frames_to_process, interesting_frames);
    }

    return;
}

#endif
