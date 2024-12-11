#ifndef __REDIS_TYPES_H
#define __REDIS_TYPES_H

#include "conn_tuple.h"
#include "protocols/events-types.h"

// Redis in-flight transaction info
typedef struct {
    __u64 request_started;
    __u64 response_last_seen;
    __u8 tags;
} redis_transaction_t;

// The struct we send to userspace, containing the connection tuple and the transaction information.
typedef struct {
    conn_tuple_t tuple;
    redis_transaction_t tx;
} redis_event_t;

// Controls the number of Redis transactions read from userspace at a time.
#define REDIS_BATCH_SIZE (BATCH_BUFFER_SIZE / sizeof(redis_event_t))

#endif /* __REDIS_TYPES_H */
