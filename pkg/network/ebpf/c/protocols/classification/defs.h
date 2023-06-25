#ifndef __PROTOCOL_CLASSIFICATION_DEFS_H
#define __PROTOCOL_CLASSIFICATION_DEFS_H

#include "ktypes.h"

#include "protocols/amqp/defs.h"
#include "protocols/http/classification-defs.h"
#include "protocols/http2/defs.h"
#include "protocols/mongo/defs.h"
#include "protocols/mysql/defs.h"
#include "protocols/redis/defs.h"
#include "protocols/sql/defs.h"

// Represents the max buffer size required to classify protocols .
// We need to round it to be multiplication of 16 since we are reading blocks of 16 bytes in read_into_buffer_skb_all_kernels.
// ATM, it is HTTP2_MARKER_SIZE + 8 bytes for padding,
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
    PROTOCOL_KAFKA,
    PROTOCOL_MONGO,
    PROTOCOL_POSTGRES,
    PROTOCOL_AMQP,
    PROTOCOL_REDIS,
    PROTOCOL_MYSQL,
    //  Add new protocols before that line.
    MAX_PROTOCOLS,
    __MAX_UINT8 = 255,
} __attribute__ ((packed)) protocol_t;

typedef enum {
    CLASSIFICATION_QUEUES_PROG = 0,
    CLASSIFICATION_DBS_PROG,
    // Add before this value.
    CLASSIFICATION_PROG_MAX,
} classification_prog_t;

typedef enum {
    DISPATCHER_KAFKA_PROG = 0,
    // Add before this value.
    DISPATCHER_PROG_MAX,
} dispatcher_prog_t;

#endif
