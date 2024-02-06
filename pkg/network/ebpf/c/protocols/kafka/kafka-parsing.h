#ifndef __KAFKA_PARSING
#define __KAFKA_PARSING

#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "protocols/kafka/types.h"
#include "protocols/kafka/parsing-maps.h"
#include "protocols/kafka/usm-events.h"

// forward declaration
static __always_inline bool kafka_allow_packet(kafka_transaction_t *kafka, struct __sk_buff* skb, skb_info_t *skb_info);
static __always_inline bool kafka_process(kafka_transaction_t *kafka_transaction, struct __sk_buff* skb, __u32 offset, kafka_telemetry_t *kafka_tel);
static __always_inline void update_topic_name_size_telemetry(kafka_telemetry_t *kafka_tel, __u16 size);

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

    if (!fetch_dispatching_arguments(&kafka->base.tup, &skb_info)) {
        log_debug("socket__kafka_flter failed to fetch arguments for tail call");
        return 0;
    }

    kafka_telemetry_t *kafka_tel = bpf_map_lookup_elem(&kafka_telemetry, &zero);
    if (kafka_tel == NULL) {
        return 0;
    }

    if (!kafka_allow_packet(kafka, skb, &skb_info)) {
        return 0;
    }
    normalize_tuple(&kafka->base.tup);

    (void)kafka_process(kafka, skb, skb_info.data_off, kafka_tel);
    return 0;
}

READ_INTO_BUFFER(topic_name_parser, TOPIC_NAME_MAX_STRING_SIZE, BLK_SIZE)

static __always_inline bool kafka_process(kafka_transaction_t *kafka_transaction, struct __sk_buff* skb, __u32 offset, kafka_telemetry_t *kafka_tel) {
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
    update_topic_name_size_telemetry(kafka_tel, topic_name_size);
    if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
        __sync_fetch_and_add(&kafka_tel->topic_name_exceeds_max_size, 1);
        // TODO: Add telemetry for topic max size
        return false;
    }
    update_topic_name_size_telemetry(kafka_tel, topic_name_size);

    bpf_memset(kafka_transaction->base.topic_name, 0, TOPIC_NAME_MAX_STRING_SIZE);
    read_into_buffer_topic_name_parser((char *)kafka_transaction->base.topic_name, skb, offset);
    offset += topic_name_size;
    kafka_transaction->base.topic_name_size = topic_name_size;

    CHECK_STRING_COMPOSED_OF_ASCII_FOR_PARSING(TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, topic_name_size, kafka_transaction->base.topic_name);

    log_debug("kafka: topic name is %s", kafka_transaction->base.topic_name);

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
    log_debug("kafka: Current tcp sequence: %lu", skb_info->tcp_seq);
    // Hack to make verifier happy on 4.14.
    conn_tuple_t tup = kafka->base.tup;
    __u32 *last_tcp_seq = bpf_map_lookup_elem(&kafka_last_tcp_seq_per_connection, &tup);
    if (last_tcp_seq != NULL && *last_tcp_seq == skb_info->tcp_seq) {
        log_debug("kafka: already seen this tcp sequence: %lu", *last_tcp_seq);
        return false;
    }
    bpf_map_update_with_telemetry(kafka_last_tcp_seq_per_connection, &tup, &skb_info->tcp_seq, BPF_ANY);
    return true;
}

// update_path_size_telemetry updates the topic name size telemetry.
static __always_inline void update_topic_name_size_telemetry(kafka_telemetry_t *kafka_tel, __u16 size) {
    // This line can be considered as a step function of the difference multiplied by difference.
    // step function of the difference is 0 if the difference is negative, and 1 if the difference is positive.
    // Thus, if the difference is negative, we will get 0, and if the difference is positive, we will get the difference.
    size = size < TOPIC_NAME_MAX_STRING_SIZE ? 0 : size - TOPIC_NAME_MAX_STRING_SIZE;
    // This line acts as a ceil function, which means that if the size is not a multiple of the bucket size, we will
    // round it up to the next bucket. Since we don't have float numbers in eBPF, we are adding the (bucket size - 1)
    // to the size, and then dividing it by the bucket size. This will give us the ceil function.
    __u8 bucket_idx = (size + KAFKA_TELEMETRY_TOPIC_NAME_BUCKET_SIZE - 1) / KAFKA_TELEMETRY_TOPIC_NAME_BUCKET_SIZE;

    // This line guarantees that the bucket index is between 0 and KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS.
    // Although, it is not needed, we keep this function to please the verifier, and to have an explicit lower bound.
    bucket_idx = bucket_idx < 0 ? 0 : bucket_idx;
    // This line guarantees that the bucket index is between 0 and KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS, and we cannot
    // exceed the upper bound.
    bucket_idx = bucket_idx > KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS ? KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS : bucket_idx;

    __sync_fetch_and_add(&kafka_tel->topic_name_size_buckets[bucket_idx], 1);
}

#endif
