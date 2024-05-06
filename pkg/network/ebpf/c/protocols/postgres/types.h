#ifndef __POSTGRES_TYPES_H
#define __POSTGRES_TYPES_H

#include "conn_tuple.h"

// This controls the number of Postgres transactions read from userspace at a time.
#define POSTGRES_BATCH_SIZE 25
#define POSTGRES_BUFFER_SIZE 64
#define POSTGRES_MAX_MESSAGES 100

// HTTP transaction information associated to a certain socket (conn_tuple_t)
typedef struct {
    char request_fragment[POSTGRES_BUFFER_SIZE];
    __u64 request_started;
    __u64 response_last_seen;
    __u8 frag_size;
} postgres_transaction_t;

typedef struct {
    conn_tuple_t tuple;
    postgres_transaction_t tx;
} postgres_event_t;

#endif
