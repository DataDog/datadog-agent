#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/http2/helpers.h"
#include "protocols/grpc/defs.h"
#include "protocols/grpc/maps_defs.h"

#define CONTENT_TYPE_IDX 31
#define GRPC_MAX_FRAMES_TO_PROCESS 10
#define GRPC_MAX_HEADERS_TO_PROCESS 10
#define GRPC_CONTENT_TYPE_LEN 11
#define GRPC_ENCODED_CONTENT_TYPE "\x1d\x75\xd0\x62\x0d\x26\x3d\x4c\x4d\x65\x64"

#define IS_GET(Idx) ((Idx) == 2)

static __always_inline bool is_encoded_grpc_content_type(const char *content_type_buf) {
    return !bpf_memcmp(content_type_buf, GRPC_ENCODED_CONTENT_TYPE, sizeof(GRPC_ENCODED_CONTENT_TYPE) - 1);
}

static __always_inline grpc_status_t is_content_type_grpc(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end, __u8 idx) {
    // We only care about indexed names
    if (!idx || idx != CONTENT_TYPE_IDX) {
        return GRPC_STATUS_UNKNOWN;
    }

    struct hpack_length len;
    if (skb_info->data_off + sizeof(len) >= frame_end) {
        return GRPC_STATUS_UNKNOWN;
    }

    bpf_skb_load_bytes(skb, skb_info->data_off, &len, sizeof(len));
    skb_info->data_off += sizeof(len);
    if (len.length < GRPC_CONTENT_TYPE_LEN) {
        return GRPC_STATUS_NOT_GRPC;
    }

    char content_type_buf[GRPC_CONTENT_TYPE_LEN];
    bpf_skb_load_bytes(skb, skb_info->data_off, content_type_buf, GRPC_CONTENT_TYPE_LEN);
    skb_info->data_off += len.length;

    return is_encoded_grpc_content_type(content_type_buf) ? GRPC_STATUS_GRPC : GRPC_STATUS_NOT_GRPC;
}

// Scan headers goes through the headers in a frame, and tries to find a
// content-type header or a GET method.
static __always_inline grpc_status_t scan_headers(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_length) {
    union field_index idx;
    grpc_status_t status = GRPC_STATUS_UNKNOWN;

    __u32 frame_end = skb_info->data_off + frame_length;
    // Check that frame_end does not go beyond the skb
    frame_end = frame_end < skb->len + 1 ? frame_end : skb->len + 1;

#pragma unroll (GRPC_MAX_HEADERS_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_HEADERS_TO_PROCESS; ++i) {
        if (skb_info->data_off >= frame_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &idx.raw, sizeof(idx.raw));
        skb_info->data_off += sizeof(idx.raw);

        if (is_literal(idx.raw)) {
            status = is_content_type_grpc(skb, skb_info, frame_end, idx.literal.index);
            if (status != GRPC_STATUS_UNKNOWN) {
                break;
            }

            continue;
        }

        // GRPC only uses POST requests
        if (is_indexed(idx.raw) && IS_GET(idx.indexed.index)) {
            status = GRPC_STATUS_NOT_GRPC;
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
    grpc_status_t status = GRPC_STATUS_UNKNOWN;
    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    struct http2_frame current_frame;

    // Make a mutable copy of skb_info
    skb_info_t info = *skb_info;

    // Loop through the HTTP2 frames in the packet
#pragma unroll(GRPC_MAX_FRAMES_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_PROCESS && status == GRPC_STATUS_UNKNOWN; ++i) {
        if (info.data_off + HTTP2_FRAME_HEADER_SIZE > skb->len) {
            break;
        }

        bpf_skb_load_bytes(skb, info.data_off, frame_buf, HTTP2_FRAME_HEADER_SIZE);
        info.data_off += HTTP2_FRAME_HEADER_SIZE;

        if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &current_frame)) {
            break;
        }

        if (current_frame.type != kHeadersFrame) {
            info.data_off += current_frame.length;
            continue;
        }

        status = scan_headers(skb, &info, current_frame.length);
    }

    return status;
}

#endif
