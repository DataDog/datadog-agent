#ifndef __PROTOCOL_CLASSIFICATION_HELPERS_H
#define __PROTOCOL_CLASSIFICATION_HELPERS_H

#include <linux/types.h>

#include "protocol-classification-defs.h"
#include "protocol-classification-maps.h"
#include "bpf_builtins.h"
#include "ip.h"

// Replaces the source and dest fields (addresses and ports).
static __always_inline void invert_conn_tuple(conn_tuple_t *original_conn, conn_tuple_t *output_conn) {
    output_conn->saddr_h = original_conn->daddr_h;
    output_conn->saddr_l = original_conn->daddr_l;
    output_conn->daddr_h = original_conn->saddr_h;
    output_conn->daddr_l = original_conn->saddr_l;
    output_conn->sport = original_conn->dport;
    output_conn->dport = original_conn->sport;
    output_conn->metadata = original_conn->metadata;
    output_conn->pid = original_conn->pid;
    output_conn->netns = original_conn->netns;
}

#define CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, min_buff_size)   \
        if (buf_size < min_buff_size) {                                     \
            return false;                                                   \
        }                                                                   \
                                                                            \
        if (buf == NULL) {                                                  \
            return false;                                                   \
        }                                                                   \

// The method checks if the given buffer starts with the HTTP2 marker as defined in https://datatracker.ietf.org/doc/html/rfc7540.
// We check that the given buffer is not empty and its size is at least 24 bytes.
static __always_inline bool is_http2(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_MARKER_SIZE)

#define HTTP2_SIGNATURE "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

    bool match = !bpf_memcmp(buf, HTTP2_SIGNATURE, sizeof(HTTP2_SIGNATURE)-1);
    return match;
}

// Checks if the given buffers start with `HTTP` prefix (represents a response) or starts with `<method> /` which represents
// a request, where <method> is one of: GET, POST, PUT, DELETE, HEAD, OPTIONS, or PATCH.
static __always_inline bool is_http(const char *buf, __u32 size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, size, HTTP_MIN_SIZE)

#define HTTP "HTTP"
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

// Returns the cached protocol for the given connection tuple, or PROTOCOL_UNKNOWN if missing.
static __always_inline protocol_t get_cached_protocol_or_default(conn_tuple_t *tup) {
    protocol_t *cached_protocol_ptr = bpf_map_lookup_elem(&connection_protocol, tup);
    if (cached_protocol_ptr == NULL) { // Checking the invert version
        conn_tuple_t conn_tuple_copy = *tup;
        conn_tuple_copy.netns = 0;
        conn_tuple_t *skb_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, tup);
        if (skb_tup_ptr == NULL) {
            return PROTOCOL_UNKNOWN;
        }

        conn_tuple_t inverse_skb_conn_tup = {0};
        invert_conn_tuple(skb_tup_ptr, &inverse_skb_conn_tup);
        inverse_skb_conn_tup.pid = tup->pid;
        inverse_skb_conn_tup.netns = tup->netns;
        inverse_skb_conn_tup.metadata = tup->metadata;

        cached_protocol_ptr = bpf_map_lookup_elem(&connection_protocol, &inverse_skb_conn_tup);
        if (cached_protocol_ptr == NULL) {
            return PROTOCOL_UNKNOWN;
        }
    }
    return *cached_protocol_ptr;
}

#endif
