#ifndef __POSTGRES_TYPES_H
#define __POSTGRES_TYPES_H

#include "conn_tuple.h"

// Maximum length of Postgres query to send to userspace.
#define POSTGRES_BUFFER_SIZE 160

// Represents the maximum number of tail calls we can use to process a single message.
#define POSTGRES_MAX_TAIL_CALLS_FOR_MAX_MESSAGES 5

// Represents the maximum number of messages we process in a single tail call.
#define POSTGRES_MAX_MESSAGES_PER_TAIL_CALL 70

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
    __u32 msg_count;
    // Saving the packet data offset is crucial for maintaining the current read position and ensuring proper utilization
    // of tail calls.
    __u32 data_off;
} postgres_tail_call_state_t;

typedef enum
{
    // reached end of TCP packet
    READ_END = 0,
    // successful read of message header
    READ_OK = 1,
    // detected TCP packet fragmentation
    READ_FRAGMENTED = 2,
} postgres_read_result_t;

// Postgres communication operates via a continuous message stream.
// Gather empirical statistics on the number of messages processed by the program.
// These statistics enable optimization of the monitoring code.
// Collect counters for each subsequent bucket:
// 0 to (BUCKET_SIZE-1), BUCKET_SIZE to (BUCKET_SIZE*2-1), (BUCKET_SIZE*2) to (BUCKET_SIZE*3-1), ...
#define PG_KERNEL_MSG_COUNT_BUCKET_SIZE 35

// maximum number of messages
#define PG_KERNEL_MAX_MESSAGES (POSTGRES_MAX_TAIL_CALLS_FOR_MAX_MESSAGES * POSTGRES_MAX_MESSAGES_PER_TAIL_CALL)
#define PG_KERNEL_MSG_COUNT_NUM_BUCKETS (PG_KERNEL_MAX_MESSAGES / PG_KERNEL_MSG_COUNT_BUCKET_SIZE)

// approximate maximum number of messages when TCP packet is not fragmented
#define PG_KERNEL_MSG_MAX_NOT_FRAGMENTED 250
#define PG_KERNEL_MAX_BUCKETS_NOT_FRAGMENTED (PG_KERNEL_MSG_MAX_NOT_FRAGMENTED / PG_KERNEL_MSG_COUNT_BUCKET_SIZE)

// postgres_kernel_msg_count_t This structure stores statistics about the number of Postgres messages in a TCP packet.
// the last bucket stores a counter of fragmented packets and the maximum count has been reached.
typedef struct {
    __u64 pg_messages_count_buckets[PG_KERNEL_MSG_COUNT_NUM_BUCKETS + 1];
} postgres_kernel_msg_count_t;

#endif
