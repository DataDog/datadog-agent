#ifndef __KAFKA_HELPERS_H
#define __KAFKA_HELPERS_H

#include "kafka-defs.h"
#include "kafka-types.h"
#include "bpf_endian.h"

typedef struct {
    uint32_t message_size;
    uint16_t api_key;
    uint16_t api_version;
    uint32_t correlation_id;
    uint16_t client_id_size;
} kafka_hdr;

static __always_inline bool is_kafka(const char* buf, __u32 buf_size) {
    if (buf_size <= 0) {
        return false;
    }

    kafka_hdr *hdr = (kafka_hdr *)buf;
    uint32_t message_size = bpf_ntohl(hdr->message_size);
    uint16_t api_key = bpf_ntohs(hdr->api_key);
    uint16_t api_version = bpf_ntohs(hdr->api_version);
    uint32_t correlation_id = bpf_ntohl(hdr->correlation_id);
    uint32_t client_id_size = bpf_ntohs(hdr->client_id_size);
    if (message_size <= 0) {
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

    const char* client_id_starting_offset = buf + sizeof(kafka_hdr);
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
        if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {
            continue;
        }
    }

    return true;
}

#endif
