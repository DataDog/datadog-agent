#ifndef __PROTOCOL_CLASSIFICATION_DEFS_H
#define __PROTOCOL_CLASSIFICATION_DEFS_H

#include "ktypes.h"
#include "compiler.h"
#include "bpf_helpers_custom.h"

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

// The maximum number of protocols per stack layer
#define MAX_ENTRIES_PER_LAYER 255

#define LAYER_API_BIT         (1 << 13)
#define LAYER_APPLICATION_BIT (1 << 14)
#define LAYER_ENCRYPTION_BIT  (1 << 15)

#define LAYER_API_MAX         (LAYER_API_BIT + MAX_ENTRIES_PER_LAYER)
#define LAYER_APPLICATION_MAX (LAYER_APPLICATION_BIT + MAX_ENTRIES_PER_LAYER)
#define LAYER_ENCRYPTION_MAX  (LAYER_ENCRYPTION_BIT + MAX_ENTRIES_PER_LAYER)

#define FLAG_FULLY_CLASSIFIED       1 << 0
#define FLAG_USM_ENABLED            1 << 1
#define FLAG_NPM_ENABLED            1 << 2
#define FLAG_TCP_CLOSE_DELETION     1 << 3
#define FLAG_SOCKET_FILTER_DELETION 1 << 4

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
    PROTOCOL_GRPC,
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

// This enum represents all existing protocol layers
//
// Each `protocol_t` entry is implicitly associated to a single
// `protocol_layer_t` value (see notes above).
//
//In order to determine which `protocol_layer_t` a `protocol_t` belongs to,
// users can call `get_protocol_layer`
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

// This wrapper type is being added so we can associate an update timestamp to
// each `protocol_stack_t` value.
//
// This timestamp acts as a heartbeat and it is used only in userspace to detect stale
// entries in the `connection_protocol` map which is currently leaking in some scenarios.
//
// Why create a wrapper type?
//
// `protocol_stack_t` is embedded in the `conn_stats_t` type, which is used
// across the whole NPM kernel code. If we added the 64-bit timestamp field
// directly to `protocol_stack_t`, we would go from 4 bytes to 12 bytes, which
// bloats the eBPF stack size of some NPM probes.  Using the wrapper type
// prevents that, because we pretty much only store the wrapper type in the
// connection_protocol map, but elsewhere in the code we're still using
// protocol_stack_t, so this is change is "transparent" to most of the code.
typedef struct {
    protocol_stack_t stack;
    __u64 updated;
} protocol_stack_wrapper_t;

typedef enum {
    CLASSIFICATION_PROG_UNKNOWN = 0,
    __PROG_APPLICATION,
    // Application classification programs go here
    CLASSIFICATION_QUEUES_PROG,
    CLASSIFICATION_DBS_PROG,
    __PROG_API,
    // API classification programs go here
    CLASSIFICATION_GRPC_PROG,
    __PROG_ENCRYPTION,
    // Encryption classification programs go here
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
    PROG_HTTP2_FRAME_PARSER,
    PROG_KAFKA,
    PROG_GRPC,
    // Add before this value.
    PROG_MAX,
} protocol_prog_t;

typedef enum {
    TLS_PROG_UNKNOWN = 0,
    // Add TLS uprobe tail calls here
    TLS_PROG_MAX,
} tls_prog_t;

#endif
