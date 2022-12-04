#ifndef __KAFKA_HELPERS_H
#define __KAFKA_HELPERS_H

#include "kafka-types.h"

// Forward declaration
static __always_inline bool try_parse_produce_request(kafka_transaction_t *kafka_transaction);
static __always_inline bool try_parse_fetch_request(kafka_transaction_t *kafka_transaction);
static __always_inline bool extract_and_set_first_topic_name(kafka_transaction_t *kafka_transaction);

static __inline int32_t read_big_endian_int32(const char* buf) {
    int32_t *val = (int32_t*)buf;
    return bpf_ntohl(*val);
}

static __inline int16_t read_big_endian_int16(const char* buf) {
    int16_t *val = (int16_t*)buf;
    return bpf_ntohs(*val);
}

static __inline bool kafka_read_big_endian_int32(kafka_transaction_t *kafka_transaction, int32_t* result) {
    if (kafka_transaction->current_offset_in_request_fragment > sizeof(kafka_transaction->request_fragment)) {
        return false;
    }
    *result = read_big_endian_int32(kafka_transaction->request_fragment + kafka_transaction->current_offset_in_request_fragment);
    kafka_transaction->current_offset_in_request_fragment += 4;
    return true;
}

static __inline bool kafka_read_big_endian_int16(kafka_transaction_t *kafka_transaction, int16_t* result) {
    if (kafka_transaction->current_offset_in_request_fragment > sizeof(kafka_transaction->request_fragment)) {
        return false;
    }
    *result = read_big_endian_int16(kafka_transaction->request_fragment + kafka_transaction->current_offset_in_request_fragment);
    kafka_transaction->current_offset_in_request_fragment += 2;
    return true;
}

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
    log_debug("kafka: message_size: %d", message_size);
    if (message_size <= 0) {
        return false;
    }

    int16_t request_api_key = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &request_api_key)) {
        return false;
    }
    log_debug("kafka: request_api_key: %d", request_api_key);
    if (request_api_key < 0 || request_api_key > KAFKA_MAX_VERSION) {
        return false;
    }
    kafka_transaction->request_api_key = request_api_key;

    int16_t request_api_version = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &request_api_version)) {
        return false;
    }
    log_debug("kafka: request_api_version: %d", request_api_version);
    if (request_api_version < 0 || request_api_version > KAFKA_MAX_API) {
        return false;
    }
    kafka_transaction->request_api_version = request_api_version;

    int32_t correlation_id = 0;
    if (!kafka_read_big_endian_int32(kafka_transaction, &correlation_id)) {
        return false;
    }
    log_debug("kafka: correlation_id: %d", correlation_id);
    if (correlation_id < 0) {
        return false;
    }
    kafka_transaction->correlation_id = correlation_id;

    const int16_t MINIMUM_API_VERSION_FOR_CLIENT_ID = 1;
    //__builtin_memset(kafka_transaction->client_id, 0, sizeof(kafka_transaction->client_id));
//    uint16_t client_id_size_final = 0;
    if (request_api_version >= MINIMUM_API_VERSION_FOR_CLIENT_ID) {
        int16_t client_id_size = 0;
        if (!kafka_read_big_endian_int16(kafka_transaction, &client_id_size)) {
            return false;
        }
        kafka_transaction->current_offset_in_request_fragment += client_id_size;
        log_debug("kafka: client_id_size: %d", client_id_size);

//        // The following code is to avoid verifier problems
//        uint32_t max_size_of_client_id_string = sizeof(kafka_transaction->client_id);
//        client_id_size_final = client_id_size < max_size_of_client_id_string ? client_id_size : max_size_of_client_id_string;
//
//        // A nullable string length might be -1 to signify null, it is supported here
//        if (client_id_size <= 0 || client_id_size > max_size_of_client_id_string) {
//            log_debug("kafka: client_id <= 0 || client_id_size > MAX_LENGTH_FOR_CLIENT_ID_STRING");
//        } else {
////            const char* client_id_in_buf = request_fragment + kafka_transaction->current_offset_in_request_fragment;
////            bpf_probe_read_kernel(kafka_transaction->client_id, client_id_size_final, (void*)client_id_in_buf);
////            log_debug("client_id: %s", kafka_transaction->client_id);
//        }
    }
    return true;
}

static __always_inline bool try_parse_request(kafka_transaction_t *kafka_transaction) {
    char *request_fragment = (char*)kafka_transaction->request_fragment;
    if (request_fragment == NULL) {
        return false;
    }

    log_debug("kafka: current_offset: %d", kafka_transaction->current_offset_in_request_fragment);
    if (kafka_transaction->current_offset_in_request_fragment > sizeof(kafka_transaction->request_fragment)) {
        return false;
    }

    switch (kafka_transaction->request_api_key) {
        case KAFKA_PRODUCE:
            return try_parse_produce_request(kafka_transaction);
            break;
        case KAFKA_FETCH:
            return try_parse_fetch_request(kafka_transaction);
            break;
        default:
            log_debug("kafka: got unsupported request_api_key: %d", kafka_transaction->request_api_key);
            return false;
    }
}

static __always_inline bool try_parse_produce_request(kafka_transaction_t *kafka_transaction) {
    log_debug("kafka: trying to parse produce request");
    if (kafka_transaction->request_api_version < 3 || kafka_transaction->request_api_version > 8) {
        return false;
    }

    int16_t transactional_id_size = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &transactional_id_size)) {
        return false;
    }
    log_debug("kafka: transactional_id_size: %d", transactional_id_size);
    if (transactional_id_size > 0) {
        kafka_transaction->current_offset_in_request_fragment += transactional_id_size;
    }

    // Skipping the acks field as we have no interest in it
    kafka_transaction->current_offset_in_request_fragment += 2;

    // Skipping the timeout_ms field as we have no interest in it
    kafka_transaction->current_offset_in_request_fragment += 4;

    // Skipping number of entries for now
    kafka_transaction->current_offset_in_request_fragment += 4;

    if (kafka_transaction->current_offset_in_request_fragment > sizeof(kafka_transaction->request_fragment)) {
        log_debug("kafka: Current offset is above the request fragment size");
        return false;
    }

    return extract_and_set_first_topic_name(kafka_transaction);
}

static __always_inline bool try_parse_fetch_request(kafka_transaction_t *kafka_transaction) {
    log_debug("kafka: Trying to parse fetch request");
    if (kafka_transaction->request_api_version != 4 && kafka_transaction->request_api_version < 7) {
        log_debug("kafka: request_api_version != 4 and < 7 not supported: %d", kafka_transaction->request_api_version);
        return false;
    }

    // Skipping all fields that we don't need to parse at the moment:
    //  replica_id => INT32
    //  max_wait_ms => INT32
    //  min_bytes => INT32
    //  max_bytes => INT32
    //  isolation_level => INT8
    //  number_of_topics => INT32
    kafka_transaction->current_offset_in_request_fragment += 21;

    if (kafka_transaction->request_api_version >= 7)
    {
        // On api version 7+, need to skip:
        //  session_id => INT32
        //  session_epoch => INT32
        kafka_transaction->current_offset_in_request_fragment += 8;
    }

    return extract_and_set_first_topic_name(kafka_transaction);
}

static __always_inline bool extract_and_set_first_topic_name(kafka_transaction_t *kafka_transaction) {
    int16_t topic_name_size = 0;
    if (!kafka_read_big_endian_int16(kafka_transaction, &topic_name_size)) {
        return false;
    }
    log_debug("kafka: topic_name_size: %d", topic_name_size);
    if (topic_name_size <= 0) {
        return false;
    }

    if (topic_name_size > TOPIC_NAME_MAX_STRING_SIZE) {
        return false;
    }

    char* topic_name_beginning_offset = kafka_transaction->request_fragment + kafka_transaction->current_offset_in_request_fragment;

    // Make the verifier happy by checking that the topic name offset doesn't exceed the total fragment buffer size
    if (topic_name_beginning_offset > kafka_transaction->request_fragment + KAFKA_BUFFER_SIZE) {
        return false;
    }

#pragma unroll(TOPIC_NAME_MAX_STRING_SIZE)
    for (int current_offset = 0; current_offset < TOPIC_NAME_MAX_STRING_SIZE; current_offset++) {
        char *source_address = topic_name_beginning_offset + current_offset;
        char *destination_address = kafka_transaction->topic_name + current_offset;

        if (current_offset >= topic_name_size) {
            break;
        }
        *destination_address = *source_address;
    }
    return true;
}

#endif
