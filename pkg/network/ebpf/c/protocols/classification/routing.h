#ifndef __PROTOCOL_ROUTING_H
#define __PROTOCOL_ROUTING_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"

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
static __always_inline classification_prog_t __get_next_program(usm_context_t *usm_ctx) {
    u16 *known_layers = &usm_ctx->routing_known_layers;
    classification_prog_t current_program = usm_ctx->routing_current_program;

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

static __always_inline void classification_next_program(struct __sk_buff *skb, usm_context_t *usm_ctx) {
    classification_prog_t next_program = __get_next_program(usm_ctx);
    if (next_program == CLASSIFICATION_PROG_UNKNOWN) {
        log_debug("classification tail-call: skb=%llu tail-end\n", skb);
        return;
    }

    // update the program "cache"
    log_debug("classification tail-call: skb=%llu from=%d to=%d\n", skb, usm_ctx->routing_current_program, next_program);
    usm_ctx->routing_current_program = next_program;

    bpf_tail_call_compat(skb, &classification_progs, next_program);
}

static __always_inline void init_routing_cache(usm_context_t *usm_ctx, protocol_stack_t *stack) {
    usm_ctx->routing_known_layers = 0;
    usm_ctx->routing_current_program = CLASSIFICATION_PROG_UNKNOWN;

    if (is_fully_classified(stack)) {
        usm_ctx->routing_known_layers = (LAYER_APPLICATION_BIT|LAYER_API_BIT|LAYER_ENCRYPTION_BIT);
        return;
    }

    if (stack->layer_application) {
        usm_ctx->routing_known_layers |= LAYER_APPLICATION_BIT;
    }
    if (stack->layer_api) {
        usm_ctx->routing_known_layers |= LAYER_API_BIT;
    }
    if (stack->layer_encryption) {
        usm_ctx->routing_known_layers |= LAYER_ENCRYPTION_BIT;
    }
}

#endif
