#ifndef __KAFKA_CLASSIFICATION_H
#define __KAFKA_CLASSIFICATION_H

#include "protocols/helpers/big_endian.h"
#include "protocols/helpers/pktbuf.h"
#include "protocols/kafka/defs.h"
#include "protocols/kafka/maps.h"
#include "protocols/kafka/types.h"

#define STRINGIFY(a) #a

// A template for verifying a given buffer is composed of the characters [a-z], [A-Z], [0-9], ".", "_", or "-",
// or, optionally, allowing any printable characters.
// The iterations reads up to MIN(max_buffer_size, real_size).
// Has to be a template and not a function, as we have pragma unroll.
#define CHECK_STRING_COMPOSED_OF_ASCII(max_buffer_size, real_size, buffer, printable_ok)                                                \
    char ch = 0;                                                                                                                        \
_Pragma( STRINGIFY(unroll(max_buffer_size)) )                                                                                           \
    for (int j = 0; j < max_buffer_size; j++) {                                                                                         \
        /* Verifies we are not exceeding the real client_id_size, and if we do, we finish the iteration as we reached */                \
        /* to the end of the buffer and all checks have been successful. */                                                             \
        if (j + 1 > real_size) {                                                                                                        \
            break;                                                                                                                      \
        }                                                                                                                               \
        ch = buffer[j];                                                                                                                 \
        if (('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {  \
            continue;                                                                                                                   \
        }                                                                                                                               \
        /* The above check is actually redundant for the printable_ok case, but removing it leads */                                    \
        /* to some compiler optimizations which the verifier doesn't agree with. */                                                     \
        if (printable_ok && (ch >= ' ' && ch <= '~')) {                                                                                 \
            continue;                                                                                                                   \
        }                                                                                                                               \
        return false;                                                                                                                   \
    }                                                                                                                                   \
    return true;

#define CHECK_STRING_VALID_TOPIC_NAME(max_buffer_size, real_size, buffer)   \
    CHECK_STRING_COMPOSED_OF_ASCII(max_buffer_size, real_size, buffer, false)

// The client ID actually allows any UTF-8 chars but we restrict it to printable ASCII characters
// for now to avoid false positives.
#define CHECK_STRING_VALID_CLIENT_ID(max_buffer_size, real_size, buffer)   \
    CHECK_STRING_COMPOSED_OF_ASCII(max_buffer_size, real_size, buffer, true)


// Reads the client id (up to CLIENT_ID_SIZE_TO_VALIDATE bytes from the given offset), and verifies if it is valid,
// namely, composed only from characters from [a-zA-Z0-9._-].
static __always_inline bool is_valid_client_id(pktbuf_t pkt, u32 offset, u16 real_client_id_size) {
    const u32 key = 0;
    // Fetch the client id buffer from per-cpu array, which gives us the ability to extend the size of the buffer,
    // as the stack is limited with the number of bytes we can allocate on.
    char *client_id = bpf_map_lookup_elem(&kafka_client_id, &key);
    if (client_id == NULL) {
        return false;
    }
    bpf_memset(client_id, 0, CLIENT_ID_SIZE_TO_VALIDATE);
    pktbuf_load_bytes_with_telemetry(pkt, offset, (char *)client_id, CLIENT_ID_SIZE_TO_VALIDATE);

    // Returns true if client_id is composed out of the characters [a-z], [A-Z], [0-9], ".", "_", or "-".
    CHECK_STRING_VALID_CLIENT_ID(CLIENT_ID_SIZE_TO_VALIDATE, real_client_id_size, client_id);
}

// Checks the given kafka header represents a valid one.
// 1. The message size should include the size of the header.
// 2. The api key is FETCH or PRODUCE.
// 3. The api version is not negative.
// 4. The version of a PRODUCE message is not 0 or bigger than 8.
// 5. The version of a FETCH message is not bigger than 11.
// 6. Correlation ID is not negative.
// 7. The client ID size if not negative.
static __always_inline bool is_valid_kafka_request_header(const kafka_header_t *kafka_header) {
    if (kafka_header->message_size < sizeof(kafka_header_t) || kafka_header->message_size  < 0) {
        return false;
    }

    if (kafka_header->api_version < 0) {
        return false;
    }

    switch (kafka_header->api_key) {
    case KAFKA_FETCH:
        if (kafka_header->api_version > KAFKA_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION) {
            // Fetch request version 12 and above is not supported.
            return false;
        }
        break;
    case KAFKA_PRODUCE:
        if (kafka_header->api_version == 0) {
            // We have seen some false positives when both request_api_version and request_api_key are 0,
            // so dropping support for this case
            return false;
        } else if (kafka_header->api_version > KAFKA_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION) {
            // Produce request version 9 and above is not supported.
            return false;
        }
        break;
    default:
        // We are only interested in fetch and produce requests
        return false;
    }

    if (kafka_header->correlation_id < 0) {
        return false;
    }

    return kafka_header->client_id_size >= -1;
}

PKTBUF_READ_INTO_BUFFER(topic_name, TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, BLK_SIZE)

// Reads the first topic name (can be multiple), up to TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE bytes from the given offset, and
// verifies if it is valid, namely, composed only from characters from [a-zA-Z0-9._-].
static __always_inline bool validate_first_topic_name(pktbuf_t pkt, u32 offset) {
    // Skipping number of entries for now
    offset += sizeof(s32);

    PKTBUF_READ_BIG_ENDIAN_WRAPPER(s16, topic_name_size, pkt, offset);
    if (topic_name_size <= 0 || topic_name_size > TOPIC_NAME_MAX_ALLOWED_SIZE) {
        return false;
    }

    const u32 key = 0;
    char *topic_name = bpf_map_lookup_elem(&kafka_topic_name, &key);
    if (topic_name == NULL) {
        return false;
    }
    bpf_memset(topic_name, 0, TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE);

    pktbuf_read_into_buffer_topic_name((char *)topic_name, pkt, offset);
    offset += topic_name_size;

    CHECK_STRING_VALID_TOPIC_NAME(TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, topic_name_size, topic_name);
}

// Getting the offset (out parameter) of the first topic name in the produce request.
static __always_inline bool get_topic_offset_from_produce_request(const kafka_header_t *kafka_header, pktbuf_t pkt, u32 *out_offset) {
    const s16 api_version = kafka_header->api_version;
    u32 offset = *out_offset;
    if (api_version >= 3) {
        PKTBUF_READ_BIG_ENDIAN_WRAPPER(s16, transactional_id_size, pkt, offset);
        if (transactional_id_size > 0) {
            offset += transactional_id_size;
        }
    }

    PKTBUF_READ_BIG_ENDIAN_WRAPPER(s16, acks, pkt, offset);
    if (acks > 1 || acks < -1) {
        // The number of acknowledgments the producer requires the leader to have received before considering a request
        // complete. Allowed values: 0 for no acknowledgments, 1 for only the leader and -1 for the full ISR.
        return false;
    }

    PKTBUF_READ_BIG_ENDIAN_WRAPPER(s32, timeout_ms, pkt, offset);
    if (timeout_ms < 0) {
        // timeout_ms cannot be negative.
        return false;
    }

    *out_offset = offset;
    return true;
}

// Getting the offset the topic name in the fetch request.
static __always_inline u32 get_topic_offset_from_fetch_request(const kafka_header_t *kafka_header) {
    // replica_id => INT32
    // max_wait_ms => INT32
    // min_bytes => INT32
    u32 offset = 3 * sizeof(s32);

    if (kafka_header->api_version >= 3) {
        // max_bytes => INT32
        offset += sizeof(s32);
        if (kafka_header->api_version >= 4) {
            // isolation_level => INT8
            offset += sizeof(s8);
            if (kafka_header->api_version >= 7) {
                // session_id => INT32
                // session_epoch => INT32
                offset += 2 * sizeof(s32);
            }
        }
    }

    return offset;
}

// Calls the relevant function, according to the api_key.
static __always_inline bool is_kafka_request(const kafka_header_t *kafka_header, pktbuf_t pkt, u32 offset) {
    // Due to old-verifiers limitations, if the request is fetch or produce, we are calculating the offset of the topic
    // name in the request, and then validate the topic. We have to have shared call for validate_first_topic_name
    // as the function is huge, rather than call validate_first_topic_name for each api_key.
    switch (kafka_header->api_key) {
    case KAFKA_PRODUCE:
        if (!get_topic_offset_from_produce_request(kafka_header, pkt, &offset)) {
            return false;
        }
        break;
    case KAFKA_FETCH:
        offset += get_topic_offset_from_fetch_request(kafka_header);
        break;
    default:
        return false;
    }
    return validate_first_topic_name(pkt, offset);
}

// Checks if the packet represents a kafka request.
static __always_inline bool __is_kafka(pktbuf_t pkt, const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, KAFKA_MIN_LENGTH);

    const kafka_header_t *header_view = (kafka_header_t *)buf;
    kafka_header_t kafka_header;
    bpf_memset(&kafka_header, 0, sizeof(kafka_header));
    kafka_header.message_size = bpf_ntohl(header_view->message_size);
    kafka_header.api_key = bpf_ntohs(header_view->api_key);
    kafka_header.api_version = bpf_ntohs(header_view->api_version);
    kafka_header.correlation_id = bpf_ntohl(header_view->correlation_id);
    kafka_header.client_id_size = bpf_ntohs(header_view->client_id_size);

    if (!is_valid_kafka_request_header(&kafka_header)) {
        return false;
    }

    u32 offset = pktbuf_data_offset(pkt) + sizeof(kafka_header_t);
    // Validate client ID
    // Client ID size can be equal to '-1' if the client id is null.
    if (kafka_header.client_id_size > 0) {
        if (!is_valid_client_id(pkt, offset, kafka_header.client_id_size)) {
            return false;
        }
        offset += kafka_header.client_id_size;
    } else if (kafka_header.client_id_size < -1) {
        return false;
    }

    return is_kafka_request(&kafka_header, pkt, offset);
}

static __always_inline bool is_kafka(struct __sk_buff *skb, skb_info_t *skb_info, const char* buf, __u32 buf_size)
{
    return __is_kafka(pktbuf_from_skb(skb, skb_info), buf, buf_size);
}

static __always_inline __maybe_unused bool tls_is_kafka(tls_dispatcher_arguments_t *tls, const char* buf, __u32 buf_size)
{
    return __is_kafka(pktbuf_from_tls(tls), buf, buf_size);
}

#endif
