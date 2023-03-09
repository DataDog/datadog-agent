#ifndef __PROTOCOL_CLASSIFICATION_DEFS_H
#define __PROTOCOL_CLASSIFICATION_DEFS_H

#include "ktypes.h"
#include "compiler.h"

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
#define CLASSIFICATION_MAX_BUFFER (HTTP2_MARKER_SIZE + 8)

// The maximum number of protocols per stack layer
#define MAX_ENTRIES_PER_LAYER 255

#define LAYER_API_BIT         (1 << 13)
#define LAYER_APPLICATION_BIT (1 << 14)
#define LAYER_ENCRYPTION_BIT  (1 << 15)

#define LAYER_API_MAX         (LAYER_API_BIT + MAX_ENTRIES_PER_LAYER)
#define LAYER_APPLICATION_MAX (LAYER_APPLICATION_BIT + MAX_ENTRIES_PER_LAYER)
#define LAYER_ENCRYPTION_MAX  (LAYER_ENCRYPTION_BIT + MAX_ENTRIES_PER_LAYER)

#define FLAG_FULLY_CLASSIFIED 1

// The enum below represents all different protocols we're able to
// classify. Entries are segmented such that it is possible to infer the
// protocol layer from its value. A `protocol_t` value can be represented by
// 16-bits which are encoded like the following:
//
// * Bits 0-7   : Represent the protocol number within a given layer
// * Bits 8-12  : Unused
// * Bits 13-15 : Designates the protocol layer
typedef enum {
    PROTOCOL_UNKNOWN = 0,

    __LAYER_API_MIN = LAYER_API_BIT,
    // Add API protocols here (eg. gRPC)
    __LAYER_API_MAX = LAYER_API_MAX,

    __LAYER_APPLICATION_MIN = LAYER_APPLICATION_BIT,
    //  Add application protocols below (eg. HTTP)
    PROTOCOL_HTTP,
    PROTOCOL_HTTP2,
    PROTOCOL_KAFKA,
    PROTOCOL_MONGO,
    PROTOCOL_POSTGRES,
    PROTOCOL_AMQP,
    PROTOCOL_REDIS,
    PROTOCOL_MYSQL,
    __LAYER_APPLICATION_MAX = LAYER_APPLICATION_MAX,

    __LAYER_ENCRYPTION_MIN = LAYER_ENCRYPTION_BIT,
    //  Add encryption protocols below (eg. TLS)
    PROTOCOL_TLS,
    __LAYER_ENCRYPTION_MAX = LAYER_ENCRYPTION_MAX,
} __attribute__ ((packed)) protocol_t;

typedef enum {
    LAYER_UNKNOWN,
    LAYER_API,
    LAYER_APPLICATION,
    LAYER_ENCRYPTION,
} __attribute__ ((packed)) protocol_layer_t;

typedef struct {
    __u8 layer_api;
    __u8 layer_application;
    __u8 layer_encryption;
    __u8 flags;
} protocol_stack_t;

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

typedef enum {
    PROG_UNKNOWN = 0,
    PROG_HTTP,
    PROG_HTTP2,
    PROG_KAFKA,
    // Add before this value.
    PROG_MAX,
} protocol_prog_t;

__maybe_unused static __always_inline protocol_prog_t protocol_to_program(protocol_t proto) {
    switch(proto) {
    case PROTOCOL_HTTP:
        return PROG_HTTP;
    case PROTOCOL_HTTP2:
        return PROG_HTTP2;
    case PROTOCOL_KAFKA:
        return PROG_KAFKA;
    default:
        log_debug("protocol doesn't have a matching program: %d\n", proto);
        return PROG_UNKNOWN;
    }
}

#endif
