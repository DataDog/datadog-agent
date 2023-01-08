#ifndef __PROTOCOL_CLASSIFICATION_HELPERS_H
#define __PROTOCOL_CLASSIFICATION_HELPERS_H

#include <linux/types.h>

#include "protocol-classification-defs.h"
#include "protocol-classification-maps.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "http2.h"

// Patch to support old kernels that don't contain bpf_skb_load_bytes, by adding a dummy implementation to bypass runtime compilation.
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 5, 0)
long bpf_skb_load_bytes_with_telemetry(const void *skb, u32 offset, void *to, u32 len) {return 0;}
#endif

#define CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, min_buff_size)   \
    if (buf_size < min_buff_size) {                                         \
        return false;                                                       \
    }                                                                       \
                                                                            \
    if (buf == NULL) {                                                      \
        return false;                                                       \
    }                                                                       \

// The method checks if the given buffer starts with the HTTP2 marker as defined in https://datatracker.ietf.org/doc/html/rfc7540.
// We check that the given buffer is not empty and its size is at least 24 bytes.
static __always_inline bool is_http2_preface(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_MARKER_SIZE)

#define HTTP2_PREFACE "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

    bool match = !bpf_memcmp(buf, HTTP2_PREFACE, sizeof(HTTP2_PREFACE)-1);

    return match;
}

// According to the https://www.rfc-editor.org/rfc/rfc7540#section-3.5
// an HTTP2 server must reply with a settings frame to the preface of HTTP2.
// The settings frame must not be related to the connection (stream_id == 0) and the length should be a multiplication
// of 6 bytes.
static __always_inline bool is_http2_server_settings(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_FRAME_HEADER_SIZE)

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

// Checks if the given buffers start with `HTTP` prefix (represents a response) or starts with `<method> /` which represents
// a request, where <method> is one of: GET, POST, PUT, DELETE, HEAD, OPTIONS, or PATCH.
static __always_inline bool is_http(const char *buf, __u32 size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, size, HTTP_MIN_SIZE)

#define HTTP "HTTP/"
#define GET "GET /"
#define POST "POST /"
#define PUT "PUT /"
#define DELETE "DELETE /"
#define HEAD "HEAD /"
#define OPTIONS1 "OPTIONS /"
#define OPTIONS2 "OPTIONS *"
#define PATCH "PATCH /"

    // memcmp returns
    // 0 when s1 == s2,
    // !0 when s1 != s2.
    bool http = !(bpf_memcmp(buf, HTTP, sizeof(HTTP)-1)
        && bpf_memcmp(buf, GET, sizeof(GET)-1)
        && bpf_memcmp(buf, POST, sizeof(POST)-1)
        && bpf_memcmp(buf, PUT, sizeof(PUT)-1)
        && bpf_memcmp(buf, DELETE, sizeof(DELETE)-1)
        && bpf_memcmp(buf, HEAD, sizeof(HEAD)-1)
        && bpf_memcmp(buf, OPTIONS1, sizeof(OPTIONS1)-1)
        && bpf_memcmp(buf, OPTIONS2, sizeof(OPTIONS2)-1)
        && bpf_memcmp(buf, PATCH, sizeof(PATCH)-1));

    return http;
}

// Determines the protocols of the given buffer. If we already classified the payload (a.k.a protocol out param
// has a known protocol), then we do nothing.
static __always_inline void classify_protocol(protocol_t *protocol, const char *buf, __u32 size) {
    if (protocol == NULL || *protocol != PROTOCOL_UNKNOWN) {
        return;
    }

    if (is_http(buf, size)) {
        *protocol = PROTOCOL_HTTP;
    } else if (is_http2(buf, size)) {
        *protocol = PROTOCOL_HTTP2;
    } else {
        *protocol = PROTOCOL_UNKNOWN;
    }

    log_debug("[protocol classification]: Classified protocol as %d %d; %s\n", *protocol, size, buf);
}

// Returns true if the packet is TCP.
static __always_inline bool is_tcp(conn_tuple_t *tup) {
    return tup->metadata & CONN_TYPE_TCP;
}

// Returns true if the payload is empty.
static __always_inline bool is_payload_empty(struct __sk_buff *skb, skb_info_t *skb_info) {
    return skb_info->data_off == skb->len;
}

// The method is used to read the data buffer from the __sk_buf struct. Similar implementation as `read_into_buffer_skb`
// from http parsing, but uses a different constant (CLASSIFICATION_MAX_BUFFER).
static __always_inline void read_into_buffer_for_classification(char *buffer, struct __sk_buff *skb, skb_info_t *info) {
    u64 offset = (u64)info->data_off;

#define BLK_SIZE (16)
    const u32 len = CLASSIFICATION_MAX_BUFFER < (skb->len - (u32)offset) ? (u32)offset + CLASSIFICATION_MAX_BUFFER : skb->len;

    unsigned i = 0;

#pragma unroll(CLASSIFICATION_MAX_BUFFER / BLK_SIZE)
    for (; i < (CLASSIFICATION_MAX_BUFFER / BLK_SIZE); i++) {
        if (offset + BLK_SIZE - 1 >= len) { break; }

        bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
        offset += BLK_SIZE;
    }

    // This part is very hard to write in a loop and unroll it.
    // Indeed, mostly because of older kernel verifiers, we want to make sure the offset into the buffer is not
    // stored on the stack, so that the verifier is able to verify that we're not doing out-of-bound on
    // the stack.
    // Basically, we should get a register from the code block above containing an fp relative address. As
    // we are doing `buffer[0]` here, there is not dynamic computation on that said register after this,
    // and thus the verifier is able to ensure that we are in-bound.
    void *buf = &buffer[i * BLK_SIZE];
    if (offset + 14 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 1);
    }
}

// A shared implementation for the runtime & prebuilt socket filter that classifies the protocols of the connections.
static __always_inline void protocol_classifier_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // We support non empty TCP payloads for classification at the moment.
    if (!is_tcp(&skb_tup) || is_payload_empty(skb, &skb_info)) {
        return;
    }

    protocol_t *cur_fragment_protocol_ptr = bpf_map_lookup_elem(&connection_protocol, &skb_tup);
    if (cur_fragment_protocol_ptr) {
        return;
    }

    protocol_t cur_fragment_protocol = PROTOCOL_UNKNOWN;
    char request_fragment[CLASSIFICATION_MAX_BUFFER];
    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, &skb_info);
    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
    classify_protocol(&cur_fragment_protocol, request_fragment, final_fragment_size);
    // If there has been a change in the classification, save the new protocol.
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        bpf_map_update_with_telemetry(connection_protocol, &skb_tup, &cur_fragment_protocol, BPF_NOEXIST);
        conn_tuple_t inverse_skb_conn_tup = skb_tup;
        flip_tuple(&inverse_skb_conn_tup);
        bpf_map_update_with_telemetry(connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_NOEXIST);
    }
}

#endif
