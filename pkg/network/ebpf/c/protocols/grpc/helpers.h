#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/defs.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/skb-common.h"
#include "protocols/grpc/defs.h"

#define GRPC_MAX_FRAMES_TO_FILTER 45
// We only try to process one frame at the moment. Trying to process more yields
// a verifier issue due to the way clang manages a pointer to the stack.
#define GRPC_MAX_FRAMES_TO_PROCESS 5
#define GRPC_MAX_HEADERS_TO_PROCESS 20

// The HPACK specification defines the specific Huffman encoding used for string
// literals in HPACK. This allows us to precomputed the encoded string for
// "application/grpc". Even though it is huffman encoded, this particular string
// is byte-aligned and can be compared without any masking on the final byte.
#define GRPC_ENCODED_CONTENT_TYPE "\x1d\x75\xd0\x62\x0d\x26\x3d\x4c\x4d\x65\x64"
#define GRPC_CONTENT_TYPE_LEN (sizeof(GRPC_ENCODED_CONTENT_TYPE) - 1)

static __always_inline bool format_http2_frame_header(http2_frame_t *out) {
    if (is_empty_frame_header((char *)out)) {
        return false;
    }

    // We extract the frame by its shape to fields.
    // See: https://datatracker.ietf.org/doc/html/rfc7540#section-4.1
    out->length = bpf_ntohl(out->length << 8);
    out->stream_id = bpf_ntohl(out->stream_id << 1);

    return out->type <= kContinuationFrame && out->length <= MAX_FRAME_SIZE && (out->stream_id == 0 || (out->stream_id % 2 == 1));
}

static __always_inline bool is_encoded_grpc_content_type(const char *content_type_buf) {
    return !bpf_memcmp(content_type_buf, GRPC_ENCODED_CONTENT_TYPE, GRPC_CONTENT_TYPE_LEN);
}

static __always_inline grpc_status_t is_content_type_grpc(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end, __u8 *content_type_buf) {
    string_literal_header_t len;
    if (skb_info->data_off + sizeof(len) > frame_end) {
        return PAYLOAD_NOT_GRPC;
    }

    bpf_skb_load_bytes(skb, skb_info->data_off, &len, sizeof(len));
    skb_info->data_off += sizeof(len);

    // Check if the content-type length allows holding *at least* "application/grpc".
    // The size *can be larger* as some implementations will for example use
    // "application/grpc+protobuf" and we want to match those.
    if (len.length < GRPC_CONTENT_TYPE_LEN) {
        return PAYLOAD_NOT_GRPC;
    }

    // Ensuring we can read at least the expected content-type length.
    if (skb_info->data_off + GRPC_CONTENT_TYPE_LEN > frame_end) {
        return PAYLOAD_NOT_GRPC;
    }

    bpf_skb_load_bytes(skb, skb_info->data_off, content_type_buf, GRPC_CONTENT_TYPE_LEN);
    skb_info->data_off += len.length;

    return is_encoded_grpc_content_type(content_type_buf) ? PAYLOAD_GRPC : PAYLOAD_NOT_GRPC;
}

// skip_header increments skb_info->data_off so that it skips the remainder of
// the current header (of which we already parsed the index value).
static __always_inline bool skip_literal_header(const struct __sk_buff *skb, skb_info_t *skb_info, __u64 index) {
    __u64 str_len = 0;
    bool is_huffman_encoded = false;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        // At this point the huffman code is not interesting due to the fact that we already read the string length,
        // We are reading the current size in order to skip it.
        if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
            return false;
        }
    }

    skb_info->data_off += str_len;
    return true;
}

// Scan headers goes through the headers in a frame, and tries to find a
// content-type header or a method header.
static __always_inline grpc_status_t scan_headers(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_length, __u8 *content_type_buf) {
    __u32 frame_end = skb_info->data_off + frame_length;
    // Check that frame_end does not go beyond the skb
    frame_end = frame_end < skb->len + 1 ? frame_end : skb->len + 1;
    __u8 current_ch;
    bool is_indexed = false;
    bool is_literal = false;
    __u64 max_bits = 0;
    __u64 index = 0;
    bool found_header = false;

    skip_dynamic_table_update(skb, skb_info, frame_end);

#pragma unroll(GRPC_MAX_HEADERS_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_HEADERS_TO_PROCESS; ++i) {
        if (skb_info->data_off >= frame_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off += sizeof(current_ch);

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

        index = 0;
        if (!read_hpack_int_with_given_current_char(skb, skb_info, current_ch, max_bits, &index)) {
            break;
        }

        if (is_indexed) {
            continue;
        }
        if (index == HTTP2_CONTENT_TYPE_IDX) {
            found_header = true;
            break;
        }

        skip_literal_header(skb, skb_info, index);
    }

    if (found_header) {
        return is_content_type_grpc(skb, skb_info, frame_end, content_type_buf);
    }

    return PAYLOAD_UNDETERMINED;
}

// is_grpc tries to determine if the packet in `skb` holds GRPC traffic. To do
// that, it goes through the HTTP2 frames looking for headers frames, then goes
// through the headers of those frames looking for:
// - a "Content-type" header. If so, try to see if it begins with "application/grpc"
//.- a GET method. GRPC only uses POST methods, the presence of any other methods
//   means this is not GRPC.
static __always_inline grpc_status_t is_grpc(const struct __sk_buff *skb, const skb_info_t *skb_info) {
    grpc_status_t status = PAYLOAD_UNDETERMINED;
    http2_frame_t current_frame = { 0 };

    frame_info_t frames[GRPC_MAX_FRAMES_TO_PROCESS];
    u32 frames_count = 0;

    // Make a mutable copy of skb_info
    skb_info_t info = *skb_info;

    // Check if the skb starts with the HTTP2 magic, advance the info->data_off
    // to the first byte after it if the magic is present.
    skip_preface(skb, &info);

    // Loop through the HTTP2 frames in the packet
#pragma unroll(GRPC_MAX_FRAMES_TO_FILTER)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_FILTER && frames_count < GRPC_MAX_FRAMES_TO_PROCESS; ++i) {
        if (info.data_off + HTTP2_FRAME_HEADER_SIZE > skb->len) {
            break;
        }

        bpf_skb_load_bytes(skb, info.data_off, &current_frame, HTTP2_FRAME_HEADER_SIZE);
        info.data_off += HTTP2_FRAME_HEADER_SIZE;
        if (!format_http2_frame_header(&current_frame)) {
            break;
        }

        if (current_frame.type == kHeadersFrame) {
            frames[frames_count++] = (frame_info_t){ .offset = info.data_off, .length = current_frame.length };
        }

        info.data_off += current_frame.length;
    }

    char content_type_buf[GRPC_CONTENT_TYPE_LEN];

#pragma unroll(GRPC_MAX_FRAMES_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_PROCESS; ++i) {
        if (i >= frames_count) {
            break;
        }

        info.data_off = frames[i].offset;

        status = scan_headers(skb, &info, frames[i].length, content_type_buf);
        if (status != PAYLOAD_UNDETERMINED) {
            break;
        }
    }

    return status;
}

#endif
