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
    bpf_map_update_with_telemetry(http2_in_flight, http2_stream_key, http2_stream_ptr, BPF_NOEXIST);
    return bpf_map_lookup_elem(&http2_in_flight, http2_stream_key);
}

//static __always_inline bool http2_seen_before(http2_transaction_t *http2, skb_info_t *skb_info) {
//    if (!skb_info || !skb_info->tcp_seq) {
//        return false;
//    }
//
//    // check if we've seen this TCP segment before. this can happen in the
//    // context of localhost traffic where the same TCP segment can be seen
//    // multiple times coming in and out from different interfaces
//    return http2->tcp_seq == skb_info->tcp_seq;
//}
//
//static __always_inline void http2_update_seen_before(http2_transaction_t *http2, skb_info_t *skb_info) {
//    if (!skb_info || !skb_info->tcp_seq) {
//        return;
//    }
//
//    http2->tcp_seq = skb_info->tcp_seq;
//}
//
//static __always_inline void http2_begin_request(http2_transaction_t *http2, http2_method_t method, char *buffer) {
////    http2->request_method = method;
//    http2->request_started = bpf_ktime_get_ns();
//    http2->response_last_seen = 0;
//    bpf_memcpy(&http2->request_fragment, buffer, HTTP2_BUFFER_SIZE);
//}
//
//static __always_inline int http2_responding(http2_transaction_t *http2) {
//    return (http2 != NULL && http2->response_status_code != 0);
//}
//
//static __always_inline int http2_process(http2_transaction_t* http2_stack,  skb_info_t *skb_info,__u64 tags) {
//    http2_packet_t packet_type = HTTP2_PACKET_UNKNOWN;
//    http2_method_t method = HTTP2_METHOD_UNKNOWN;
//    __u64 response_code;
//
//    if (http2_stack->packet_type > 0) {
//        packet_type = http2_stack->packet_type;
//    }
//
//    if (packet_type != 0){
//        log_debug("[http2] ----------------------------------\n");
//        log_debug("[http2] The method is %d\n", method);
//        log_debug("[http2] The packet_type is %d\n", packet_type);
//        log_debug("[http2] the response status code is %d\n", http2_stack->response_status_code);
//        log_debug("[http2] the end of stream is %d\n", http2_stack->end_of_stream);
//        log_debug("[http2] ----------------------------------\n");
//    }
//
//    if (packet_type == 3) {
//        packet_type = HTTP2_RESPONSE;
//    } else if (packet_type == 2) {
//        packet_type = HTTP2_REQUEST;
//    }
//
//    http2_transaction_t *http2 = http2_fetch_state(http2_stack, packet_type);
//    if (!http2 || http2_seen_before(http2, skb_info)) {
//        log_debug("[http2] the http2 has been seen before!\n");
//        return 0;
//    }
//
//    if (packet_type == HTTP2_REQUEST) {
//        log_debug("[http2] http2_process request: type=%d method=%d\n", packet_type, method);
//        http2_begin_request(http2, method, (char *)http2_stack->request_fragment);
//        http2_update_seen_before(http2, skb_info);
//    } else if (packet_type == HTTP2_RESPONSE) {
//        log_debug("[http2] http2_begin_response: htx=%llx status=%d\n", http2, http2->response_status_code);
//        http2_update_seen_before(http2, skb_info);
//    }
//
//    if (http2_stack->response_status_code > 0) {
//        http2_transaction_t *trans = bpf_map_lookup_elem(&http2_in_flight, &http2->old_tup);
//        if (trans != NULL) {
//            const __u32 zero = 0;
//            http_transaction_t *http = bpf_map_lookup_elem(&http_trans_alloc, &zero);
//            if (http == NULL) {
//                return 0;
//            }
//            bpf_memset(http, 0, sizeof(http_transaction_t));
//            bpf_memcpy(&http->tup, &trans->tup, sizeof(conn_tuple_t));
//
//            http->request_fragment[0] = 'z';
//            http->request_fragment[1] = http2->path_size;
//            bpf_memcpy(&http->request_fragment[8], trans->path, HTTP2_MAX_PATH_LEN);
//
//            // todo: take it out to a function?!
//            if (trans->request_method == 2) {
//                log_debug("[slavin] found http2 get");
//                method = HTTP2_GET;
//            } else if (trans->request_method == 3) {
//                log_debug("[slavin] found http2 post");
//                method = HTTP2_POST;
//            }
//
//            // todo: take it out to a function and add all the other options as well.
//            switch(http2_stack->response_status_code) {
//            case k200:
//                response_code = 200;
//                break;
//            case k204:
//                response_code = 204;
//                break;
//            case k206:
//                response_code = 206;
//                break;
//            case k400:
//                response_code = 400;
//                break;
//            case k500:
//                response_code = 500;
//                break;
//            }
//
//            http->response_status_code = response_code;
//            http->request_started = trans->request_started;
//            http->request_method = method;
//            http->response_last_seen = bpf_ktime_get_ns();
//            http->owned_by_src_port = trans->owned_by_src_port;
//            http->tcp_seq = trans->tcp_seq;
//            http->tags = trans->tags;
//
//            http_batch_enqueue(http);
//            bpf_map_delete_elem(&http2_in_flight, &http2_stack->tup);
//        }
//    }
//
//    return 0;
//}

// read_var_int reads an unsigned variable length integer off the
// beginning of p. n is the parameter as described in
// https://httpwg.org/specs/rfc7541.html#rfc.section.5.1.
//
// n must always be between 1 and 8.
//
// The returned remain buffer is either a smaller suffix of p, or err != nil.
// The error is errNeedMore if p doesn't contain a complete integer.
static __always_inline bool read_var_int(heap_buffer_t *heap_buffer, __u64 factor, __u8 *out){
    const __u16 offset = heap_buffer->offset % HTTP2_BUFFER_SIZE;

    if (heap_buffer->size <= offset) {
        return false;
    }
    // TODO: verifier is happy now.
    if (HTTP2_BUFFER_SIZE-1 <= offset) {
        return false;
    }
    __u8 current_char_as_number = heap_buffer->fragment[offset];
    current_char_as_number &= (1 << factor) - 1;

    if (current_char_as_number < (1 << factor) - 1) {
        heap_buffer->offset = offset + 1;
        *out = current_char_as_number;
        return true;
    }

    // TODO: compare with original code if needed.
    return false;
}

static __always_inline __u64 get_dynamic_counter(conn_tuple_t *tup) {
    // global counter is the counter which help us with the calc of the index in our internal hpack dynamic table
    __u64 *counter_ptr = bpf_map_lookup_elem(&http2_dynamic_counter_table, tup);
    if (counter_ptr != NULL) {
        return *counter_ptr;
    }
    return 0;
}

static __always_inline void set_dynamic_counter(conn_tuple_t *tup, __u64 *counter) {
    bpf_map_update_elem(&http2_dynamic_counter_table, tup, counter, BPF_ANY);
}

// TODO: Fix documentation
// parse_field_indexed is handling the case which the header frame is part of the static table.
static __always_inline __u8 parse_field_indexed(http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 stream_id, heap_buffer_t *heap_buffer){
    __u8 index = 0;
    if (!read_var_int(heap_buffer, 7, &index)) {
        return 0;
    }

    // TODO: can improve by declaring MAX_INTERESTING_STATIC_TABLE_INDEX
    if (index < MAX_INTERESTING_STATIC_TABLE_INDEX) {
        static_table_entry_t* static_value = bpf_map_lookup_elem(&http2_static_table, &index);
        if (static_value == NULL) {
            return 0;
        }
        headers_to_process->index = index;
        headers_to_process->stream_id = stream_id;
        headers_to_process->type = kStaticHeader;
        return 1;
    } else if (index <= MAX_STATIC_TABLE_INDEX) {
        return 0;
    }

    __u64 global_counter = get_dynamic_counter(&http2_ctx->tup);
    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    http2_ctx->dynamic_index.index = global_counter - (index - (MAX_STATIC_TABLE_INDEX + 1));

    dynamic_table_entry_t *dynamic_value_new = bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index);
    if (dynamic_value_new == NULL) {
        return 0;
    }

    headers_to_process->index = http2_ctx->dynamic_index.index;
    headers_to_process->stream_id = stream_id;
    headers_to_process->type = kExistingDynamicHeader;

    return 1;
}

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline __u8 parse_field_literal(http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 stream_id, heap_buffer_t *heap_buffer){
    __u64 counter = get_dynamic_counter(&http2_ctx->tup);
    counter++;
    set_dynamic_counter(&http2_ctx->tup, &counter);

    __u8 index = 0;
    if (!read_var_int(heap_buffer, 6, &index)) {
        return 0;
    }

    __u8 str_len = 0;
    // The key is new and inserted into the dynamic table. So we are skipping the new value.

    if (index < MAX_INTERESTING_STATIC_TABLE_INDEX) {
        // TODO, if index != 0, that's weird.
        static_table_entry_t *static_value = bpf_map_lookup_elem(&http2_static_table, &index);
        if (static_value == NULL) {
            str_len = 0;
            if (!read_var_int(heap_buffer, 6, &str_len)) {
                return 0;
            }
            heap_buffer->offset += str_len;

            if (index == 0) {
                str_len = 0;
                if (!read_var_int(heap_buffer, 6, &str_len)) {
                    return 0;
                }
                heap_buffer->offset += str_len;
            }
            return 0;
        }
    } else if (index <= MAX_STATIC_TABLE_INDEX) {
        return 0;
    }

    str_len = 0;
    if (!read_var_int(heap_buffer, 6, &str_len)) {
        return 0;
    }

    if (str_len >= HTTP2_MAX_PATH_LEN || index != 5){
        heap_buffer->offset += str_len;
        return 0;
    }

    const __u16 offset = heap_buffer->offset < HTTP2_BUFFER_SIZE - 1 ? heap_buffer->offset : HTTP2_BUFFER_SIZE - 1;
    heap_buffer->offset += str_len;

    if (offset >= HTTP2_BUFFER_SIZE - HTTP2_MAX_PATH_LEN) {
        return 0;
    }
    dynamic_table_entry_t dynamic_value = {};
    dynamic_value.string_len = str_len;

    // create the new dynamic value which will be added to the internal table.
    bpf_memcpy(dynamic_value.buffer, &heap_buffer->fragment[offset % HTTP2_BUFFER_SIZE], HTTP2_MAX_PATH_LEN);

    http2_ctx->dynamic_index.index = counter;
    bpf_map_update_elem(&http2_dynamic_table, &http2_ctx->dynamic_index, &dynamic_value, BPF_ANY);

    headers_to_process->index = counter;
    headers_to_process->stream_id = stream_id;
    headers_to_process->type = kNewDynamicHeader;
    return 1;
}

// This function reads the http2 headers frame.
static __always_inline __u8 filter_relevant_headers(http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 stream_id, heap_buffer_t *heap_buffer) {
    char current_ch;
    __u16 offset = 0;
    __u8 interesting_headers = 0;
    const __u16 buffer_size = heap_buffer->size;

#pragma unroll (HTTP2_MAX_HEADERS_COUNT)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT; ++headers_index) {
        offset = heap_buffer->offset;
        if (buffer_size <= offset) {
            break;
        }
        if (HTTP2_BUFFER_SIZE <= offset) {
            break;
        }
        offset %= HTTP2_BUFFER_SIZE;
        current_ch = heap_buffer->fragment[offset];
        if ((current_ch&128) != 0) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            interesting_headers += parse_field_indexed(http2_ctx, &headers_to_process[interesting_headers], stream_id, heap_buffer);
        } else if ((current_ch&192) == 64) {
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 11
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            interesting_headers += parse_field_literal(http2_ctx, &headers_to_process[interesting_headers], stream_id, heap_buffer);
        }
    }

    return interesting_headers;
}

static __always_inline __u8 filter_http2_frames(struct __sk_buff *skb, http2_ctx_t *http2_ctx, http2_frame_t *frames_to_process, __u32 *max_offset) {
    __u32 offset = http2_ctx->skb_info.data_off;

    // length cannot be 9
    char frame_buf[10];
    bpf_memset((char*)frame_buf, 0, 10);

    __u8 frame_index = 0;
    bool is_end_of_stream = false;
    bool is_headers_frame = false;
    bool is_data_frame_end_of_stream = false;
    frame_type_t frame_type;

#pragma unroll (HTTP2_MAX_FRAMES_PER_ITERATION)
    for (int iteration = 0; iteration < HTTP2_MAX_FRAMES_PER_ITERATION; ++iteration) {
        if (offset + HTTP2_FRAME_HEADER_SIZE >= skb->len) {
            break;
        }

        // read frame.
        bpf_skb_load_bytes_with_telemetry(skb, offset, frame_buf, HTTP2_FRAME_HEADER_SIZE);
        offset += HTTP2_FRAME_HEADER_SIZE;

        if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &frames_to_process[frame_index].header)){
            log_debug("[http2] unable to read_http2_frame_header (%d) offset %lu", frame_index, offset);
            break;
        }
        frames_to_process[frame_index].offset = offset;

        offset += frames_to_process[frame_index].header.length;

        frame_type = frames_to_process[frame_index].header.type;

        // filter frame
        is_end_of_stream = (frames_to_process[frame_index].header.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
        is_data_frame_end_of_stream = is_end_of_stream && (frame_type == kDataFrame);
        is_headers_frame = frame_type == kHeadersFrame;
        if (!is_data_frame_end_of_stream && !is_headers_frame) {
            // Skipping the frame payload.
            continue;
        }

        ++frame_index;
    }

    *max_offset += offset - http2_ctx->skb_info.data_off;
    return frame_index;
}

static __always_inline void read_into_buffer_skb_http2(char *buffer, struct __sk_buff *skb, u64 offset) {
#define BLK_SIZE (16)
    const u32 len = HTTP2_BUFFER_SIZE < (skb->len - (u32)offset) ? (u32)offset + HTTP2_BUFFER_SIZE : skb->len;

    unsigned i = 0;
#pragma unroll(HTTP2_BUFFER_SIZE / BLK_SIZE)
    for (; i < (HTTP2_BUFFER_SIZE / BLK_SIZE); i++) {
        if (offset + BLK_SIZE - 1 >= len) { break; }

        bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
        offset += BLK_SIZE;
    }

    // This part is very hard to write in a loop and unroll it.
    // Indeed, mostly because of older kernel verifiers, we want to make sure the offset into the buffer is not
    // stored on the stack, so that the verifier is able to verify that we're not doing out-of-bound on
    // the stack.
    // Basically, we should get a register from the code block above containing an fp relative address. As
    // we are doing `buffer[0]` here, there is not dynamic computation on that said register after this,
    // and thus the verifier is able to ensure that we are in-bound.
    void *buf = &buffer[i * BLK_SIZE];
    if (i * BLK_SIZE >= HTTP2_BUFFER_SIZE) {
        return;
    } else if (offset + 14 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 1);
    }
}

static __always_inline void process_headers(http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __s32 interesting_headers, http2_stream_key_t *http2_stream_key_template) {
    http2_stream_t *current_stream;
    http2_header_t *current_header;

#pragma unroll (HTTP2_MAX_HEADERS_COUNT)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_HEADERS_COUNT; ++iteration) {
        if (iteration >= interesting_headers) {
            break;
        }

        current_header = &headers_to_process[iteration];
        log_debug("[http2]stream %lu; found header of type %d; index of %d", current_header->stream_id, current_header->type, current_header->index);

        http2_stream_key_template->stream_id = current_header->stream_id;
        current_stream = http2_fetch_stream(http2_stream_key_template);
        if (current_stream == NULL) {
            break;
        }

        if (current_header->type == kStaticHeader) {
            // fetch static value
            static_table_entry_t* static_value = bpf_map_lookup_elem(&http2_static_table, &current_header->index);
            if (static_value == NULL) {
                // report error
                break;
            }

            log_debug("[http2]stream %lu; index %d", current_header->stream_id, current_header->index);
            if (static_value->key == kMethod){
                // TODO: mark request
                current_stream->request_started = bpf_ktime_get_ns();
                current_stream->request_method = static_value->value;
                log_debug("[http2]stream %lu; is post", current_header->stream_id);
            } else if (static_value->key == kStatus) {
                // TODO: mark response
                current_stream->response_status_code = static_value->value;
                log_debug("[http2]stream %lu; status code is %d", current_header->stream_id, static_value->value);
            }
        } else if (current_header->type == kNewDynamicHeader || current_header->type == kExistingDynamicHeader) {
            http2_ctx->dynamic_index.index = current_header->index;
            dynamic_table_entry_t* dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index.index);
            if (dynamic_value == NULL) {
                // report error
                break;
            }
            // TODO: reuse same struct
            current_stream->path_size = dynamic_value->string_len;
            bpf_memcpy(current_stream->path, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
            log_debug("[http2]stream %lu; path_size is %d", current_header->stream_id, current_stream->path_size);
        }
    }
}

static __always_inline void process_relevant_http2_frames(struct __sk_buff *skb, http2_ctx_t *http2_ctx, http2_frame_t *frames_to_process, __u8 number_of_frames) {
    const __u32 zero = 0;
    heap_buffer_t *heap_buffer = bpf_map_lookup_elem(&http2_heap_buffer, &zero);
    if (heap_buffer == NULL) {
        return;
    }
    bpf_memset(heap_buffer, 0, sizeof(heap_buffer_t));

    http2_headers_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, sizeof(http2_headers_t));

    __u8 interesting_headers = 0;
    struct http2_frame *current_frame_header;

#pragma unroll (HTTP2_MAX_FRAMES_PER_ITERATION)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_FRAMES_PER_ITERATION; iteration++) {
        if (iteration > number_of_frames) {
            break;
        }

        current_frame_header = &frames_to_process[iteration].header;
        if (current_frame_header->type == kDataFrame) {
            // TODO: handle end of stream.
            continue;
        }

        // headers frame
        heap_buffer->size = HTTP2_BUFFER_SIZE < current_frame_header->length ? HTTP2_BUFFER_SIZE : current_frame_header->length;

        // read headers payload
        read_into_buffer_skb_http2((char*)heap_buffer->fragment, skb, frames_to_process[iteration].offset);

        // process headers
        interesting_headers += filter_relevant_headers(http2_ctx, headers_to_process->array, current_frame_header->stream_id, heap_buffer);
        // if end of stream, process end of stream
    }

    process_headers(http2_ctx, headers_to_process->array, interesting_headers, &http2_ctx->http2_stream_key);
}

static __always_inline __u32 http2_entrypoint(struct __sk_buff *skb, http2_ctx_t *http2_ctx) {
    const __u32 zero = 0;
    http2_frames_t *frames_to_process = bpf_map_lookup_elem(&http2_frames_to_process, &zero);
    if (frames_to_process == NULL) {
        return -1;
    }
    bpf_memset(frames_to_process, 0, sizeof(http2_frames_t));

    __u32 max_offset = 0;
    __u8 interesting_frames = filter_http2_frames(skb, http2_ctx, frames_to_process->array, &max_offset);
    if (interesting_frames > 0) {
        process_relevant_http2_frames(skb, http2_ctx, frames_to_process->array, interesting_frames);
    }

    return max_offset;
}

#endif
