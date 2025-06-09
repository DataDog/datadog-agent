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

typedef enum {
    REDIS_NO_ERR = 0,
    REDIS_ERR_UNKNOWN = 1,
    REDIS_ERR_ERR = 2,
    REDIS_ERR_WRONGTYPE = 3,
    REDIS_ERR_NOAUTH = 4,
    REDIS_ERR_NOPERM = 5,
    REDIS_ERR_BUSY = 6,
    REDIS_ERR_NOSCRIPT = 7,
    REDIS_ERR_LOADING = 8,
    REDIS_ERR_READONLY = 9,
    REDIS_ERR_EXECABORT = 10,
    REDIS_ERR_MASTERDOWN = 11,
    REDIS_ERR_MISCONF = 12,
    REDIS_ERR_CROSSSLOT = 13,
    REDIS_ERR_TRYAGAIN = 14,
    REDIS_ERR_ASK = 15,
    REDIS_ERR_MOVED = 16,
    REDIS_ERR_CLUSTERDOWN = 17,
    REDIS_ERR_NOREPLICAS = 18,
    REDIS_ERR_OOM = 19,
    REDIS_ERR_NOQUORUM = 20,
    REDIS_ERR_BUSYKEY = 21,
    REDIS_ERR_UNBLOCKED = 22,
    REDIS_ERR_WRONGPASS = 23,
    REDIS_ERR_INVALIDOBJ = 24
} __attribute__ ((packed)) redis_error_t;

// Redis in-flight transaction info
typedef struct {
    char buf[MAX_KEY_LEN];        // 128 bytes
    __u64 request_started;        // 8 bytes
    __u64 response_last_seen;     // 8 bytes
    __u16 buf_len;               // 2 bytes
    redis_error_t error;          // 1 byte
    redis_command_t command;      // 1 byte
    __u8 tags;                   // 1 byte
    bool truncated;              // 1 byte
    bool is_error;               // 1 byte
} redis_transaction_t;

// The struct we send to userspace, containing the connection tuple and the transaction information.
typedef struct {
    conn_tuple_t tuple;
    redis_transaction_t tx;
} redis_event_t;

// Controls the number of Redis transactions read from userspace at a time.
#define REDIS_BATCH_SIZE (BATCH_BUFFER_SIZE / sizeof(redis_event_t))

#endif /* __REDIS_TYPES_H */
