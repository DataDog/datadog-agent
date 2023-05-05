#ifndef __PROTOCOL_ROUTING_H
#define __PROTOCOL_ROUTING_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"
#include "protocols/classification/routing-helpers.h"

// This function essentially encodes all routing aspects of tail-calls.
//
// For example, if this function gets called from `CLASSIFICATION_QUEUES_PROG`
// the return value will be likely `CLASSIFICATION_DBS_PROG` (which is the next
// program that belongs to the same stack layer), but that depends whether or
// not the application layer protocol is known at the time of the call. When a
// certain protocol layer is known, the function "skips" to the entry-point of
// the next layer and so forth.
static __always_inline classification_prog_t __get_next_program(usm_context_t *usm_ctx) {
    classification_prog_t current_program = usm_ctx->routing_current_program;
    u16 current_layer_bit = get_current_program_layer(current_program);
    bool current_layer_known = usm_ctx->routing_skip_layers & current_layer_bit;

    if (is_last_program_from_layer(current_program) || current_layer_known) {
        // there are not other programs for this layer to be executed, so we
        // skip to the first program of the next layer that is not known
        usm_ctx->routing_skip_layers |= current_layer_bit;
        return next_layer_entrypoint(usm_ctx);
    }

    return current_program+1;
}

static __always_inline void classification_next_program(struct __sk_buff *skb, usm_context_t *usm_ctx) {
    classification_prog_t next_program = __get_next_program(usm_ctx);
    if (next_program == CLASSIFICATION_PROG_UNKNOWN || next_program == CLASSIFICATION_PROG_MAX) {
        log_debug("classification tail-call: skb=%llu tail-end\n", skb);
        return;
    }

    log_debug("classification tail-call: skb=%llu from=%d to=%d\n", skb, usm_ctx->routing_current_program, next_program);
    usm_ctx->routing_current_program = next_program;
    bpf_tail_call_compat(skb, &classification_progs, next_program);
}

// init_routing_cache is executed once per packet, at the socket filter entrypoint.
// the information loaded here is persisted throughout multiple bpf tail-calls and
// it's used for the purposes of figuring out which BPF program to execute next.
static __always_inline void init_routing_cache(usm_context_t *usm_ctx, protocol_stack_t *stack) {
    usm_ctx->routing_skip_layers = 0;
    usm_ctx->routing_current_program = CLASSIFICATION_PROG_UNKNOWN;

    // We skip a given layer in two cases:
    // 1) If the protocol for that layer is known
    // 2) If there are no programs registered for that layer
    if (stack->layer_application || is_application_layer_empty()) {
        usm_ctx->routing_skip_layers |= LAYER_APPLICATION_BIT;
    }
    if (stack->layer_api || is_api_layer_empty()) {
        usm_ctx->routing_skip_layers |= LAYER_API_BIT;
    }
    if (stack->layer_encryption || is_encryption_layer_empty()) {
        usm_ctx->routing_skip_layers |= LAYER_ENCRYPTION_BIT;
    }
}

#endif
