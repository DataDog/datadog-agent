#ifndef __PROTOCOL_ROUTING_H
#define __PROTOCOL_ROUTING_H

#include "ktypes.h"
#include "protocols/classification/defs.h"
#include "protocols/classification/stack-helpers.h"
#include "protocols/classification/routing-helpers.h"

// This entry point is needed to bypass a memory limit on socket filters.
// There is a limitation on number of instructions can be attached to a socket filter,
// as we classify more protocols, we reached that limit, thus we workaround it
// by using tail call.
BPF_PROG_ARRAY(classification_progs, CLASSIFICATION_PROG_MAX)

// This function essentially encodes all routing aspects of tail-calls.
//
// For example, if this function gets called from `CLASSIFICATION_QUEUES_PROG`
// the return value will be likely `CLASSIFICATION_DBS_PROG` (which is the next
// program that belongs to the same stack layer), but that depends whether or
// not the application layer protocol is known at the time of the call. When a
// certain protocol layer is known, the function "skips" to the entry-point of
// the next layer and so forth.
static __always_inline classification_prog_t __get_next_program(classification_context_t *classification_ctx) {
    classification_prog_t current_program = classification_ctx->routing_current_program;
    u16 current_layer_bit = get_current_program_layer(current_program);
    bool current_layer_known = classification_ctx->routing_skip_layers & current_layer_bit;

    if (has_available_program(current_program) && !current_layer_known) {
        // advance to the next program belonging to this protocol layer
        return current_program+1;
    }

    // there are not other programs belonging to the same layer to be executed,
    // so we skip to the first program of the next layer that is not known
    classification_ctx->routing_skip_layers |= current_layer_bit;
    return next_layer_entrypoint(classification_ctx);
}

static __always_inline void classification_next_program(struct __sk_buff *skb, classification_context_t *classification_ctx) {
    classification_prog_t next_program = __get_next_program(classification_ctx);
    if (next_program == CLASSIFICATION_PROG_UNKNOWN || next_program == CLASSIFICATION_PROG_MAX) {
        log_debug("classification tail-call: skb=%p tail-end", skb);
        return;
    }

    log_debug("classification tail-call: skb=%p from=%d to=%d", skb, classification_ctx->routing_current_program, next_program);
    classification_ctx->routing_current_program = next_program;
    bpf_tail_call_compat(skb, &classification_progs, next_program);
}

// init_routing_cache is executed once per packet, at the socket filter entrypoint.
// the information loaded here is persisted throughout multiple bpf tail-calls and
// it's used for the purposes of figuring out which BPF program to execute next.
static __always_inline void init_routing_cache(classification_context_t *classification_ctx, protocol_stack_t *stack) {
    classification_ctx->routing_skip_layers = 0;
    classification_ctx->routing_current_program = CLASSIFICATION_PROG_UNKNOWN;

    // No protocol stack, nothing to mark for skipping
    if (!stack) {
        return;
    }

    // We skip a given layer in two cases:
    // 1) If the protocol for that layer is known
    // 2) If there are no programs registered for that layer
    if (stack->layer_application || !has_available_program(__PROG_APPLICATION)) {
        classification_ctx->routing_skip_layers |= LAYER_APPLICATION_BIT;
    }
    if (stack->layer_api || !has_available_program(__PROG_API)) {
        classification_ctx->routing_skip_layers |= LAYER_API_BIT;
    }
    if (stack->layer_encryption || !has_available_program(__PROG_ENCRYPTION)) {
        classification_ctx->routing_skip_layers |= LAYER_ENCRYPTION_BIT;
    }
}

#endif
