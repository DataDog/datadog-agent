#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

typedef enum {
    KAFKA_PRODUCE = 0,
    KAFKA_FETCH
} __attribute__ ((packed)) kafka_operation_t;

typedef struct {
    int32_t message_size;
    int16_t api_key;
    int16_t api_version;
    int32_t correlation_id;
    int16_t client_id_size;
} __attribute__ ((packed)) kafka_header_t;

typedef struct {
    int16_t buffer_size;
    int16_t offset;
    const char* buffer;
    kafka_header_t header;
} kafka_context_t;

#endif
