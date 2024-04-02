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
#include "protocols/classification/usm-context.h"
#include "protocols/classification/routing.h"
#include "protocols/grpc/defs.h"
#include "protocols/grpc/helpers.h"
#include "protocols/http/classification-helpers.h"
#include "protocols/http2/helpers.h"
#include "protocols/kafka/kafka-classification.h"
#include "protocols/mongo/helpers.h"
#include "protocols/mysql/helpers.h"
#include "protocols/redis/helpers.h"
#include "protocols/postgres/helpers.h"
#include "protocols/tls/tls.h"

// Some considerations about multiple protocol classification:
//
// * There are 3 protocol layers: API, Application and Encryption
//
// * Each protocol belongs to a specific layer (a `protocol_t` value encodes both the
// protocol ID itself and the protocol layer it belongs to)
//
// * Once a layer is "known" (for example, the application-layer protocol is
// classified), we only attempt to classify the remaining layers;
//
// * Protocol classification can be sliced/grouped into multiple BPF tail call
// programs (this is what we currently have now, but it is worth noting that in the
// new design all protocols from a given program must belong to the same layer)
//
// * If all 3 layers of a connection are known we don't do anything; In addition to
// that, there is a helper `mark_as_fully_classified` that works as a sort of
// special-case for this. For example, if we're in a socket filter context and we
// have classified a connection as a MySQL (application-level), we can call this
// helper to indicate that no further classification attempts are necessary (there
// won't be any api-level protocols above MySQL and if we were able to determine
// the application-level protocol from a socket filter context, it means we're not
// dealing with encrypted traffic).
// Calling this helper is optional and it works mostly as an optimization;
//
// * The tail-call jumping between different programs is completely abstracted by the
// `classification_next_program` helper. This helper knows how to either select the
// next program from a given layer, or to skip a certain layer if the protocol is
// already known;
//
// So, for example, if we have a connection that doesn't have any classified
// protocols yet calling `classification_next_program multiple` times will result in
// traversing all programs from all layers in the sequence defined in the routing.h
// file.  If, for example, application-layer is known, calling this helper multiple
// times will result in traversing only the api and encryption-layer programs

static __always_inline bool is_protocol_classification_supported() {
    __u64 val = 0;
    LOAD_CONSTANT("protocol_classification_enabled", val);
    return val > 0;
}

// updates the the protocol stack and adds the current layer to the routing skip list
static __always_inline void update_protocol_information(usm_context_t *usm_ctx, protocol_stack_t *stack, protocol_t proto) {
    set_protocol(stack, proto);
    usm_ctx->routing_skip_layers |= proto;
}

// Check if the connections is used for gRPC traffic.
static __always_inline void classify_grpc(usm_context_t *usm_ctx, protocol_stack_t *protocol_stack, struct __sk_buff *skb, skb_info_t *skb_info) {
    grpc_status_t status = is_grpc(skb, skb_info);
    if (status == PAYLOAD_UNDETERMINED) {
        return;
    }

    if (status == PAYLOAD_GRPC) {
        update_protocol_information(usm_ctx, protocol_stack, PROTOCOL_GRPC);
    }

    // Whether the traffic is gRPC or not, we can mark the stack as fully
    // classified now.
    mark_as_fully_classified(protocol_stack);
}

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

// A shared implementation for the runtime & prebuilt socket filter that classifies the protocols of the connections.
__maybe_unused static __always_inline void protocol_classifier_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // We support non empty TCP payloads for classification at the moment.
    if (!is_tcp(&skb_tup) || is_payload_empty(&skb_info)) {
        return;
    }

    usm_context_t *usm_ctx = usm_context_init(skb, &skb_tup, &skb_info);
    if (!usm_ctx) {
        return;
    }

    protocol_stack_t *protocol_stack = get_protocol_stack(&usm_ctx->tuple);
    if (!protocol_stack) {
        return;
    }

    if (is_fully_classified(protocol_stack) || is_protocol_layer_known(protocol_stack, LAYER_ENCRYPTION)) {
        return;
    }

    // Load information that will be later on used to route tail-calls
    init_routing_cache(usm_ctx, protocol_stack);

    const char *buffer = &(usm_ctx->buffer.data[0]);
    // TLS classification
    if (is_tls(buffer, usm_ctx->buffer.size, skb_info.data_end)) {
        update_protocol_information(usm_ctx, protocol_stack, PROTOCOL_TLS);
        // The connection is TLS encrypted, thus we cannot classify the protocol
        // using the socket filter and therefore we can bail out;
        return;
    }

    // If application-layer is known we don't bother to check for HTTP protocols and skip to the next layers
    protocol_t app_layer_proto = get_protocol_from_stack(protocol_stack, LAYER_APPLICATION);
    if (app_layer_proto != PROTOCOL_UNKNOWN && app_layer_proto != PROTOCOL_HTTP2) {
        goto next_program;
    }

    if (app_layer_proto == PROTOCOL_UNKNOWN) {
        app_layer_proto =  classify_applayer_protocols(buffer, usm_ctx->buffer.size);
    }

    if (app_layer_proto != PROTOCOL_UNKNOWN) {
        update_protocol_information(usm_ctx, protocol_stack, app_layer_proto);

        if (app_layer_proto == PROTOCOL_HTTP2) {
            // If we found HTTP2, then we try to classify its content.
            goto next_program;
        }

        mark_as_fully_classified(protocol_stack);
        return;
    }

 next_program:
    classification_next_program(skb, usm_ctx);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_queues(struct __sk_buff *skb) {
    usm_context_t *usm_ctx = usm_context(skb);
    if (!usm_ctx) {
        return;
    }
    const char *buffer = &(usm_ctx->buffer.data[0]);
    protocol_t cur_fragment_protocol = classify_queue_protocols(skb, &usm_ctx->skb_info, buffer, usm_ctx->buffer.size);
    if (!cur_fragment_protocol) {
        goto next_program;
    }

    protocol_stack_t *protocol_stack = get_protocol_stack(&usm_ctx->tuple);
    if (!protocol_stack) {
        return;
    }
    update_protocol_information(usm_ctx, protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified(protocol_stack);

 next_program:
    classification_next_program(skb, usm_ctx);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_dbs(struct __sk_buff *skb) {
    usm_context_t *usm_ctx = usm_context(skb);
    if (!usm_ctx) {
        return;
    }

    const char *buffer = &usm_ctx->buffer.data[0];
    protocol_t cur_fragment_protocol = classify_db_protocols(&usm_ctx->tuple, buffer, usm_ctx->buffer.size);
    if (!cur_fragment_protocol) {
        goto next_program;
    }

    protocol_stack_t *protocol_stack = get_protocol_stack(&usm_ctx->tuple);
    if (!protocol_stack) {
        return;
    }

    update_protocol_information(usm_ctx, protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified(protocol_stack);
 next_program:
    classification_next_program(skb, usm_ctx);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_grpc(struct __sk_buff *skb) {
    usm_context_t *usm_ctx = usm_context(skb);
    if (!usm_ctx) {
        return;
    }

    protocol_stack_t *protocol_stack = get_protocol_stack(&usm_ctx->tuple);
    if (!protocol_stack) {
        return;
    }

    // The GRPC classification program can be called without a prior
    // classification of HTTP2, which is a precondition.
    protocol_t app_layer_proto = get_protocol_from_stack(protocol_stack, LAYER_APPLICATION);
    if (app_layer_proto == PROTOCOL_HTTP2) {
        classify_grpc(usm_ctx, protocol_stack, skb, &usm_ctx->skb_info);
    }

    classification_next_program(skb, usm_ctx);
}

#endif
