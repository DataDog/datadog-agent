#ifndef __PROTOCOL_DISPATCHER_HELPERS_H
#define __PROTOCOL_DISPATCHER_HELPERS_H

#include <linux/types.h>

#include "protocol-classification-defs.h"
#include "protocol-classification-maps.h"
#include "protocol-classification-helpers.h"
#include "ip.h"

// A shared implementation for the runtime & prebuilt socket filter that classifies & dispatches the protocols of the connections.
static __always_inline void protocol_dispatcher_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // We handle the payload if it is an non empty TCP packet, or termination TCP packet.
    if (!is_tcp(&skb_tup) || (is_payload_empty(skb, &skb_info) && !is_tcp_termination(&skb_info))) {
        return;
    }

    // Making sure we've not processed the same tcp segment, which can happen when a single packet travels different
    // interfaces.
    if (has_sequence_seen_before(&skb_tup, &skb_info)) {
        return;
    }

    // TODO: Share with protocol classification
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
    protocol_t local_protocol = choose_protocol(sock_tup_protocol, inverse_skb_tup_protocol);

    // If we've already identified the protocol of the socket, no need to read the buffer and try to classify it.
    if (local_protocol == PROTOCOL_UNCLASSIFIED || local_protocol == PROTOCOL_UNKNOWN) {
        char request_fragment[CLASSIFICATION_MAX_BUFFER];
        bpf_memset(request_fragment, 0, sizeof(request_fragment));
        read_into_buffer_for_classification((char *)request_fragment, skb, &skb_info);
        classify_protocol(&local_protocol, request_fragment, sizeof(request_fragment));
    }

    log_debug("[protocol_classifier_entrypoint]: Classifying protocol as: %d\n", local_protocol);
    // If there has been a change in the classification, save the new protocol.
    if (sock_tup_protocol != local_protocol) {
        bpf_map_update_with_telemetry(connection_protocol, &cached_sock_conn_tup, &local_protocol, BPF_ANY);
    }
    if (inverse_skb_tup_protocol != local_protocol) {
        bpf_map_update_with_telemetry(connection_protocol, &inverse_skb_conn_tup, &local_protocol, BPF_ANY);
    }

    // dispatch if possible
    bpf_tail_call_compat(skb, &protocols_progs, local_protocol);
}

#endif
