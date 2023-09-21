#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "ip.h"
#include "map-defs.h"

#include "protocols/classification/defs.h"
#include "protocols/http/types.h"
#include "protocols/http2/decoding-common.h"
#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/maps-defs.h"
#include "protocols/http2/tls-decoding.h"
#include "protocols/http2/usm-events.h"

// Similar to read_var_int, but with a small optimization of getting the current character as input argument.
static __always_inline bool read_var_int_with_given_current_char(struct __sk_buff *skb, skb_info_t *skb_info, __u8 current_char_as_number, __u8 max_number_for_bits, __u8 *out) {
    current_char_as_number &= max_number_for_bits;

    if (current_char_as_number < max_number_for_bits) {
        *out = current_char_as_number;
        return true;
    }

    if (skb_info->data_off <= skb->len) {
        __u8 next_char = 0;
        bpf_skb_load_bytes(skb, skb_info->data_off, &next_char, sizeof(next_char));
        if ((next_char & 128) == 0) {
            skb_info->data_off++;
            *out = current_char_as_number + next_char & 127;
            return true;
        }
    }

    return false;
}

// read_var_int reads an unsigned variable length integer off the
// beginning of p. n is the parameter as described in
// https://httpwg.org/specs/rfc7541.html#rfc.section.5.1.
//
// n must always be between 1 and 8.
//
// The returned remain buffer is either a smaller suffix of p, or err != nil.
static __always_inline bool read_var_int(struct __sk_buff *skb, skb_info_t *skb_info, __u8 max_number_for_bits, __u8 *out) {
    if (skb_info->data_off > skb->len) {
        return false;
    }
    __u8 current_char_as_number = 0;
    bpf_skb_load_bytes(skb, skb_info->data_off, &current_char_as_number, sizeof(current_char_as_number));
    skb_info->data_off++;

    return read_var_int_with_given_current_char(skb, skb_info, current_char_as_number, max_number_for_bits, out);
}

// parse_field_indexed is handling the case which the header frame is part of the static table.
READ_INTO_BUFFER(path, HTTP2_MAX_PATH_LEN, BLK_SIZE)

// parse_field_literal handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline bool parse_field_literal(struct __sk_buff *skb, skb_info_t *skb_info, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter) {
    __u8 str_len = 0;
    if (!read_var_int(skb, skb_info, MAX_6_BITS, &str_len)) {
        return false;
    }
    // The key is new and inserted into the dynamic table. So we are skipping the new value.

    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        if (!read_var_int(skb, skb_info, MAX_6_BITS, &str_len)) {
            return false;
        }
        goto end;
    }
    if (str_len > HTTP2_MAX_PATH_LEN || index != kIndexPath || headers_to_process == NULL) {
        goto end;
    }

    __u32 final_size = str_len < HTTP2_MAX_PATH_LEN ? str_len : HTTP2_MAX_PATH_LEN;
    if (skb_info->data_off + final_size > skb->len) {
        goto end;
    }

    headers_to_process->index = global_dynamic_counter - 1;
    headers_to_process->type = kNewDynamicHeader;
    headers_to_process->new_dynamic_value_offset = skb_info->data_off;
    headers_to_process->new_dynamic_value_size = str_len;
    (*interesting_headers_counter)++;
end:
    skb_info->data_off += str_len;
    return true;
}

// This function reads the http2 headers frame.
static __always_inline __u8 filter_relevant_headers(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u32 frame_length) {
    __u8 current_ch;
    __u8 interesting_headers = 0;
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
        if (!read_var_int_with_given_current_char(skb, skb_info, current_ch, max_bits, &index)) {
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
            (*global_dynamic_counter)++;
            // 6.2.1 Literal Header Field with Incremental Indexing
            // top two bits are 11
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
            if (!parse_field_literal(skb, skb_info, current_header, index, *global_dynamic_counter, &interesting_headers)) {
                break;
            }
        }
    }

    return interesting_headers;
}

static __always_inline void process_headers(struct __sk_buff *skb, dynamic_table_index_t *dynamic_index, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers) {
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
            } else if (current_header->index >= k200 && current_header->index <= k500) {
                current_stream->response_status_code = *static_value;
            } else if (current_header->index == kEmptyPath) {
                log_debug("[http2_debug] empty path");
                current_stream->path_size = HTTP_ROOT_PATH_LEN;
                bpf_memcpy(current_stream->request_path, HTTP_ROOT_PATH, HTTP_ROOT_PATH_LEN);
            } else if (current_header->index == kIndexPath) {
                log_debug("[http2_debug] index path");
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

static __always_inline void process_headers_frame(struct __sk_buff *skb, http2_stream_t *current_stream, skb_info_t *skb_info, conn_tuple_t *tup, dynamic_table_index_t *dynamic_index, struct http2_frame *current_frame_header) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = filter_relevant_headers(skb, skb_info, tup, dynamic_index, headers_to_process, current_frame_header->length);
    if (interesting_headers > 0) {
        process_headers(skb, dynamic_index, current_stream, headers_to_process, interesting_headers);
    }
}

static __always_inline void parse_frame(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, http2_ctx_t *http2_ctx, struct http2_frame *current_frame) {
    http2_ctx->http2_stream_key.stream_id = current_frame->stream_id;
    http2_stream_t *current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
    if (current_stream == NULL) {
        skb_info->data_off += current_frame->length;
        return;
    }

    if (current_frame->type == kHeadersFrame) {
        process_headers_frame(skb, current_stream, skb_info, tup, &http2_ctx->dynamic_index, current_frame);
    } else {
        skb_info->data_off += current_frame->length;
    }

    if ((current_frame->flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key);
    }

    return;
}

static __always_inline void skip_preface(struct __sk_buff *skb, skb_info_t *skb_info) {
    if (skb_info->data_off + HTTP2_MARKER_SIZE <= skb->len) {
        char preface[HTTP2_MARKER_SIZE];
        bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
        bpf_skb_load_bytes(skb, skb_info->data_off, preface, HTTP2_MARKER_SIZE);
        if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
            skb_info->data_off += HTTP2_MARKER_SIZE;
        }
    }
}

static __always_inline __u8 find_relevant_headers(struct __sk_buff *skb, skb_info_t *skb_info, http2_frame_with_offset *frames_array) {
    bool is_headers_frame, is_data_end_of_stream;
    __u8 interesting_frame_index = 0;
    struct http2_frame current_frame = {};

    // Filter preface.
    skip_preface(skb, skb_info);

#pragma unroll(HTTP2_MAX_FRAMES_TO_FILTER)
    for (__u32 iteration = 0; iteration < HTTP2_MAX_FRAMES_TO_FILTER; ++iteration) {
        // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
        if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb->len) {
            break;
        }
        if (interesting_frame_index >= HTTP2_MAX_FRAMES_ITERATIONS) {
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
        is_headers_frame = current_frame.type == kHeadersFrame;
        is_data_end_of_stream = ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) && (current_frame.type == kDataFrame);
        if (is_headers_frame || is_data_end_of_stream) {
            frames_array[interesting_frame_index].frame = current_frame;
            frames_array[interesting_frame_index].offset = skb_info->data_off;
            interesting_frame_index++;
        }
        skb_info->data_off += current_frame.length;
    }

    return interesting_frame_index;
}

SEC("socket/http2_filter")
int socket__http2_filter(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        return 0;
    }

    // If we detected a tcp termination we should stop processing the packet, and clear its dynamic table by deleting the counter.
    if (is_tcp_termination(&dispatcher_args_copy.skb_info)) {
        // Deleting the entry for the original tuple.
        bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
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
    http2_tail_call_state_t iteration_value = {};
    bpf_memset(iteration_value.frames_array, 0, HTTP2_MAX_FRAMES_ITERATIONS * sizeof(http2_frame_with_offset));

    // Some functions might change and override fields in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, having a local copy of skb_info.
    skb_info_t local_skb_info = dispatcher_args_copy.skb_info;

    // filter frames
    iteration_value.frames_count = find_relevant_headers(skb, &local_skb_info, iteration_value.frames_array);
    if (iteration_value.frames_count == 0) {
        return 0;
    }

    // We have couple of interesting headers, launching tail calls to handle them.
    if (bpf_map_update_elem(&http2_iterations, &dispatcher_args_copy, &iteration_value, BPF_NOEXIST) >= 0) {
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

    // Some functions might change and override fields in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, having a local copy of skb_info.
    skb_info_t local_skb_info = dispatcher_args_copy.skb_info;

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

    if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS || tail_call_state->iteration >= tail_call_state->frames_count) {
        goto delete_iteration;
    }
    http2_frame_with_offset current_frame = tail_call_state->frames_array[tail_call_state->iteration];

    const __u32 zero = 0;
    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }

    // create the http2 ctx for the current http2 frame.
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);
    http2_ctx->dynamic_index.tup = dispatcher_args_copy.tup;
    local_skb_info.data_off = current_frame.offset;

    parse_frame(skb, &local_skb_info, &dispatcher_args_copy.tup, http2_ctx, &current_frame.frame);
    if (local_skb_info.data_off >= skb->len) {
        goto delete_iteration;
    }

    // update the tail calls state when the http2 decoding part was completed successfully.
    tail_call_state->iteration += 1;
    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS && tail_call_state->iteration < tail_call_state->frames_count) {
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_PARSER);
    }

delete_iteration:
    bpf_map_delete_elem(&http2_iterations, &dispatcher_args_copy);

    return 0;
}

/* TLS */

SEC("uprobe/http2_tls_entry")
int uprobe__http2_tls_entry(struct pt_regs *ctx) {
    log_debug("http2_tls_entry: after tail call");
    const u32 zero = 0;

    tls_dispatcher_arguments_t *info = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (info == NULL) {
        log_debug("[http2_tls_entry] could not get tls info from map");
        return 0;
    }

    log_debug("[http2_tls_entry] info: buf=%p, len=%lu, off=%lu", info->buf, info->len, info->off);

    // TODO: from tls_info: add bool from tls_finish to trigger this case
    //
    // If we detected a tcp termination we should stop processing the packet, and clear its dynamic table by deleting the counter.
    /* if (is_tcp_termination(&dispatcher_args_copy.skb_info)) { */
    /*     // Deleting the entry for the original tuple. */
    /*     bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup); */
    /*     // In case of local host, the protocol will be deleted for both (client->server) and (server->client), */
    /*     // so we won't reach for that path again in the code, so we're deleting the opposite side as well. */
    /*     flip_tuple(&dispatcher_args_copy.tup); */
    /*     bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup); */
    /*     return 0; */
    /* } */

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t iteration_value = {};
    bpf_memset(iteration_value.frames_array, 0, HTTP2_MAX_FRAMES_ITERATIONS * sizeof(http2_frame_with_offset));

    // filter frames
    iteration_value.frames_count = find_relevant_headers_tls(info, iteration_value.frames_array);
    log_debug("[http2_tls_entry] frames count: %d\n", iteration_value.frames_count);
    if (iteration_value.frames_count == 0) {
        return 0;
    }

    // We have couple of interesting headers, launching tail calls to handle them.
    if (bpf_map_update_elem(&http2_tls_iterations, &zero, &iteration_value, BPF_ANY) >= 0) {
        // We managed to cache the iteration_value in the http2_iterations map.
        bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_FRAMES_PARSER);
    }

    return 0;
}

SEC("uprobe/http2_tls_frames_parser")
int uprobe__http2_tls_frames_parser(struct pt_regs *ctx) {
    log_debug("http2_tls_frames_parser: after tail call");
    const u32 zero = 0;

    tls_dispatcher_arguments_t *info = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (info == NULL) {
        log_debug("[http2_tls_entry] could not get tls info from map");
        return 0;
    }

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&http2_tls_iterations, &zero);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS || tail_call_state->iteration >= tail_call_state->frames_count) {
        goto exit;
    }
    http2_frame_with_offset current_frame = tail_call_state->frames_array[tail_call_state->iteration];

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto exit;
    }

    // create the http2 ctx for the current http2 frame.
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = info->tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);
    http2_ctx->dynamic_index.tup = info->tup;
    info->off = current_frame.offset;

    parse_frame_tls(info, http2_ctx, &current_frame.frame);
    if (info->off >= info->len) {
        goto exit;
    }

    /* // update the tail calls state when the http2 decoding part was completed successfully. */
    tail_call_state->iteration += 1;
    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS && tail_call_state->iteration < tail_call_state->frames_count) {
        bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_FRAMES_PARSER);
    }

exit:

    return 0;
}

#endif
