#ifndef __PROTOCOL_ROUTING_HELPERS_H
#define __PROTOCOL_ROUTING_HELPERS_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"

// TODO: Move these bytes forward to avoid conflicts with other eBPF programs
#define LAYER_CACHE_CB_OFFSET 0
#define PROGRAM_CACHE_CB_OFFSET 3

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

#endif
