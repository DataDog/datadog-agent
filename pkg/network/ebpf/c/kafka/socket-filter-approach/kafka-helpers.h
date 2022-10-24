#ifndef __KAFKA_HELPERS_H
#define __KAFKA_HELPERS_H

#include "kafka-types.h"

static __inline int32_t read_big_endian_int32(const char* buf) {
  const int32_t length = *((int32_t*)buf);
  return bpf_ntohl(length);
}

static __inline int16_t read_big_endian_int16(const char* buf) {
//    log_debug("read_big_endian_int16: %d %d", buf[0], buf[1]);
    const int16_t length = *((int16_t*)buf);
//    log_debug("read_big_endian_int16 ---: %d", length);
//    log_debug("read_big_endian_int16 --2: %d", bpf_ntohs(length));
    return bpf_ntohs(length);
}

// Checking if the buffer represents kafka message
static __always_inline bool is_kafka(const char* buf, __u32 buf_size) {
    if (buf_size < KAFKA_MIN_SIZE) {
        log_debug("buffer size is less than KAFKA_MIN_SIZE");
        return false;
    }

    if (buf == NULL) {
        log_debug("buf == NULL");
        return false;
    }

    // Kafka size field is 4 bytes
//    const int32_t message_size = read_big_endian_int32(buf) + 4;
    const int32_t message_size = read_big_endian_int32(buf);
    //log_debug("message_size = %d", message_size);
    //log_debug("buf_size = %d", buf_size);

    // Enforcing count to be exactly message_size + 4 to mitigate mis-classification.
    // However, this will miss long messages broken into multiple reads.
//    if (message_size < 0 || buf_size != (__u32)message_size) {
    if (message_size <= 0) {
//        log_debug("message_size < 0 || buf_size != (__u32)message_size");
        //log_debug("message_size <= 0");
        return false;
    }

    const int16_t request_api_key = read_big_endian_int16(buf+4);
    log_debug("request_api_key: %d", request_api_key);
    if (request_api_key < 0 || request_api_key > KAFKA_MAX_VERSION) {
        log_debug("request_api_key < 0 || request_api_key > KAFKA_MAX_VERSION");
        return false;
    }

    const int16_t request_api_version = read_big_endian_int16(buf + 6);
    log_debug("request_api_version: %d", request_api_version);
    if (request_api_version < 0 || request_api_version > KAFKA_MAX_API) {
        log_debug("request_api_version < 0 || request_api_version > KAFKA_MAX_API");
        return false;
    }

    const int32_t correlation_id = read_big_endian_int32(buf + 8);
    log_debug("correlation_id: %d", correlation_id);
    if (correlation_id < 0) {
        log_debug("correlation_id < 0");
        return false;
    }

    const int16_t MINIMUM_API_VERSION_FOR_CLIENT_ID = 1;
    const uint32_t MAX_LENGTH_FOR_CLIENT_ID_STRING = 50;
    char client_id[MAX_LENGTH_FOR_CLIENT_ID_STRING] = {0};
    __builtin_memset(client_id, 0, sizeof(client_id));
    if (request_api_version >= MINIMUM_API_VERSION_FOR_CLIENT_ID) {
        const int16_t client_id_size = read_big_endian_int16(buf + 12);
        log_debug("client_id_size: %d", client_id_size);
        if (client_id_size <=0 || client_id_size > MAX_LENGTH_FOR_CLIENT_ID_STRING) {
            log_debug("client_id <=0 || client_id_size > MAX_LENGTH_FOR_CLIENT_ID_STRING");
        }
        else
        {
            const char* client_id_in_buf = buf + 14;
    //        log_debug("%c", client_id[0]);
    //        log_debug("%c", client_id[1]);
    //        log_debug("%c", client_id[2]);
    //        log_debug("%c", client_id[3]);
//            __builtin_memcpy(client_id, client_id_in_buf, client_id_size);
            bpf_probe_read_kernel_with_telemetry(client_id, client_id_size, (void*)client_id_in_buf);
//            for (int16_t i = 0; i < client_id_size; i++) {
                //client_id[i] = client_id_in_buf[i];
//            }
            log_debug("client_id: %s", client_id);
        }
    }

    return true;
}

#endif
