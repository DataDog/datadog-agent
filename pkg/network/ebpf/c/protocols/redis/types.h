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
    REDIS_NO_ERR,
    REDIS_ERR_UNKNOWN,
    REDIS_ERR_ERR,
    REDIS_ERR_WRONGTYPE,
    REDIS_ERR_NOAUTH,
    REDIS_ERR_NOPERM,
    REDIS_ERR_BUSY,
    REDIS_ERR_NOSCRIPT,
    REDIS_ERR_LOADING,
    REDIS_ERR_READONLY,
    REDIS_ERR_EXECABORT,
    REDIS_ERR_MASTERDOWN,
    REDIS_ERR_MISCONF,
    REDIS_ERR_CROSSSLOT,
    REDIS_ERR_TRYAGAIN,
    REDIS_ERR_ASK,
    REDIS_ERR_MOVED,
    REDIS_ERR_CLUSTERDOWN,
    REDIS_ERR_NOREPLICAS,
    REDIS_ERR_OOM,
    REDIS_ERR_NOQUORUM,
    REDIS_ERR_BUSYKEY,
    REDIS_ERR_UNBLOCKED,
    REDIS_ERR_WRONGPASS,
    REDIS_ERR_INVALIDOBJ
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
