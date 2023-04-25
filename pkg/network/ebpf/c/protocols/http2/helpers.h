#ifndef __HTTP2_HELPERS_H
#define __HTTP2_HELPERS_H

#include "bpf_endian.h"

#include "protocols/http2/defs.h"
#include "protocols/http2/usm-events.h"

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

    return out->type <= kContinuationFrame;
}

// The method checks if the given buffer starts with the HTTP2 marker as defined in https://datatracker.ietf.org/doc/html/rfc7540.
// We check that the given buffer is not empty and its size is at least 24 bytes.
static __always_inline bool is_http2_preface(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_MARKER_SIZE);

#define HTTP2_PREFACE "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

    bool match = !bpf_memcmp(buf, HTTP2_PREFACE, sizeof(HTTP2_PREFACE)-1);

    return match;
}

// According to the https://www.rfc-editor.org/rfc/rfc7540#section-3.5
// an HTTP2 server must reply with a settings frame to the preface of HTTP2.
// The settings frame must not be related to the connection (stream_id == 0) and the length should be a multiplication
// of 6 bytes.
static __always_inline bool is_http2_server_settings(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_FRAME_HEADER_SIZE);

    struct http2_frame frame_header;
    if (!read_http2_frame_header(buf, buf_size, &frame_header)) {
        return false;
    }

    return frame_header.type == kSettingsFrame && frame_header.stream_id == 0 && frame_header.length % HTTP2_SETTINGS_SIZE == 0;
}

// The method checks if the given buffer starts with the HTTP2 marker as defined in https://datatracker.ietf.org/doc/html/rfc7540.
// We check that the given buffer is not empty and its size is at least 24 bytes.
static __always_inline bool is_http2(const char* buf, __u32 buf_size) {
    return is_http2_preface(buf, buf_size) || is_http2_server_settings(buf, buf_size);
}

#endif
