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
static __always_inline bool read_var_int(heap_buffer_t *heap_buffer, __u64 factor, __u8 *out, u32 stream_id){
    __u16 offset = heap_buffer->offset % HTTP2_BUFFER_SIZE;

    if (heap_buffer->size <= offset) {
        return false;
    }
    // TODO: verifier is happy now.
    if (HTTP2_BUFFER_SIZE-1 <= offset) {
        return false;
    }
    const __u8 current_char_as_number_s = heap_buffer->fragment[offset];
    __u8 current_char_as_number = current_char_as_number_s;
    current_char_as_number &= (1 << factor) - 1;

    heap_buffer->offset = offset + 1;
    if (current_char_as_number < (1 << factor) - 1) {
        *out = current_char_as_number;
        return true;
    }

    const u16 offset2 = heap_buffer->offset;
    if (offset2 < heap_buffer->size && offset2 < HTTP2_BUFFER_SIZE) {
        const __u8 b = heap_buffer->fragment[offset2];
        current_char_as_number += b & 127;
        if ((b & 128 ) == 0) {
            heap_buffer->offset = offset2 + 1;
            *out = current_char_as_number;
            return true;
        }
    }

    return false;
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
static __always_inline parse_result_t parse_field_indexed(dispatcher_arguments_t *iterations_key, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 stream_id, heap_buffer_t *heap_buffer){
    __u8 index = 0;
    if (!read_var_int(heap_buffer, 7, &index, stream_id)) {
        return HEADER_ERROR;
    }

    // TODO: can improve by declaring MAX_INTERESTING_STATIC_TABLE_INDEX
    if (index < MAX_STATIC_TABLE_INDEX) {
        if (bpf_map_lookup_elem(&http2_static_table, &index) == NULL) {
            return HEADER_NOT_INTERESTING;
        }
        headers_to_process->index = index;
        headers_to_process->stream_id = stream_id;
        headers_to_process->type = kStaticHeader;
        return HEADER_INTERESTING;
    }

    __u64 global_counter = get_dynamic_counter(&iterations_key->tup);
    // we change the index to fit our internal dynamic table implementation index.
    // the index is starting from 1 so we decrease 62 in order to be equal to the given index.
    http2_ctx->dynamic_index.index = global_counter - (index - MAX_STATIC_TABLE_INDEX);

    if (bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index) == NULL) {
        return HEADER_NOT_INTERESTING;
    }
    headers_to_process->index = http2_ctx->dynamic_index.index;
    headers_to_process->stream_id = stream_id;
    headers_to_process->type = kDynamicHeader;

    return HEADER_INTERESTING;
}

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline parse_result_t parse_field_literal(dispatcher_arguments_t *iterations_key, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 stream_id, heap_buffer_t *heap_buffer){
    __u64 counter = get_dynamic_counter(&iterations_key->tup);
    counter++;
    set_dynamic_counter(&iterations_key->tup, counter);

    __u8 index = 0;
    if (!read_var_int(heap_buffer, 6, &index, stream_id)) {
        return HEADER_ERROR;
    }

    __u8 str_len = 0;
    // The key is new and inserted into the dynamic table. So we are skipping the new value.

    if (index < MAX_STATIC_TABLE_INDEX) {
        // TODO, if index != 0, that's weird.
        if (bpf_map_lookup_elem(&http2_static_table, &index) == NULL) {
            str_len = 0;
            if (!read_var_int(heap_buffer, 6, &str_len, stream_id)) {
                return HEADER_ERROR;
            }
            heap_buffer->offset += str_len;

            if (index == 0) {
                str_len = 0;
                if (!read_var_int(heap_buffer, 6, &str_len, stream_id)) {
                    return HEADER_ERROR;
                }
                heap_buffer->offset += str_len;
            }
            return HEADER_NOT_INTERESTING;
        }
    }


    str_len = 0;
    if (!read_var_int(heap_buffer, 6, &str_len, stream_id)) {
        return HEADER_ERROR;
    }

    // if the index is not path or the len of string is bigger then we support, we continue.
    if (str_len >= HTTP2_MAX_PATH_LEN || index != kIndexPath){
        heap_buffer->offset += str_len;
        return HEADER_NOT_INTERESTING;
    }

    const __u16 offset = heap_buffer->offset < HTTP2_BUFFER_SIZE - 1 ? heap_buffer->offset : HTTP2_BUFFER_SIZE - 1;
    heap_buffer->offset += str_len;

    if (offset >= HTTP2_BUFFER_SIZE - HTTP2_MAX_PATH_LEN) {
        return HEADER_NOT_INTERESTING;
    }
    dynamic_table_entry_t dynamic_value = {};
    dynamic_value.string_len = str_len;

    // create the new dynamic value which will be added to the internal table.
    bpf_memcpy(dynamic_value.buffer, &heap_buffer->fragment[offset % HTTP2_BUFFER_SIZE], HTTP2_MAX_PATH_LEN);

    http2_ctx->dynamic_index.index = counter - 1;
    bpf_map_update_elem(&http2_dynamic_table, &http2_ctx->dynamic_index, &dynamic_value, BPF_ANY);

    headers_to_process->index = counter - 1;
    headers_to_process->stream_id = stream_id;
    headers_to_process->type = kDynamicHeader;
    return HEADER_INTERESTING;
}

// This function reads the http2 headers frame.
static __always_inline __u8 filter_relevant_headers(dispatcher_arguments_t *iterations_key, http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u32 stream_id, heap_buffer_t *heap_buffer) {
    char current_ch;
    __u16 offset = 0;
    __u8 interesting_headers = 0;
    const __u16 buffer_size = heap_buffer->size;
    parse_result_t res;

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
            res = parse_field_indexed(iterations_key, http2_ctx, &headers_to_process[interesting_headers], stream_id, heap_buffer);
            if (res == HEADER_ERROR) {
                break;
            }
            interesting_headers += res == HEADER_INTERESTING;
        } else if ((current_ch&192) == 64) {
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 11
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            res = parse_field_literal(iterations_key, http2_ctx, &headers_to_process[interesting_headers], stream_id, heap_buffer);
            if (res == HEADER_ERROR) {
                break;
            }
            interesting_headers += res == HEADER_INTERESTING;
        }
    }

    return interesting_headers;
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

static __always_inline void process_headers(http2_ctx_t *http2_ctx, http2_header_t *headers_to_process, __u8 interesting_headers) {
    http2_stream_t *current_stream;
    http2_header_t *current_header;
    http2_stream_key_t *http2_stream_key_template = &http2_ctx->http2_stream_key;

    // TODO: use a lower bound as we have much less "interesting" headers than HTTP2_MAX_HEADERS_COUNT.
#pragma unroll (HTTP2_MAX_HEADERS_COUNT)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_HEADERS_COUNT; ++iteration) {
        if (iteration >= interesting_headers) {
            break;
        }

        current_header = &headers_to_process[iteration];

        http2_stream_key_template->stream_id = current_header->stream_id;
        current_stream = http2_fetch_stream(http2_stream_key_template);
        if (current_stream == NULL) {
            break;
        }

        if (current_header->type == kStaticHeader) {
            // fetch static value
            static_table_entry_t* static_value = bpf_map_lookup_elem(&http2_static_table, &current_header->index);
            if (static_value == NULL) {
                log_debug("http2 error static header was not found for stream %lu; index %d", http2_stream_key_template->stream_id, current_header->index);
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
        } else if (current_header->type == kDynamicHeader) {
            http2_ctx->dynamic_index.index = current_header->index;
            dynamic_table_entry_t* dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, &http2_ctx->dynamic_index);
            if (dynamic_value == NULL) {
                log_debug("http2 error dynamic header was not found for stream %lu; index %d", http2_stream_key_template->stream_id, current_header->index);
                break;
            }
            // TODO: reuse same struct
            current_stream->path_size = dynamic_value->string_len;
            bpf_memcpy(current_stream->request_path, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
        }
    }
}

static __always_inline void handle_end_of_stream(frame_type_t type, http2_stream_key_t *http2_stream_key_template) {
    http2_stream_t *current_stream = http2_fetch_stream(http2_stream_key_template);
    if (current_stream == NULL) {
        return;
    }

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

static __always_inline void process_headers_frame(struct __sk_buff *skb, dispatcher_arguments_t *iterations_key, http2_ctx_t *http2_ctx, struct http2_frame *current_frame_header) {
    const __u32 zero = 0;

    // Allocating a buffer on the heap (percpu array), the buffer represents the frame payload.
    heap_buffer_t *frame_payload = bpf_map_lookup_elem(&http2_heap_buffer, &zero);
    if (frame_payload == NULL) {
        return;
    }
    bpf_memset(frame_payload, 0, sizeof(heap_buffer_t));

    // TODO: We should allocate lower number than HTTP2_MAX_HEADERS_COUNT, as we know we have 2-4 max interesting headers
    // in one frame.
    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_headers_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process->array, 0, HTTP2_MAX_HEADERS_COUNT * sizeof(http2_header_t));

    // Search for relevant headers.

    // TODO: Introduce MIN macro
    // headers frame
    frame_payload->size = HTTP2_BUFFER_SIZE < current_frame_header->length ? HTTP2_BUFFER_SIZE : current_frame_header->length;

    // read headers payload
    read_into_buffer_skb_http2((char*)frame_payload->fragment, skb, iterations_key->skb_info.data_off + HTTP2_FRAME_HEADER_SIZE);

    __u8 interesting_headers = filter_relevant_headers(iterations_key, http2_ctx, headers_to_process->array, current_frame_header->stream_id, frame_payload);
    if (interesting_headers > 0) {
        process_headers(http2_ctx, headers_to_process->array, interesting_headers);
    }
}

static __always_inline __u32 http2_entrypoint(struct __sk_buff *skb, dispatcher_arguments_t *iterations_key, http2_ctx_t *http2_ctx) {
    __u32 offset = iterations_key->skb_info.data_off;
    // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
    if (offset + HTTP2_FRAME_HEADER_SIZE > skb->len) {
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
        return -1;
    }

    // Check if we should process the frame.
    switch (current_frame.type) {
    case kHeadersFrame:
        // We always want to process headers frame.
        process_headers_frame(skb, iterations_key, http2_ctx, &current_frame);
        break;
    case kDataFrame:
        if ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
            // We want to process data frame if and only if, it is an end of stream.
            break;
        }
        // fallthrough
    default:
        // Should not process the frame.
        goto end;
    }

    // If the payload is end of stream, call handle_end_of_stream.
    if ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
        http2_ctx->http2_stream_key.stream_id = current_frame.stream_id;
        handle_end_of_stream(current_frame.type, &http2_ctx->http2_stream_key);
    }

end:
    return HTTP2_FRAME_HEADER_SIZE + current_frame.length;
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
        bpf_map_update_with_telemetry(http2_iterations, &iterations_key, &iteration_value, BPF_NOEXIST);
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
    iterations_key.skb_info.data_off += tail_call_state->offset;

    // perform the http2 decoding part.
    __u32 read_size = http2_entrypoint(skb, &iterations_key, http2_ctx);
    if (read_size <= 0 || read_size == -1) {
        goto delete_iteration;
    }
    if (iterations_key.skb_info.data_off + read_size >= skb->len) {
        goto delete_iteration;
    }

    // update the tail calls state when the http2 decoding part was completed successfully.
    tail_call_state->iteration += 1;
    tail_call_state->offset += read_size;
    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS) {
        bpf_tail_call_compat(skb, &protocols_progs, PROTOCOL_HTTP2);
    }

delete_iteration:
    bpf_map_delete_elem(&http2_iterations, &iterations_key);

    return 0;
}

#endif
