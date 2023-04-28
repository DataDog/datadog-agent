#ifndef __KAFKA_PARSING
#define __KAFKA_PARSING

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "tracer.h"
#include "protocols/kafka/types.h"
#include "protocols/kafka/parsing-maps.h"
#include "protocols/kafka/usm-events.h"

// forward declaration
static __always_inline bool kafka_allow_packet(kafka_transaction_t *kafka, struct __sk_buff* skb, skb_info_t *skb_info);
static __always_inline bool kafka_process(kafka_transaction_t *kafka_transaction, struct __sk_buff* skb, __u32 offset);

// A template for verifying a given buffer is composed of the characters [a-z], [A-Z], [0-9], ".", "_", or "-".
// The iterations reads up to MIN(max_buffer_size, real_size).
// Has to be a template and not a function, as we have pragma unroll.
#define CHECK_STRING_COMPOSED_OF_ASCII_FOR_PARSING(max_buffer_size, real_size, buffer)                                                      \
    char ch = 0;                                                                                                                            \
_Pragma( STRINGIFY(unroll(max_buffer_size)) )                                                                                               \
    for (int j = 0; j < max_buffer_size; j++) {                                                                                             \
        /* Verifies we are not exceeding the real client_id_size, and if we do, we finish the iteration as we reached */                    \
        /* to the end of the buffer and all checks have been successful. */                                                                 \
        if (j + 1 <= real_size) {                                                                                                           \
            ch = buffer[j];                                                                                                                 \
            if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {  \
                continue;                                                                                                                   \
            }                                                                                                                               \
            return false;                                                                                                                   \
        }                                                                                                                                   \
    }                                                                                                                                       \

SEC("socket/kafka_filter")
int socket__kafka_filter(struct __sk_buff* skb) {
    const u32 zero = 0;
    skb_info_t skb_info;
    kafka_transaction_t *kafka = bpf_map_lookup_elem(&kafka_heap, &zero);
    if (kafka == NULL) {
        log_debug("socket__kafka_filter: kafka_transaction state is NULL\n");
        return 0;
    }
    bpf_memset(kafka, 0, sizeof(kafka_transaction_t));

    if (!fetch_dispatching_arguments(&kafka->base.tup, &skb_info)) {
        log_debug("socket__kafka_filter failed to fetch arguments for tail call\n");
        return 0;
    }

    if (!kafka_allow_packet(kafka, skb, &skb_info)) {
        return 0;
    }
    normalize_tuple(&kafka->base.tup);

    (void)kafka_process(kafka, skb, skb_info.data_off);
    return 0;
}

static __always_inline void parser_read_into_buffer_topic_name(char *buffer, struct __sk_buff *skb, u32 initial_offset) {
    u64 offset = (u64)initial_offset;

#define BLK_SIZE (16)
    const u32 len = TOPIC_NAME_MAX_STRING_SIZE < (skb->len - (u32)offset) ? (u32)offset + TOPIC_NAME_MAX_STRING_SIZE : skb->len;

    unsigned i = 0;

#pragma unroll(TOPIC_NAME_MAX_STRING_SIZE / BLK_SIZE)
    for (; i < (TOPIC_NAME_MAX_STRING_SIZE / BLK_SIZE); i++) {
        if (offset + BLK_SIZE - 1 >= len) { break; }

        bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
        offset += BLK_SIZE;
    }

    // This part is very hard to write in a loop and unroll it.
    // Indeed, mostly because of older kernel verifiers, we want to make sure the offset into the buffer is not
    // stored on the stack, so that the verifier is able to verify that we're not doing out-of-bound on
    // the stack.
    // Basically, we should get a register from the code block above containing an fp relative address. As
    // we are doing `buffer[0]` here, there is not dynamic computation on that said register after this,
    // and thus the verifier is able to ensure that we are in-bound.
    void *buf = &buffer[i * BLK_SIZE];
    if (i * BLK_SIZE >= TOPIC_NAME_MAX_STRING_SIZE) {
        return;
    } else if (offset + 14 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 1);
    }
}

static __always_inline bool kafka_process(kafka_transaction_t *kafka_transaction, struct __sk_buff* skb, __u32 offset) {
    /*
        We perform Kafka request validation as we can get kafka traffic that is not relevant for parsing (unsupported requests, responses, etc)
    */

    kafka_header_t kafka_header;
    bpf_memset(&kafka_header, 0, sizeof(kafka_header));
    bpf_skb_load_bytes_with_telemetry(skb, offset, (char *)&kafka_header, sizeof(kafka_header));
    kafka_header.message_size = bpf_ntohl(kafka_header.message_size);
    kafka_header.api_key = bpf_ntohs(kafka_header.api_key);
    kafka_header.api_version = bpf_ntohs(kafka_header.api_version);
    kafka_header.correlation_id = bpf_ntohl(kafka_header.correlation_id);
    kafka_header.client_id_size = bpf_ntohs(kafka_header.client_id_size);

    log_debug("kafka: kafka_header.api_key: %d\n", kafka_header.api_key);
    log_debug("kafka: kafka_header.api_version: %d\n", kafka_header.api_version);

    if (!is_valid_kafka_request_header(&kafka_header)) {
        return false;
    }

    kafka_transaction->base.request_api_key = kafka_header.api_key;
    kafka_transaction->base.request_api_version = kafka_header.api_version;

    offset += sizeof(kafka_header_t);

    // Validate client ID
    // Client ID size can be equal to '-1' if the client id is null.
    if (kafka_header.client_id_size > 0) {
        if (!is_valid_client_id(skb, offset, kafka_header.client_id_size)) {
            return false;
        }
        offset += kafka_header.client_id_size;
    } else if (kafka_header.client_id_size < -1) {
        return false;
    }

    switch (kafka_header.api_key) {
    case KAFKA_PRODUCE:
        if (!get_topic_offset_from_produce_request(&kafka_header, skb, &offset)) {
            return false;
        }
        break;
    case KAFKA_FETCH:
        offset += get_topic_offset_from_fetch_request(&kafka_header);
        break;
    default:
        return false;
    }

    // Skipping number of entries for now
    offset += sizeof(s32);
    READ_BIG_ENDIAN_WRAPPER(s16, topic_name_size, skb, offset);
    if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
        return false;
    }
    bpf_memset(kafka_transaction->base.topic_name, 0, TOPIC_NAME_MAX_STRING_SIZE);
    parser_read_into_buffer_topic_name((char *)kafka_transaction->base.topic_name, skb, offset);
    offset += topic_name_size;
    kafka_transaction->base.topic_name_size = topic_name_size;

    CHECK_STRING_COMPOSED_OF_ASCII_FOR_PARSING(TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, topic_name_size, kafka_transaction->base.topic_name);

    log_debug("kafka: topic name is %s\n", kafka_transaction->base.topic_name);

    kafka_batch_enqueue(&kafka_transaction->base);
    return true;
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
    // Hack to make verifier happy on 4.14.
    conn_tuple_t tup = kafka->base.tup;
    __u32 *last_tcp_seq = bpf_map_lookup_elem(&kafka_last_tcp_seq_per_connection, &tup);
    if (last_tcp_seq != NULL && *last_tcp_seq == skb_info->tcp_seq) {
        log_debug("kafka: already seen this tcp sequence: %lu\n", *last_tcp_seq);
        return false;
    }
    bpf_map_update_with_telemetry(kafka_last_tcp_seq_per_connection, &tup, &skb_info->tcp_seq, BPF_ANY);
    return true;
}

#endif
