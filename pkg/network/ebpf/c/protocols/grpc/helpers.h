#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/defs.h"
#include "protocols/http2/helpers.h"
#include "protocols/grpc/defs.h"

#define GRPC_MAX_FRAMES_TO_FILTER 10
// We only try to process one frame at the moment. Trying to process more yields
// a verifier issue due to the way clang manages a pointer to the stack.
#define GRPC_MAX_FRAMES_TO_PROCESS 1
#define GRPC_MAX_HEADERS_TO_PROCESS 10

// The HPACK specification defines the specific Huffman encoding used for string
// literals in HPACK. This allows us to precomputed the encoded string for
// "application/grpc". Even though it is huffman encoded, this particular string
// is byte-aligned and can be compared without any masking on the final byte.
#define GRPC_ENCODED_CONTENT_TYPE "\x1d\x75\xd0\x62\x0d\x26\x3d\x4c\x4d\x65\x64"
#define GRPC_CONTENT_TYPE_LEN (sizeof(GRPC_ENCODED_CONTENT_TYPE) - 1)

static __always_inline void check_and_skip_magic(const struct __sk_buff *skb, skb_info_t *info) {
    if (info->data_off + HTTP2_MARKER_SIZE >= skb->len) {
        return;
    }

    char buf[HTTP2_MARKER_SIZE];
    bpf_skb_load_bytes(skb, info->data_off, buf, sizeof(buf));
    if (is_http2_preface(buf, sizeof(buf))) {
        info->data_off += HTTP2_MARKER_SIZE;
    }
}

static __always_inline bool is_encoded_grpc_content_type(const char *content_type_buf) {
    return !bpf_memcmp(content_type_buf, GRPC_ENCODED_CONTENT_TYPE, GRPC_CONTENT_TYPE_LEN);
}

static __always_inline grpc_status_t is_content_type_grpc(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end, __u8 idx) {
    // We only care about indexed names
    if (idx != HTTP2_CONTENT_TYPE_IDX) {
        return PAYLOAD_UNDETERMINED;
    }

    string_literal_header len;
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

    char content_type_buf[GRPC_CONTENT_TYPE_LEN];
    bpf_skb_load_bytes(skb, skb_info->data_off, content_type_buf, GRPC_CONTENT_TYPE_LEN);
    skb_info->data_off += len.length;

    return is_encoded_grpc_content_type(content_type_buf) ? PAYLOAD_GRPC : PAYLOAD_NOT_GRPC;
}

// skip_header increments skb_info->data_off so that it skips the remainder of
// the current header (of which we already parsed the index value).
static __always_inline void skip_literal_header(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end, __u8 idx) {
    string_literal_header len;
    if (skb_info->data_off + sizeof(len) > frame_end) {
        return;
    }

    bpf_skb_load_bytes(skb, skb_info->data_off, &len, sizeof(len));
    skb_info->data_off += sizeof(len) + len.length;

    // If the index is zero, that means the header name is not indexed, so we
    // have to skip both the name and the index.
    if (!idx && skb_info->data_off + sizeof(len) <= frame_end) {
        bpf_skb_load_bytes(skb, skb_info->data_off, &len, sizeof(len));
        skb_info->data_off += sizeof(len) + len.length;
    }

    return;
}

// Scan headers goes through the headers in a frame, and tries to find a
// content-type header or a method header.
static __always_inline grpc_status_t scan_headers(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_length) {
    field_index idx;
    grpc_status_t status = PAYLOAD_UNDETERMINED;

    __u32 frame_end = skb_info->data_off + frame_length;
    // Check that frame_end does not go beyond the skb
    frame_end = frame_end < skb->len + 1 ? frame_end : skb->len + 1;

#pragma unroll(GRPC_MAX_HEADERS_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_HEADERS_TO_PROCESS; ++i) {
        if (skb_info->data_off >= frame_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &idx.raw, sizeof(idx.raw));
        skb_info->data_off += sizeof(idx.raw);

        if (is_literal(idx.raw)) {
            // Having a literal, with an index pointing to a ":method" key means a
            // request method that is not POST or GET. gRPC only uses POST, so
            // finding a :method here is an indicator of non-GRPC content.
            if (idx.literal.index == kGET || idx.literal.index == kPOST) {
                status = PAYLOAD_NOT_GRPC;
                break;
            }

            status = is_content_type_grpc(skb, skb_info, frame_end, idx.literal.index);
            if (status != PAYLOAD_UNDETERMINED) {
                break;
            }

            skip_literal_header(skb, skb_info, frame_end, idx.literal.index);

            continue;
        }

        // The header is fully indexed, check if it is a :method GET header, in
        // which case we can tell that this is not gRPC, as it uses only POST
        // requests.
        if (is_indexed(idx.raw) && idx.indexed.index == kGET) {
            status = PAYLOAD_NOT_GRPC;
            break;
        }
    }

    return status;
}

// is_grpc tries to determine if the packet in `skb` holds GRPC traffic. To do
// that, it goes through the HTTP2 frames looking for headers frames, then goes
// through the headers of those frames looking for:
// - a "Content-type" header. If so, try to see if it begins with "application/grpc"
//.- a GET method. GRPC only uses POST methods, the presence of any other methods
//   means this is not GRPC.
static __always_inline grpc_status_t is_grpc(const struct __sk_buff *skb, const skb_info_t *skb_info) {
    grpc_status_t status = PAYLOAD_UNDETERMINED;
    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    struct http2_frame current_frame;

    frame_info_t frames[GRPC_MAX_FRAMES_TO_PROCESS];
    u32 frames_count = 0;

    // Make a mutable copy of skb_info
    skb_info_t info = *skb_info;

    // Check if the skb starts with the HTTP2 magic, advance the info->data_off
    // to the first byte after it if the magic is present.
    check_and_skip_magic(skb, &info);

    // Loop through the HTTP2 frames in the packet
#pragma unroll(GRPC_MAX_FRAMES_TO_FILTER)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_FILTER && frames_count < GRPC_MAX_FRAMES_TO_PROCESS; ++i) {
        if (info.data_off + HTTP2_FRAME_HEADER_SIZE > skb->len) {
            break;
        }

        bpf_skb_load_bytes(skb, info.data_off, frame_buf, HTTP2_FRAME_HEADER_SIZE);
        info.data_off += HTTP2_FRAME_HEADER_SIZE;

        if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &current_frame)) {
            break;
        }

        if (current_frame.type == kHeadersFrame) {
            frames[frames_count++] = (frame_info_t){ .offset = info.data_off, .length = current_frame.length };
        }

        info.data_off += current_frame.length;
    }

#pragma unroll(GRPC_MAX_FRAMES_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_PROCESS && status == PAYLOAD_UNDETERMINED; ++i) {
        if (i >= frames_count) {
            break;
        }

        info.data_off = frames[i].offset;

        status = scan_headers(skb, &info, frames[i].length);
    }

    return status;
}

#endif
