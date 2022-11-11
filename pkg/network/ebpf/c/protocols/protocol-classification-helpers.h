#ifndef __PROTOCOL_CLASSIFICATION_HELPERS_H
#define __PROTOCOL_CLASSIFICATION_HELPERS_H

#include <linux/types.h>

#include "protocol-classification-defs.h"
#include "protocol-classification-maps.h"
#include "bpf_builtins.h"
#include "ip.h"

// Patch to support old kernels that don't contain bpf_skb_load_bytes, by adding a dummy implementation to bypass runtime compilation.
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 5, 0)
long bpf_skb_load_bytes(const void *skb, u32 offset, void *to, u32 len) {return 0;}
#endif

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
    if (protocol == NULL || (*protocol != PROTOCOL_UNKNOWN && *protocol != PROTOCOL_UNCLASSIFIED)) {
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

// Decides if the protocol_classifier should process the packet. We process not empty TCP packets.
static __always_inline bool should_process_packet(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup) {
    // we're only interested in TCP traffic
    if (!(tup->metadata & CONN_TYPE_TCP)) {
        return false;
    }

    bool empty_payload = skb_info->data_off == skb->len;
    return !empty_payload;
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

        bpf_skb_load_bytes(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
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
        bpf_skb_load_bytes(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes(skb, offset, buf, 1);
    }
}

// checks if we have seen that tcp packet before. It can happen if a packet travels multiple interfaces or retransmissions.
static __always_inline bool has_sequence_seen_before(conn_tuple_t *tup, skb_info_t *skb_info) {
    if (!skb_info || !skb_info->tcp_seq) {
        return false;
    }

    u32 *tcp_seq = bpf_map_lookup_elem(&connection_states, tup);

    // check if we've seen this TCP segment before. this can happen in the
    // context of localhost traffic where the same TCP segment can be seen
    // multiple times coming in and out from different interfaces
    if (tcp_seq != NULL && *tcp_seq == skb_info->tcp_seq) {
        return true;
    }

    bpf_map_update_elem(&connection_states, tup, &skb_info->tcp_seq, BPF_ANY);
    return false;
}

// Returns the cached protocol for the given connection tuple, or PROTOCOL_UNCLASSIFIED if missing.
static __always_inline protocol_t get_cached_protocol_or_default(conn_tuple_t *tup) {
    protocol_t *cached_protocol_ptr = bpf_map_lookup_elem(&connection_protocol, tup);
    if (cached_protocol_ptr != NULL) {
        return *cached_protocol_ptr;
    }

    return PROTOCOL_UNCLASSIFIED;
}

// Given protocols for the socket connection tuple, and the inverse skb connection tuple, the function returns
// the final protocol among the two.
// If the sock_tup_protocol is unclassified, then it does not matter what's the value of the inverse_skb_tup_protocol,
// we will take it. If the inverse_skb_tup_protocol is unclassified as well, then it does not matter which "unclassified"
// we choose. If it is unknown or classified, then we should choose it.
// If the sock_tup_protocol is unknown, then we take the inverse_skb_tup_protocol if it is classified or unknown.
// If both are unknown, then it does not matter which "unknown" we choose. If the inverse_skb_tup_protocol is classified,
// then for sure we should choose it.
// On any other case take sock_tup_protocol.
static __always_inline protocol_t choose_protocol(protocol_t sock_tup_protocol, protocol_t inverse_skb_tup_protocol) {
    if ((sock_tup_protocol == PROTOCOL_UNCLASSIFIED) ||
        (sock_tup_protocol == PROTOCOL_UNKNOWN && inverse_skb_tup_protocol != PROTOCOL_UNCLASSIFIED)) {
        return inverse_skb_tup_protocol;
    }

    // On any other case, we give the priority to the classified protocol for the socket connection tuple
    return sock_tup_protocol;
}

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

// A shared implementation for the runtime & prebuilt socket filter that classifies the protocols of the connections.
static __always_inline void protocol_classifier_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // We process a non empty TCP packets, rather than that - we skip the packet.
    if (!should_process_packet(skb, &skb_info, &skb_tup)) {
        return;
    }

    // Making sure we've not processed the same tcp segment, which can happen when a single packet travels different
    // interfaces.
    if (has_sequence_seen_before(&skb_tup, &skb_info)) {
        return;
    }

    conn_tuple_t *cached_sock_conn_tup_ptr = bpf_map_lookup_elem(&skb_conn_tuple_to_socket_conn_tuple, &skb_tup);
    if (cached_sock_conn_tup_ptr == NULL) {
        return;
    }

    conn_tuple_t cached_sock_conn_tup = *cached_sock_conn_tup_ptr;
    conn_tuple_t inverse_skb_conn_tup = {0};
    invert_conn_tuple(&skb_tup, &inverse_skb_conn_tup);
    inverse_skb_conn_tup.pid = 0;
    inverse_skb_conn_tup.netns = 0;

    protocol_t sock_tup_protocol = get_cached_protocol_or_default(&cached_sock_conn_tup);
    protocol_t inverse_skb_tup_protocol = get_cached_protocol_or_default(&inverse_skb_conn_tup);
    protocol_t cur_fragment_protocol = choose_protocol(sock_tup_protocol, inverse_skb_tup_protocol);

    // If we've already identified the protocol of the socket, no need to read the buffer and try to classify it.
    if (cur_fragment_protocol == PROTOCOL_UNCLASSIFIED || cur_fragment_protocol == PROTOCOL_UNKNOWN) {
        char request_fragment[CLASSIFICATION_MAX_BUFFER];
        bpf_memset(request_fragment, 0, sizeof(request_fragment));
        read_into_buffer_for_classification((char *)request_fragment, skb, &skb_info);
        classify_protocol(&cur_fragment_protocol, request_fragment, sizeof(request_fragment));
    }

    log_debug("[protocol_classifier_entrypoint]: Classifying protocol as: %d\n", cur_fragment_protocol);
    // If there has been a change in the classification, save the new protocol.
    if (sock_tup_protocol != cur_fragment_protocol) {
        bpf_map_update_with_telemetry(connection_protocol, &cached_sock_conn_tup, &cur_fragment_protocol, BPF_ANY);
    }
    if (inverse_skb_tup_protocol != cur_fragment_protocol) {
        bpf_map_update_with_telemetry(connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_ANY);
    }
}

#endif
