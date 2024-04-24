#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/http2/decoding-defs.h"
#include "protocols/http2/defs.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/skb-common.h"
#include "protocols/grpc/defs.h"

// Number of frames to filter in a single packet, while looking for the first headers frame.
#define GRPC_MAX_FRAMES_TO_FILTER 90
// Number of headers to process in a headers frame, while looking for the content-type header.
#define GRPC_MAX_HEADERS_TO_PROCESS 10

// The HPACK specification defines the specific Huffman encoding used for string
// literals in HPACK. This allows us to precomputed the encoded string for
// "application/grpc". Even though it is huffman encoded, this particular string
// is byte-aligned and can be compared without any masking on the final byte.
#define GRPC_ENCODED_CONTENT_TYPE "\x1d\x75\xd0\x62\x0d\x26\x3d\x4c\x4d\x65\x64"
#define GRPC_CONTENT_TYPE_LEN (sizeof(GRPC_ENCODED_CONTENT_TYPE) - 1)

static __always_inline grpc_status_t is_content_type_grpc(struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end, __u8 idx) {
    // We only care about indexed names
    if (idx != HTTP2_CONTENT_TYPE_IDX) {
        return PAYLOAD_UNDETERMINED;
    }

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

    char content_type_buf[GRPC_CONTENT_TYPE_LEN];
    bpf_skb_load_bytes(skb, skb_info->data_off, content_type_buf, GRPC_CONTENT_TYPE_LEN);
    skb_info->data_off += len.length;

    return bpf_memcmp(content_type_buf, GRPC_ENCODED_CONTENT_TYPE, GRPC_CONTENT_TYPE_LEN) == 0? PAYLOAD_GRPC : PAYLOAD_NOT_GRPC;
}

// Scan headers goes through the headers in a frame, and tries to find a
// content-type header or a method header.
static __always_inline grpc_status_t scan_headers(struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_length) {
    __u8 current_ch;
    grpc_status_t status = PAYLOAD_UNDETERMINED;

    __u64 index = 0;
    __u64 max_bits = 0;
    __u32 frame_end = skb_info->data_off + frame_length;
    // Check that frame_end does not go beyond the skb
    frame_end = frame_end < skb_info->data_end + 1 ? frame_end : skb_info->data_end + 1;

    handle_dynamic_table_update(skb, skb_info);

#pragma unroll(GRPC_MAX_HEADERS_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_HEADERS_TO_PROCESS; ++i) {
        if (skb_info->data_off >= frame_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off++;

        if ((current_ch & 128) != 0) {
            // indexed character, so we can skip it.
            continue;
        }

        // We either have literal header with indexing, literal header without indexing or literal header never indexed.
        // if it is literal header with indexing, the max bits are 6, for the other two, the max bits are 4.
        max_bits = (current_ch & 192) == 64 ? MAX_6_BITS : MAX_4_BITS;
        index = 0;
        if (!read_hpack_int_with_given_current_char(skb, skb_info, current_ch, max_bits, &index)) {
            break;
        }

        status = is_content_type_grpc(skb, skb_info, frame_end, index);
        if (status != PAYLOAD_UNDETERMINED) {
            break;
        }

        if (!process_and_skip_literal_headers(skb, skb_info, index)){
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
static __always_inline grpc_status_t is_grpc(struct __sk_buff *skb, const skb_info_t *skb_info) {
    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    http2_frame_t current_frame;

    bool found_headers = false;

    // Make a mutable copy of skb_info
    skb_info_t info = *skb_info;

    // Check if the skb starts with the HTTP2 magic, advance the info->data_off
    // to the first byte after it if the magic is present.
    skip_preface(skb, &info);

    // Loop through the HTTP2 frames in the packet
#pragma unroll(GRPC_MAX_FRAMES_TO_FILTER)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_FILTER; ++i) {
        if (info.data_off + HTTP2_FRAME_HEADER_SIZE > skb_info->data_end) {
            break;
        }

        bpf_skb_load_bytes(skb, info.data_off, frame_buf, HTTP2_FRAME_HEADER_SIZE);
        info.data_off += HTTP2_FRAME_HEADER_SIZE;

        if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &current_frame)) {
            break;
        }

        if (current_frame.type == kHeadersFrame) {
            found_headers = true;
            break;
        }

        info.data_off += current_frame.length;
    }

    if (!found_headers) {
        return PAYLOAD_UNDETERMINED;
    }
    return scan_headers(skb, &info, current_frame.length);
}

#endif
