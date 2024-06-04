#ifndef __HTTP2_DECODING_TLS_H
#define __HTTP2_DECODING_TLS_H

#include "protocols/http2/decoding-common.h"
#include "protocols/http2/usm-events.h"
#include "protocols/http/types.h"

READ_INTO_USER_BUFFER_WITHOUT_TELEMETRY(http2_path, HTTP2_MAX_PATH_LEN)

// Similar to tls_read_hpack_int, but with a small optimization of getting the
// current character as input argument.
static __always_inline bool tls_read_hpack_int_with_given_current_char(tls_dispatcher_arguments_t *info, __u64 current_char_as_number, __u64 max_number_for_bits, __u64 *out) {
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
    if (bpf_probe_read_user(&next_char, sizeof(__u8), info->buffer_ptr + info->data_off) >= 0 && (next_char & 128) == 0) {
        info->data_off++;
        *out = current_char_as_number + (next_char & 127);
        return true;
    }

    return false;
}

// tls_read_hpack_int reads an unsigned variable length integer as specified in the
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
static __always_inline bool tls_read_hpack_int(tls_dispatcher_arguments_t *info, __u64 max_number_for_bits, __u64 *out, bool *is_huffman_encoded) {
    __u64 current_char_as_number = 0;
    if (bpf_probe_read_user(&current_char_as_number, sizeof(__u8), info->buffer_ptr + info->data_off) < 0) {
        return false;
    }
    info->data_off++;
    // We are only interested in the first bit of the first byte, which indicates if it is huffman encoded or not.
    // See: https://datatracker.ietf.org/doc/html/rfc7541#appendix-B for more details on huffman code.
    *is_huffman_encoded = (current_char_as_number & 128) > 0;

    return tls_read_hpack_int_with_given_current_char(info, current_char_as_number, max_number_for_bits, out);
}

// Handles a literal header, and updates the offset. This function is meant to run on not interesting literal headers.
static __always_inline bool tls_process_and_skip_literal_headers(tls_dispatcher_arguments_t *info, __u64 index) {
    __u64 str_len = 0;
    bool is_huffman_encoded = false;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!tls_read_hpack_int(info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        info->data_off += str_len;
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        // At this point the huffman code is not interesting due to the fact that we already read the string length,
        // We are reading the current size in order to skip it.
        if (!tls_read_hpack_int(info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
            return false;
        }
    }
    info->data_off += str_len;
    return true;
}

// tls_parse_field_literal parses a header with a literal value.
//
// We are only interested in path headers, that we will store in our internal
// dynamic table, and will skip headers that are not path headers.
static __always_inline bool tls_parse_field_literal(tls_dispatcher_arguments_t *info, http2_header_t *headers_to_process, __u64 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter, http2_telemetry_t *http2_tel, bool save_header) {
    __u64 str_len = 0;
    bool is_huffman_encoded = false;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!tls_read_hpack_int(info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        info->data_off += str_len;
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        // At this point the huffman code is not interesting due to the fact that we already read the string length,
        // We are reading the current size in order to skip it.
        if (!tls_read_hpack_int(info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
            return false;
        }
        goto end;
    }

    // Path headers in HTTP2 that are not "/" or "/index.html"  are represented
    // with an indexed name, literal value, reusing the index 4 and 5 in the
    // static table. A different index means that the header is not a path, so
    // we skip it.
    if (is_path_index(index)) {
        update_path_size_telemetry(http2_tel, str_len);
    } else if ((!is_status_index(index)) && (!is_method_index(index))) {
        goto end;
    }

    // We skip if:
    // - The string is too big
    // - This is not a path
    // - We won't be able to store the header info
    if (headers_to_process == NULL) {
        goto end;
    }

    if (info->data_off + str_len > info->data_end) {
        __sync_fetch_and_add(&http2_tel->literal_value_exceeds_frame, 1);
        goto end;
    }

    if (save_header) {
        headers_to_process->index = global_dynamic_counter - 1;
        headers_to_process->type = kNewDynamicHeader;
    } else {
        headers_to_process->type = kNewDynamicHeaderNotIndexed;
    }
    headers_to_process->original_index = index;
    headers_to_process->new_dynamic_value_offset = info->data_off;
    headers_to_process->new_dynamic_value_size = str_len;
    headers_to_process->is_huffman_encoded = is_huffman_encoded;
    // If the string len (`str_len`) is in the range of [0, HTTP2_MAX_PATH_LEN], and we don't exceed packet boundaries
    // (info->off + str_len <= info->len) and the index is kIndexPath, then we have a path header,
    // and we're increasing the counter. In any other case, we're not increasing the counter.
    *interesting_headers_counter += (str_len > 0 && str_len <= HTTP2_MAX_PATH_LEN);
end:
    info->data_off += str_len;
    return true;
}

// tls_handle_dynamic_table_update handles the dynamic table size update.
static __always_inline void tls_handle_dynamic_table_update(tls_dispatcher_arguments_t *info){
    // To determine the size of the dynamic table update, we read an integer representation byte by byte.
    // We continue reading bytes until we encounter a byte without the Most Significant Bit (MSB) set,
    // indicating that we've consumed the complete integer. While in the context of the dynamic table
    // update, we set the state as true if the MSB is set, and false otherwise. Then, we proceed to the next byte.
    // More on the feature - https://httpwg.org/specs/rfc7541.html#rfc.section.6.3.
    __u8 current_ch;
    bpf_probe_read_user(&current_ch, sizeof(current_ch), info->buffer_ptr + info->data_off);
    // If the top 3 bits are 001, then we have a dynamic table size update.
    if ((current_ch & 224) == 32) {
        info->data_off++;
    #pragma unroll(HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS)
        for (__u8 iter = 0; iter < HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS; ++iter) {
            bpf_probe_read_user(&current_ch, sizeof(current_ch), info->buffer_ptr + info->data_off);
            info->data_off++;
            if ((current_ch & 128) == 0) {
                return;
            }
        }
    }
}

// tls_filter_relevant_headers parses the http2 headers frame, and filters headers
// that are relevant for us, to be processed later on.
// The return value is the number of relevant headers that were found and inserted
// in the `headers_to_process` table.
static __always_inline __u8 tls_filter_relevant_headers(tls_dispatcher_arguments_t *info, dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u32 frame_length, http2_telemetry_t *http2_tel) {
    __u8 current_ch;
    __u8 interesting_headers = 0;
    http2_header_t *current_header;
    const __u32 frame_end = info->data_off + frame_length;
    const __u32 end = frame_end < info->data_end + 1 ? frame_end : info->data_end + 1;
    bool is_indexed = false;
    bool is_literal = false;
    __u64 max_bits = 0;
    __u64 index = 0;

    __u64 *global_dynamic_counter = get_dynamic_counter(&info->tup);
    if (global_dynamic_counter == NULL) {
        return 0;
    }

    tls_handle_dynamic_table_update(info);

#pragma unroll(HTTP2_MAX_PSEUDO_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_PSEUDO_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (info->data_off >= end) {
            break;
        }
        bpf_probe_read_user(&current_ch, sizeof(current_ch), info->buffer_ptr + info->data_off);
        info->data_off++;

        is_indexed = (current_ch & 128) != 0;
        is_literal = (current_ch & 192) == 64;
        // If all (is_indexed, is_literal, is_dynamic_table_update) are false, then we
        // have a literal header field without indexing (prefix 0000) or literal header field never indexed (prefix 0001).

        max_bits = MAX_4_BITS;
        // If we're in an indexed header - the max bits are 7.
        max_bits = is_indexed ? MAX_7_BITS : max_bits;
        // else, if we're in a literal header - the max bits are 6.
        max_bits = is_literal ? MAX_6_BITS : max_bits;
        // otherwise, we're in literal header without indexing or literal header never indexed - and for both, the
        // max bits are 4.
        // See RFC7541 - https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.2

        index = 0;
        if (!tls_read_hpack_int_with_given_current_char(info, current_ch, max_bits, &index)) {
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
            continue;
        }
        // Increment the global dynamic counter for each literal header field.
        // We're not increasing the counter for literal without indexing or literal never indexed.
        __sync_fetch_and_add(global_dynamic_counter, is_literal);

        // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
        if (!tls_parse_field_literal(info, current_header, index, *global_dynamic_counter, &interesting_headers, http2_tel, is_literal)) {
            break;
        }
    }

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (info->data_off >= end) {
            break;
        }
        bpf_probe_read_user(&current_ch, sizeof(current_ch), info->buffer_ptr + info->data_off);
        info->data_off++;

        is_indexed = (current_ch & 128) != 0;
        is_literal = (current_ch & 192) == 64;
        // If all (is_indexed, is_literal, is_dynamic_table_update) are false, then we
        // have a literal header field without indexing (prefix 0000) or literal header field never indexed (prefix 0001).

        max_bits = MAX_4_BITS;
        // If we're in an indexed header - the max bits are 7.
        max_bits = is_indexed ? MAX_7_BITS : max_bits;
        // else, if we're in a literal header - the max bits are 6.
        max_bits = is_literal ? MAX_6_BITS : max_bits;
        // otherwise, we're in literal header without indexing or literal header never indexed - and for both, the
        // max bits are 4.
        // See RFC7541 - https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.2

        index = 0;
        if (!tls_read_hpack_int_with_given_current_char(info, current_ch, max_bits, &index)) {
            break;
        }

        if (is_indexed) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            continue;
        }
        // Increment the global dynamic counter for each literal header field.
        // We're not increasing the counter for literal without indexing or literal never indexed.
        __sync_fetch_and_add(global_dynamic_counter, is_literal);

        // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
        if (!tls_process_and_skip_literal_headers(info, index)) {
            break;
        }
    }

    return interesting_headers;
}

// tls_process_headers processes the headers that were filtered in
// tls_filter_relevant_headers, looking for requests path, status code, and method.
static __always_inline void tls_process_headers(tls_dispatcher_arguments_t *info, dynamic_table_index_t *dynamic_index, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers, http2_telemetry_t *http2_tel) {
    http2_header_t *current_header;
    dynamic_table_entry_t dynamic_value = {};

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING; ++iteration) {
        if (iteration >= interesting_headers) {
            break;
        }

        current_header = &headers_to_process[iteration];

        if (current_header->type == kStaticHeader) {
            if (is_method_index(current_header->index)) {
                // TODO: mark request
                current_stream->request_method.static_table_entry = current_header->index;
                current_stream->request_method.finalized = true;
                __sync_fetch_and_add(&http2_tel->request_seen, 1);
            } else if (is_status_index(current_header->index)) {
                current_stream->status_code.static_table_entry = current_header->index;
                current_stream->status_code.finalized = true;
                __sync_fetch_and_add(&http2_tel->response_seen, 1);
            } else if (is_path_index(current_header->index)) {
                current_stream->path.static_table_entry = current_header->index;
                current_stream->path.finalized = true;
            }
            continue;
        }

        dynamic_index->index = current_header->index;
        if (current_header->type == kExistingDynamicHeader) {
            dynamic_table_entry_t *dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, dynamic_index);
            if (dynamic_value == NULL) {
                break;
            }
            if (is_path_index(dynamic_value->original_index)) {
                current_stream->path.length = dynamic_value->string_len;
                current_stream->path.is_huffman_encoded = dynamic_value->is_huffman_encoded;
                current_stream->path.finalized = true;
                bpf_memcpy(current_stream->path.raw_buffer, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
            } else if (is_status_index(dynamic_value->original_index)) {
                bpf_memcpy(current_stream->status_code.raw_buffer, dynamic_value->buffer, HTTP2_STATUS_CODE_MAX_LEN);
                current_stream->status_code.is_huffman_encoded = dynamic_value->is_huffman_encoded;
                current_stream->status_code.finalized = true;
            } else if (is_method_index(dynamic_value->original_index)) {
                bpf_memcpy(current_stream->request_method.raw_buffer, dynamic_value->buffer, HTTP2_METHOD_MAX_LEN);
                current_stream->request_method.is_huffman_encoded = dynamic_value->is_huffman_encoded;
                current_stream->request_method.length = dynamic_value->string_len;
                current_stream->request_method.finalized = true;
            }
        } else {
            // We're in new dynamic header or new dynamic header not indexed states.
            read_into_user_buffer_http2_path(dynamic_value.buffer, info->buffer_ptr + current_header->new_dynamic_value_offset);
            // If the value is indexed - add it to the dynamic table.
            if (current_header->type == kNewDynamicHeader) {
                dynamic_value.string_len = current_header->new_dynamic_value_size;
                dynamic_value.is_huffman_encoded = current_header->is_huffman_encoded;
                dynamic_value.original_index = current_header->original_index;
                bpf_map_update_elem(&http2_dynamic_table, dynamic_index, &dynamic_value, BPF_ANY);
            }
            if (is_path_index(current_header->original_index)) {
                current_stream->path.length = current_header->new_dynamic_value_size;
                current_stream->path.is_huffman_encoded = current_header->is_huffman_encoded;
                current_stream->path.finalized = true;
                bpf_memcpy(current_stream->path.raw_buffer, dynamic_value.buffer, HTTP2_MAX_PATH_LEN);
            } else if (is_status_index(current_header->original_index)) {
                bpf_memcpy(current_stream->status_code.raw_buffer, dynamic_value.buffer, HTTP2_STATUS_CODE_MAX_LEN);
                current_stream->status_code.is_huffman_encoded = current_header->is_huffman_encoded;
                current_stream->status_code.finalized = true;
            } else if (is_method_index(current_header->original_index)) {
                bpf_memcpy(current_stream->request_method.raw_buffer, dynamic_value.buffer, HTTP2_METHOD_MAX_LEN);
                current_stream->request_method.is_huffman_encoded = current_header->is_huffman_encoded;
                current_stream->request_method.length = current_header->new_dynamic_value_size;
                current_stream->request_method.finalized = true;
            }
        }
    }
}

static __always_inline void tls_process_headers_frame(tls_dispatcher_arguments_t *info, http2_stream_t *current_stream, dynamic_table_index_t *dynamic_index, http2_frame_t *current_frame_header, http2_telemetry_t *http2_tel) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = tls_filter_relevant_headers(info, dynamic_index, headers_to_process, current_frame_header->length, http2_tel);
    tls_process_headers(info, dynamic_index, current_stream, headers_to_process, interesting_headers, http2_tel);
}

// http2_tls_handle_first_frame is the entry point of our HTTP2+TLS processing.
// It is responsible for getting and filtering the first frame present in the
// buffer we get from the TLS uprobes.
//
// This first frame needs special handling as it may be split between multiple
// two buffers, and we may have the first part of the first frame from the
// processing of the previous buffer, in which case http2_tls_handle_first_frame
// will try to complete the frame.
//
// Once we have the first frame, we can continue to the regular frame filtering
// program.
SEC("uprobe/http2_tls_handle_first_frame")
int uprobe__http2_tls_handle_first_frame(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of tls_dispatcher_arguments, so the next prog will start to
    // read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    handle_first_frame(pkt, (__u32*)&args->data_off, &dispatcher_args_copy.tup);
    return 0;
}

// http2_tls_filter finds and filters the HTTP2 frames from the buffer got from
// the TLS probes. Interesting frames are saved to be parsed in
// http2_tls_headers_parser.
SEC("uprobe/http2_tls_filter")
int uprobe__http2_tls_filter(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of the tls_dispatcher_arguments, so the next prog will start
    // to read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    filter_frame(pkt, &dispatcher_args_copy, &dispatcher_args_copy.tup);
    return 0;
}


// The program is responsible for parsing all headers frames. For each headers frame we parse the headers,
// fill the dynamic table with the new interesting literal headers, and modifying the streams accordingly.
// The program can be called multiple times (via "self call" of tail calls) in case we have more frames to parse
// than the maximum number of frames we can process in a single tail call.
// The program is being called after uprobe__http2_tls_filter, and it is being called only if we have interesting frames.
// The program calls uprobe__http2_dynamic_table_cleaner to clean the dynamic table if needed.
SEC("uprobe/http2_tls_headers_parser")
int uprobe__http2_tls_headers_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the
    // `off` field of tls_dispatcher_arguments, so the next prog will start to
    // read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    pktbuf_t pkt = pktbuf_from_tls(ctx, &dispatcher_args_copy);

    // Some functions might change and override data_off field in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, storing the original value of the offset.
    __u32 original_off = pktbuf_data_offset(pkt);

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    pktbuf_map_lookup_option_t arr[] = {
        [PKTBUF_TLS] = {
            .map = &tls_http2_iterations,
            .key = &dispatcher_args_copy,
        },
    };
    http2_tail_call_state_t *tail_call_state = pktbuf_map_lookup(pkt, arr);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }

    http2_telemetry_t *http2_tel = get_telemetry(pkt);
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

    http2_stream_t *current_stream = NULL;

    #pragma unroll(HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL)
    for (__u16 index = 0; index < HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        // This check must be next to the access of the array, otherwise the verifier will complain.
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }
        current_frame = frames_array[tail_call_state->iteration];
        tail_call_state->iteration += 1;

        if (current_frame.frame.type != kHeadersFrame) {
            continue;
        }

        http2_ctx->http2_stream_key.stream_id = current_frame.frame.stream_id;
        current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
        if (current_stream == NULL) {
            continue;
        }
        pktbuf_set_offset(pkt, current_frame.offset);
        current_stream->tags |= args->tags;
        tls_process_headers_frame(&dispatcher_args_copy, current_stream, &http2_ctx->dynamic_index, &current_frame.frame, http2_tel);
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS &&
        tail_call_state->iteration < tail_call_state->frames_count &&
        tail_call_state->iteration < HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER) {
        bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_HEADERS_PARSER);
    }
    // Zeroing the iteration index to call EOS parser
    tail_call_state->iteration = 0;
    bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_DYNAMIC_TABLE_CLEANER);

delete_iteration:
    // restoring the original value.
    pktbuf_set_offset(pkt, original_off);
    pktbuf_map_delete(pkt, arr);

    return 0;
}

// The program is responsible for cleaning the dynamic table.
// The program calls uprobe__http2_tls_eos_parser to finalize the streams and enqueue them to be sent to the user mode.
SEC("uprobe/http2_dynamic_table_cleaner")
int uprobe__http2_dynamic_table_cleaner(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the `off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    dynamic_counter_t *dynamic_counter = bpf_map_lookup_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
    if (dynamic_counter == NULL) {
        goto next;
    }

    // We're checking if the difference between the current value of the dynamic global table, to the previous index we
    // cleaned, is bigger than our threshold. If so, we need to clean the table.
    if (dynamic_counter->value - dynamic_counter->previous <= HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD) {
        goto next;
    }

    dynamic_table_index_t dynamic_index = {
        .tup = dispatcher_args_copy.tup,
    };

    #pragma unroll(HTTP2_DYNAMIC_TABLE_CLEANUP_ITERATIONS)
    for (__u16 index = 0; index < HTTP2_DYNAMIC_TABLE_CLEANUP_ITERATIONS; index++) {
        // We should reserve the last HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD entries in the dynamic table.
        // So if we're about to delete an entry that is in the last HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD entries,
        // we should stop the cleanup.
        if (dynamic_counter->previous + HTTP2_DYNAMIC_TABLE_CLEANUP_THRESHOLD >= dynamic_counter->value) {
            break;
        }
        // Setting the current index.
        dynamic_index.index = dynamic_counter->previous;
        // Trying to delete the entry, it might not exist, so we're ignoring the return value.
        bpf_map_delete_elem(&http2_dynamic_table, &dynamic_index);
        // Incrementing the previous index.
        dynamic_counter->previous++;
    }

next:
    bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_EOS_PARSER);

    return 0;
}

// The program is responsible for parsing all frames that mark the end of a stream.
// We consider a frame as marking the end of a stream if it is either:
//  - An headers or data frame with END_STREAM flag set.
//  - An RST_STREAM frame.
// The program is being called after http2_dynamic_table_cleaner, and it finalizes the streams and enqueue them
// to be sent to the user mode.
// The program is ready to be called multiple times (via "self call" of tail calls) in case we have more frames to
// process than the maximum number of frames we can process in a single tail call.
SEC("uprobe/http2_tls_eos_parser")
int uprobe__http2_tls_eos_parser(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t dispatcher_args_copy;
    // We're not calling fetch_dispatching_arguments as, we need to modify the `off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    dispatcher_args_copy = *args;

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&tls_http2_iterations, &dispatcher_args_copy);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&tls_http2_telemetry, &zero);
    if (http2_tel == NULL) {
        goto delete_iteration;
    }

    http2_frame_with_offset *frames_array = tail_call_state->frames_array;
    http2_frame_with_offset current_frame;

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);

    bool is_rst = false, is_end_of_stream = false;
    http2_stream_t *current_stream = NULL;

    #pragma unroll(HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL)
    for (__u16 index = 0; index < HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }

        current_frame = frames_array[tail_call_state->iteration];
        // Having this condition after assignment and not before is due to a verifier issue.
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        tail_call_state->iteration += 1;

        is_rst = current_frame.frame.type == kRSTStreamFrame;
        is_end_of_stream = (current_frame.frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
        if (!is_rst && !is_end_of_stream) {
            continue;
        }

        http2_ctx->http2_stream_key.stream_id = current_frame.frame.stream_id;
        // A new stream must start with a request, so if it does not exist, we should not process it.
        current_stream = bpf_map_lookup_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
        if (current_stream == NULL) {
            continue;
        }

        // When we accept an RST, it means that the current stream is terminated.
        // See: https://datatracker.ietf.org/doc/html/rfc7540#section-6.4
        // If rst, and stream is empty (no status code, or no response) then delete from inflight
        if (is_rst && (!current_stream->status_code.finalized || !current_stream->request_method.finalized || !current_stream->path.finalized)) {
            bpf_map_delete_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
            continue;
        }

        if (is_rst) {
            __sync_fetch_and_add(&http2_tel->end_of_stream_rst, 1);
        } else if ((current_frame.frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
            __sync_fetch_and_add(&http2_tel->end_of_stream, 1);
        }
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key, http2_tel);

        // If we reached here, it means that we saw End Of Stream. If the End of Stream came from a request,
        // thus we except it to have a valid path. If the End of Stream came from a response, we except it to
        // be after seeing a request, thus it should have a path as well.
        if ((!current_stream->path.finalized) || (!current_stream->request_method.finalized)) {
            bpf_map_delete_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
        }
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS &&
        tail_call_state->iteration < tail_call_state->frames_count &&
        tail_call_state->iteration < HTTP2_MAX_FRAMES_FOR_EOS_PARSER) {
        bpf_tail_call_compat(ctx, &tls_process_progs, TLS_HTTP2_EOS_PARSER);
    }

delete_iteration:
    bpf_map_delete_elem(&tls_http2_iterations, &dispatcher_args_copy);

    return 0;
}

// http2_tls_termination is responsible for cleaning up the state of the HTTP2
// decoding once the TLS connection is terminated.
SEC("uprobe/http2_tls_termination")
int uprobe__http2_tls_termination(struct pt_regs *ctx) {
    const __u32 zero = 0;

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        return 0;
    }

    bpf_map_delete_elem(&tls_http2_iterations, &args->tup);

    terminated_http2_batch_enqueue(&args->tup);
    // Deleting the entry for the original tuple.
    bpf_map_delete_elem(&http2_remainder, &args->tup);
    bpf_map_delete_elem(&http2_dynamic_counter_table, &args->tup);
    // In case of local host, the protocol will be deleted for both (client->server) and (server->client),
    // so we won't reach for that path again in the code, so we're deleting the opposite side as well.
    flip_tuple(&args->tup);
    bpf_map_delete_elem(&http2_dynamic_counter_table, &args->tup);
    bpf_map_delete_elem(&http2_remainder, &args->tup);

    return 0;
}
#endif
