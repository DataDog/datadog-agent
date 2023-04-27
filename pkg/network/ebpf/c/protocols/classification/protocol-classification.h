#ifndef __PROTOCOL_CLASSIFICATION_H
#define __PROTOCOL_CLASSIFICATION_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "tracer-stats.h"

#include "protocols/amqp/helpers.h"
#include "protocols/classification/common.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/structs.h"
#include "protocols/http/classification-helpers.h"
#include "protocols/http2/helpers.h"
#include "protocols/kafka/kafka-classification.h"
#include "protocols/mongo/helpers.h"
#include "protocols/mysql/helpers.h"
#include "protocols/redis/helpers.h"
#include "protocols/postgres/helpers.h"
#include "protocols/tls/tls.h"

// Checks if a given buffer is http, http2, gRPC.
static __always_inline protocol_t classify_applayer_protocols(const char *buf, __u32 size) {
    if (is_http(buf, size)) {
        return PROTOCOL_HTTP;
    }
    if (is_http2(buf, size)) {
        return PROTOCOL_HTTP2;
    }

    return PROTOCOL_UNKNOWN;
}

// Checks if a given buffer is redis, mongo, postgres, or mysql.
static __always_inline protocol_t classify_db_protocols(conn_tuple_t *tup, const char *buf, __u32 size) {
    if (is_redis(buf, size)) {
        return PROTOCOL_REDIS;
    }

    if (is_mongo(tup, buf, size)) {
        return PROTOCOL_MONGO;
    }

    if (is_postgres(buf, size)) {
        return PROTOCOL_POSTGRES;
    }

    if (is_mysql(tup, buf, size)) {
        return PROTOCOL_MYSQL;
    }

    return PROTOCOL_UNKNOWN;
}

// Checks if a given buffer is amqp, and soon - kafka..
static __always_inline protocol_t classify_queue_protocols(struct __sk_buff *skb, skb_info_t *skb_info, const char *buf, __u32 size) {
    if (is_amqp(buf, size)) {
        return PROTOCOL_AMQP;
    }
    if (is_kafka(skb, skb_info, buf, size)) {
        return PROTOCOL_KAFKA;
    }

    return PROTOCOL_UNKNOWN;
}

__maybe_unused static __always_inline void save_protocol(conn_tuple_t *skb_tup, protocol_t cur_fragment_protocol) {
    bpf_map_update_with_telemetry(connection_protocol, skb_tup, &cur_fragment_protocol, BPF_NOEXIST);
    conn_tuple_t inverse_skb_conn_tup;
    bpf_memcpy(&inverse_skb_conn_tup, skb_tup, sizeof(conn_tuple_t));
    flip_tuple(&inverse_skb_conn_tup);
    bpf_map_update_with_telemetry(connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_NOEXIST);
}

// A shared implementation for the runtime & prebuilt socket filter that classifies the protocols of the connections.
__maybe_unused static __always_inline void protocol_classifier_entrypoint(struct __sk_buff *skb) {
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

    // Currently TLS is marked by a connection tag rather than the protocol stack,
    // but as we add support for multiple protocols in the stack, we should revisit this implementation,
    // and unify it with the following if clause.
    //
    // The connection is TLS encrypted, thus we cannot classify the protocol using socket filter.
    if (is_tls_connection_cached(&skb_tup)) {
        return;
    }

    protocol_t *cur_fragment_protocol_ptr = bpf_map_lookup_elem(&connection_protocol, &skb_tup);
    if (cur_fragment_protocol_ptr) {
        return;
    }

    // Get the buffer the fragment will be read into from a per-cpu array map.
    // This will avoid doing unaligned stack access while parsing the protocols,
    // which is forbidden and will make the verifier fail.
    const u32 key = 0;
    char *request_fragment = bpf_map_lookup_elem(&classification_buf, &key);
    if (request_fragment == NULL) {
        log_debug("could not get classification buffer from map");
        return;
    }

    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);
    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;

    // In the context of socket filter, we can classify the protocol if it is plain text,
    // so if the protocol is encrypted, then we have to rely on our uprobes to classify correctly the protocol.
    if (is_tls(request_fragment, final_fragment_size)) {
        const bool t = true;
        bpf_map_update_with_telemetry(tls_connection, &skb_tup, &t, BPF_ANY);
        flip_tuple(&skb_tup);
        bpf_map_update_with_telemetry(tls_connection, &skb_tup, &t, BPF_ANY);
        return;
    }

    protocol_t cur_fragment_protocol = classify_applayer_protocols(request_fragment, final_fragment_size);
    // If there has been a change in the classification, save the new protocol.
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        save_protocol(&skb_tup, cur_fragment_protocol);
        return;
    }

    bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_QUEUES_PROG);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_queues(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // Get the buffer the fragment will be read into from a per-cpu array map.
    // This will avoid doing unaligned stack access while parsing the protocols,
    // which is forbidden and will make the verifier fail.
    const u32 key = 0;
    char *request_fragment = bpf_map_lookup_elem(&classification_buf, &key);
    if (request_fragment == NULL) {
        log_debug("could not get classification buffer from map");
        return;
    }
    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);

    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;

    protocol_t cur_fragment_protocol = classify_queue_protocols(skb, &skb_info, request_fragment, final_fragment_size);
    // If there has been a change in the classification, save the new protocol.
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        save_protocol(&skb_tup, cur_fragment_protocol);
        return;
    }

    bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_DBS_PROG);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_dbs(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // Get the buffer the fragment will be read into from a per-cpu array map.
    // This will avoid doing unaligned stack access while parsing the protocols,
    // which is forbidden and will make the verifier fail.
    const u32 key = 0;
    char *request_fragment = bpf_map_lookup_elem(&classification_buf, &key);
    if (request_fragment == NULL) {
        log_debug("could not get classification buffer from map");
        return;
    }
    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, skb_info.data_off);

    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;

    protocol_t cur_fragment_protocol = classify_db_protocols(&skb_tup, request_fragment, final_fragment_size);

    // If there has been a change in the classification, save the new protocol.
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        save_protocol(&skb_tup, cur_fragment_protocol);
    }
}

#endif
