#ifndef __POSTGRES_HELPERS_H
#define __POSTGRES_HELPERS_H

#include "defs.h"
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

// Classify ping query of postgres.
static __always_inline bool is_ping(const char *buf, __u32 buf_size) {
    if (buf_size < sizeof(POSTGRES_PING_BODY)) {
        return false;
    }
    char tmp[sizeof(POSTGRES_PING_BODY)] = {0};
    // Cannot use bpf_memcpy here because of verifier error of "misaligned stack access".
#pragma unroll (sizeof(POSTGRES_PING_BODY))
    for (int i = 0; i < sizeof(POSTGRES_PING_BODY); i++) {
        tmp[i] = buf[i];
    }
    return check_command(tmp, POSTGRES_PING_BODY, sizeof(POSTGRES_PING_BODY));
}

// is_postgres_query checks if the buffer is a regular Postgres message.
static __always_inline bool is_postgres_query(const char *buf, __u32 buf_size) {
    CHECK_PRELIMINARY_BUFFER_CONDITIONS(buf, buf_size, sizeof(struct pg_message_header));

    struct pg_message_header *hdr = (struct pg_message_header *)buf;

    // We only classify queries for now
    if (hdr->message_tag != POSTGRES_QUERY_MAGIC_BYTE && hdr->message_tag != POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE) {
        return false;
    }

    __u32 message_len = bpf_ntohl(hdr->message_len);
    if (message_len < POSTGRES_MIN_PAYLOAD_LEN || message_len > POSTGRES_MAX_PAYLOAD_LEN) {
        return false;
    }

    return is_sql_command(buf + sizeof(*hdr), buf_size - sizeof(*hdr)) || is_ping(buf + sizeof(*hdr), buf_size - sizeof(*hdr));
}

static __always_inline bool is_postgres(const char *buf, __u32 buf_size) {
    return is_postgres_query(buf, buf_size) || is_postgres_connect(buf, buf_size);
}

#endif // __POSTGRES_HELPERS_H
