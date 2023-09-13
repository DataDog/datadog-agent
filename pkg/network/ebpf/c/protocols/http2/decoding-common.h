#ifndef DECODING_COMMON_H_
#define DECODING_COMMON_H_

#include "protocols/http2/helpers.h"

// A similar implementation of read_http2_frame_header, but instead of getting both a char array and an out parameter,
// we get only the out parameter (equals to struct http2_frame * representation of the char array) and we perform the
// field adjustments we have in read_http2_frame_header.
static __always_inline bool format_http2_frame_header(struct http2_frame *out) {
    if (is_empty_frame_header((char *)out)) {
        return false;
    }

    // We extract the frame by its shape to fields.
    // See: https://datatracker.ietf.org/doc/html/rfc7540#section-4.1
    out->length = bpf_ntohl(out->length << 8);
    out->stream_id = bpf_ntohl(out->stream_id << 1);

    return out->type <= kContinuationFrame;
}

#endif // DECODING_COMMON_H_
