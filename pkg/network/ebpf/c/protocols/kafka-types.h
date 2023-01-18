#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

typedef enum
{
    KAFKA_PRODUCE = 0,
    KAFKA_FETCH
} kafka_operation_t;

typedef struct {
    int32_t message_size;
    int16_t api_key;
    int16_t api_version;
    int32_t correlation_id;
    int16_t client_id_size;
} kafka_header;

typedef struct {
    uint32_t offset;
} kafka_context;

#endif
