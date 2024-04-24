#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

#include "defs.h"

typedef enum {
    KAFKA_PRODUCE = 0,
    KAFKA_FETCH
} __attribute__ ((packed)) kafka_operation_t;

typedef struct {
    __s32 message_size;
    __s16 api_key;
    __s16 api_version;
    __s32 correlation_id;
    __s16 client_id_size;
} __attribute__ ((packed)) kafka_header_t;

#define KAFKA_MIN_LENGTH (sizeof(kafka_header_t))

typedef struct kafka_transaction_t {
    __u64 request_started;
    // Request API key and version are 16-bit in the protocol but we store
    // them as u8 to reduce memory usage of the map since the APIs and
    // versions we support don't need more than 8 bits.
    __u8 request_api_key;
    __u8 request_api_version;
    char topic_name[TOPIC_NAME_MAX_STRING_SIZE];
    __u16 topic_name_size;
    __u32 records_count;
} kafka_transaction_t;

typedef struct kafka_event_t {
    conn_tuple_t tup;
    kafka_transaction_t transaction;
} kafka_event_t;

typedef struct kafka_transaction_key_t {
    conn_tuple_t tuple;
    __s32 correlation_id;
} kafka_transaction_key_t;

typedef enum {
    KAFKA_FETCH_RESPONSE_PARTITION_START = 0,
    KAFKA_FETCH_RESPONSE_PARTITION_ABORTED_TRANSACTIONS,
    KAFKA_FETCH_RESPONSE_RECORD_BATCHES_ARRAY_START,
    KAFKA_FETCH_RESPONSE_RECORD_BATCH_START,
    KAFKA_FETCH_RESPONSE_RECORD_BATCH_LENGTH,
    KAFKA_FETCH_RESPONSE_RECORD_BATCH_MAGIC,
    KAFKA_FETCH_RESPONSE_RECORD_BATCH_RECORDS_COUNT,
    KAFKA_FETCH_RESPONSE_RECORD_BATCH_END,
    KAFKA_FETCH_RESPONSE_PARTITION_END,
} __attribute__ ((packed)) kafka_response_state;

typedef struct kafka_response_context_t {
    kafka_response_state state;
    // The number of remainder bytes stored from the previous packet into
    // in remainder_buf. The maximum value is 3, even though remainder_buf
    // needs to have space for 4 bytes to make building of the value easier.
    // Used when a fetch response is split over multiple TCP segments.
    __u8 remainder;
    char remainder_buf[4];
    __s32 record_batches_num_bytes;
    __s32 record_batch_length;
    __u32 expected_tcp_seq;
    // The offset to start reading data from the next packet, carried
    // over from processing of the previous packet. Used when a fetch response
    // is split over multiple TCP segments.
    __s32 carry_over_offset;
    __u32 partitions_count;
    kafka_transaction_t transaction;
} kafka_response_context_t;

// Used as a scratch buffer, one per CPU.
typedef struct kafka_info_t {
    kafka_response_context_t response;
    kafka_event_t event;
} kafka_info_t;

// kafka_telemetry_t is used to hold the Kafka kernel telemetry.
typedef struct {
    // The array topic_name_size_buckets maps a bucket index to the number of occurrences observed for topic name lengths
    // For example, topic_name_size_buckets[0] = 10 indicates that 10 topic names were observed with lengths less than 25
    __u64 topic_name_size_buckets[KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS];
} kafka_telemetry_t;

#endif
