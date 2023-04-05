#ifndef __PROTOCOL_ROUTING_H
#define __PROTOCOL_ROUTING_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"

// TODO: Move this forward to avoid conflicts with other eBPF programs
#define LAYER_CACHE_CB_OFFSET 0
#define PROGRAM_CACHE_CB_OFFSET 3

#define LAYER_ENTRYPOINT(program)                             \
    do {                                                        \
        if (current_program == CLASSIFICATION_PROG_UNKNOWN) {   \
            return program;                                     \
        }                                                       \
    } while(0)

#define LAYER_TRANSITION(a, b)                \
    do {                                        \
        if (current_program == a) {             \
            return b;                           \
        }                                       \
    } while(0)

#define LAYER_END(layer_mask)                           \
    do {                                                \
        current_program = CLASSIFICATION_PROG_UNKNOWN;  \
        *known_layers |= layer_mask;                    \
    } while(0)

// The purpose of caching all known (classified) layers and the current program in the skb->cb
// field is to avoid one eBPF map lookup per tail call
// (Note that skb->cb data is persisted across tail-calls)
static __always_inline u16* __get_layer_cache(struct __sk_buff *skb) {
    return (u16 *)&skb->cb[LAYER_CACHE_CB_OFFSET];
}

static __always_inline classification_prog_t* __get_program_cache(struct __sk_buff *skb) {
    return (classification_prog_t *)&skb->cb[PROGRAM_CACHE_CB_OFFSET];
}

static __always_inline void __init_layer_cache(struct __sk_buff *skb, protocol_stack_t *stack) {
    u16 *known_layers = __get_layer_cache(skb);
    if (is_fully_classified(stack)) {
        *known_layers = (LAYER_APPLICATION_BIT|LAYER_API_BIT|LAYER_ENCRYPTION_BIT);
        return;
    }

    if (stack->layer_application) {
        *known_layers |= LAYER_APPLICATION_BIT;
    }
    if (stack->layer_api) {
        *known_layers |= LAYER_API_BIT;
    }
    if (stack->layer_encryption) {
        *known_layers |= LAYER_ENCRYPTION_BIT;
    }
}

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
