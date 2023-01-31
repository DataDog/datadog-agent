#ifndef __KAFKA_H
#define __KAFKA_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "tracer.h"
#include "kafka-types.h"
#include "kafka-helpers.h"
#include "events.h"

#include <uapi/linux/ptrace.h>

USM_EVENTS_INIT(kafka, kafka_transaction_batch_entry_t, KAFKA_BATCH_SIZE);

static __always_inline bool kafka_seen_before(kafka_transaction_t *kafka, skb_info_t *skb_info) {
    if (!skb_info || !skb_info->tcp_seq) {
        return false;
    }

    // check if we've seen this TCP segment before. this can happen in the
    // context of localhost traffic where the same TCP segment can be seen
    // multiple times coming in and out from different interfaces
    return kafka->base.tcp_seq == skb_info->tcp_seq;
}

static __always_inline void kafka_update_seen_before(kafka_transaction_t *kafka_transaction, skb_info_t *skb_info) {
    if (!skb_info || !skb_info->tcp_seq) {
        return;
    }

    log_debug("kafka: kafka_update_seen_before: ktx=%llx old_seq=%llu seq=%llu\n", kafka_transaction, kafka_transaction->base.tcp_seq, skb_info->tcp_seq);
    kafka_transaction->base.tcp_seq = skb_info->tcp_seq;
}

static __always_inline int kafka_process(kafka_transaction_t *kafka_transaction) {
    if (!try_parse_request_header(kafka_transaction)) {
        return 0;
    }
    if (!try_parse_request(kafka_transaction)) {
        return 0;
    }
    log_debug("kafka: topic name is %s\n", kafka_transaction->base.topic_name);

    kafka_batch_enqueue(&kafka_transaction->base);
    return 0;
}

// this function is called by the socket-filter program to decide whether or not we should inspect
// the contents of a certain packet, in order to avoid the cost of processing packets that are not
// of interest such as empty ACKs, UDP data or encrypted traffic.
static __always_inline bool kafka_allow_packet(kafka_transaction_t *kafka, struct __sk_buff* skb, skb_info_t *skb_info) {
    // we're only interested in TCP traffic
    if (!(kafka->base.tup.metadata&CONN_TYPE_TCP)) {
        return false;
    }

    // if payload data is empty or if this is an encrypted packet, we only
    // process it if the packet represents a TCP termination
    bool empty_payload = skb_info->data_off == skb->len;
    if (empty_payload) {
        return skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST);
    }

    // Check that we didn't see this tcp segment before so we won't process
    // the same traffic twice
    log_debug("kafka: Current tcp sequence: %lu\n", skb_info->tcp_seq);
    __u32 *last_tcp_seq = bpf_map_lookup_elem(&kafka_last_tcp_seq_per_connection, &kafka->base.tup);
    if (last_tcp_seq != NULL && *last_tcp_seq == skb_info->tcp_seq) {
        log_debug("kafka: already seen this tcp sequence: %lu\n", *last_tcp_seq);
        return false;
    }
    bpf_map_update_elem(&kafka_last_tcp_seq_per_connection, &kafka->base.tup, &skb_info->tcp_seq, BPF_ANY);
    return true;
}

#endif
