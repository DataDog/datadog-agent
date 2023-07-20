#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/grpc/defs.h"
#include "protocols/grpc/maps_defs.h"
#include "protocols/http2/helpers.h"

#define GRPC_MAX_FRAMES_TO_PROCESS 5
#define GRPC_MAX_HEADERS_TO_PROCESS 5
#define GRPC_CONTENT_TYPE_LEN 11
#define GRPC_ENCODED_CONTENT_TYPE "\x1d\x75\xd0\x62\x0d\x26\x3d\x4c\x4d\x65\x64"

static __always_inline bool is_encoded_grpc_content_type(const char *content_type_buf) {
    return !bpf_memcmp(content_type_buf, GRPC_ENCODED_CONTENT_TYPE, sizeof(GRPC_ENCODED_CONTENT_TYPE) - 1);

    //return (buf[0] == 0x1d
         //&& buf[1] == 0x75
         //&& buf[2] == 0xd0
         //&& buf[3] == 0x62
         //&& buf[4] == 0x0d
         //&& buf[5] == 0x26
         //&& buf[6] == 0x3d
         //&& buf[7] == 0x4c
         //&& buf[8] == 0x65
         //&& buf[9] == 0x64
         //&& buf[10] == 0x75
}

static __always_inline grpc_status_t check_literal(struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end, __u8 idx) {
    // Indexed name
    if (idx) {
        log_debug("[grpc] indexed name: %d\n", idx); // 31 == Content-type

        struct hpack_length len;
        if (skb_info->data_off + sizeof(len) >= frame_end) {
            return GRPC_STATUS_UNKNOWN;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &len, sizeof(len));
        skb_info->data_off += sizeof(len);
        log_debug("[grpc] value length = %lu\n", len.length);

        if (len.length < GRPC_CONTENT_TYPE_LEN) {
            log_debug("[grpc] value length too small\n");
            return GRPC_STATUS_UNKNOWN;
        }

        //if (skb_info->data_off + GRPC_CONTENT_TYPE_LEN >= frame_end) {
            //log_debug("[grpc] content type length too big for load bytes\n");
            //return GRPC_STATUS_UNKNOWN;
        //}

        char content_type_buf[GRPC_CONTENT_TYPE_LEN];
        bpf_skb_load_bytes(skb, skb_info->data_off, content_type_buf, GRPC_CONTENT_TYPE_LEN);
        skb_info->data_off += len.length;

        bool is_grpc = is_encoded_grpc_content_type(content_type_buf);

        if (is_grpc) {
            log_debug("[grpc] found grpc content-type!\n");
            return GRPC_STATUS_GRPC;
        } else {
            log_debug("[grpc] not found grpc content-type :(\n");
        }
    }

    return GRPC_STATUS_UNKNOWN;
}

static __always_inline grpc_status_t process_headers(struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_length) {
    union field_index idx;
    grpc_status_t status;

    const __u32 frame_end = skb_info->data_off + frame_length;
    const __u32 end = frame_end < skb->len + 1 ? frame_end : skb->len + 1;

#pragma unroll (GRPC_MAX_HEADERS_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_HEADERS_TO_PROCESS; ++i) {
        if (skb_info->data_off >= end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &idx.raw, sizeof(idx.raw));
        skb_info->data_off++;

         if (is_indexed(idx.raw)) {
            log_debug("[grpc] found indexed header; idx=%lu\n", idx.indexed.index);
            // TODO: Check if POST or GET
            // Size: 1; no change to buf and size here
            continue;
        } else if (is_literal(idx.raw)) {
            log_debug("[grpc] found literal header; idx=%lu\n", idx.literal.index);
            status = check_literal(skb, skb_info, frame_end, idx.literal.index);
            if (status) {
              return status;
            }
        }
    }

   return GRPC_STATUS_UNKNOWN;
}

static __always_inline grpc_status_t is_grpc(struct __sk_buff *skb, skb_info_t *skb_info) {
    grpc_status_t status;
    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    struct http2_frame current_frame;

    log_debug("[grpc] ENTRY: skb len = %lu; skb data off = %lu\n", skb->len, skb_info->data_off);

#pragma unroll(GRPC_MAX_FRAMES_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_PROCESS; ++i) {
        if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb->len) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, frame_buf, HTTP2_FRAME_HEADER_SIZE);
        skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;

        if (!read_http2_frame_header(frame_buf, HTTP2_FRAME_HEADER_SIZE, &current_frame)) {
            log_debug("[grpc] unable to read http2 frame header\n");
            break;
        }

        if (current_frame.type != kHeadersFrame) {
            log_debug("[grpc] not a headers frame; frame length = %lu\n", current_frame.length);
            skb_info->data_off += current_frame.length;
            continue;
        }

        log_debug("[grpc] headers frame; data_off = %lu, frame length = %lu\n", skb_info->data_off, current_frame.length);
        status = process_headers(skb, skb_info, current_frame.length);
        if (status != GRPC_STATUS_UNKNOWN) {
            return status;
        }
    }

    return GRPC_STATUS_UNKNOWN;
}

#endif
