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

// This struct is used in the map lookup that returns the active batch for a certain CPU core
typedef struct {
    __u32 cpu;
    // page_num can be obtained from (kafka_batch_state_t->idx % KAFKA_BATCHES_PER_CPU)
    __u32 page_num;
} kafka_batch_key_t;

typedef struct {
    conn_tuple_t tup;

    __u16 request_api_key;
    __u16 request_api_version;
    __u32 correlation_id;

    // this field is used to disambiguate segments in the context of keep-alives
    // we populate it with the TCP seq number of the request and then the response segments
    __u32 tcp_seq;

    __u32 current_offset_in_request_fragment;
    char topic_name[TOPIC_NAME_MAX_STRING_SIZE];
} kafka_transaction_batch_entry_t;

// Kafka transaction information associated to a certain socket (tuple_t)
typedef struct {
    char request_fragment[KAFKA_BUFFER_SIZE];
    kafka_transaction_batch_entry_t base;
} kafka_transaction_t;

//typedef struct {
//    // idx is a monotonic counter used for uniquely determining a batch within a CPU core
//    // this is useful for detecting race conditions that result in a batch being overridden
//    // before it gets consumed from userspace
//    __u64 idx;
//    // idx_to_flush is used to track which batches were flushed to userspace
//    // * if idx_to_flush == idx, the current index is still being appended to;
//    // * if idx_to_flush < idx, the batch at idx_to_notify needs to be sent to userspace;
//    // (note that idx will never be less than idx_to_flush);
//    __u64 idx_to_flush;
//} kafka_batch_state_t;
//
//typedef struct {
//    __u64 idx;
//    __u8 pos;
//    kafka_transaction_batch_entry_t txs[KAFKA_BATCH_SIZE];
//} kafka_batch_t;

#endif
