#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

#include "protocols/http2/helpers.h"

#define GRPC_MAX_FRAMES_TO_PROCESS 5

static __always_inline bool read_frame_header(const char *buf, __u32 size, __u32 offset, struct http2_frame *out) {
    if (!buf || offset < 0) {
        return false;
    }

    if (offset + HTTP2_FRAME_HEADER_SIZE < 0 || offset + HTTP2_FRAME_HEADER_SIZE > size) {
        return false;
    }

    if (is_empty_frame_header(buf + offset)) {
        return false;
    }

    *out = *((struct http2_frame *)(buf + offset)); // This causes a verifier issue
    out->length = bpf_ntohl(out->length << 8);
    out->stream_id = bpf_ntohl(out->stream_id << 1);

    return out->type <= kContinuationFrame;
}

static __always_inline bool is_grpc(const char *buf, __u32 size) {
    struct http2_frame current_frame;
    int offset = 0;

#pragma unroll(GRPC_MAX_FRAMES_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_PROCESS; ++i) {
        if (!read_frame_header(buf, size, offset, &current_frame)) {
            log_debug("[grpc] unable to read_http2_frame_header\n");
            return false;
        }
        offset += HTTP2_FRAME_HEADER_SIZE; // The issue is also gone by removing this
    }

    return false;
}

#endif
