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

typedef struct {
    conn_tuple_t tup;
    __u16 request_api_key;
    __u16 request_api_version;
    char topic_name[TOPIC_NAME_MAX_STRING_SIZE];
    __u16 topic_name_size;
    __u32 records_count;
} kafka_transaction_t;

// kafka_telemetry_t is used to hold the Kafka kernel telemetry.
typedef struct {
    // The array topic_name_size_buckets maps a bucket index to the number of occurrences observed for topic name lengths
    // For example, topic_name_size_buckets[0] = 10 indicates that 10 topic names were observed with lengths less than 25
    __u64 topic_name_size_buckets[KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS];
} kafka_telemetry_t;

#endif
