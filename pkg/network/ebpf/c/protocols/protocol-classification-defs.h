#ifndef __PROTOCOL_CLASSIFICATION_DEFS_H
#define __PROTOCOL_CLASSIFICATION_DEFS_H

#include <linux/types.h>

// Represents the max buffer size required to classify protocols .
// We need to round it to be multiplication of 16 since we are reading blocks of 16 bytes in read_into_buffer_skb_all_kernels.
// ATM, it is HTTP2_MARKER_SIZE + 8 bytes for padding,
#define CLASSIFICATION_MAX_BUFFER (HTTP2_MARKER_SIZE + 8)

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "HTTP/2 Connection Preface" section
#define HTTP2_MARKER_SIZE 24

// The minimal HTTP response has 17 characters: HTTP/1.1 200 OK\r\n
// The minimal HTTP request has 16 characters: GET x HTTP/1.1\r\n
#define HTTP_MIN_SIZE 16

// RabbitMQ supported classes.
// Ref: https://www.rabbitmq.com/resources/specs/amqp0-9-1.pdf
#define AMQP_CONNECTION_CLASS 10
#define AMQP_BASIC_CLASS 60

// RabbitMQ supported connections.
#define AMQP_METHOD_CONNECTION_START 10
#define AMQP_METHOD_CONNECTION_START_OK 11

// RabbitMQ supported methods types.
#define AMQP_METHOD_CONSUME 20
#define AMQP_METHOD_PUBLISH 40
#define AMQP_METHOD_DELIVER 60
#define AMQP_FRAME_METHOD_TYPE 1

#define AMQP_MIN_FRAME_LENGTH 8
#define AMQP_MIN_PAYLOAD_LENGTH 11

#define REDIS_MIN_FRAME_LENGTH 3

// The enum below represents all different protocols we know to classify.
// We set the size of the enum to be 8 bits, by adding max value (max uint8 which is 255) and
// `__attribute__ ((packed))` to tell the compiler to use as minimum bits as needed. Due to our max
// value we will use 8 bits for the enum.
typedef enum {
    PROTOCOL_UNCLASSIFIED = 0,
    PROTOCOL_UNKNOWN,
    PROTOCOL_HTTP,
    PROTOCOL_HTTP2,
    PROTOCOL_TLS,
    PROTOCOL_AMQP = 8,
    PROTOCOL_REDIS = 9,
    //  Add new protocols before that line.
    MAX_PROTOCOLS,
    __MAX_UINT8 = 255,
} __attribute__ ((packed)) protocol_t;

#endif
