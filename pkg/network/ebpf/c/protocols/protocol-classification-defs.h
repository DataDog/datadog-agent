#ifndef __PROTOCOL_CLASSIFICATION_DEFS_H
#define __PROTOCOL_CLASSIFICATION_DEFS_H

#include <linux/types.h>

#include "amqp-defs.h"
#include "mongo-defs.h"

// Represents the max buffer size required to classify protocols.
// ATM, it is HTTP2_MARKER_SIZE.
#define CLASSIFICATION_MAX_BUFFER (HTTP2_MARKER_SIZE)

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "HTTP/2 Connection Preface" section
#define HTTP2_MARKER_SIZE 24

// The minimal HTTP response has 17 characters: HTTP/1.1 200 OK\r\n
// The minimal HTTP request has 16 characters: GET x HTTP/1.1\r\n
#define HTTP_MIN_SIZE 16

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
    PROTOCOL_MONGO = 6,
    PROTOCOL_AMQP = 8,
    PROTOCOL_REDIS = 9,
    //  Add new protocols before that line.
    MAX_PROTOCOLS,
    __MAX_UINT8 = 255,
} __attribute__ ((packed)) protocol_t;

#endif
