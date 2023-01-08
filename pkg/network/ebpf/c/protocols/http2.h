#ifndef __HTTP2_H
#define __HTTP2_H

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Format" section
#define HTTP2_FRAME_HEADER_SIZE 9
#define HTTP2_SETTINGS_SIZE 6

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "http2-defs.h"
#include "http-types.h"
#include "protocol-classification-defs.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "http2-decoding.h"

// All types of http2 frames exist in the protocol.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Type Registry" section.
typedef enum {
    kDataFrame          = 0,
    kHeadersFrame       = 1,
    kPriorityFrame      = 2,
    kRSTStreamFrame     = 3,
    kSettingsFrame      = 4,
    kPushPromiseFrame   = 5,
    kPingFrame          = 6,
    kGoAwayFrame        = 7,
    kWindowUpdateFrame  = 8,
    kContinuationFrame  = 9,
} __attribute__ ((packed)) frame_type_t;

// Struct which represent the http2 frame by its fields.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540#section-4.1 for frame format.
struct http2_frame {
    uint32_t length;
    frame_type_t type;
    uint8_t flags;
    uint32_t stream_id;
};

static __always_inline uint32_t as_uint32_t(unsigned char input) {
    return (uint32_t)input;
}

// This function checks if the http2 frame header is empty.
static __always_inline bool is_empty_frame_header(const char *frame) {
#pragma unroll
    for (uint32_t i = 0; i < HTTP2_FRAME_HEADER_SIZE; i++) {
        if (frame[i] != 0) {
            return false;
        }
    }
    return true;
}


// This function reads the http2 frame header and validate the frame.
static __always_inline bool read_http2_frame_header(const char *buf, size_t buf_size, struct http2_frame *out) {
    if (buf == NULL) {
        return false;
    }

    if (buf_size < HTTP2_FRAME_HEADER_SIZE) {
        return false;
    }

    if (is_empty_frame_header(buf)) {
        return false;
    }

// We extract the frame by its shape to fields.
// See: https://datatracker.ietf.org/doc/html/rfc7540#section-4.1
    out->length = as_uint32_t(buf[0])<<16 | as_uint32_t(buf[1])<<8 | as_uint32_t(buf[2]);
    out->type = (frame_type_t)buf[3];
    out->flags = (uint8_t)buf[4];
    out->stream_id = (as_uint32_t(buf[5]) << 24 |
                      as_uint32_t(buf[6]) << 16 |
                      as_uint32_t(buf[7]) << 8 |
                      as_uint32_t(buf[8])) & 2147483647;

    return true;
}

// This function filters the needed frames from the http2 session.
static __always_inline void process_http2_frames(http2_transaction_t* http2_transaction, struct __sk_buff *skb) {
    log_debug("[http2] http2 process_http2_frames");

    struct http2_frame current_frame = {};

#pragma unroll
    // Iterate till max frames to avoid high connection rate.
    for (uint32_t i = 0; i < HTTP2_MAX_FRAMES; ++i) {
        if (http2_transaction->current_offset_in_request_fragment + HTTP2_FRAME_HEADER_SIZE > skb->len) {
        log_debug("[http2] size is too big!");
          return;
        }

        // Load the current frame into http2_frame strct in order to filter the needed frames.
        if (http2_transaction->current_offset_in_request_fragment > sizeof(http2_transaction->request_fragment)) {
            return;
        }

        if (!read_http2_frame_header(http2_transaction->request_fragment + http2_transaction->current_offset_in_request_fragment, HTTP2_FRAME_HEADER_SIZE, &current_frame)){
            return;
        }

        http2_transaction->current_offset_in_request_fragment += HTTP2_FRAME_HEADER_SIZE;

        // End of stream my apper in the data frame as well as the header frame.
        if (current_frame.type == kDataFrame && current_frame.flags == 1){
           log_debug("[http2] ********* End of stream flag was found!!! *********", current_frame.stream_id);
        }

        if (current_frame.length == 0) {
            continue;
        }

        // Filter all types of frames except header frame.
        if (current_frame.type != kHeadersFrame) {
            http2_transaction->current_offset_in_request_fragment += (__u32)current_frame.length;
            continue;
        }

        // End of stream my apper in the header frame as well.
        if (current_frame.flags == 1){
           log_debug("[http2] ********* End of stream flag was found!!! *********", current_frame.stream_id);
        }

        // Verify size of pos with max of XX not bigger then the packet.
        if (http2_transaction->current_offset_in_request_fragment + (__u32)current_frame.length > skb->len) {
            return;
        }

        // Load the current frame into http2_frame strct in order to filter the needed frames.
        if (!decode_http2_headers_frame(http2_transaction, current_frame.length)){
            log_debug("[http2] unable to read http2 header frame");
            return;
        }

        http2_transaction->current_offset_in_request_fragment += (__u32)current_frame.length;
    }
}

#endif
