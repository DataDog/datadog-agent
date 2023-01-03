#ifndef __PROTOCOL_CLASSIFICATION_DEFS_H
#define __PROTOCOL_CLASSIFICATION_DEFS_H

#include <linux/types.h>

#include "amqp-defs.h"
#include "http2-defs.h"
#include "http-classification-defs.h"
#include "mongo-defs.h"
#include "redis-defs.h"

// Represents the max buffer size required to classify protocols.
// ATM, it is HTTP2_MARKER_SIZE.
#define CLASSIFICATION_MAX_BUFFER (HTTP2_MARKER_SIZE)

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
