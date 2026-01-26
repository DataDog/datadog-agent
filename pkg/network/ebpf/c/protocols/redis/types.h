#ifndef __REDIS_TYPES_H
#define __REDIS_TYPES_H

#include "conn_tuple.h"
#include "protocols/events-types.h"
#include "protocols/redis/defs.h"

#define bool _Bool
#define true 1
#define false 0

typedef enum {
    REDIS_UNKNOWN = 0,
    REDIS_GET = 1,
    REDIS_SET = 2,
    REDIS_PING = 3,

    // This is the last command in the enum, used to determine the size of the enum.
    __MAX_REDIS_COMMAND
} __attribute__ ((packed)) redis_command_t;

// Represents redis key name.
typedef struct {
    char buf[MAX_KEY_LEN];
    __u16 len;
    bool truncated;
} redis_key_data_t;

// Redis in-flight transaction info
typedef struct {
    __u64 request_started;
    __u64 response_last_seen;
    redis_command_t command;
    __u8 tags;
    bool is_error;
} redis_transaction_t;

// The struct we send to userspace, containing the connection tuple and the transaction information.
typedef struct {
    conn_tuple_t tuple;
    redis_transaction_t tx;
} redis_event_t;

// The struct we send to userspace, containing the connection tuple and the transaction information.
typedef struct {
    redis_event_t header;
    redis_key_data_t key;
} redis_with_key_event_t;

// Controls the number of Redis transactions read from userspace at a time.
#define REDIS_WITH_KEY_BATCH_SIZE (BATCH_BUFFER_SIZE / sizeof(redis_with_key_event_t))
#define REDIS_BATCH_SIZE (BATCH_BUFFER_SIZE / sizeof(redis_event_t))

#endif /* __REDIS_TYPES_H */
