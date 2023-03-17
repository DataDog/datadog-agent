#ifndef __PROTOCOL_ROUTING_H
#define __PROTOCOL_ROUTING_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"

#define LAYER_CACHE_CB_OFFSET 0
#define PROGRAM_CACHE_CB_OFFSET 3
#define CLASSIFICATION_PROG_UNKNOWN CLASSIFICATION_PROG_MAX

// The purpose of caching all known (classified) layers and the current program in the skb->cb
// field is to avoid one eBPF map lookup per tail call
static __always_inline u16* __get_layer_cache(struct __sk_buff *skb) {
    return (u16 *)&skb->cb[LAYER_CACHE_CB_OFFSET];
}

static __always_inline classification_prog_t* __get_program_cache(struct __sk_buff *skb) {
    return (classification_prog_t *)&skb->cb[PROGRAM_CACHE_CB_OFFSET];
}

// This function essentially encodes all routing aspects of tail-calls. For
// example, if this function gets called from `CLASSIFICATION_QUEUES_PROG` the
// return value will be likely `CLASSIFICATION_DBS_PROG` (which is the next
// program that belongs to the same stack layer), but that depends whether or
// not the application layer protocol is known at the time of the call. When a
// certain protocol layer is known, the function "skips" to the entry-point of
// the next layer and so forth.
// TODO: maybe come-up with a macro to define these switch statements in a more
// friendly way?
static __always_inline classification_prog_t __get_next_program(struct __sk_buff *skb) {
    u16 known_layers = *__get_layer_cache(skb);
    classification_prog_t current_program = *__get_program_cache(skb);

    if (known_layers&LAYER_APPLICATION_BIT) {
        goto api;
    }
    // add application-layer program routing here
    switch(current_program) {
    case CLASSIFICATION_QUEUES_PROG:
        return CLASSIFICATION_DBS_PROG;
    default:
        // add here the entry-point of the application layer
        return CLASSIFICATION_QUEUES_PROG;
    }

 api:
    if (known_layers&LAYER_API_BIT) {
        goto encryption;
    }
    // add api-layer program routing here
    switch(current_program) {
    default:
        // add here the entry-point of the api layer
        return CLASSIFICATION_PROG_UNKNOWN;
    }

 encryption:
    if (known_layers&LAYER_ENCRYPTION_BIT) {
        return CLASSIFICATION_PROG_UNKNOWN;
    }
    // add encryption-layer program routing here
    switch(current_program) {
    default:
        // add here the entry-point of the encryption layer
        return CLASSIFICATION_PROG_UNKNOWN;
    }
}

static __always_inline void classification_next_program(struct __sk_buff *skb) {
    classification_prog_t next_program = __get_next_program(skb);
    if (next_program == CLASSIFICATION_PROG_UNKNOWN) {
        return;
    }

    // update the program "cache"
    classification_prog_t *current_program = __get_program_cache(skb);
    log_debug("classification tail-call: skb=%llu from=%d to=%d\n", skb, *current_program, next_program);
    *current_program = next_program;

    bpf_tail_call_compat(skb, &classification_progs, next_program);
}

#endif
