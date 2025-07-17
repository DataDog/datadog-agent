#ifndef __KAFKA_CLASSIFICATION_H
#define __KAFKA_CLASSIFICATION_H

#include "protocols/helpers/big_endian.h"
#include "protocols/helpers/pktbuf.h"
#include "protocols/kafka/defs.h"
#include "protocols/kafka/maps.h"
#include "protocols/kafka/types.h"

#define STRINGIFY(a) #a

// The UUID must be v4 and its variant (17th digit) may be 0x8, 0x9, 0xA or 0xB
#define IS_UUID_V4(topic_id) \
    (topic_id[6] >> 4) == 4 && \
    0x8 <= (topic_id[8] >> 4) && (topic_id[8] >> 4) <= 0xB

// A template for verifying a given buffer is composed of the characters [a-z], [A-Z], [0-9], ".", "_", or "-",
// or, optionally, allowing any printable characters.
// The iterations reads up to MIN(max_buffer_size, real_size).
// Has to be a template and not a function, as we have pragma unroll.
#define CHECK_STRING_COMPOSED_OF_ASCII(max_buffer_size, real_size, buffer, printable_ok)                                                \
    char ch = 0;                                                                                                                        \
_Pragma( STRINGIFY(unroll(max_buffer_size)) )                                                                                           \
    for (unsigned int j = 0; j < max_buffer_size; j++) {                                                                                         \
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

// Client string (client id/software name/software version) allows any UTF-8 chars but we restrict it to printable ASCII characters
// for now to avoid false positives.
#define CHECK_STRING_VALID_CLIENT_STRING(max_buffer_size, real_size, buffer)   \
    CHECK_STRING_COMPOSED_OF_ASCII(max_buffer_size, real_size, buffer, true)

// Buffers
PKTBUF_READ_INTO_BUFFER_WITHOUT_TELEMETRY(topic_name, TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE, BLK_SIZE)
PKTBUF_READ_INTO_BUFFER_WITHOUT_TELEMETRY(client_id, CLIENT_ID_SIZE_TO_VALIDATE, BLK_SIZE)
PKTBUF_READ_INTO_BUFFER_WITHOUT_TELEMETRY(client_string, CLIENT_STRING_SIZE_TO_VALIDATE, BLK_SIZE)

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
    pktbuf_read_into_buffer_client_id(client_id, pkt, offset);

    // Returns true if client_id is composed out of the characters [a-z], [A-Z], [0-9], ".", "_", or "-".
    CHECK_STRING_VALID_CLIENT_ID(CLIENT_ID_SIZE_TO_VALIDATE, real_client_id_size, client_id);
}

// Used to validate client software name and software version.
// The same buffer is used for all of them to cut down on instruction count in the verifier.
// valid means composed only from characters from [a-zA-Z0-9._-].
static __always_inline bool is_valid_client_string(pktbuf_t pkt, u32 offset, u16 real_string_size, char *client_string) {
    if (real_string_size == 0) {
        return true;
    }

    bpf_memset(client_string, 0, CLIENT_STRING_SIZE_TO_VALIDATE);
    pktbuf_read_into_buffer_client_string(client_string, pkt, offset);

    // Returns whether composed of the characters [a-z], [A-Z], [0-9], ".", "_", or "-".
    CHECK_STRING_VALID_CLIENT_STRING(CLIENT_STRING_SIZE_TO_VALIDATE, real_string_size, client_string);
}

// Checks the given kafka header represents a valid one.
// * The message size should include the size of the header.
// * The api version is not negative.
// * Correlation ID is not negative.
// * The client ID size if not negative.
static __always_inline bool is_valid_kafka_request_header(const kafka_header_t *kafka_header) {
    if (kafka_header->message_size < sizeof(kafka_header_t) || kafka_header->message_size  < 0) {
        return false;
    }

    if (kafka_header->api_version < 0) {
        return false;
    }

    if (kafka_header->correlation_id < 0) {
        return false;
    }

    return kafka_header->client_id_size >= -1;
}

// Checks the given kafka api key (= operation) and api version is supported and wanted by us.
static __always_inline bool is_supported_api_version_for_classification(s16 api_key, s16 api_version) {
    switch (api_key) {
    case KAFKA_FETCH:
        if (api_version < KAFKA_CLASSIFICATION_MIN_SUPPORTED_FETCH_REQUEST_API_VERSION) {
            return false;
        }
        if (api_version > KAFKA_CLASSIFICATION_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION) {
            return false;
        }
        break;
    case KAFKA_PRODUCE:
        if (api_version < KAFKA_CLASSIFICATION_MIN_SUPPORTED_PRODUCE_REQUEST_API_VERSION) {
            // We have seen some false positives when both request_api_version and request_api_key are 0,
            // so dropping support for this case
            return false;
        } else if (api_version > KAFKA_CLASSIFICATION_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION) {
            return false;
        }
        break;
    default:
        // We are only interested in fetch and produce requests
        return false;
    }

    // if we didn't hit any of the above checks, we are good to go.
    return true;
}
//static __always_inline bool is_supported_api_version_for_classification(s16 api_key, s16 api_version) {
//    return (api_key == KAFKA_FETCH &&
//         api_version >= KAFKA_CLASSIFICATION_MIN_SUPPORTED_FETCH_REQUEST_API_VERSION &&
//         api_version <= KAFKA_CLASSIFICATION_MAX_SUPPORTED_FETCH_REQUEST_API_VERSION) ||
//        (api_key == KAFKA_PRODUCE &&
//         api_version >= KAFKA_CLASSIFICATION_MIN_SUPPORTED_PRODUCE_REQUEST_API_VERSION &&
//         api_version <= KAFKA_CLASSIFICATION_MAX_SUPPORTED_PRODUCE_REQUEST_API_VERSION) ||
//        (api_key == KAFKA_API_VERSIONS &&
//         api_version >= KAFKA_CLASSIFICATION_MIN_SUPPORTED_API_VERSIONS_REQUEST_API_VERSION &&
//         api_version <= KAFKA_CLASSIFICATION_MAX_SUPPORTED_API_VERSIONS_REQUEST_API_VERSION);
//}

static __always_inline bool isMSBSet(uint8_t byte) {
    return (byte & 0x80) != 0;
}

// Parses a varint of maximum size two bytes. The maximum size is (0x7f << 7) |
// 0x7f == 16383 bytes. This is more than enough for the topic name size which
// is a maximum of 255 bytes.
static __always_inline int parse_varint_u16(u16 *out, u16 in, u32 *bytes)
{
    *bytes = 1;

    u8 first = in & 0xff;
    u8 second = in >> 8;
    u16 tmp = 0;

    tmp |= first & 0x7f;
    if (isMSBSet(first)) {
        *bytes += 1;
        tmp |= ((u16)(second & 0x7f)) << 7;

        if (isMSBSet(second)) {
            // varint larger than two bytes.
            return false;
        }
    }

    // When lengths are stored as varints in the protocol, they are always
    // stored as N + 1.
    *out = tmp - 1;
    return true;
}

static __always_inline bool skip_varint_number_of_topics(pktbuf_t pkt, u32 *offset) {
    u8 bytes[2] = {};

    // Should be safe to assume that there is always more than one byte present,
    // since there will be the topic name etc after the number of topics.
    if (*offset + sizeof(bytes) > pktbuf_data_end(pkt)) {
        return false;
    }

    pktbuf_load_bytes(pkt, *offset, bytes, sizeof(bytes));

    *offset += 1;
    if (isMSBSet(bytes[0])) {
        *offset += 1;

        if (isMSBSet(bytes[1])) {
            // More than 16383 topics?
            return false;
        }
    }

    return true;
}

// Skips a varint of up to `max_bytes` (4).  The `skip_varint_number_of_topics`
// above could potentially be merged with this, although that leads to a small
// growth in the number of instructions processed.
//
// Note there is an assumption that there are at least `max_bytes` available in
// the packet (even if the varint actually occupies a lesser amount of space).
//
// A return value of false indicates an error in reading the varint.
static __always_inline __maybe_unused bool skip_varint(pktbuf_t pkt, u32 *offset, u32 max_bytes) {
    u8 bytes[4] = {};

    if (max_bytes == 0 || max_bytes > sizeof(bytes)) {
        return false;
    }

    if (*offset + max_bytes > pktbuf_data_end(pkt)) {
        return false;
    }

    pktbuf_load_bytes(pkt, *offset, bytes, max_bytes);

    #pragma unroll
    for (u32 i = 0; i < max_bytes; i++) {
        // Note that doing *offset += i + 1 before the return true instead of
        // this leads to a compiler error due to the optimizer not being to
        // unroll the loop.
        *offset += 1;
        if (!isMSBSet(bytes[i])) {
            return true;
        }
    }

    // MSB set on last byte, so our max_bytes wasn't enough.
    return false;
}

// `flexible` indicates that the API version is a flexible version as described in
// https://cwiki.apache.org/confluence/display/KAFKA/KIP-482%3A+The+Kafka+Protocol+should+Support+Optional+Tagged+Fields
static __always_inline s16 read_nullable_string_size(pktbuf_t pkt, bool flexible, u32 *offset) {
    u16 topic_name_size_raw = 0;
    // We assume we can always read two bytes. Even if the varint for the topic
    // name size is just one byte, the topic name itself will at least occupy
    // one more byte so reading two bytes is safe (we advance the offset based
    // on the number of actual bytes in the varint).
    if (*offset + sizeof(topic_name_size_raw) > pktbuf_data_end(pkt)) {
        return 0;
    }

    pktbuf_load_bytes_with_telemetry(pkt, *offset, &topic_name_size_raw, sizeof(topic_name_size_raw));

    s16 topic_name_size = 0;
    if (flexible) {
        u16 topic_name_size_tmp = 0;
        u32 varint_bytes = 0;

        if (!parse_varint_u16(&topic_name_size_tmp, topic_name_size_raw, &varint_bytes)) {
            return 0;
        }

        topic_name_size = topic_name_size_tmp;
        *offset += varint_bytes;
    } else {
        topic_name_size = bpf_ntohs(topic_name_size_raw);
        *offset += sizeof(topic_name_size_raw);
    }

    return topic_name_size;
}

// Reads the first topic name (can be multiple), up to TOPIC_NAME_MAX_STRING_SIZE_TO_VALIDATE bytes from the given offset, and
// verifies if it is valid, namely, composed only from characters from [a-zA-Z0-9._-].
static __always_inline bool validate_first_topic_name(pktbuf_t pkt, bool flexible, u32 offset) {
    // Skipping number of entries for now
    if (flexible) {
        if (!skip_varint_number_of_topics(pkt, &offset)) {
            return false;
        }
    } else {
        offset += sizeof(s32);
    }

    s16 topic_name_size = read_nullable_string_size(pkt, flexible, &offset);
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

// Reads the first topic id (can be multiple) from the given offset,
// verifies if it is a valid UUID version 4
// This function is used for v13+ and so it assumes flexible is true
static __always_inline bool validate_first_topic_id(pktbuf_t pkt, u32 offset) {
    // The topic id is a UUID, which is 16 bytes long.
    // It is in network byte order (big-endian)
    u8 topic_id[16] = {};

    // Skipping number of entries for now
    if (!skip_varint_number_of_topics(pkt, &offset)) {
        return false;
    }

    if (offset + sizeof(topic_id) > pktbuf_data_end(pkt)) {
        return false;
    }

    pktbuf_load_bytes_with_telemetry(pkt, offset, topic_id, sizeof(topic_id));
    offset += sizeof(topic_id);

    return IS_UUID_V4(topic_id);
}

// Flexible API version can have an arbitrary number of tagged fields.  We don't
// need to handle these but we do need to skip them to get at the normal fields
// which we are interested in.  However, we would need to parse the list of fields
// to find out how much we need to skip over.  For now, we assume (and assert)
// that there are no tagged fields.
static __always_inline bool skip_request_tagged_fields(pktbuf_t pkt, u32 *offset) {
    if (*offset >= pktbuf_data_end(pkt)) {
        return false;
    }

    u8 num_tagged_fields = 0;

    pktbuf_load_bytes(pkt, *offset, &num_tagged_fields, sizeof(num_tagged_fields));
    *offset += sizeof(num_tagged_fields);

    // We don't support parsing tagged fields for now.
    return num_tagged_fields == 0;
}

// Getting the offset (out parameter) of the first topic name in the produce request.
static __always_inline bool get_topic_offset_from_produce_request(const kafka_header_t *kafka_header, pktbuf_t pkt, u32 *out_offset, s16 *out_acks) {
    const s16 api_version = kafka_header->api_version;
    u32 offset = *out_offset;
    bool flexible = api_version >= 9;

    if (flexible && !skip_request_tagged_fields(pkt, &offset)) {
        return false;
    }

    if (api_version >= 3) {
        // The transactional ID for flex versions can, in theory, have a size
        // larger than that which can be represented in a two-byte varint, but
        // that seems unlikely, so just reuse the same nullable string read that
        // we use for the topic names.
        s16 transactional_id_size = read_nullable_string_size(pkt, flexible, &offset);

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
    if (out_acks != NULL) {
        *out_acks = acks;
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
static __always_inline bool get_topic_offset_from_fetch_request(const kafka_header_t *kafka_header, pktbuf_t pkt, u32 *offset) {
    u32 api_version = kafka_header->api_version;

    if (api_version >= 12) {
        if (!skip_request_tagged_fields(pkt, offset)) {
            return false;
        }
    }

    // replica_id => INT32 (doesn't exist in v15+)
    // max_wait_ms => INT32
    // min_bytes => INT32
    if (kafka_header->api_version >= 15) {
        *offset += 2 * sizeof(s32);
    } else {
        *offset += 3 * sizeof(s32);
    }

    if (api_version >= 3) {
        // max_bytes => INT32
        *offset += sizeof(s32);
        if (api_version >= 4) {
            // isolation_level => INT8
            *offset += sizeof(s8);
            if (api_version >= 7) {
                // session_id => INT32
                // session_epoch => INT32
                *offset += 2 * sizeof(s32);
            }
        }
    }

    return true;
}

// Checks if the packet represents a kafka fetch or produce request.
static __always_inline bool is_kafka_fetch_or_produce_request(const kafka_header_t *kafka_header, pktbuf_t pkt, u32 offset) {
    // Due to old-verifiers limitations, if the request is fetch or produce, we are calculating the offset of the topic
    // name in the request, and then validate the topic. We have to have shared call for validate_first_topic_name
    // as the function is huge, rather than call validate_first_topic_name for each api_key.
    bool flexible = false;
    bool topic_id_instead_of_name = false;
    if (kafka_header->api_key == KAFKA_PRODUCE) {
        if (!get_topic_offset_from_produce_request(kafka_header, pkt, &offset, NULL)) {
            return false;
        }
        flexible = kafka_header->api_version >= 9;
    } else if (kafka_header->api_key == KAFKA_FETCH) {
        if (!get_topic_offset_from_fetch_request(kafka_header, pkt, &offset)) {
            return false;
        }
        flexible = kafka_header->api_version >= 12;
        topic_id_instead_of_name = kafka_header->api_version >= 13;
    } else {
        return false;
    }

    if (topic_id_instead_of_name) {
        return validate_first_topic_id(pkt, offset);
    }

    return validate_first_topic_name(pkt, flexible, offset);
}

// Checks if the packet represents a kafka request.
static __always_inline bool __is_kafka_fetch_or_produce(pktbuf_t pkt, const char* buf, __u32 buf_size) {
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

    if(!is_supported_api_version_for_classification(kafka_header.api_key, kafka_header.api_version)) {
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

    return is_kafka_fetch_or_produce_request(&kafka_header, pkt, offset);
}

// Checks if the packet represents a kafka request.
// Classification flags are used to split the classification into multiple programs (0 will match all requests).
// (e.g. one for produce and fetch requests, another for api versions).
static __always_inline bool __is_kafka_api_versions(pktbuf_t pkt, const char* buf, __u32 buf_size) {
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

    if(kafka_header.api_key != KAFKA_API_VERSIONS ||
        kafka_header.api_version < KAFKA_CLASSIFICATION_MIN_SUPPORTED_API_VERSIONS_REQUEST_API_VERSION ||
        kafka_header.api_version > KAFKA_CLASSIFICATION_MAX_SUPPORTED_API_VERSIONS_REQUEST_API_VERSION) {
        return false;
    }

    const u32 key = 0;
    // Use a buffer from per-cpu array as the stack is limited.
    char *client_string = bpf_map_lookup_elem(&kafka_client_string, &key);
    if (client_string == NULL) {
        return false;
    }

    u32 offset = pktbuf_data_offset(pkt) + sizeof(kafka_header_t);
    // Validate client ID
    // Client ID size can be equal to '-1' if the client id is null.
    if (kafka_header.client_id_size <= 0) {
        return false;
    }
    if (!is_valid_client_string(pkt, offset, kafka_header.client_id_size, client_string)) {
        return false;
    }
    offset += kafka_header.client_id_size;

    // API versions request is special, it doesn't have a topic name.
    // We handle it separately.
    if (kafka_header.api_key != KAFKA_API_VERSIONS) {
        return false;
    }

    // we only support flexible versions
    if (!skip_request_tagged_fields(pkt, &offset)) {
        return false;
    }

    // Verify client software name
    s16 client_software_name_size = read_nullable_string_size(pkt, true, &offset);
    if (client_software_name_size <= 0) {
        return false;
    }
    if (!is_valid_client_string(pkt, offset, client_software_name_size, client_string)) {
        return false;
    }
    offset += client_software_name_size;

    // Verify client software version
    s16 client_software_version_size = read_nullable_string_size(pkt, true, &offset);
    if (client_software_version_size <= 0) {
        return false;
    }
    if (!is_valid_client_string(pkt, offset, client_software_version_size, client_string)) {
        return false;
    }
    offset += client_software_version_size;

    // Another tagged fields at the end of the request
    if (!skip_request_tagged_fields(pkt, &offset)) {
        return false;
    }

    // we should be at the end of the packet now
    return offset == pktbuf_data_end(pkt);
}

static __always_inline bool is_kafka_fetch_or_produce(struct __sk_buff *skb, skb_info_t *skb_info, const char* buf, __u32 buf_size)
{
    pktbuf_t pkt = pktbuf_from_skb(skb, skb_info);
    return __is_kafka_fetch_or_produce(pkt, buf, buf_size);
}

static __always_inline __maybe_unused bool tls_is_kafka_fetch_or_produce(struct pt_regs *ctx, tls_dispatcher_arguments_t *tls, const char* buf, __u32 buf_size)
{
    pktbuf_t pkt = pktbuf_from_tls(ctx, tls);
    return __is_kafka_fetch_or_produce(pkt, buf, buf_size);
}

static __always_inline bool is_kafka_api_versions(struct __sk_buff *skb, skb_info_t *skb_info, const char* buf, __u32 buf_size)
{
    pktbuf_t pkt = pktbuf_from_skb(skb, skb_info);
    return __is_kafka_api_versions(pkt, buf, buf_size);
}

static __always_inline __maybe_unused bool tlx_is_kafka_api_versions(struct pt_regs *ctx, tls_dispatcher_arguments_t *tls, const char* buf, __u32 buf_size)
{
    pktbuf_t pkt = pktbuf_from_tls(ctx, tls);
    return __is_kafka_api_versions(pkt, buf, buf_size);
}

#endif
