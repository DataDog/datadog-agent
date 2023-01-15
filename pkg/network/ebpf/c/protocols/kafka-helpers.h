#ifndef __KAFKA_HELPERS_H
#define __KAFKA_HELPERS_H

#include "kafka-defs.h"
#include "kafka-types.h"
#include "bpf_endian.h"

static __always_inline bool is_kafka(const char* buf, __u32 buf_size) {
    if (buf_size <= 0) {
        return false;
    }
    uint32_t offset = 0;

    int32_t message_size = bpf_ntohl(*(int32_t*)buf);
    if (message_size <= 0) {
        return false;
    }
    offset += sizeof(message_size);

    int16_t request_api_key = bpf_ntohs(*(int16_t*)(buf + offset));
    if (request_api_key != KAFKA_FETCH && request_api_key != KAFKA_PRODUCE) {
        // We are only interested in fetch and produce requests
        return false;
    }
    offset += sizeof(request_api_key);

    int16_t request_api_version = bpf_ntohs(*(int16_t*)(buf + offset));
    if (request_api_version < 0 || request_api_version > KAFKA_MAX_SUPPORTED_REQUEST_API_VERSION) {
        return false;
    }
    if ((request_api_version == 0) && (request_api_key == KAFKA_PRODUCE)) {
        // We have seen some false positives when both request_api_version and request_api_key are 0,
        // so dropping support for this case
        return false;
    }
    offset += sizeof(request_api_version);

    int32_t correlation_id = bpf_ntohl(*(int32_t*)(buf + offset));
    if (correlation_id < 0) {
        return false;
    }
    offset += sizeof(correlation_id);

    int16_t client_id_size = bpf_ntohs(*(int16_t*)(buf + offset));
    if (client_id_size < 0) {
         return false;
    }
    offset += sizeof(client_id_size);

    char ch = 0;
#pragma unroll(CLIENT_ID_SIZE_TO_VALIDATE)
    for (int i = 0; i < CLIENT_ID_SIZE_TO_VALIDATE; i++) {
        if (i == client_id_size - 1) {
            break;
        }
        ch = buf[i];
        if (ch == 0) {
            return false;
        }
        if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {
            continue;
        }
    }

    return true;
}

#endif
