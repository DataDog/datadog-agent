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

    return is_kafka_request_header(&kafka_context) || is_kafka_request(&kafka_context);
}

static __always_inline bool is_kafka_request_header(kafka_context_t* kafka_context) {
    kafka_header_t *kafka_header = &kafka_context->header;

    if (kafka_header->message_size < sizeof(kafka_header_t)) {
        return false;
    }

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

    if (kafka_header->api_version < 0 || kafka_header->api_version > KAFKA_MAX_SUPPORTED_REQUEST_API_VERSION) {
        return false;
    }

    if (kafka_header->correlation_id < 0) {
        return false;
    }

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

static  __always_inline bool is_kafka_request(kafka_context_t* kafka_context) {
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
    log_debug("kafka: trying to parse produce request");
    int16_t api_version = kafka_context->header.api_version;
    if (api_version >= 9) {
        log_debug("kafka: Produce request version 9 and above is not supported: %d", api_version);
        return false;
    }

    if (api_version >= 3) {
        int16_t transactional_id_size = 0;
        if (!kafka_read_big_endian_int16(kafka_context, &transactional_id_size)) {
            return false;
        }
        log_debug("kafka: transactional_id_size: %d", transactional_id_size);
        if (transactional_id_size > 0) {
            kafka_context->offset += transactional_id_size;
        }
    }

    int16_t acs = 0;
    if (!kafka_read_big_endian_int16(kafka_context, &acs)) {
        return false;
    }

    if (acs > 1 || acs < -1) {
        // The number of acknowledgments the producer requires the leader to have received before considering a request
        // complete. Allowed values: 0 for no acknowledgments, 1 for only the leader and -1 for the full ISR.
        return false;
    }

    int32_t timeout_ms = 0;
    if (!kafka_read_big_endian_int32(kafka_context, &timeout_ms)) {
        return false;
    }

    if (timeout_ms < 0) {
        // timeout_ms cannot be negative.
        return false;
    }

    return true;
    //return extract_and_set_first_topic_name(kafka_transaction);
}

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

static __always_inline int16_t read_big_endian_int16(const char* buf) {
    int16_t *val = (int16_t*)buf;
    return bpf_ntohs(*val);
}

static __always_inline int32_t read_big_endian_int32(const char* buf) {
    int32_t *val = (int32_t*)buf;
    return bpf_ntohl(*val);
}

static __always_inline bool kafka_read_big_endian_int16(kafka_context_t *kafka_context, int16_t* result) {
    ENSURE_LIMITS(kafka_context, sizeof(int16_t), false);
    const char *beg = &kafka_context->buffer[kafka_context->offset];
    *result = read_big_endian_int16(beg);
    kafka_context->offset += sizeof(int16_t);
    return true;
}

#endif
