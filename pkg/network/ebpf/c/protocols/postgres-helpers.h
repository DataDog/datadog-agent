#ifndef __POSTGRES_HELPERS_H
#define __POSTGRES_HELPERS_H

#include "postgres-defs.h"
#include "sql-helpers.h"

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

    struct pg_message_header *hdr = (struct pg_message_header *)buf;

    // We only classify queries for now
    if (hdr->message_tag != POSTGRES_QUERY_MAGIC_BYTE) {
        return false;
    }

    __u32 message_len = bpf_ntohl(hdr->message_len);
    if (message_len < POSTGRES_MIN_PAYLOAD_LEN || message_len > POSTGRES_MAX_PAYLOAD_LEN) {
        return false;
    }

    return is_sql_command(buf + sizeof(*hdr), buf_size - sizeof(*hdr));
}

static __always_inline bool is_postgres(const char *buf, __u32 buf_size) {
    return is_postgres_query(buf, buf_size) || is_postgres_connect(buf, buf_size);
}

#endif // __POSTGRES_HELPERS_H
