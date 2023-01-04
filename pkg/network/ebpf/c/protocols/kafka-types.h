#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

typedef enum
{
    KAFKA_PRODUCE = 0,
    KAFKA_FETCH
} kafka_operation_t;

// The maximum request API version for fetch request is 13
// The maximum request API version for produce is 9
// So setting it to the maximum between the 2
// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MAX_SUPPORTED_REQUEST_API_VERSION 13

#define CLIENT_ID_MAX_STRING_SIZE 30

#endif
