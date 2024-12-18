#ifndef __POSTGRES_TYPES_H
#define __POSTGRES_TYPES_H

#include "conn_tuple.h"

// Maximum length of Postgres query to send to userspace.
#define POSTGRES_BUFFER_SIZE 160

// Represents the maximum number of tail calls we can use to process a single message.
#define POSTGRES_MAX_TAIL_CALLS_FOR_MAX_MESSAGES 3

// Represents the maximum number of messages we process in a single tail call.
#define POSTGRES_MAX_MESSAGES_PER_TAIL_CALL 60

// maximum number of messages to fetch
#define POSTGRES_MAX_TOTAL_MESSAGES (POSTGRES_MAX_TAIL_CALLS_FOR_MAX_MESSAGES * POSTGRES_MAX_MESSAGES_PER_TAIL_CALL)

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

typedef struct {
    __u8 total_msg_count;
    // Saving the packet data offset is crucial for maintaining the current read position and ensuring proper utilization
    // of tail calls.
    __u32 data_off;
} postgres_tail_call_state_t;

// Postgres communication operates via a continuous message stream.
// Gather empirical statistics on the number of messages processed by the program.
// These statistics enable optimization of the monitoring code.
// Collect counters for each subsequent bucket.
// bucket 0: count 0 to 100, bucket 1: 100 to 119, bucket 2: 120 to 139 etc.
#define PG_KERNEL_MSG_COUNT_NUM_BUCKETS 5
#define PG_KERNEL_MSG_COUNT_BUCKET_SIZE 20
#define PG_KERNEL_MSG_COUNT_FIRST_BUCKET 100

// This structure stores statistics about the number of Postgres messages in a TCP packet.
typedef struct {
    __u64 reached_max_messages;
    __u64 fragmented_packets;
    __u64 msg_count_buckets[PG_KERNEL_MSG_COUNT_NUM_BUCKETS];
} postgres_kernel_msg_count_t;

#endif
