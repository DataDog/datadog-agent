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

    // This is the last command in the enum, used to determine the size of the enum.
    __MAX_REDIS_COMMAND
} __attribute__ ((packed)) redis_command_t;

// Redis in-flight transaction info
typedef struct {
    char buf[MAX_KEY_LEN];
    __u64 request_started;
    __u64 response_last_seen;
    __u16 buf_len;
    redis_command_t command;
    __u8 tags;
    bool truncated;
    bool is_error;
} redis_transaction_t;

// The struct we send to userspace, containing the connection tuple and the transaction information.
typedef struct {
    conn_tuple_t tuple;
    redis_transaction_t tx;
} redis_event_t;

// Controls the number of Redis transactions read from userspace at a time.
#define REDIS_BATCH_SIZE (BATCH_BUFFER_SIZE / sizeof(redis_event_t))

#endif /* __REDIS_TYPES_H */
