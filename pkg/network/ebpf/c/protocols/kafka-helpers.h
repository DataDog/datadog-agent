#ifndef __KAFKA_HELPERS_H
#define __KAFKA_HELPERS_H

#include "kafka-defs.h"
#include "kafka-types.h"
#include "bpf_endian.h"

static bool is_kafka_header(const char* buf, __u32 buf_size);

static __always_inline bool is_kafka(const char* buf, __u32 buf_size) {
    if (buf_size <= 0) {
        return false;
    }

    return is_kafka_header(buf, buf_size);
}

static __always_inline bool is_kafka_header(const char* buf, __u32 buf_size) {
    if (buf_size < sizeof(kafka_header)) {
        return false;
    }

    kafka_header *header = (kafka_header *)buf;
    int32_t message_size = bpf_ntohl(header->message_size);
    int16_t api_key = bpf_ntohs(header->api_key);
    int16_t api_version = bpf_ntohs(header->api_version);
    int32_t correlation_id = bpf_ntohl(header->correlation_id);
    int32_t client_id_size = bpf_ntohs(header->client_id_size);

    if (message_size < sizeof(kafka_header)) {
        return false;
    }

    if (api_key != KAFKA_FETCH && api_key != KAFKA_PRODUCE) {
        // We are only interested in fetch and produce requests
        return false;
    }

    if (api_version < 0 || api_version > KAFKA_MAX_SUPPORTED_REQUEST_API_VERSION) {
        return false;
    }
    if ((api_version == 0) && (api_key == KAFKA_PRODUCE)) {
        // We have seen some false positives when both request_api_version and request_api_key are 0,
        // so dropping support for this case
        return false;
    }

    if (correlation_id < 0) {
        return false;
    }

    if (client_id_size < 0) {
         return false;
    }

    log_debug("kafka: client_id_size: %d", client_id_size);

    const char* client_id_starting_offset = buf + sizeof(kafka_header);
    char ch = 0;
#pragma unroll(CLIENT_ID_SIZE_TO_VALIDATE)
    for (unsigned i = 0; i < CLIENT_ID_SIZE_TO_VALIDATE; i++) {
        if (i >= client_id_size) {
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

    return true;
}
#endif
