#ifndef __PROTOCOL_DISPATCHER_HELPERS_H
#define __PROTOCOL_DISPATCHER_HELPERS_H

#include "ktypes.h"

#include "ip.h"

#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/structs.h"
#include "protocols/classification/dispatcher-maps.h"
#include "protocols/http/classification-helpers.h"
#include "protocols/http/usm-events.h"
#include "protocols/http2/helpers.h"
#include "protocols/http2/usm-events.h"
#include "protocols/kafka/kafka-classification.h"
#include "protocols/kafka/usm-events.h"
#include "protocols/postgres/helpers.h"
#include "protocols/postgres/usm-events.h"
#include "protocols/redis/helpers.h"
#include "protocols/redis/usm-events.h"

__maybe_unused static __always_inline protocol_prog_t protocol_to_program(protocol_t proto) {
    switch(proto) {
    case PROTOCOL_HTTP:
        return PROG_HTTP;
    case PROTOCOL_HTTP2:
        return PROG_HTTP2_HANDLE_FIRST_FRAME;
    case PROTOCOL_KAFKA:
        return PROG_KAFKA;
    case PROTOCOL_POSTGRES:
        return PROG_POSTGRES;
    case PROTOCOL_REDIS:
        return PROG_REDIS;
    default:
        if (proto != PROTOCOL_UNKNOWN) {
            log_debug("protocol doesn't have a matching program: %d", proto);
        }
        return PROG_UNKNOWN;
    }
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
    return tcp_seq != NULL && *tcp_seq == skb_info->tcp_seq;
}

// Saves the current TCP sequence number in the connection states map. This is used to prevent
// dispatching the same packet multiple times. The sequence number is only saved if the packet is not
// a TCP termination packet. This is to avoid saving the sequence number of packets that are not
// part of the connection anymore.
static __always_inline void cache_tcp_seq(conn_tuple_t *tup, skb_info_t *skb_info) {
    if (skb_info && skb_info->tcp_seq && !is_tcp_termination(skb_info)) {
        bpf_map_update_with_telemetry(connection_states, tup, &skb_info->tcp_seq, BPF_ANY);
    }
}

// Checks if the protocol is supported and enabled by the dispatcher. This is used to determine if we should
// dispatch the packet to the protocol dispatcher or not.
static __always_inline bool is_protocol_supported_for_dispatcher(protocol_t proto) {
    switch (proto) {
    case PROTOCOL_HTTP:
        return is_http_monitoring_enabled();
    case PROTOCOL_HTTP2:
        return is_http2_monitoring_enabled();
    case PROTOCOL_POSTGRES:
        return is_postgres_monitoring_enabled();
    case PROTOCOL_REDIS:
        return is_redis_enabled();
    case PROTOCOL_KAFKA:
        return is_kafka_monitoring_enabled();
    default:
        return false;
    }
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
    } else if (is_postgres_monitoring_enabled() && is_postgres(buf, size)) {
        *protocol = PROTOCOL_POSTGRES;
    } else if (is_redis_enabled() && is_redis(buf, size)) {
        *protocol = PROTOCOL_REDIS;
    } else {
        *protocol = PROTOCOL_UNKNOWN;
    }

    log_debug("[protocol_dispatcher_classifier]: Classified protocol as %d %d; %s", *protocol, size, buf);
}

static __always_inline void dispatcher_delete_protocol_stack(conn_tuple_t *tuple, protocol_stack_t *stack) {
    bool flipped = normalize_tuple(tuple);
    delete_protocol_stack(tuple, stack, FLAG_SOCKET_FILTER_DELETION);
    if (flipped) {
        flip_tuple(tuple);
    }
}

// A shared implementation for the runtime & prebuilt socket filter that classifies & dispatches the protocols of the connections.
static __always_inline void protocol_dispatcher_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    if (skb_tup.dport == 9092 || skb_tup.sport == 9092) {
        log_debug("kafka-test | sport %d | dport %d", skb_tup.sport, skb_tup.dport);
    }
#ifndef COMPILE_PREBUILT
    struct task_struct *task = (void *)bpf_get_current_task();
    skb_tup.pid = BPF_CORE_READ(task, tgid);
    log_debug("guy: pid %d;", skb_tup.pid);
#endif // COMPILE_PREBUILT

    bool tcp_termination = is_tcp_termination(&skb_info);
    // We don't process non tcp packets, nor empty tcp packets which are not tcp termination packets.
    if (!is_tcp(&skb_tup) || (is_payload_empty(&skb_info) && !tcp_termination)) {
        return;
    }

    // Making sure we've not processed the same tcp segment, which can happen when a single packet travels different
    // interfaces.
    bool processed_packet = has_sequence_seen_before(&skb_tup, &skb_info);
    protocol_stack_t *stack = get_protocol_stack_if_exists(&skb_tup);

    protocol_t cur_fragment_protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    if (tcp_termination) {
        bpf_map_delete_elem(&connection_states, &skb_tup);
        dispatcher_delete_protocol_stack(&skb_tup, stack);
    } else if (is_protocol_layer_known(stack, LAYER_ENCRYPTION)) {
        // If we have a TLS connection and we're not in the middle of a TCP termination, we can skip the packet.
        return;
    }

    if (processed_packet) {
        return;
    }

    if (cur_fragment_protocol == PROTOCOL_UNKNOWN) {
        log_debug("[protocol_dispatcher_entrypoint]: %p was not classified", skb);
        char request_fragment[CLASSIFICATION_MAX_BUFFER];
        bpf_memset(request_fragment, 0, sizeof(request_fragment));
        read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);
        const size_t payload_length = skb_info.data_end - skb_info.data_off;
        const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
        classify_protocol_for_dispatcher(&cur_fragment_protocol, &skb_tup, request_fragment, final_fragment_size);
        if (is_kafka_monitoring_enabled() && cur_fragment_protocol == PROTOCOL_UNKNOWN) {
            bpf_tail_call_compat(skb, &dispatcher_classification_progs, DISPATCHER_KAFKA_PROG);
        }
        log_debug("[protocol_dispatcher_entrypoint]: %p Classifying protocol as: %d", skb, cur_fragment_protocol);
        // If there has been a change in the classification, save the new protocol.
        if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
            stack = get_or_create_protocol_stack(&skb_tup);
            if (!stack) {
                // should never happen, but it is required by the eBPF verifier
                return;
            }

            // This is used to signal the tracer program that this protocol stack
            // is also shared with our USM program for the purposes of deletion.
            // For more context refer to the comments in `delete_protocol_stack`
            set_protocol_flag(stack, FLAG_USM_ENABLED);
            set_protocol(stack, cur_fragment_protocol);
            log_debug("guy: set protocol %d; for pid %d", cur_fragment_protocol, skb_tup.pid);
        }
    }

    if (is_protocol_supported_for_dispatcher(cur_fragment_protocol)) {
        // We need to make sure we don't dispatch the same packet multiple times.
        cache_tcp_seq(&skb_tup, &skb_info);

        // dispatch if possible
        const u32 zero = 0;
        dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
        if (args == NULL) {
            log_debug("dispatcher failed to save arguments for tail call");
            return;
        }
        bpf_memset(args, 0, sizeof(dispatcher_arguments_t));
        bpf_memcpy(&args->tup, &skb_tup, sizeof(conn_tuple_t));
        bpf_memcpy(&args->skb_info, &skb_info, sizeof(skb_info_t));

        log_debug("dispatching to protocol number: %d", cur_fragment_protocol);
        bpf_tail_call_compat(skb, &protocols_progs, protocol_to_program(cur_fragment_protocol));
    }
}

static __always_inline void dispatch_kafka(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};
    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

#ifndef COMPILE_PREBUILT
    struct task_struct *task = (void *)bpf_get_current_task();
    skb_tup.pid = BPF_CORE_READ(task, tgid);
#endif // COMPILE_PREBUILT

    char request_fragment[CLASSIFICATION_MAX_BUFFER];
    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);
    const size_t payload_length = skb_info.data_end - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
    protocol_t cur_fragment_protocol = PROTOCOL_UNKNOWN;
    if (is_kafka(skb, &skb_info, request_fragment, final_fragment_size)) {
        log_debug("guy | kafka | pid: %d", skb_tup.pid);
        cur_fragment_protocol = PROTOCOL_KAFKA;
        update_protocol_stack(&skb_tup, cur_fragment_protocol);
    }

    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        // We need to make sure we don't dispatch the same packet multiple times.
        cache_tcp_seq(&skb_tup, &skb_info);
        // dispatch if possible
        const u32 zero = 0;
        dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
        if (args == NULL) {
            log_debug("dispatcher failed to save arguments for tail call");
            return;
        }
        bpf_memset(args, 0, sizeof(dispatcher_arguments_t));
        bpf_memcpy(&args->tup, &skb_tup, sizeof(conn_tuple_t));
        bpf_memcpy(&args->skb_info, &skb_info, sizeof(skb_info_t));

        // dispatch if possible
        log_debug("dispatching to protocol number: %d", cur_fragment_protocol);
        bpf_tail_call_compat(skb, &protocols_progs, protocol_to_program(cur_fragment_protocol));
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
