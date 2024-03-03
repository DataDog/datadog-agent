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
} kafka_transaction_batch_entry_t;

// Kafka transaction information associated to a certain socket (tuple_t)
typedef struct {
    // this field is used to disambiguate segments in the context of keep-alives
    // we populate it with the TCP seq number of the request and then the response segments
    __u32 tcp_seq;

    __u32 current_offset_in_request_fragment;
    kafka_transaction_batch_entry_t base;
} kafka_transaction_t;

// kafka_telemetry_t is used to hold the Kafka kernel telemetry.
typedef struct {
    __u64 topic_name_exceeds_max_size;
    // Count of topic name sizes that are divided into buckets.
    __u64 topic_name_size_buckets[KAFKA_TELEMETRY_TOPIC_NAME_NUM_OF_BUCKETS + 1];
} kafka_telemetry_t;

#endif
