#ifndef TLS_DECODING_H_
#define TLS_DECODING_H_

#include "bpf_builtins.h"
/* #include "bpf_helpers.h" */
/* #include "ip.h" */
/* #include "map-defs.h" */

/* #include "protocols/classification/defs.h" */
/* #include "protocols/http/types.h" */
#include "helpers.h"
#include "protocols/http/buffer.h"
#include "protocols/http2/decoding-common.h"
#include "protocols/http2/maps-defs.h"
/* #include "protocols/http2/usm-events.h" */
#include "protocols/tls/https-maps.h"

READ_INTO_USER_BUFFER(http2_preface, HTTP2_MARKER_SIZE)
READ_INTO_USER_BUFFER(http2_frame_header, HTTP2_FRAME_HEADER_SIZE)
READ_INTO_USER_BUFFER(http2_char, sizeof(__u8))
READ_INTO_USER_BUFFER(path, HTTP2_MAX_PATH_LEN)

static __always_inline void skip_preface_tls(http2_tls_info_t *info) {
    if (info->offset + HTTP2_MARKER_SIZE <= info->len) {
        char preface[HTTP2_MARKER_SIZE];
        bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
        read_into_user_buffer_http2_preface(preface, info->buf + info->offset);
        if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
            info->offset += HTTP2_MARKER_SIZE;
        }
    }
}

// Similar to read_var_int_tls, but with a small optimization of getting the current character as input argument.
static __always_inline bool read_var_int_with_given_current_char_tls(http2_tls_info_t *info, __u8 current_char_as_number, __u8 max_number_for_bits, __u8 *out) {
    current_char_as_number &= max_number_for_bits;

    if (current_char_as_number < max_number_for_bits) {
        *out = current_char_as_number;
        return true;
    }

    if (info->offset <= info->len) {
        __u8 next_char = 0;
        read_into_user_buffer_http2_char(&next_char, info->buf + info->offset);
        if ((next_char & 128) == 0) {
            info->offset++;
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
static __always_inline bool read_var_int_tls(http2_tls_info_t *info, __u8 max_number_for_bits, __u8 *out) {
    if (info->offset > info->len) {
        return false;
    }
    __u8 current_char_as_number = 0;
    read_into_user_buffer_http2_char(&current_char_as_number, info->buf + info->offset);
    info->offset++;

    return read_var_int_with_given_current_char_tls(info, current_char_as_number, max_number_for_bits, out);
}

static __always_inline __u8 find_relevant_headers_tls(http2_tls_info_t *info, http2_frame_with_offset *frames_array) {
    bool is_headers_frame, is_data_end_of_stream;
    __u8 interesting_frame_index = 0;
    struct http2_frame current_frame = {};

    (void)is_data_end_of_stream;

    // Filter preface.
    skip_preface_tls(info);

#pragma unroll(HTTP2_MAX_FRAMES_TO_FILTER)
    for (__u32 iteration = 0; iteration < HTTP2_MAX_FRAMES_TO_FILTER; ++iteration) {
        // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
        if (info->offset + HTTP2_FRAME_HEADER_SIZE > info->len) {
            break;
        }
        if (interesting_frame_index >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }

        read_into_user_buffer_http2_frame_header((char *)&current_frame, info->buf + info->offset);
        info->offset += HTTP2_FRAME_HEADER_SIZE;
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
            frames_array[interesting_frame_index].offset = info->offset;
            interesting_frame_index++;
        }
        info->offset += current_frame.length;
    }

    return interesting_frame_index;
}

// parse_field_literal_tls handling the case when the key is part of the static table and the value is a dynamic string
// which will be stored in the dynamic table.
static __always_inline bool parse_field_literal_tls(http2_tls_info_t *info, http2_header_t *headers_to_process, __u8 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter) {
    __u8 str_len = 0;
    if (!read_var_int_tls(info, MAX_6_BITS, &str_len)) {
        return false;
    }
    // The key is new and inserted into the dynamic table. So we are skipping the new value.

    if (index == 0) {
        info->offset += str_len;
        str_len = 0;
        if (!read_var_int_tls(info, MAX_6_BITS, &str_len)) {
            return false;
        }
        goto end;
    }
    if (str_len > HTTP2_MAX_PATH_LEN || index != kIndexPath || headers_to_process == NULL) {
        goto end;
    }

    __u32 final_size = str_len < HTTP2_MAX_PATH_LEN ? str_len : HTTP2_MAX_PATH_LEN;
    if (info->offset + final_size > info->len) {
        goto end;
    }

    headers_to_process->index = global_dynamic_counter - 1;
    headers_to_process->type = kNewDynamicHeader;
    headers_to_process->new_dynamic_value_offset = info->offset;
    headers_to_process->new_dynamic_value_size = str_len;
    (*interesting_headers_counter)++;
end:
    info->offset += str_len;
    return true;
}

static __always_inline __u8 filter_relevant_headers_tls(http2_tls_info_t *info, dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u32 frame_length) {
    __u8 current_ch;
    __u8 interesting_headers = 0;
    http2_header_t *current_header;
    const __u32 frame_end = info->offset + frame_length;
    const __u32 end = frame_end < info->len + 1 ? frame_end : info->len + 1;
    bool is_literal = false;
    bool is_indexed = false;
    __u8 max_bits = 0;
    __u8 index = 0;

    __u64 *global_dynamic_counter = get_dynamic_counter(&info->conn);
    if (global_dynamic_counter == NULL) {
        return 0;
    }

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (info->offset >= end) {
            break;
        }
        read_into_user_buffer_http2_char(&current_ch, info->buf + info->offset);
        info->offset++;

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
        if (!read_var_int_with_given_current_char_tls(info, current_ch, max_bits, &index)) {
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
            if (!parse_field_literal_tls(info, current_header, index, *global_dynamic_counter, &interesting_headers)) {
                break;
            }
        }
    }

    log_debug("[filter_relevant_headers_tls] interesting headers: %d\n", interesting_headers);

    return interesting_headers;
}

static __always_inline void process_headers_tls(http2_tls_info_t *info, dynamic_table_index_t *dynamic_index, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers) {
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
            read_into_user_buffer_path((char *)&dynamic_value.buffer, info->buf + current_header->new_dynamic_value_offset);
            bpf_map_update_elem(&http2_dynamic_table, dynamic_index, &dynamic_value, BPF_ANY);
            current_stream->path_size = current_header->new_dynamic_value_size;
            bpf_memcpy(current_stream->request_path, dynamic_value.buffer, HTTP2_MAX_PATH_LEN);
        }
    }
}

static __always_inline void process_headers_frame_tls(http2_tls_info_t *info, http2_stream_t *current_stream, dynamic_table_index_t *dynamic_index, struct http2_frame *current_frame_header) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = filter_relevant_headers_tls(info, dynamic_index, headers_to_process, current_frame_header->length);
    if (interesting_headers > 0) {
        process_headers_tls(info, dynamic_index, current_stream, headers_to_process, interesting_headers);
    }
}

static __always_inline void parse_frame_tls(http2_tls_info_t *info, http2_ctx_t *http2_ctx, struct http2_frame *current_frame) {
    http2_ctx->http2_stream_key.stream_id = current_frame->stream_id;
    http2_stream_t *current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
    if (current_stream == NULL) {
        info->offset += current_frame->length;
        return;
    }

    if (current_frame->type == kHeadersFrame) {
        process_headers_frame_tls(info, current_stream, &http2_ctx->dynamic_index, current_frame);
    } else {
        info->offset += current_frame->length;
    }

    if ((current_frame->flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key);
    }

    return;
}

#endif // TLS_DECODING_H_
