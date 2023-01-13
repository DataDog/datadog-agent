#ifndef __MYSQL_DEFS_H
#define __MYSQL_DEFS_H

#define MYSQL_MIN_LENGTH 5

#define MYSQL_COMMAND_QUERY 0x3
#define MYSQL_PREPARE_QUERY 0x16
#define MYSQL_SERVER_GREETING_V10 0xa
#define MYSQL_SERVER_GREETING_V9 0x9
#define MAX_VERSION_COMPONENT 3
#define MIN_VERSION_SIZE 5

// MySQL header format. Starts with 24 bits (3 bytes) of the length of the payload, a one byte of sequence id,
// a one byte to represent the message type.
typedef struct {
    __u32 payload_length:24;
    __u8 seq_id;
    __u8 command_type;
} __attribute__((packed)) mysql_hdr;

#endif
