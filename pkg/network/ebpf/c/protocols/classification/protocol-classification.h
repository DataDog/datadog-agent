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
// TLS/Redis misclassification diagnostics: increment_telemetry_count + the diag event map.
#include "tracer/telemetry.h"
#include "protocols/classification/tls-misclassification-diag.h"

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
        // TLS-misclassification diagnostics: the third of three is_redis() call sites, counted
        // unconditionally so all three are directly comparable. See
        // protocols/classification/tls-misclassification-counters.h.
        if (is_tls_diag_enabled()) {
            tls_diag_count(TLS_DIAG_CTR_SOCKET_FILTER_REDIS);
        }
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

    classification_context_t *classification_ctx = classification_context_init(skb, &skb_tup, &skb_info);
    if (!classification_ctx) {
        return;
    }

    protocol_stack_t *protocol_stack = get_protocol_stack_if_exists(&classification_ctx->tuple);

    if (is_fully_classified(protocol_stack)) {
        return;
    }

    bool encryption_layer_known = is_protocol_layer_known(protocol_stack, LAYER_ENCRYPTION);

    // Load information that will be later on used to route tail-calls
    init_routing_cache(classification_ctx, protocol_stack);

    const char *buffer = &(classification_ctx->buffer.data[0]);

    protocol_t app_layer_proto = get_protocol_from_stack(protocol_stack, LAYER_APPLICATION);

    tls_record_header_t tls_hdr = {0};

    if ((app_layer_proto == PROTOCOL_UNKNOWN || app_layer_proto == PROTOCOL_POSTGRES) && is_tls(skb, skb_info.data_off, skb_info.data_end, &tls_hdr)) {
        protocol_stack = get_or_create_protocol_stack(&classification_ctx->tuple);
        if (!protocol_stack) {
            return;
        }
        // TLS classification
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

    // TLS/Redis misclassification diagnostics (see jmw/tls-misclassification).
    //
    if (is_tls_diag_enabled() && !encryption_layer_known) {
        tls_record_header_t diag_hdr = {0};
        if (is_tls_record_header_plausible(skb, skb_info.data_off, skb_info.data_end, &diag_hdr)) {
            // These two branches are mutually exclusive, and the distinction matters for
            // attribution. The is_tls() call above is gated on the app layer being UNKNOWN or
            // POSTGRES, so when any other protocol is already recorded is_tls() never ran at all —
            // nothing was "rejected", and counting a rejection there would over-attribute to
            // link 1 and inflate its totals.
            if (app_layer_proto == PROTOCOL_UNKNOWN || app_layer_proto == PROTOCOL_POSTGRES) {
                // is_tls() was reachable and, since control reached here, returned false on bytes
                // that do look like a TLS record. Attribute why.
                if (skb_info.data_off + sizeof(tls_record_header_t) + diag_hdr.length > skb_info.data_end) {
                    // Link 1a. read_tls_record_header()'s final bounds check requires the whole
                    // record to fit inside this packet. Records run to 16 KB while MSS is ~1460,
                    // so genuine TLS reads as "not TLS" and classification falls through to the
                    // app-layer classifiers.
                    increment_telemetry_count(tls_reject_record_exceeds_packet);
                    record_tls_misclassification_event(&skb_tup, TLS_DIAG_RECORD_EXCEEDS_PACKET, app_layer_proto, diag_hdr.content_type);
                } else if (diag_hdr.content_type == TLS_HANDSHAKE &&
                           !is_valid_tls_handshake(skb, skb_info.data_off, skb_info.data_end, &diag_hdr)) {
                    // Link 1b, structural variant. The record fits, so size is not why is_tls()
                    // failed; is_valid_tls_handshake() rejected it. That accepts ONLY ClientHello
                    // and ServerHello and requires handshake_length + 4 == record length, so
                    // Certificate/ServerKeyExchange/Finished records, records carrying several
                    // coalesced handshake messages, and fragmented handshake messages all land
                    // here. This is how a brand-new connection can be misclassified, with no
                    // missed handshake involved.
                    increment_telemetry_count(tls_reject_handshake_invalid);
                    record_tls_misclassification_event(&skb_tup, TLS_DIAG_HANDSHAKE_INVALID, app_layer_proto, diag_hdr.content_type);
                }
            } else {
                // Link 3. The app layer is already recorded, so is_tls() can never run again for
                // this connection and the wrong answer is pinned regardless of
                // FLAG_FULLY_CLASSIFIED. Purely observational; no state is modified here.
                increment_telemetry_count(tls_locked_out_by_applayer);
                record_tls_misclassification_event(&skb_tup, TLS_DIAG_LOCKED_OUT_BY_APPLAYER, app_layer_proto, diag_hdr.content_type);
            }
        }
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
        protocol_stack = get_or_create_protocol_stack(&classification_ctx->tuple);
        if (!protocol_stack) {
            return;
        }
        update_protocol_information(classification_ctx, protocol_stack, app_layer_proto);

        if (app_layer_proto == PROTOCOL_HTTP2) {
            // If we found HTTP2, then we try to classify its content.
            goto next_program;
        }

        mark_as_fully_classified(protocol_stack);
        return;
    }

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
    mark_as_fully_classified(protocol_stack);

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

    // TLS/Redis misclassification diagnostics (see jmw/tls-misclassification and
    // protocols/classification/tls-misclassification-diag.h).
    //
    // Link 2 of the suspected chain. is_redis() accepts a buffer on the strength of a single byte
    // matching one of 14 RESP type markers, so it matches arbitrary ciphertext roughly 5.5% of the
    // time. Two independent signals that a match here is spurious:
    //
    //   a) the buffer begins with a plausible TLS record header — near-conclusive, since no real
    //      RESP frame starts with a valid content_type plus TLS version; and
    //   b) Redis was matched on a port Redis never serves. Classification is purely content-based
    //      (there are no port heuristics anywhere in it), so this is a false positive by
    //      definition rather than a heuristic judgement.
    //
    // Both are observational only — the classification below is left exactly as it was.
    if (is_tls_diag_enabled()) {
        tls_record_header_t diag_hdr = {0};
        bool looks_like_tls = is_tls_record_header_plausible(skb, classification_ctx->skb_info.data_off, classification_ctx->skb_info.data_end, &diag_hdr);
        if (looks_like_tls) {
            increment_telemetry_count(applayer_match_on_tls_payload);
            record_tls_misclassification_event(&classification_ctx->tuple, TLS_DIAG_APPLAYER_ON_TLS_PAYLOAD, cur_fragment_protocol, diag_hdr.content_type);
        }
        if (cur_fragment_protocol == PROTOCOL_REDIS &&
            !is_standard_redis_port(classification_ctx->tuple.sport, classification_ctx->tuple.dport)) {
            increment_telemetry_count(redis_match_on_nonstandard_port);
            record_tls_misclassification_event(&classification_ctx->tuple, TLS_DIAG_REDIS_NONSTANDARD_PORT, cur_fragment_protocol, looks_like_tls ? diag_hdr.content_type : 0);
        }
    }

    protocol_stack_t *protocol_stack = get_or_create_protocol_stack(&classification_ctx->tuple);
    if (!protocol_stack) {
        return;
    }

    update_protocol_information(classification_ctx, protocol_stack, cur_fragment_protocol);
    mark_as_fully_classified(protocol_stack);
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
