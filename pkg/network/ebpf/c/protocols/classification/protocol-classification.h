#ifndef __PROTOCOL_CLASSIFICATION_H
#define __PROTOCOL_CLASSIFICATION_H

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "port_range.h"

#include "protocols/amqp/helpers.h"
#include "protocols/classification/classification-context.h"
#include "protocols/classification/common.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/maps.h"
#include "protocols/classification/structs.h"
#include "protocols/classification/stack-helpers.h"
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
#include "tracer/telemetry.h"

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

// updates the the protocol stack and adds the current layer to the routing skip list
static __always_inline void update_protocol_information(classification_context_t *classification_ctx, protocol_stack_t *stack, protocol_t proto) {
    set_protocol(stack, proto);
    classification_ctx->routing_skip_layers |= proto;
}

// Helper to record classification attempt histogram when a connection becomes fully classified
static __always_inline void record_classification_attempt_histogram(__u32 attempts) {
    if (attempts == 0) {
        return;
    }
    if (attempts == 1) {
        increment_telemetry_count(protocol_classifier_classified_after_1_attempt);
    } else if (attempts == 2) {
        increment_telemetry_count(protocol_classifier_classified_after_2_attempts);
    } else if (attempts == 3) {
        increment_telemetry_count(protocol_classifier_classified_after_3_attempts);
    } else {
        increment_telemetry_count(protocol_classifier_classified_after_4_plus_attempts);
    }
}

// Helper to track when mark_as_fully_classified is called and record the attempt histogram
static __always_inline void mark_as_fully_classified_with_telemetry(protocol_stack_t *stack, conn_tuple_t *tuple) {
    if (!stack) {
        return;
    }
    
    // Only record telemetry if this is the first time we're marking as fully classified
    if (!(stack->flags & FLAG_FULLY_CLASSIFIED)) {
        increment_telemetry_count(protocol_classifier_mark_fully_classified_calls);
        
        // Record the histogram of classification attempts
        protocol_stack_wrapper_t *wrapper = get_protocol_stack_wrapper_if_exists(tuple);
        if (wrapper) {
            record_classification_attempt_histogram(wrapper->classification_attempts);
        }
    }
    
    stack->flags |= FLAG_FULLY_CLASSIFIED;
}

// Check if the connections is used for gRPC traffic.
static __always_inline void classify_grpc(classification_context_t *classification_ctx, protocol_stack_t *protocol_stack, struct __sk_buff *skb, skb_info_t *skb_info) {
    grpc_status_t status = is_grpc(skb, skb_info);
    if (status == PAYLOAD_UNDETERMINED) {
        return;
    }

    if (status == PAYLOAD_GRPC) {
        update_protocol_information(classification_ctx, protocol_stack, PROTOCOL_GRPC);
    }

    // Whether the traffic is gRPC or not, we can mark the stack as fully
    // classified now.
    mark_as_fully_classified_with_telemetry(protocol_stack, &classification_ctx->tuple);
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

// Helper macro to record early exit timing in protocol classifier
#define RECORD_CLASSIFIER_EARLY_EXIT(counter_name, time_field_name, start_ns) \
    do { \
        increment_telemetry_count(counter_name); \
        __u64 key = 0; \
        telemetry_t *val = bpf_map_lookup_elem(&telemetry, &key); \
        if (val != NULL) { \
            __u64 duration = bpf_ktime_get_ns() - start_ns; \
            __sync_fetch_and_add(&val->time_field_name, duration); \
        } \
    } while(0)

// A shared implementation for the runtime & prebuilt socket filter that classifies the protocols of the connections.
__maybe_unused static __always_inline void protocol_classifier_entrypoint(struct __sk_buff *skb) {
    // Capture start time for timing metrics
    __u64 entrypoint_start_ns = bpf_ktime_get_ns();

    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        RECORD_CLASSIFIER_EARLY_EXIT(protocol_classifier_entrypoint_read_conn_tuple_failed_calls,
                                     protocol_classifier_entrypoint_read_conn_tuple_failed_time_ns,
                                     entrypoint_start_ns);
        return;
    }

    // We support non empty TCP payloads for classification at the moment.
    if (!is_tcp(&skb_tup) || is_payload_empty(&skb_info)) {
        RECORD_CLASSIFIER_EARLY_EXIT(protocol_classifier_entrypoint_not_tcp_or_empty_calls,
                                     protocol_classifier_entrypoint_not_tcp_or_empty_time_ns,
                                     entrypoint_start_ns);
        return;
    }

    classification_context_t *classification_ctx = classification_context_init(skb, &skb_tup, &skb_info);
    if (!classification_ctx) {
        RECORD_CLASSIFIER_EARLY_EXIT(protocol_classifier_entrypoint_context_init_failed_calls,
                                     protocol_classifier_entrypoint_context_init_failed_time_ns,
                                     entrypoint_start_ns);
        return;
    }

    // Get the wrapper once - we can derive protocol_stack from it and also check classification_attempts
    protocol_stack_wrapper_t *wrapper = get_protocol_stack_wrapper_if_exists(&classification_ctx->tuple);
    protocol_stack_t *protocol_stack = wrapper ? &wrapper->stack : NULL;

    // Debug: track when protocol_stack is NULL vs when it exists but isn't fully classified
    if (!protocol_stack) {
        increment_telemetry_count(protocol_classifier_entrypoint_no_protocol_stack_calls);
    } else if (!is_fully_classified(protocol_stack)) {
        increment_telemetry_count(protocol_classifier_entrypoint_stack_not_fully_classified_calls);
        // More detailed debug: check if FLAG_FULLY_CLASSIFIED is set
        if (protocol_stack->flags & FLAG_FULLY_CLASSIFIED) {
            // Flag IS set but is_fully_classified still returned false - shouldn't happen!
            increment_telemetry_count(protocol_classifier_entrypoint_flag_set_but_not_classified_calls);
        } else if (protocol_stack->layer_application > 0) {
            // Has app layer but flag not set
            increment_telemetry_count(protocol_classifier_entrypoint_has_app_layer_no_flag_calls);
        } else {
            // Stack exists but has NO app layer at all - empty/unclassified stack
            increment_telemetry_count(protocol_classifier_entrypoint_empty_stack_calls);
        }
    }

    if (is_fully_classified(protocol_stack)) {
        // Track early exit when connection is already fully classified
        RECORD_CLASSIFIER_EARLY_EXIT(protocol_classifier_entrypoint_already_classified_calls,
                                     protocol_classifier_entrypoint_already_classified_time_ns,
                                     entrypoint_start_ns);
        return;
    }

    // Check if we've already given up on this connection (hit max attempts limit previously).
    // We check this early to avoid the cost of get_or_create_protocol_stack for connections
    // that we've already decided can't be classified.
    if (wrapper && should_give_up_classification(wrapper->classification_attempts)) {
        // Already hit the limit on a previous call - just exit without doing any more work
        RECORD_CLASSIFIER_EARLY_EXIT(protocol_classifier_gave_up_classification_calls,
                                     protocol_classifier_gave_up_classification_time_ns,
                                     entrypoint_start_ns);
        return;
    }

    // Ensure the protocol stack wrapper exists before incrementing classification attempts.
    // This is needed because increment_classification_attempts operates on the wrapper,
    // and we need the wrapper to exist so the attempt count is properly tracked for the histogram.
    protocol_stack = get_or_create_protocol_stack(&classification_ctx->tuple);
    if (!protocol_stack) {
        return;
    }

    // Increment classification attempts for this connection (only if we're doing actual classification work)
    __u32 attempts = increment_classification_attempts(&classification_ctx->tuple);

    // Check if we've NOW exceeded max classification attempts - if so, give up and mark as fully classified
    // This prevents wasting CPU cycles on connections that can't be classified (e.g., data-only packets)
    if (should_give_up_classification(attempts)) {
        mark_as_fully_classified(protocol_stack);
        RECORD_CLASSIFIER_EARLY_EXIT(protocol_classifier_gave_up_classification_calls,
                                     protocol_classifier_gave_up_classification_time_ns,
                                     entrypoint_start_ns);
        return;
    }

    bool encryption_layer_known = is_protocol_layer_known(protocol_stack, LAYER_ENCRYPTION);

    // Load information that will be later on used to route tail-calls
    init_routing_cache(classification_ctx, protocol_stack);

    const char *buffer = &(classification_ctx->buffer.data[0]);

    protocol_t app_layer_proto = get_protocol_from_stack(protocol_stack, LAYER_APPLICATION);

    tls_record_header_t tls_hdr = {0};

    if ((app_layer_proto == PROTOCOL_UNKNOWN || app_layer_proto == PROTOCOL_POSTGRES) && is_tls(skb, skb_info.data_off, skb_info.data_end, &tls_hdr)) {
        // TLS classification (protocol_stack already exists from get_or_create above)
        increment_telemetry_count(protocol_classifier_detected_tls_calls);
        update_protocol_information(classification_ctx, protocol_stack, PROTOCOL_TLS);
        if (tls_hdr.content_type != TLS_HANDSHAKE) {
            // If the TLS record is not a handshake, we can stop here as we've already marked the protocol as TLS
            // and there is no need to look for additional handshake tags
            return;
        }

        // Parse TLS handshake payload
        tls_info_t *tags = get_or_create_tls_enhanced_tags(&classification_ctx->tuple);
        if (tags) {
            // The packet is a TLS handshake, so trigger tail calls to extract metadata from the payload
            __u32 offset = classification_ctx->skb_info.data_off + sizeof(tls_record_header_t);
            __u32 data_end = classification_ctx->skb_info.data_end;
            if (is_tls_handshake_client_hello(skb, offset, data_end)) {
                bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_TLS_CLIENT_PROG);
                return;
            }
            if (is_tls_handshake_server_hello(skb, offset, data_end)) {
                bpf_tail_call_compat(skb, &classification_progs, CLASSIFICATION_TLS_SERVER_PROG);
                return;
            }
        }
        return;
    }

    // If we have already classified the encryption layer, we can skip the rest of the classification
    if (encryption_layer_known) {
        return;
    }

    if (app_layer_proto != PROTOCOL_UNKNOWN && app_layer_proto != PROTOCOL_HTTP2) {
        goto next_program;
    }

    if (app_layer_proto == PROTOCOL_UNKNOWN) {
        app_layer_proto =  classify_applayer_protocols(buffer, classification_ctx->buffer.size);
    }

    if (app_layer_proto != PROTOCOL_UNKNOWN) {
        // protocol_stack already exists from get_or_create above
        // Track which protocol was detected
        if (app_layer_proto == PROTOCOL_HTTP) {
            increment_telemetry_count(protocol_classifier_detected_http_calls);
        } else if (app_layer_proto == PROTOCOL_HTTP2) {
            increment_telemetry_count(protocol_classifier_detected_http2_calls);
        }
        
        update_protocol_information(classification_ctx, protocol_stack, app_layer_proto);

        if (app_layer_proto == PROTOCOL_HTTP2) {
            // If we found HTTP2, then we try to classify its content.
            goto next_program;
        }

        mark_as_fully_classified_with_telemetry(protocol_stack, &classification_ctx->tuple);
        return;
    }

    // No protocol detected - track as unknown
    increment_telemetry_count(protocol_classifier_detected_unknown_calls);

 next_program:
    classification_next_program(skb, classification_ctx);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_tls_handshake_client(struct __sk_buff *skb) {
    classification_context_t *classification_ctx = classification_context(skb);
    if (!classification_ctx) {
        return;
    }
    tls_info_t* tls_info = get_tls_enhanced_tags(&classification_ctx->tuple);
    if (!tls_info) {
        return;
    }
    __u32 offset = classification_ctx->skb_info.data_off + sizeof(tls_record_header_t);
    __u32 data_end = classification_ctx->skb_info.data_end;
    if (!parse_client_hello(skb, offset, data_end, tls_info)) {
        return;
    }
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_tls_handshake_server(struct __sk_buff *skb) {
    classification_context_t *classification_ctx = classification_context(skb);
    if (!classification_ctx) {
        return;
    }
    tls_info_t* tls_info = get_tls_enhanced_tags(&classification_ctx->tuple);
    if (!tls_info) {
        return;
    }
    __u32 offset = classification_ctx->skb_info.data_off + sizeof(tls_record_header_t);
    __u32 data_end = classification_ctx->skb_info.data_end;
    if (!parse_server_hello(skb, offset, data_end, tls_info)) {
        return;
    }
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_queues(struct __sk_buff *skb) {
    classification_context_t *classification_ctx = classification_context(skb);
    if (!classification_ctx) {
        return;
    }
    const char *buffer = &(classification_ctx->buffer.data[0]);
    protocol_t cur_fragment_protocol = classify_queue_protocols(skb, &classification_ctx->skb_info, buffer, classification_ctx->buffer.size);
    if (!cur_fragment_protocol) {
        goto next_program;
    }

    protocol_stack_t *protocol_stack = get_or_create_protocol_stack(&classification_ctx->tuple);
    if (!protocol_stack) {
        return;
    }
    update_protocol_information(classification_ctx, protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified_with_telemetry(protocol_stack, &classification_ctx->tuple);

 next_program:
    classification_next_program(skb, classification_ctx);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_dbs(struct __sk_buff *skb) {
    classification_context_t *classification_ctx = classification_context(skb);
    if (!classification_ctx) {
        return;
    }

    const char *buffer = &classification_ctx->buffer.data[0];
    protocol_t cur_fragment_protocol = classify_db_protocols(&classification_ctx->tuple, buffer, classification_ctx->buffer.size);
    if (!cur_fragment_protocol) {
        goto next_program;
    }

    protocol_stack_t *protocol_stack = get_or_create_protocol_stack(&classification_ctx->tuple);
    if (!protocol_stack) {
        return;
    }

    update_protocol_information(classification_ctx, protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified_with_telemetry(protocol_stack, &classification_ctx->tuple);
 next_program:
    classification_next_program(skb, classification_ctx);
}

__maybe_unused static __always_inline void protocol_classifier_entrypoint_grpc(struct __sk_buff *skb) {
    classification_context_t *classification_ctx = classification_context(skb);
    if (!classification_ctx) {
        return;
    }

    // gRPC classification can happen only if the application layer is known
    // So if we don't have a protocol stack, we can continue to the next program.
    protocol_stack_t *protocol_stack = get_protocol_stack_if_exists(&classification_ctx->tuple);
    if (protocol_stack) {
        // The GRPC classification program can be called without a prior
        // classification of HTTP2, which is a precondition.
        protocol_t app_layer_proto = get_protocol_from_stack(protocol_stack, LAYER_APPLICATION);
        if (app_layer_proto == PROTOCOL_HTTP2) {
            classify_grpc(classification_ctx, protocol_stack, skb, &classification_ctx->skb_info);
        }
    }

    classification_next_program(skb, classification_ctx);
}

#endif
