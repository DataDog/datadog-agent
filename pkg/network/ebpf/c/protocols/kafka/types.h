#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

typedef enum {
    KAFKA_PRODUCE = 0,
    KAFKA_FETCH
} __attribute__ ((packed)) kafka_operation_t;

typedef struct {
    s32 message_size;
    s16 api_key;
    s16 api_version;
    s32 correlation_id;
    s16 client_id_size;
} __attribute__ ((packed)) kafka_header_t;

#endif
