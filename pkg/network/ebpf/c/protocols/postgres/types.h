#ifndef __POSTGRES_TYPES_H
#define __POSTGRES_TYPES_H

#include "conn_tuple.h"

// Controls the number of Postgres transactions read from userspace at a time.
#define POSTGRES_BATCH_SIZE 25

// Maximum length of postgres query to send to userspace.
#define POSTGRES_BUFFER_SIZE 64

// Maximum number of Postgres messages we can parse for a single packet.
#define POSTGRES_MAX_MESSAGES 80

// Postgres transaction information we store in the kernel.
typedef struct {
    // The Postgres query we are currently parsing. Stored up to POSTGRES_BUFFER_SIZE bytes.
    char request_fragment[POSTGRES_BUFFER_SIZE];
    __u64 request_started;
    __u64 response_last_seen;
    // The actual size of the query stored in request_fragment.
    __u32 original_query_size;
    __u8 tags;
} postgres_transaction_t;

// The struct we send to userspace, containing the connection tuple and the transaction information.
typedef struct {
    conn_tuple_t tuple;
    postgres_transaction_t tx;
} postgres_event_t;

#endif
