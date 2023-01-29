#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

typedef enum
{
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
    const char* buffer;
    uint32_t buffer_size;
    uint32_t offset;
    kafka_header_t header;
} __attribute__ ((packed)) kafka_context_t;

#endif
