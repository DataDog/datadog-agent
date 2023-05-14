#ifndef __MYSQL_DEFS_H
#define __MYSQL_DEFS_H

// Each MySQL command starts with mysql_hdr, thus the minimum length is sizeof(mysql_hdr).
#define MYSQL_MIN_LENGTH 5

// Taken from https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_com_query.html
#define MYSQL_COMMAND_QUERY 0x3
// Taken from https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_com_stmt_prepare.html
#define MYSQL_PREPARE_QUERY 0x16
// Taken from https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_connection_phase_packets_protocol_handshake_v10.html.
#define MYSQL_SERVER_GREETING_V10 0xa
// Taken from https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_connection_phase_packets_protocol_handshake_v9.html.
#define MYSQL_SERVER_GREETING_V9 0x9
// Represents <digit><digit><dot>
#define MAX_VERSION_COMPONENT 3
// Represents <digit>
#define MIN_BUGFIX_VERSION_COMPONENT 1
// Represents <digit><dot>
#define MIN_MINOR_VERSION_COMPONENT 2
// Minium version string is <digit>.<digit>.<digit>
#define MIN_VERSION_SIZE 5

// MySQL header format. Starts with 24 bits (3 bytes) of the length of the payload, a one byte of sequence id,
// a one byte to represent the message type.
typedef struct {
    __u32 payload_length:24;
    __u8 seq_id;
    __u8 command_type;
} __attribute__((packed)) mysql_hdr;

#endif
