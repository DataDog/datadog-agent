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
/* #include "protocols/http2/maps-defs.h" */
/* #include "protocols/http2/usm-events.h" */
#include "protocols/tls/https-maps.h"

READ_INTO_USER_BUFFER(http2_preface, HTTP2_MARKER_SIZE)
READ_INTO_USER_BUFFER(http2_frame_header, HTTP2_FRAME_HEADER_SIZE)

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

#endif // TLS_DECODING_H_
