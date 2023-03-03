#ifndef __PROTOCOL_STACK_HELPERS_H
#define __PROTOCOL_STACK_HELPERS_H

#include "ktypes.h"
#include "protocols/classification/defs.h"

static __always_inline protocol_layer_t protocol_layer_get(protocol_t proto) {
    u16 layer_bit = proto&(LAYER_API_BIT|LAYER_APPLICATION_BIT|LAYER_ENCRYPTION_BIT);

    switch(layer_bit) {
    case LAYER_API_BIT:
        return LAYER_API;
    case LAYER_APPLICATION_BIT:
        return LAYER_APPLICATION;
    case LAYER_ENCRYPTION_BIT:
        return LAYER_ENCRYPTION;
    }

    return LAYER_UNKNOWN;
}

static __always_inline void protocol_set(protocol_stack_t *stack, protocol_t proto) {
    if (!stack || proto == PROTOCOL_UNKNOWN) {
        return;
    }

    protocol_layer_t layer = protocol_layer_get(proto);
    if (!layer) {
        return;
    }

    // this is the the number of the protocol without the layer bit set
    __u8 proto_num = (__u8)proto;
    switch(layer) {
    case LAYER_API:
        stack->layer_api = proto_num;
        return;
    case LAYER_APPLICATION:
        stack->layer_application = proto_num;
        return;
    case LAYER_ENCRYPTION:
        stack->layer_encryption = proto_num;
        return;
    default:
        return;
    }
}

static __always_inline protocol_t protocol_get(protocol_stack_t *stack, protocol_layer_t layer) {
    if (!stack) {
        return PROTOCOL_UNKNOWN;
    }

    __u16 proto_num = 0;
    __u16 layer_bit = 0;
    switch(layer) {
    case LAYER_API:
        proto_num = stack->layer_api;
        layer_bit = LAYER_API_BIT;
        break;
    case LAYER_APPLICATION:
        proto_num = stack->layer_application;
        layer_bit = LAYER_APPLICATION_BIT;
        break;
    case LAYER_ENCRYPTION:
        proto_num = stack->layer_encryption;
        layer_bit = LAYER_ENCRYPTION_BIT;
        break;
    default:
        break;
    }

    if (!proto_num) {
        return PROTOCOL_UNKNOWN;
    }

    return proto_num | layer_bit;
}

static __always_inline bool protocol_layer_known(protocol_stack_t *stack, protocol_layer_t layer) {
    if (!stack) {
        return false;
    }

    protocol_t proto = protocol_get(stack, layer);
    return proto != PROTOCOL_UNKNOWN;
}

static __always_inline bool is_fully_classified(protocol_stack_t *stack) {
    if (!stack) {
        return false;
    }

    return stack->flags&FLAG_FULLY_CLASSIFIED ||
        (stack->layer_api > 0 &&
         stack->layer_application > 0 &&
         stack->layer_encryption > 0);
}

static __always_inline void mark_as_fully_classified(protocol_stack_t *stack) {
    if (!stack) {
        return;
    }

    stack->flags |= FLAG_FULLY_CLASSIFIED;
}

static __always_inline protocol_layer_t protocol_next_layer(protocol_stack_t *stack, protocol_layer_t current_layer) {
    if (!stack || is_fully_classified(stack)) {
        return LAYER_UNKNOWN;
    }

    switch(current_layer) {
    case LAYER_APPLICATION:
        goto api;
    case LAYER_API:
        goto encryption;
    default:
        break;
    }

    if (!stack->layer_application) {
        return LAYER_APPLICATION;
    }
 api:
    if (!stack->layer_api) {
        return LAYER_API;
    }
 encryption:
    if (!stack->layer_encryption) {
        return LAYER_ENCRYPTION;
    }

    return LAYER_UNKNOWN;
}

#endif
