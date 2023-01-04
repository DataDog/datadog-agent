#ifndef SQL_HELPERS_H
#define SQL_HELPERS_H

#include "bpf_builtins.h"
#include "sql-defs.h"

// Check that we can read the amount of memory we want, then to the comparison.
// Note: we use `sizeof(command) - 1` to *not* compare with the null-terminator of
// the strings.
#define check_command(buf, command, buf_size) ( \
    ((sizeof(command) - 1) <= buf_size)         \
    && !bpf_memcmp((buf), &(command), sizeof(command) - 1))

// is_sql_command check that there is an SQL query in buf. We only check the
// most commonly used SQL queries
static __always_inline bool is_sql_command(const char *buf, __u32 buf_size) {
    char tmp[SQL_COMMAND_MAX_SIZE];

    // Convert what would be the query to uppercase to match queries like
    // 'select * from table'
#pragma unroll (SQL_COMMAND_MAX_SIZE)
    for (int i = 0; i < SQL_COMMAND_MAX_SIZE; i++) {
        if ('a' <= buf[i] && buf[i] <= 'z') {
            tmp[i] = buf[i] - 'a' +'A';
        } else {
            tmp[i] = buf[i];
        }
    }

    return check_command(tmp, SQL_ALTER, buf_size)
        || check_command(tmp, SQL_CREATE, buf_size)
        || check_command(tmp, SQL_DELETE, buf_size)
        || check_command(tmp, SQL_DROP, buf_size)
        || check_command(tmp, SQL_INSERT, buf_size)
        || check_command(tmp, SQL_SELECT, buf_size)
        || check_command(tmp, SQL_UPDATE, buf_size);
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

    if (buf_size <= sizeof(struct pg_message_header)) {
        return false;
    }

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

#endif // SQL_HELPERS_H
