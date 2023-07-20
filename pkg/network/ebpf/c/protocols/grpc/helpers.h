#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/grpc/defs.h"
#include "protocols/http2/helpers.h"

#define GRPC_MAX_FRAMES_TO_PROCESS 5

static __always_inline grpc_status_t is_grpc(struct __sk_buff *skb, skb_info_t *skb_info) {
    char frame_buf[HTTP2_FRAME_HEADER_SIZE];
    struct http2_frame current_frame;

    log_debug("[grpc] skb len = %lu; skb data off = %lu\n", skb->len, skb_info->data_off);

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
            log_debug("[grpc] not a headers frame\n" );
            skb_info->data_off += current_frame.length;
            continue;
        }

        log_debug("[grpc] headers frame");
    }

    return GRPC_STATUS_UNKNOWN;
}

#endif
