#ifndef __KAFKA_PARSING_H
#define __KAFKA_PARSING_H

#include "types.h"

// Forward declaration
static __always_inline bool try_parse_produce_request(kafka_transaction_t *kafka_transaction);
static __always_inline bool try_parse_fetch_request(kafka_transaction_t *kafka_transaction);
static __always_inline bool extract_and_set_first_topic_name(kafka_transaction_t *kafka_transaction);
static __always_inline bool kafka_read_big_endian_int16(kafka_transaction_t *kafka_transaction, int16_t* result);
static __always_inline int16_t read_big_endian_int16(const char* buf);
static __always_inline bool kafka_read_big_endian_int32(kafka_transaction_t *kafka_transaction, int32_t* result);
static __always_inline int32_t read_big_endian_int32(const char* buf);

// Checking if the buffer represents kafka message
static __always_inline bool try_parse_request_header(kafka_transaction_t *kafka_transaction) {
    char *request_fragment = kafka_transaction->request_fragment;
    if (request_fragment == NULL) {
        return false;
    }

    int32_t message_size = 0;
    if (!kafka_read_big_endian_int32(kafka_transaction, &message_size)) {
        return false;
    }
    log_debug("kafka: message_size: %d\n", message_size);
    if (message_size <= 0) {
        return false;
    }

    int16_t request_api_key = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &request_api_key)) {
        return false;
    }
    log_debug("kafka: request_api_key: %d\n", request_api_key);
    if (request_api_key != KAFKA_FETCH && request_api_key != KAFKA_PRODUCE) {
        // We are only interested in fetch and produce requests
        return false;
    }
    kafka_transaction->base.request_api_key = request_api_key;

    int16_t request_api_version = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &request_api_version)) {
        return false;
    }

    log_debug("kafka: request_api_version: %d\n", request_api_version);
    switch (request_api_key) {
    case KAFKA_FETCH:
        if (request_api_version > KAFKA_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION) {
            // Fetch request version 12 and above is not supported.
            return false;
        }
        break;
    case KAFKA_PRODUCE:
        if (request_api_version == 0) {
            // We have seen some false positives when both request_api_version and request_api_key are 0,
            // so dropping support for this case
            return false;
        } else if (request_api_version > KAFKA_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION) {
            // Produce request version 9 and above is not supported.
            return false;
        }
        break;
    default:
        // We are only interested in fetch and produce requests
        return false;
    }
    kafka_transaction->base.request_api_version = request_api_version;

    int32_t correlation_id = 0;
    if (!kafka_read_big_endian_int32(kafka_transaction, &correlation_id)) {
        return false;
    }
    log_debug("kafka: correlation_id: %d\n", correlation_id);
    if (correlation_id < 0) {
        return false;
    }
    kafka_transaction->base.correlation_id = correlation_id;

    int16_t client_id_size = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &client_id_size)) {
        return false;
    }
    if (client_id_size < 0) {
        return false;
    }
    log_debug("kafka: client_id_size: %d\n", client_id_size);

    const char* client_id_starting_offset = kafka_transaction->request_fragment + kafka_transaction->base.current_offset_in_request_fragment;
    char ch = 0;
#pragma unroll(CLIENT_ID_SIZE_TO_VALIDATE)
    for (unsigned i = 0; i < CLIENT_ID_SIZE_TO_VALIDATE; i++) {
        if (i >= client_id_size) {
            break;
        }
        if (client_id_starting_offset > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE) {
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

        return false;
    }
    kafka_transaction->base.current_offset_in_request_fragment += client_id_size;

    return true;
}

static __always_inline bool try_parse_request(kafka_transaction_t *kafka_transaction) {
    char *request_fragment = (char*)kafka_transaction->request_fragment;
    if (request_fragment == NULL) {
        return false;
    }

//    log_debug("kafka: current_offset: %d\n", kafka_transaction->base.current_offset_in_request_fragment);
    if (kafka_transaction->base.current_offset_in_request_fragment > sizeof(kafka_transaction->request_fragment)) {
        return false;
    }

    switch (kafka_transaction->base.request_api_key) {
        case KAFKA_PRODUCE:
            return try_parse_produce_request(kafka_transaction);
            break;
        case KAFKA_FETCH:
            return try_parse_fetch_request(kafka_transaction);
            break;
        default:
            log_debug("kafka: got unsupported request_api_key: %d\n", kafka_transaction->base.request_api_key);
            return false;
    }
}

static __always_inline bool try_parse_produce_request(kafka_transaction_t *kafka_transaction) {
    log_debug("kafka: trying to parse produce request\n");
    if (kafka_transaction->base.request_api_version >= 9) {
        log_debug("kafka: Produce request version 9 and above is not supported: %d\n", kafka_transaction->base.request_api_version);
        return false;
    }

    if (kafka_transaction->base.request_api_version >= 3) {
        int16_t transactional_id_size = 0;
        if (!kafka_read_big_endian_int16(kafka_transaction, &transactional_id_size)) {
            return false;
        }
        log_debug("kafka: transactional_id_size: %d\n", transactional_id_size);
        if (transactional_id_size > 0) {
            kafka_transaction->base.current_offset_in_request_fragment += transactional_id_size;
        }
    }

    int16_t acs = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &acs)) {
        return false;
    }

    if (acs > 1 || acs < -1) {
        // The number of acknowledgments the producer requires the leader to have received before considering a request
        // complete. Allowed values: 0 for no acknowledgments, 1 for only the leader and -1 for the full ISR.
        return false;
    }

    int32_t timeout_ms = 0;
    if (!kafka_read_big_endian_int32(kafka_transaction, &timeout_ms)) {
        return false;
    }

    if (timeout_ms < 0) {
        // timeout_ms cannot be negative.
        return false;
    }

    return extract_and_set_first_topic_name(kafka_transaction);
}

static __always_inline bool try_parse_fetch_request(kafka_transaction_t *kafka_transaction) {
    log_debug("kafka: trying to parse fetch request\n");
    if (kafka_transaction->base.request_api_version >= 12) {
        log_debug("kafka: fetch request version 12 and above is not supported: %d\n", kafka_transaction->base.request_api_version);
        return false;
    }

    // Skipping all fields that we don't need to parse at the moment:

    // replica_id => INT32
    // max_wait_ms => INT32
    // min_bytes => INT32
    kafka_transaction->base.current_offset_in_request_fragment += 12;

    if (kafka_transaction->base.request_api_version >= 3) {
        // max_bytes => INT32
        kafka_transaction->base.current_offset_in_request_fragment += 4;

        if (kafka_transaction->base.request_api_version >= 4) {
            // isolation_level => INT8
            kafka_transaction->base.current_offset_in_request_fragment += 1;

            if (kafka_transaction->base.request_api_version >= 7) {
                // session_id => INT32
                // session_epoch => INT32
                kafka_transaction->base.current_offset_in_request_fragment += 8;
            }
        }
    }

    return extract_and_set_first_topic_name(kafka_transaction);
}

static __always_inline bool extract_and_set_first_topic_name(kafka_transaction_t *kafka_transaction) {
    // Skipping number of entries for now
    kafka_transaction->base.current_offset_in_request_fragment += 4;

    if (kafka_transaction->base.current_offset_in_request_fragment > sizeof(kafka_transaction->request_fragment)) {
        log_debug("kafka: Current offset is above the request fragment size\n");
        return false;
    }

    int16_t topic_name_size = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &topic_name_size)) {
        return false;
    }
    log_debug("kafka: topic_name_size: %d\n", topic_name_size);
    if (topic_name_size <= 0) {
        return false;
    }

    if (topic_name_size > TOPIC_NAME_MAX_STRING_SIZE) {
        return false;
    }

    // Using the barrier macro instructs the compiler to not keep memory values cached in registers across the assembler instruction
    // If we don't use it here, the verifier will classify registers with false type and fail to load the program
    barrier();
    char* topic_name_beginning_offset = kafka_transaction->request_fragment + kafka_transaction->base.current_offset_in_request_fragment;

    // Make the verifier happy by checking that the topic name offset doesn't exceed the total fragment buffer size
    if (topic_name_beginning_offset > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE ||
            topic_name_beginning_offset + TOPIC_NAME_MAX_STRING_SIZE > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE) {
        return false;
    }

    bpf_memcpy(kafka_transaction->base.topic_name, topic_name_beginning_offset, TOPIC_NAME_MAX_STRING_SIZE);

    // Making sure the topic name is a-z, A-Z, 0-9, dot, dash or underscore.
#pragma unroll(TOPIC_NAME_MAX_STRING_SIZE)
    for (int i = 0; i < TOPIC_NAME_MAX_STRING_SIZE; i++) {
        char ch = kafka_transaction->base.topic_name[i];
        if (ch == 0) {
            if (i < 3) {
                 log_debug("kafka: warning: topic name is %s (shorter than 3 letters), this could be a false positive\n", kafka_transaction->base.topic_name);
            }
            return i == topic_name_size;
        }
        if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {
            continue;
        }
        return false;
    }
    return true;
}

static __always_inline bool kafka_read_big_endian_int32(kafka_transaction_t *kafka_transaction, int32_t* result) {
    // Using the barrier macro instructs the compiler to not keep memory values cached in registers across the assembler instruction
    // If we don't use it here, the verifier will classify registers with false type and fail to load the program
    barrier();
    char* current_offset = kafka_transaction->request_fragment + kafka_transaction->base.current_offset_in_request_fragment;
    if (current_offset < kafka_transaction->request_fragment || current_offset > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE) {
        return false;
    }
    *result = read_big_endian_int32(current_offset);
    kafka_transaction->base.current_offset_in_request_fragment += 4;
    return true;
}

static __always_inline bool kafka_read_big_endian_int16(kafka_transaction_t *kafka_transaction, int16_t* result) {
    // Using the barrier macro instructs the compiler to not keep memory values cached in registers across the assembler instruction
    // If we don't use it here, the verifier will classify registers with false type and fail to load the program
    barrier();
    char* current_offset = kafka_transaction->request_fragment + kafka_transaction->base.current_offset_in_request_fragment;
    if (current_offset < kafka_transaction->request_fragment || current_offset > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE) {
        return false;
    }
    *result = read_big_endian_int16(current_offset);
    kafka_transaction->base.current_offset_in_request_fragment += 2;
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
