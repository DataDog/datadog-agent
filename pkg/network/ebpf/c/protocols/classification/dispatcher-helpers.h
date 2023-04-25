#ifndef __PROTOCOL_DISPATCHER_HELPERS_H
#define __PROTOCOL_DISPATCHER_HELPERS_H

#include "ktypes.h"

#include "ip.h"

#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/structs.h"
#include "protocols/classification/dispatcher-maps.h"
#include "protocols/http/classification-helpers.h"
#include "protocols/http2/helpers.h"
#include "protocols/kafka/kafka-classification.h"

static __always_inline bool is_http_monitoring_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("http_monitoring_enabled", val);
    return val > 0;
}

static __always_inline bool is_http2_monitoring_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("http2_monitoring_enabled", val);
    return val > 0;
}

static __always_inline bool is_kafka_monitoring_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("kafka_monitoring_enabled", val);
    return val > 0;
}

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

// Determines the protocols of the given buffer. If we already classified the payload (a.k.a protocol out param
// has a known protocol), then we do nothing.
static __always_inline void classify_protocol_for_dispatcher(protocol_t *protocol, conn_tuple_t *tup, const char *buf, __u32 size) {
    if (protocol == NULL || *protocol != PROTOCOL_UNKNOWN) {
        return;
    }

    if (is_http_monitoring_enabled() && is_http(buf, size)) {
        *protocol = PROTOCOL_HTTP;
    } else if (is_http2_monitoring_enabled() && is_http2(buf, size)) {
        *protocol = PROTOCOL_HTTP2;
    } else {
        *protocol = PROTOCOL_UNKNOWN;
    }

    log_debug("[protocol_dispatcher_classifier]: Classified protocol as %d %d; %s\n", *protocol, size, buf);
}

static __always_inline void update_dispatcher_connection_protocol(conn_tuple_t* skb_tup, protocol_t cur_fragment_protocol) {
    bpf_map_update_with_telemetry(dispatcher_connection_protocol, skb_tup, &cur_fragment_protocol, BPF_NOEXIST);
    conn_tuple_t inverse_skb_conn_tup = *skb_tup;
    flip_tuple(&inverse_skb_conn_tup);
    bpf_map_update_with_telemetry(dispatcher_connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_NOEXIST);
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
        read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);
        const size_t payload_length = skb->len - skb_info.data_off;
        const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
        classify_protocol_for_dispatcher(&cur_fragment_protocol, &skb_tup, request_fragment, final_fragment_size);
        if (is_kafka_monitoring_enabled() && cur_fragment_protocol == PROTOCOL_UNKNOWN) {
            bpf_tail_call_compat(skb, &dispatcher_classification_progs, DISPATCHER_KAFKA_PROG);
        }
        log_debug("[protocol_dispatcher_entrypoint]: %p Classifying protocol as: %d\n", skb, cur_fragment_protocol);
        // If there has been a change in the classification, save the new protocol.
        if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
            update_dispatcher_connection_protocol(&skb_tup, cur_fragment_protocol);
        }
    } else {
        cur_fragment_protocol = *cur_fragment_protocol_ptr;
    }

    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        // dispatch if possible
        const u32 zero = 0;
        dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
        if (args == NULL) {
            log_debug("dispatcher failed to save arguments for tail call\n");
            return;
        }
        bpf_memset(args, 0, sizeof(dispatcher_arguments_t));
        bpf_memcpy(&args->tup, &skb_tup, sizeof(conn_tuple_t));
        bpf_memcpy(&args->skb_info, &skb_info, sizeof(skb_info_t));

        log_debug("dispatching to protocol number: %d\n", cur_fragment_protocol);
        bpf_tail_call_compat(skb, &protocols_progs, cur_fragment_protocol);
    }
}

static __always_inline void dispatch_kafka(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};
    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    char request_fragment[CLASSIFICATION_MAX_BUFFER];
    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);
    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
    protocol_t cur_fragment_protocol = PROTOCOL_UNKNOWN;
    if (is_kafka(skb, &skb_info, request_fragment, final_fragment_size)) {
        cur_fragment_protocol = PROTOCOL_KAFKA;
    }
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        update_dispatcher_connection_protocol(&skb_tup, cur_fragment_protocol);
    }
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        // dispatch if possible
        const u32 zero = 0;
        dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
        if (args == NULL) {
            log_debug("dispatcher failed to save arguments for tail call\n");
            return;
        }
        bpf_memset(args, 0, sizeof(dispatcher_arguments_t));
        bpf_memcpy(&args->tup, &skb_tup, sizeof(conn_tuple_t));
        bpf_memcpy(&args->skb_info, &skb_info, sizeof(skb_info_t));

        // dispatch if possible
        log_debug("dispatching to protocol number: %d\n", cur_fragment_protocol);
        bpf_tail_call_compat(skb, &protocols_progs, cur_fragment_protocol);
    }
    return;
}

static __always_inline bool fetch_dispatching_arguments(conn_tuple_t *tup, skb_info_t *skb_info) {
    const __u32 zero = 0;
    dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    bpf_memcpy(tup, &args->tup, sizeof(conn_tuple_t));
    bpf_memcpy(skb_info, &args->skb_info, sizeof(skb_info_t));

    return true;
}

#endif
