#ifndef __PROTOCOL_CLASSIFICATION_H
#define __PROTOCOL_CLASSIFICATION_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "port_range.h"

#include "protocols/amqp/helpers.h"
#include "protocols/classification/common.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/structs.h"
#include "protocols/classification/stack-helpers.h"
#include "protocols/http/classification-helpers.h"
#include "protocols/http2/helpers.h"
#include "protocols/kafka/kafka-classification.h"
#include "protocols/mongo/helpers.h"
#include "protocols/mysql/helpers.h"
#include "protocols/redis/helpers.h"
#include "protocols/postgres/helpers.h"

// Checks if a given buffer is http, http2, gRPC.
static __always_inline protocol_t classify_http_protocols(const char *buf, __u32 size) {
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

static __always_inline protocol_stack_t* get_protocol_stack(conn_tuple_t *tup) {
    if (!tup) {
        return NULL;
    }

    conn_tuple_t normalized_tup = *tup;
    normalize_tuple(&normalized_tup);
    protocol_stack_t* stack = bpf_map_lookup_elem(&connection_protocol, &normalized_tup);
    if (stack) {
        return stack;
    }

    // this code path is executed once during the entire connection lifecycle
    protocol_stack_t empty_stack = {0};
    bpf_map_update_with_telemetry(connection_protocol, &normalized_tup, &empty_stack, BPF_NOEXIST);
    return bpf_map_lookup_elem(&connection_protocol, &normalized_tup);
}

static __always_inline classification_buffer_t* get_buffer() {
    // Get the buffer the fragment will be read into from a per-cpu array map.
    // This will avoid doing unaligned stack access while parsing the protocols,
    // which is forbidden and will make the verifier fail.
    const u32 key = 0;
    return bpf_map_lookup_elem(&classification_buf, &key);
}

static __always_inline void init_buffer(struct __sk_buff *skb, skb_info_t *skb_info, classification_buffer_t* buffer) {
    bpf_memset(buffer->data, 0, sizeof(buffer->data));
    read_into_buffer_for_classification((char *)buffer->data, skb, skb_info);
    const size_t payload_length = skb->len - skb_info->data_off;
    buffer->size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
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

    protocol_stack_t *protocol_stack = get_protocol_stack(&skb_tup);
    if (!protocol_stack) {
        return;
    }

    classification_buffer_t *buffer = get_buffer();
    if (!buffer) {
        return;
    }
    init_buffer(skb, &skb_info, buffer);

    protocol_layer_t next_layer = protocol_next_layer(protocol_stack, LAYER_UNKNOWN);
    if (next_layer != LAYER_APPLICATION) {
        // for now we're only decoding application-layer protocols
        // but this is where a tail call to the next layer to decode would be added
        return;
    }

    protocol_t cur_fragment_protocol = classify_http_protocols(buffer->data, buffer->size);
    if (!cur_fragment_protocol) {
        bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_QUEUES_PROG);
    }

    protocol_set(protocol_stack, cur_fragment_protocol);
    if (cur_fragment_protocol == PROTOCOL_HTTP) {
        mark_as_fully_classified(protocol_stack);
    }

    // TODO: once we have other protocol layers we should add something like the following
    // next_layer = protocol_next_layer(protocol_stack, LAYER_APPLICATION);
    // switch(next_layer) {
    // case LAYER_API:
    //     bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_API_PROG);
    // case LAYER_ENCRYPTION:
    //     bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_ENCRYPTION_PROG);
    // }
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_queues(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    classification_buffer_t *buffer = get_buffer();
    if (!buffer) {
        return;
    }

    protocol_t cur_fragment_protocol = classify_queue_protocols(skb, &skb_info, buffer->data, buffer->size);
    if (!cur_fragment_protocol) {
        bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_DBS_PROG);
    }

    protocol_stack_t *protocol_stack = get_protocol_stack(&skb_tup);
    if (!protocol_stack) {
        return;
    }
    protocol_set(protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified(protocol_stack);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_dbs(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    classification_buffer_t *buffer = get_buffer();
    if (!buffer) {
        return;
    }

    protocol_t cur_fragment_protocol = classify_db_protocols(&skb_tup, buffer->data, buffer->size);
    if (!cur_fragment_protocol) {
        return;
    }

    protocol_stack_t *protocol_stack = get_protocol_stack(&skb_tup);
    if (!protocol_stack) {
        return;
    }
    protocol_set(protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified(protocol_stack);
}

#endif
