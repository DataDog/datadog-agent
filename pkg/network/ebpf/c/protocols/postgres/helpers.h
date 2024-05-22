#ifndef __POSTGRES_HELPERS_H
#define __POSTGRES_HELPERS_H

#include "defs.h"
#include "protocols/helpers/pktbuf.h"
#include "protocols/sql/helpers.h"

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
static __always_inline bool is_postgres_query(struct pg_message_header *hdr, const char *buf, __u32 buf_size) {
    // We only classify queries for now
    if (hdr->message_tag != POSTGRES_QUERY_MAGIC_BYTE && hdr->message_tag != POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
        return false;
    }

    return is_sql_command(buf, sizeof(*hdr), buf_size);
}

#define MAX_PARSE_BASE_MESSAGE (CLASSIFICATION_MAX_BUFFER - sizeof(struct pg_message_header) - SQL_COMMAND_MAX_SIZE)

static __always_inline bool is_postgres_parse(struct pg_message_header *hdr, const char *buf, __u32 buf_size) {
    // We only classify queries for now
    if (hdr->message_tag != POSTGRES_PARSE_MAGIC_BYTE) {
        return false;
    }

    int offset = 0;
    bool found = false;
#pragma unroll(MAX_PARSE_BASE_MESSAGE)
    for (; offset < MAX_PARSE_BASE_MESSAGE; offset++) {
        if (offset + sizeof(struct pg_message_header) >= buf_size) {
            break;
        }
        if (buf[offset+sizeof(struct pg_message_header)] == '\0') {
            found = true;
            break;
        }
    }

    if (!found) {
        return false;
    }

    offset += 1 + sizeof(struct pg_message_header);
    return is_sql_command(buf, offset, buf_size);
}


static __always_inline bool is_postgres(const char *buf, __u32 buf_size) {
    if (is_postgres_connect(buf, buf_size)) {
        return true;
    }
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, sizeof(struct pg_message_header));
    struct pg_message_header *hdr = (struct pg_message_header *)buf;
    __u32 message_len = bpf_ntohl(hdr->message_len);

    if (message_len < POSTGRES_MIN_PAYLOAD_LEN || message_len > POSTGRES_MAX_PAYLOAD_LEN) {
        return false;
    }
    return is_postgres_query(hdr, buf, buf_size) || is_postgres_parse(hdr, buf, buf_size);
}

#endif // __POSTGRES_HELPERS_H
