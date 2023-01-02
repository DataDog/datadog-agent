#ifndef __PROTOCOL_CLASSIFICATION_HELPERS_H
#define __PROTOCOL_CLASSIFICATION_HELPERS_H

#include <linux/stddef.h>
#include <linux/types.h>

#include "bpf_builtins.h"
#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "http2.h"
#include "ip.h"
#include "map-defs.h"
#include "protocol-classification-defs.h"
#include "protocol-classification-maps.h"
#include "protocols/protocol-classification-sql-defs.h"

// Patch to support old kernels that don't contain bpf_skb_load_bytes, by adding a dummy implementation to bypass runtime compilation.
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 5, 0)
long bpf_skb_load_bytes_with_telemetry(const void *skb, u32 offset, void *to, u32 len) {return 0;}
#endif

#define CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, min_buff_size) \
    do {                                                                  \
        if (buf_size < min_buff_size) {                                   \
            return false;                                                 \
        }                                                                 \
                                                                          \
        if (buf == NULL) {                                                \
            return false;                                                 \
        }                                                                 \
    } while (0)


// The method checks if the given buffer starts with the HTTP2 marker as defined in https://datatracker.ietf.org/doc/html/rfc7540.
// We check that the given buffer is not empty and its size is at least 24 bytes.
static __always_inline bool is_http2_preface(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_MARKER_SIZE);

#define HTTP2_PREFACE "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

    bool match = !bpf_memcmp(buf, HTTP2_PREFACE, sizeof(HTTP2_PREFACE)-1);

    return match;
}

// According to the https://www.rfc-editor.org/rfc/rfc7540#section-3.5
// an HTTP2 server must reply with a settings frame to the preface of HTTP2.
// The settings frame must not be related to the connection (stream_id == 0) and the length should be a multiplication
// of 6 bytes.
static __always_inline bool is_http2_server_settings(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, HTTP2_FRAME_HEADER_SIZE);

    struct http2_frame frame_header;
    if (!read_http2_frame_header(buf, buf_size, &frame_header)) {
        return false;
    }

    return frame_header.type == kSettingsFrame && frame_header.stream_id == 0 && frame_header.length % HTTP2_SETTINGS_SIZE == 0;
}

// The method checks if the given buffer starts with the HTTP2 marker as defined in https://datatracker.ietf.org/doc/html/rfc7540.
// We check that the given buffer is not empty and its size is at least 24 bytes.
static __always_inline bool is_http2(const char* buf, __u32 buf_size) {
    return is_http2_preface(buf, buf_size) || is_http2_server_settings(buf, buf_size);
}

// The method checks if the given buffer includes the protocol header which must be sent in the start of a new connection.
// Ref: https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf
static __always_inline bool is_amqp_protocol_header(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, AMQP_MIN_FRAME_LENGTH);

#define AMQP_PREFACE "AMQP"

    bool match = !bpf_memcmp(buf, AMQP_PREFACE, sizeof(AMQP_PREFACE)-1);

    return match;
}

// The method checks if the given buffer is an AMQP message.
// Ref: https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf
static __always_inline bool is_amqp(const char* buf, __u32 buf_size) {
    // New connection should start with protocol header of AMQP.
    // Ref https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf.
    if (is_amqp_protocol_header(buf, buf_size)) {
        return true;
    }

    // Validate that we will be able to get from the buffer the class and method ids.
    if (buf_size < AMQP_MIN_PAYLOAD_LENGTH) {
       return false;
    }

    uint8_t frame_type = buf[0];
    // Check only for method frame type.
    if (frame_type != AMQP_FRAME_METHOD_TYPE) {
        return false;
    }

    // We extract the class id and method id by big indian from the buffer.
    // Ref https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf.
    __u16 class_id = buf[7] << 8 | buf[8];
    __u16 method_id = buf[9] << 8 | buf[10];

    // ConnectionStart, ConnectionStartOk, BasicPublish, BasicDeliver, BasicConsume are the most likely methods to
    // consider for the classification.
    if (class_id == AMQP_CONNECTION_CLASS) {
        return  method_id == AMQP_METHOD_CONNECTION_START || method_id == AMQP_METHOD_CONNECTION_START_OK;
    }

    if (class_id == AMQP_BASIC_CLASS) {
        return method_id == AMQP_METHOD_PUBLISH || method_id == AMQP_METHOD_DELIVER || method_id == AMQP_METHOD_CONSUME;
    }

    return false;
}

// Checks if the given buffers start with `HTTP` prefix (represents a response) or starts with `<method> /` which represents
// a request, where <method> is one of: GET, POST, PUT, DELETE, HEAD, OPTIONS, or PATCH.
static __always_inline bool is_http(const char *buf, __u32 size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, size, HTTP_MIN_SIZE);

#define HTTP "HTTP/"
#define GET "GET /"
#define POST "POST /"
#define PUT "PUT /"
#define DELETE "DELETE /"
#define HEAD "HEAD /"
#define OPTIONS1 "OPTIONS /"
#define OPTIONS2 "OPTIONS *"
#define PATCH "PATCH /"

    // memcmp returns
    // 0 when s1 == s2,
    // !0 when s1 != s2.
    bool http = !(bpf_memcmp(buf, HTTP, sizeof(HTTP)-1)
        && bpf_memcmp(buf, GET, sizeof(GET)-1)
        && bpf_memcmp(buf, POST, sizeof(POST)-1)
        && bpf_memcmp(buf, PUT, sizeof(PUT)-1)
        && bpf_memcmp(buf, DELETE, sizeof(DELETE)-1)
        && bpf_memcmp(buf, HEAD, sizeof(HEAD)-1)
        && bpf_memcmp(buf, OPTIONS1, sizeof(OPTIONS1)-1)
        && bpf_memcmp(buf, OPTIONS2, sizeof(OPTIONS2)-1)
        && bpf_memcmp(buf, PATCH, sizeof(PATCH)-1));

    return http;
}

// Regular format of postgres message: | byte tag | int32_t len | string payload |
// From https://www.postgresql.org/docs/current/protocol-overview.html:
// The first byte of a message identifies the message type, and the next four
// bytes specify the length of the rest of the message (this length count
// includes itself, but not the message-type byte). The remaining contents of
// the message are determined by the message type
struct pg_message_header {
    __u8 message_tag;
    __u32 message_len; // Big-endian: use bpf_ntohl to read this field
} __attribute__((packed));

// Postgres Startup Message (used when a client connects to the server) differs
// from other messages by not having a message tag.
struct pg_startup_header {
    __u32 message_len; // Big-endian: use bpf_ntohl to read this field
    __u32 version; // Big-endian: use bpf_ntohl to read this field
};

// is_postgres_connect checks if the buffer is a Postgres startup message.
static __always_inline bool is_postgres_connect(const char *buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, POSTGRES_STARTUP_MIN_LEN);

    struct pg_startup_header *hdr = (struct pg_startup_header *)buf;

    if (bpf_ntohl(hdr->version) != PG_STARTUP_VERSION) {
        return false;
    }

    // Check if we can find the user param. Postgres uses C-style strings, so
    // we also check for the terminating null byte.
    return !bpf_memcmp(buf + sizeof(*hdr), PG_STARTUP_USER_PARAM, sizeof(PG_STARTUP_USER_PARAM));
}

// is_postgres_query checks if the buffer is a regular Postgres message.
static __always_inline bool is_postgres_query(const char *buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, sizeof(struct pg_message_header));

    struct pg_message_header hdr;
    hdr.message_tag = *buf;
    hdr.message_len = *(__u32*)(buf+1);

    // We only classify queries for now
    if (hdr.message_tag != POSTGRES_QUERY_MAGIC_BYTE) {
        return false;
    }

    __u32 message_len = bpf_ntohl(hdr.message_len);
    if (message_len < POSTGRES_MIN_PAYLOAD_LEN || message_len > POSTGRES_MAX_PAYLOAD_LEN) {
        return false;
    }

    return is_sql_command(buf + sizeof(hdr), buf_size - sizeof(hdr));
}

static __always_inline bool is_postgres(const char *buf, __u32 buf_size) {
    return is_postgres_query(buf, buf_size) || is_postgres_connect(buf, buf_size);
}

// Checks the buffer represent a standard response (OK) or any of redis commands
// https://redis.io/commands/
static __always_inline bool check_supported_ascii_and_crlf(const char* buf, __u32 buf_size, int index_to_start_from) {
    bool found_cr = false;
    char current_char;
    int i = index_to_start_from;
#pragma unroll(CLASSIFICATION_MAX_BUFFER)
    for (; i < CLASSIFICATION_MAX_BUFFER; i++) {
        current_char = buf[i];
        if (current_char == '\r') {
            found_cr = true;
            break;
        } else if ('A' <= current_char && current_char <= 'Z') {
            continue;
        } else if ('a' <= current_char && current_char <= 'z') {
            continue;
        } else if (current_char == '.' || current_char == ' ' || current_char == '-' || current_char == '_') {
            continue;
        }
        return false;
    }

    if (!found_cr || i+1 >= buf_size) {
        return false;
    }
    return buf[i+1] == '\n';
}

// Checks the buffer represents an error according to https://redis.io/docs/reference/protocol-spec/#resp-errors
static __always_inline bool check_err_prefix(const char* buf, __u32 buf_size) {
#define ERR "-ERR "
#define WRONGTYPE "-WRONGTYPE "

    // memcmp returns
    // 0 when s1 == s2,
    // !0 when s1 != s2.
    bool match = !(bpf_memcmp(buf, ERR, sizeof(ERR)-1)
        && bpf_memcmp(buf, WRONGTYPE, sizeof(WRONGTYPE)-1));

    return match;
}

static __always_inline bool check_integer_and_crlf(const char* buf, __u32 buf_size, int index_to_start_from) {
    bool found_cr = false;
    char current_char;
    int i = index_to_start_from;
#pragma unroll(CLASSIFICATION_MAX_BUFFER)
    for (; i < CLASSIFICATION_MAX_BUFFER; i++) {
        current_char = buf[i];
        if (current_char == '\r') {
            found_cr = true;
            break;
        } else if ('0' <= current_char && current_char <= '9') {
            continue;
        }

        return false;
    }

    if (!found_cr || i+1 >= buf_size) {
        return false;
    }
    return buf[i+1] == '\n';
}

static __always_inline bool is_redis(const char* buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, REDIS_MIN_FRAME_LENGTH);

    char first_char = buf[0];
    switch (first_char) {
    case '+':
        return check_supported_ascii_and_crlf(buf, buf_size, 1);
    case '-':
        return check_err_prefix(buf, buf_size);
    case ':':
    case '$':
    case '*':
        return check_integer_and_crlf(buf, buf_size, 1);
    default:
        return false;
    }
}

// Determines the protocols of the given buffer. If we already classified the payload (a.k.a protocol out param
// has a known protocol), then we do nothing.
static __always_inline void classify_protocol(protocol_t *protocol, const char *buf, __u32 size) {
    if (protocol == NULL || *protocol != PROTOCOL_UNKNOWN) {
        return;
    }

    if (is_http(buf, size)) {
        *protocol = PROTOCOL_HTTP;
    } else if (is_http2(buf, size)) {
        *protocol = PROTOCOL_HTTP2;
    } else if (is_postgres(buf, size)) {
        *protocol = PROTOCOL_POSTGRES;
    } else if (is_amqp(buf, size)) {
        *protocol = PROTOCOL_AMQP;
    } else if (is_redis(buf, size)) {
        *protocol = PROTOCOL_REDIS;
    } else {
        *protocol = PROTOCOL_UNKNOWN;
    }

    log_debug("[protocol classification]: Classified protocol as %d %d; %s\n", *protocol, size, buf);
}

// Returns true if the packet is TCP.
static __always_inline bool is_tcp(conn_tuple_t *tup) {
    return tup->metadata & CONN_TYPE_TCP;
}

// Returns true if the payload is empty.
static __always_inline bool is_payload_empty(struct __sk_buff *skb, skb_info_t *skb_info) {
    return skb_info->data_off == skb->len;
}

// The method is used to read the data buffer from the __sk_buf struct. Similar implementation as `read_into_buffer_skb`
// from http parsing, but uses a different constant (CLASSIFICATION_MAX_BUFFER).
static __always_inline void read_into_buffer_for_classification(char *buffer, struct __sk_buff *skb, skb_info_t *info) {
    u64 offset = (u64)info->data_off;

#define BLK_SIZE (16)
    const u32 len = CLASSIFICATION_MAX_BUFFER < (skb->len - (u32)offset) ? (u32)offset + CLASSIFICATION_MAX_BUFFER : skb->len;

    unsigned i = 0;

#pragma unroll(CLASSIFICATION_MAX_BUFFER / BLK_SIZE)
    for (; i < (CLASSIFICATION_MAX_BUFFER / BLK_SIZE); i++) {
        if (offset + BLK_SIZE - 1 >= len) { break; }

        bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
        offset += BLK_SIZE;
    }

    // This part is very hard to write in a loop and unroll it.
    // Indeed, mostly because of older kernel verifiers, we want to make sure the offset into the buffer is not
    // stored on the stack, so that the verifier is able to verify that we're not doing out-of-bound on
    // the stack.
    // Basically, we should get a register from the code block above containing an fp relative address. As
    // we are doing `buffer[0]` here, there is not dynamic computation on that said register after this,
    // and thus the verifier is able to ensure that we are in-bound.
    void *buf = &buffer[i * BLK_SIZE];

    // Check that we have enough room in the request fragment buffer. Even
    // though that's not strictly needed here, the verifier does not know that,
    // so this check makes it happy.
    if (i * BLK_SIZE >= CLASSIFICATION_MAX_BUFFER) {
        return;
    } else if (offset + 14 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 1);
    }
}

// A shared implementation for the runtime & prebuilt socket filter that classifies the protocols of the connections.
static __always_inline void protocol_classifier_entrypoint(struct __sk_buff *skb) {
    skb_info_t skb_info = {0};
    conn_tuple_t skb_tup = {0};

    // Exporting the conn tuple from the skb, alongside couple of relevant fields from the skb.
    if (!read_conn_tuple_skb(skb, &skb_info, &skb_tup)) {
        return;
    }

    // We support non empty TCP payloads for classification at the moment.
    if (!is_tcp(&skb_tup) || is_payload_empty(skb, &skb_info)) {
        return;
    }

    protocol_t *cur_fragment_protocol_ptr = bpf_map_lookup_elem(&connection_protocol, &skb_tup);
    if (cur_fragment_protocol_ptr) {
        return;
    }

    protocol_t cur_fragment_protocol = PROTOCOL_UNKNOWN;

    // Get the buffer the fragment will be read into from a per-cpu array map.
    // This will avoid doing unaligned stack access while parsing the protocols,
    // which is forbidden and will make the verifier fail.
    const u32 key = 0;
    char *request_fragment = bpf_map_lookup_elem(&classification_buf, &key);
    if (request_fragment == NULL) {
        log_debug("could not get classification buffer from map");
        return;
    }

    bpf_memset(request_fragment, 0, sizeof(request_fragment));
    read_into_buffer_for_classification((char *)request_fragment, skb, &skb_info);

    const size_t payload_length = skb->len - skb_info.data_off;
    const size_t final_fragment_size = payload_length < CLASSIFICATION_MAX_BUFFER ? payload_length : CLASSIFICATION_MAX_BUFFER;
    classify_protocol(&cur_fragment_protocol, request_fragment, final_fragment_size);
    // If there has been a change in the classification, save the new protocol.
    if (cur_fragment_protocol != PROTOCOL_UNKNOWN) {
        bpf_map_update_with_telemetry(connection_protocol, &skb_tup, &cur_fragment_protocol, BPF_NOEXIST);
        conn_tuple_t inverse_skb_conn_tup = skb_tup;
        flip_tuple(&inverse_skb_conn_tup);
        bpf_map_update_with_telemetry(connection_protocol, &inverse_skb_conn_tup, &cur_fragment_protocol, BPF_NOEXIST);
    }
}

#endif
