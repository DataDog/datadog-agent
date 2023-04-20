#ifndef __PROTOCOL_ROUTING_H
#define __PROTOCOL_ROUTING_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"
#include "protocols/classification/routing-helpers.h"

// These macros are meant to improve the readability of the `__get_next_program` function below
// LAYER_ENTRYPOINT(program) designates the first (socket-filter) program to be executed for a given layer
#define LAYER_ENTRYPOINT(program)                               \
    do {                                                        \
        if (current_program == CLASSIFICATION_PROG_UNKNOWN) {   \
            return program;                                     \
        }                                                       \
    } while(0)

// LAYER_TRANSITION designates a sequence of socket-filter programs within a given layer
#define LAYER_TRANSITION(from, to)              \
    do {                                        \
        if (current_program == from) {          \
            return to;                          \
        }                                       \
    } while(0)

#define LAYER_END(layer_mask)                           \
    do {                                                \
        current_program = CLASSIFICATION_PROG_UNKNOWN;  \
        *known_layers |= layer_mask;                    \
    } while(0)

// This function essentially encodes all routing aspects of tail-calls. For
// example, if this function gets called from `CLASSIFICATION_QUEUES_PROG` the
// return value will be likely `CLASSIFICATION_DBS_PROG` (which is the next
// program that belongs to the same stack layer), but that depends whether or
// not the application layer protocol is known at the time of the call. When a
// certain protocol layer is known, the function "skips" to the entry-point of
// the next layer and so forth.
static __always_inline classification_prog_t __get_next_program(struct __sk_buff *skb) {
    u16 *known_layers = __get_layer_cache(skb);
    classification_prog_t current_program = *__get_program_cache(skb);

    if (*known_layers&LAYER_APPLICATION_BIT) {
        goto api;
    }

    // Add Application-layer routing here
    LAYER_ENTRYPOINT(CLASSIFICATION_QUEUES_PROG);
    LAYER_TRANSITION(CLASSIFICATION_QUEUES_PROG, CLASSIFICATION_DBS_PROG);
    LAYER_END(LAYER_APPLICATION_BIT);

 api:
    if (*known_layers&LAYER_API_BIT) {
        goto encryption;
    }

    // Add API-layer routing here
    LAYER_END(LAYER_API_BIT);

 encryption:
    // Add Encryption-layer routing here
    LAYER_END(LAYER_ENCRYPTION_BIT);
    return CLASSIFICATION_PROG_UNKNOWN;
}

static __always_inline void classification_next_program(struct __sk_buff *skb) {
    classification_prog_t next_program = __get_next_program(skb);
    if (next_program == CLASSIFICATION_PROG_UNKNOWN) {
        log_debug("classification tail-call: skb=%llu tail-end\n", skb);
        return;
    }

    // update the program "cache"
    classification_prog_t *current_program = __get_program_cache(skb);
    log_debug("classification tail-call: skb=%llu from=%d to=%d\n", skb, *current_program, next_program);
    *current_program = next_program;

    bpf_tail_call_compat(skb, &classification_progs, next_program);
}

#endif
