#ifndef __PROTOCOL_CLASSIFICATION_H
#define __PROTOCOL_CLASSIFICATION_H

#include "protocol-classification-common.h"
#include "protocol-classification-defs.h"
#include "protocol-classification-maps.h"
#include "protocol-classification-structs.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "amqp-helpers.h"
#include "http-classification-helpers.h"
#include "http2-helpers.h"
#include "mongo-helpers.h"
#include "redis-helpers.h"

// Determines the protocols of the given buffer. If we already classified the payload (a.k.a protocol out param
// has a known protocol), then we do nothing.
static __always_inline void classify_protocol(protocol_t *protocol, conn_tuple_t *tup, const char *buf, __u32 size) {
    if (protocol == NULL || *protocol != PROTOCOL_UNKNOWN) {
        return;
    }

    if (is_http(buf, size)) {
        *protocol = PROTOCOL_HTTP;
    } else if (is_http2(buf, size)) {
        *protocol = PROTOCOL_HTTP2;
    } else if (is_amqp(buf, size)) {
        *protocol = PROTOCOL_AMQP;
    } else if (is_redis(buf, size)) {
        *protocol = PROTOCOL_REDIS;
    } else if (is_mongo(tup, buf, size)) {
        *protocol = PROTOCOL_MONGO;
    } else if (is_postgres(tup, buf, size)) {
        *protocol = PROTOCOL_POSTGRES;
    } else {
        *protocol = PROTOCOL_UNKNOWN;
    }

    log_debug("[protocol classification]: Classified protocol as %d %d; %s\n", *protocol, size, buf);
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
    read_into_buffer_for_classification((char *)request_fragment, skb, &skb_info);
    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
    classify_protocol(&cur_fragment_protocol, &skb_tup, request_fragment, final_fragment_size);
    // If there has been a change in the classification, save the new protocol.
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        bpf_map_update_with_telemetry(connection_protocol, &skb_tup, &cur_fragment_protocol, BPF_NOEXIST);
        conn_tuple_t inverse_skb_conn_tup = skb_tup;
        flip_tuple(&inverse_skb_conn_tup);
        bpf_map_update_with_telemetry(connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_NOEXIST);
    }
}

#endif
