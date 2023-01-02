#ifndef __HTTP2_H
#define __HTTP2_H

#include "bpf_endian.h"

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Format" section
#define HTTP2_FRAME_HEADER_SIZE 9
#define HTTP2_SETTINGS_SIZE 6

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
    uint32_t length : 24;
    frame_type_t type;
    uint8_t flags;
    uint8_t reserved : 1;
    uint32_t stream_id : 31;
} __attribute__ ((packed));

// This function checks if the http2 frame header is empty.
static __always_inline bool is_empty_frame_header(const char *frame) {
#define EMPTY_FRAME_HEADER "\0\0\0\0\0\0\0\0\0"

    return !bpf_memcmp(frame, EMPTY_FRAME_HEADER, sizeof(EMPTY_FRAME_HEADER) - 1);
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
    *out = *((struct http2_frame*)buf);
    out->length = bpf_ntohl(out->length << 8);
    out->stream_id = bpf_ntohl(out->stream_id << 1);

    return true;
}

#endif
