#ifndef __PROTOCOL_DISPATCHER_HELPERS_H
#define __PROTOCOL_DISPATCHER_HELPERS_H

#include <linux/types.h>

#include "protocol-dispatcher-maps.h"
#include "protocol-classification-helpers.h"
#include "ip.h"

// Returns true if the payload represents a TCP termination by checking if the tcp flags contains TCPHDR_FIN or TCPHDR_RST.
static __always_inline bool is_tcp_termination(skb_info_t *skb_info) {
    return skb_info->tcp_flags & (TCPHDR_FIN | TCPHDR_RST);
}

static __always_inline bool is_tcp_ack(skb_info_t *skb_info) {
    return skb_info->tcp_flags == TCPHDR_ACK;
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

// A shared implementation for the runtime & prebuilt socket filter that classifies & dispatches the protocols of the connections.
static __always_inline void protocol_dispatcher_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // We don't process non tcp packets, nor empty tcp packets which are not tcp termination packets, nor ACK only packets.
    if (!is_tcp(&skb_tup) || is_tcp_ack(&skb_info) || (is_payload_empty(skb, &skb_info) && !is_tcp_termination(&skb_info))) {
        return;
    }

    // Making sure we've not processed the same tcp segment, which can happen when a single packet travels different
    // interfaces.
    if (has_sequence_seen_before(&skb_tup, &skb_info)) {
        return;
    }

    protocol_t cur_fragment_protocol = PROTOCOL_UNKNOWN;
    // TODO: Share with protocol classification
    protocol_t *cur_fragment_protocol_ptr = bpf_map_lookup_elem(&dispatcher_connection_protocol, &skb_tup);
    if (cur_fragment_protocol_ptr == NULL) {
        log_debug("[protocol_dispatcher_entrypoint]: %p was not classified\n", skb);
        char request_fragment[CLASSIFICATION_MAX_BUFFER];
        bpf_memset(request_fragment, 0, sizeof(request_fragment));
        read_into_buffer_for_classification((char *)request_fragment, skb, &skb_info);
        const size_t payload_length = skb->len - skb_info.data_off;
        const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
        classify_protocol(&cur_fragment_protocol, &skb_tup, request_fragment, final_fragment_size);
        log_debug("[protocol_dispatcher_entrypoint]: %p Classifying protocol as: %d\n", skb, cur_fragment_protocol);
        // If there has been a change in the classification, save the new protocol.
        if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
            bpf_map_update_with_telemetry(dispatcher_connection_protocol, &skb_tup, &cur_fragment_protocol, BPF_NOEXIST);
            conn_tuple_t inverse_skb_conn_tup = skb_tup;
            flip_tuple(&inverse_skb_conn_tup);
            bpf_map_update_with_telemetry(dispatcher_connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_NOEXIST);
        }
    } else {
        cur_fragment_protocol = *cur_fragment_protocol_ptr;
    }

    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        // dispatch if possible
        bpf_tail_call_compat(skb, &protocols_progs, cur_fragment_protocol);
    }
}

#endif
