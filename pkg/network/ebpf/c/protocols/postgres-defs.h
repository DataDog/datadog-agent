#ifndef __POSTGRES_DEFS_H
#define __POSTGRES_DEFS_H

// The minimum size we want to be able to check for a startup message. This size includes:
// - The length field: 4 bytes
// - The protocol major version: 2 bytes
// - The protocol minior version: 2 bytes
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

#endif // __POSTGRES_DEFS_H
