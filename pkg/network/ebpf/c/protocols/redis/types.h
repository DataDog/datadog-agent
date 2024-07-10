#ifndef __REDIS_TYPES_H
#define __REDIS_TYPES_H

#include "conn_tuple.h"

// Controls the number of Redis transactions read from userspace at a time.
#define REDIS_BATCH_SIZE 25

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

#endif /* __REDIS_TYPES_H */
