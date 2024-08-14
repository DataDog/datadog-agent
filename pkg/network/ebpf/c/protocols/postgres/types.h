#ifndef __POSTGRES_TYPES_H
#define __POSTGRES_TYPES_H

#include "conn_tuple.h"

// Controls the number of Postgres transactions read from userspace at a time.
#define POSTGRES_BATCH_SIZE 17

// Maximum length of Postgres query to send to userspace.
#define POSTGRES_BUFFER_SIZE 160

// Maximum number of Postgres messages we can parse for a single packet.
#define POSTGRES_MAX_MESSAGES 40

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

// Postgres communication is done through a message stream.
// Each TCP packet can include multiple postgres messages.
// Postgres telemetry helps to find empirical statistics about the number of messages in each packet,
// these statistics allow to optimize monitoring code.
// It is expected that there can be from 1 to PG_KERNEL_MSG_COUNT_MAX messages in a packet.
// Collect counters for each subsequent bucket:
// 0 to (BUCKET_SIZE-1), BUCKET_SIZE to (BUCKET_SIZE*2-1), (BUCKET_SIZE*2) to (BUCKET_SIZE*3-1), ...
#define PG_KERNEL_MSG_COUNT_NUM_BUCKETS 8
#define PG_KERNEL_MSG_COUNT_BUCKET_SIZE (POSTGRES_MAX_MESSAGES / PG_KERNEL_MSG_COUNT_NUM_BUCKETS)
#define PG_KERNEL_MSG_COUNT_MAX (PG_KERNEL_MSG_COUNT_NUM_BUCKETS * PG_KERNEL_MSG_COUNT_BUCKET_SIZE)
#define PG_KERNEL_MSG_COUNT_BUCKET_INDEX(count) (count / PG_KERNEL_MSG_COUNT_BUCKET_SIZE)

// postgres_kernel_msg_count_t This structure stores statistics about the number of Postgres messages in a TCP packet.
typedef struct {
    __u64 pg_messages_count_buckets[PG_KERNEL_MSG_COUNT_NUM_BUCKETS];
} postgres_kernel_msg_count_t;

#endif
