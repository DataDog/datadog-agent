#ifndef __KAFKA_HELPERS_H
#define __KAFKA_HELPERS_H

#include "kafka-defs.h"
#include "kafka-types.h"
#include "bpf_endian.h"

// Forward declaration
static bool is_kafka_request_header(kafka_context_t* kafka_context);
static bool is_kafka_request(kafka_context_t* kafka_context);
static void fill_kafka_header(kafka_context_t* kafka_context);
static bool try_parse_produce_request(kafka_context_t *kafka_context);
static bool try_parse_fetch_request(kafka_context_t *kafka_context);
//static bool validate_first_topic_name(kafka_context_t *kafka_context);
static bool kafka_read_big_endian_int32(kafka_context_t *kafka_context, int32_t* result);
static bool kafka_read_big_endian_int16(kafka_context_t *kafka_context, int16_t* result);
static int32_t read_big_endian_int32(const char* buf);
static int16_t read_big_endian_int16(const char* buf);

#define MIN(a,b) (((a)<(b))?(a):(b))

#define ENSURE_LIMITS(kafka_context, space, ret)                                                    \
    ({                                                                                              \
        s64 remaining_length = (s64)kafka_context->buffer_size - (s64)kafka_context->offset - 1;    \
        if(remaining_length < (space)) {                                                            \
            return (ret);                                                                           \
        }                                                                                           \
        remaining_length = (s64)CLASSIFICATION_MAX_BUFFER - (s64)kafka_context->offset - 1;         \
        if(remaining_length < (space)) {                                                            \
            return (ret);                                                                           \
        }                                                                                           \
    })

static __always_inline bool is_kafka(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, KAFKA_MIN_LENGTH);

    kafka_context_t kafka_context;
    kafka_context.buffer = buf;
    kafka_context.buffer_size = MIN(buf_size, CLASSIFICATION_MAX_BUFFER);
    fill_kafka_header(&kafka_context);
    log_debug("kafka: kafka_context->offset: %u\n", kafka_context.offset);

    return is_kafka_request_header(&kafka_context) && is_kafka_request(&kafka_context);
}

static __always_inline bool is_kafka_request_header(kafka_context_t* kafka_context) {
    kafka_header_t *kafka_header = &kafka_context->header;

    if (kafka_header->message_size < sizeof(kafka_header_t)) {
        return false;
    }

//    log_debug("kafka: message size: %d\n", kafka_header->message_size);

    switch (kafka_header->api_key) {
    case KAFKA_FETCH:
        break;
    case KAFKA_PRODUCE:
        if (kafka_header->api_version == 0) {
            // We have seen some false positives when both request_api_version and request_api_key are 0,
            // so dropping support for this case
            return false;
        }
        break;
    default:
        // We are only interested in fetch and produce requests
        return false;
    }

//    log_debug("kafka: api key: %d\n", kafka_header->api_key);

    if (kafka_header->api_version < 0 || kafka_header->api_version > KAFKA_MAX_SUPPORTED_REQUEST_API_VERSION) {
        return false;
    }

//    log_debug("kafka: api version: %d\n", kafka_header->api_version);

    if (kafka_header->correlation_id < 0) {
        return false;
    }

//    log_debug("kafka: correlation id: %d\n", kafka_header->correlation_id);

    if (kafka_header->client_id_size < 0) {
         return false;
    }

    log_debug("kafka: client_id_size: %d", kafka_header->client_id_size);

    ENSURE_LIMITS(kafka_context, kafka_header->client_id_size, false);

    const char* client_id_starting_offset = kafka_context->buffer + sizeof(kafka_header_t);
    char ch = 0;
#pragma unroll(CLIENT_ID_SIZE_TO_VALIDATE)
    for (unsigned i = 0; i < CLIENT_ID_SIZE_TO_VALIDATE; i++) {
        if (i >= kafka_header->client_id_size) {
            break;
        }
        ch = client_id_starting_offset[i];
        if (ch == 0) {
            return false;
        }
        // Assuming no UTF-8 characters in the client id as we didn't see any such so far
        if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {
            continue;
        }
    }

//    if (kafka_header->client_id_size > 0) {
//        log_debug("kafka: client id: %s\n", client_id_starting_offset);
//    }

    kafka_context->offset += kafka_header->client_id_size;
    return true;
}

static __always_inline void fill_kafka_header(kafka_context_t* kafka_context) {
    kafka_header_t *header_view = (kafka_header_t *)kafka_context->buffer;
    kafka_header_t *kafka_header = &kafka_context->header;
    kafka_header->message_size = bpf_ntohl(header_view->message_size);
    kafka_header->api_key = bpf_ntohs(header_view->api_key);
    kafka_header->api_version = bpf_ntohs(header_view->api_version);
    kafka_header->correlation_id = bpf_ntohl(header_view->correlation_id);
    kafka_header->client_id_size = bpf_ntohs(header_view->client_id_size);
    kafka_context->offset += sizeof(kafka_header_t);
}

static __always_inline bool is_kafka_request(kafka_context_t* kafka_context) {
    switch (kafka_context->header.api_key) {
        case KAFKA_PRODUCE:
            return try_parse_produce_request(kafka_context);
        case KAFKA_FETCH:
            return try_parse_fetch_request(kafka_context);
        default:
            return false;
    }
}

static __always_inline bool try_parse_produce_request(kafka_context_t *kafka_context) {
    log_debug("kafka: trying to parse produce request\n");
    int16_t api_version = kafka_context->header.api_version;
    if (api_version >= 9) {
        log_debug("kafka: Produce request version 9 and above is not supported: %d\n", api_version);
        return false;
    }

    if (api_version >= 3) {
        int16_t transactional_id_size = 0;
        if (!kafka_read_big_endian_int16(kafka_context, &transactional_id_size)) {
            return false;
        }
        log_debug("kafka: transactional_id_size: %d\n", transactional_id_size);
        if (transactional_id_size > 0) {
            kafka_context->offset += transactional_id_size;
        }
    }

//    log_debug("kafka: kafka_context->buffer[kafka_context->offset]: %x\n", kafka_context->buffer[kafka_context->offset]);
//    log_debug("kafka: kafka_context->buffer[kafka_context->offset + 1]: %x\n", kafka_context->buffer[kafka_context->offset + 1]);
//    log_debug("kafka: kafka_context->buffer[kafka_context->offset + 2]: %x\n", kafka_context->buffer[kafka_context->offset + 2]);

    int16_t acks = 0;
    if (!kafka_read_big_endian_int16(kafka_context, &acks)) {
        return false;
    }
//    log_debug("kafka: required acks: %d\n", acks);

    if (acks > 1 || acks < -1) {
        // The number of acknowledgments the producer requires the leader to have received before considering a request
        // complete. Allowed values: 0 for no acknowledgments, 1 for only the leader and -1 for the full ISR.
        return false;
    }

    int32_t timeout_ms = 0;
    if (!kafka_read_big_endian_int32(kafka_context, &timeout_ms)) {
        return false;
    }
//    log_debug("kafka: timeout ms: %d\n", timeout_ms);

    if (timeout_ms < 0) {
        // timeout_ms cannot be negative.
        return false;
    }
    return true;
//    return validate_first_topic_name(kafka_context);
}

//static __always_inline bool validate_first_topic_name(kafka_context_t *kafka_context) {
//    // Skipping number of entries for now
//    kafka_context->offset += 4;
//
//    if (kafka_context->offset > sizeof(kafka_context->buffer_size)) {
//        log_debug("kafka: Current offset is above the buffer size\n");
//        return false;
//    }
//
//    int16_t topic_name_size = 0;
//    if (!kafka_read_big_endian_int16(kafka_context, &topic_name_size)) {
//        return false;
//    }
//    log_debug("kafka: topic_name_size: %d\n", topic_name_size);
//    if (topic_name_size <= 0) {
//        return false;
//    }
//
//    if (topic_name_size > TOPIC_NAME_MAX_STRING_SIZE) {
//        return false;
//    }
//
//    ENSURE_LIMITS(kafka_context, topic_name_size, false);
//
//    // Using the barrier macro instructs the compiler to not keep memory values cached in registers across the assembler instruction
//    // If we don't use it here, the verifier will classify registers with false type and fail to load the program
////    barrier();
//    const char* topic_name_beginning_offset = kafka_context->buffer + kafka_context->offset;
//
////    // Make the verifier happy by checking that the topic name offset doesn't exceed the total fragment buffer size
////    if (topic_name_beginning_offset > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE ||
////            topic_name_beginning_offset + TOPIC_NAME_MAX_STRING_SIZE > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE) {
////        return false;
////    }
//
////    __builtin_memcpy(kafka_transaction->base.topic_name, topic_name_beginning_offset, TOPIC_NAME_MAX_STRING_SIZE);
//
//    // Making sure the topic name is a-z, A-Z, 0-9, dot, dash or underscore.
//#pragma unroll(TOPIC_NAME_MAX_STRING_SIZE)
//    for (int i = kafka_context->offset; i < TOPIC_NAME_MAX_STRING_SIZE; i++) {
//        char ch = topic_name_beginning_offset[i];
//        if (ch == 0) {
//            if (i < 3) {
//                 log_debug("kafka: warning: topic name is %s (shorter than 3 letters), this could be a false positive\n", topic_name_beginning_offset);
//            }
//            return i == topic_name_size;
//        }
//        if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {
//            continue;
//        }
//        return false;
//    }
//    return true;
//}

static __always_inline bool try_parse_fetch_request(kafka_context_t *kafka_context) {
    return false;
}

static __always_inline bool kafka_read_big_endian_int32(kafka_context_t *kafka_context, int32_t* result) {
    ENSURE_LIMITS(kafka_context, sizeof(int32_t), false);
    const char *beg = &kafka_context->buffer[kafka_context->offset];
    *result = read_big_endian_int32(beg);
    kafka_context->offset += sizeof(int32_t);
    return true;
}

static __always_inline bool kafka_read_big_endian_int16(kafka_context_t *kafka_context, int16_t* result) {
    ENSURE_LIMITS(kafka_context, sizeof(int16_t), false);
    const char *beg = &kafka_context->buffer[kafka_context->offset];
    *result = read_big_endian_int16(beg);
    kafka_context->offset += sizeof(int16_t);
    return true;
}

static __always_inline int32_t read_big_endian_int32(const char* buf) {
    int32_t *val = (int32_t*)buf;
    return bpf_ntohl(*val);
}

static __always_inline int16_t read_big_endian_int16(const char* buf) {
    int16_t *val = (int16_t*)buf;
    return bpf_ntohs(*val);
}

#endif
