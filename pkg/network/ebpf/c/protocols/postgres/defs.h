#ifndef __POSTGRES_DEFS_H
#define __POSTGRES_DEFS_H

// The minimum size we want to be able to check for a startup message. This size includes:
// - The length field: 4 bytes
// - The protocol major version: 2 bytes
// - The protocol minor version: 2 bytes
// - The "user" string, as the first connection parameter name: 5 bytes
#define POSTGRES_STARTUP_MIN_LEN 13

// Postgres protocol version, in big endian, as described in the protocol
// specification. This is version "3.0".
#define PG_STARTUP_VERSION 196608
#define PG_STARTUP_USER_PARAM "user"

// From https://www.postgresql.org/docs/current/protocol-overview.html:
// The first byte of a message identifies the message type, and the next four
// bytes specify the length of the rest of the message (this length count
// includes itself, but not the message-type byte). The remaining contents of
// the message are determined by the message type. Some messages do not have
// a payload at all, so the minimum size, including the length itself, is
// 4 bytes.
#define POSTGRES_MIN_PAYLOAD_LEN 4
// Assume typical query message size is below an artificial limit.
// 30000 is copied from postgres code base:
// https://github.com/postgres/postgres/tree/master/src/interfaces/libpq/fe-protocol3.c#L94
#define POSTGRES_MAX_PAYLOAD_LEN 30000

#define POSTGRES_QUERY_MAGIC_BYTE 'Q'
#define POSTGRES_PARSE_MAGIC_BYTE 'P'
#define POSTGRES_COMMAND_COMPLETE_MAGIC_BYTE 'C'

#define POSTGRES_PING_BODY "-- ping"

#define POSTGRES_SKIP_STRING_ITERATIONS 8
#define SKIP_STRING_FAILED 0

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

#endif // __POSTGRES_DEFS_H
