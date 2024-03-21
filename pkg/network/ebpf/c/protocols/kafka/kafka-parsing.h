#ifndef __KAFKA_PARSING
#define __KAFKA_PARSING

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
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
        log_debug("socket__kafka_filter: kafka_transaction state is NULL");
        return 0;
    }
    bpf_memset(kafka, 0, sizeof(kafka_transaction_t));

    if (!fetch_dispatching_arguments(&kafka->tup, &skb_info)) {
        log_debug("socket__kafka_filter failed to fetch arguments for tail call");
        return 0;
    }

    if (!kafka_allow_packet(kafka, skb, &skb_info)) {
        return 0;
    }
    normalize_tuple(&kafka->tup);

    (void)kafka_process(kafka, skb, skb_info.data_off);
    return 0;
}

READ_INTO_BUFFER(topic_name_parser, TOPIC_NAME_MAX_STRING_SIZE, BLK_SIZE)

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

    log_debug("kafka: kafka_header.api_key: %d", kafka_header.api_key);
    log_debug("kafka: kafka_header.api_version: %d", kafka_header.api_version);

    if (!is_valid_kafka_request_header(&kafka_header)) {
        return false;
    }

    kafka_transaction->request_api_key = kafka_header.api_key;
    kafka_transaction->request_api_version = kafka_header.api_version;

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
    bpf_memset(kafka_transaction->topic_name, 0, TOPIC_NAME_MAX_STRING_SIZE);
    read_into_buffer_topic_name_parser((char *)kafka_transaction->topic_name, skb, offset);
    offset += topic_name_size;
    kafka_transaction->topic_name_size = topic_name_size;

    CHECK_STRING_COMPOSED_OF_ASCII_FOR_PARSING(TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, topic_name_size, kafka_transaction->topic_name);

    log_debug("kafka: topic name is %s", kafka_transaction->topic_name);

    kafka_batch_enqueue(kafka_transaction);
    return true;
}

// this function is called by the socket-filter program to decide whether or not we should inspect
// the contents of a certain packet, in order to avoid the cost of processing packets that are not
// of interest such as empty ACKs, UDP data or encrypted traffic.
static __always_inline bool kafka_allow_packet(kafka_transaction_t *kafka, struct __sk_buff* skb, skb_info_t *skb_info) {
    // we're only interested in TCP traffic
    if (!(kafka->tup.metadata&CONN_TYPE_TCP)) {
        return false;
    }

    // if payload data is empty or if this is an encrypted packet, we only
    // process it if the packet represents a TCP termination
    bool empty_payload = skb_info->data_off == skb->len;
    if (empty_payload) {
        return skb_info->tcp_flags&(TCPHDR_FIN|TCPHDR_RST);
    }

    return true;
}

#endif
