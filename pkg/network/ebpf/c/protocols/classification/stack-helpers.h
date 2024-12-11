#ifndef __PROTOCOL_STACK_HELPERS_H
#define __PROTOCOL_STACK_HELPERS_H

#include "ktypes.h"
#include "protocols/classification/defs.h"

// get_protocol_layer retrieves the `protocol_layer_t` associated to the given `protocol_t`.
// Example:
// get_protocol_layer(PROTOCOL_HTTP) => LAYER_APPLICATION
// get_protocol_layer(PROTOCOL_TLS)  => LAYER_ENCRYPTION
static __always_inline protocol_layer_t get_protocol_layer(protocol_t proto) {
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

// set_protocol adds `proto` to the given `stack`
static __always_inline void set_protocol(protocol_stack_t *stack, protocol_t proto) {
    if (!stack || proto == PROTOCOL_UNKNOWN) {
        return;
    }

    protocol_layer_t layer = get_protocol_layer(proto);
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

// is_fully_classified returns true if all layers are set or if
// `mark_as_fully_classified` was previously called for this `stack`
static __always_inline bool is_fully_classified(protocol_stack_t *stack) {
    if (!stack) {
        return false;
    }

    return stack->flags&FLAG_FULLY_CLASSIFIED ||
        (stack->layer_api > 0 &&
         stack->layer_application > 0 &&
         stack->layer_encryption > 0);
}

// mark_as_fully_classified is intended to be used as an "optimization" helper
// so a protocol stack can be treated as fully classified even if some layers
// are missing.
// For example, if we determine from a socket-filter program that a
// connection has Kafka traffic, we can call `set_protocol(stack, PROTOCOL_KAFKA)`
// and then `mark_as_fully_classified(stack)` to indicate that no further
// classification attempts are necessary, since there can't be an encryption
// layer protocol nor an API-level protocol above Kafka.
static __always_inline void mark_as_fully_classified(protocol_stack_t *stack) {
    if (!stack) {
        return;
    }

    stack->flags |= FLAG_FULLY_CLASSIFIED;
}


// get_protocol_from_stack returns the `protocol_t` value that belongs to the given `layer`
// Example: If we had a `protocol_stack_t` with HTTP, calling `get_protocol_from_stack(stack, LAYER_APPLICATION)
// would return PROTOCOL_HTTP;
__maybe_unused static __always_inline protocol_t get_protocol_from_stack(protocol_stack_t *stack, protocol_layer_t layer) {
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

// is_protocol_layer_known returns true when `stack` contains a protocol at the given `layer`
__maybe_unused static __always_inline bool is_protocol_layer_known(protocol_stack_t *stack, protocol_layer_t layer) {
    if (!stack) {
        return false;
    }

    protocol_t proto = get_protocol_from_stack(stack, layer);
    return proto != PROTOCOL_UNKNOWN;
}

// merge_protocol_stacks modifies `this` by merging it with `that`
static __always_inline void merge_protocol_stacks(protocol_stack_t *this, protocol_stack_t *that) {
    if (!this || !that) {
        return;
    }

    if (!this->layer_api) {
        this->layer_api = that->layer_api;
    }
    if (!this->layer_application) {
        this->layer_application = that->layer_application;
    }
    if (!this->layer_encryption) {
        this->layer_encryption = that->layer_encryption;
    }

    this->flags |= that->flags;
}

static __always_inline void set_protocol_flag(protocol_stack_t *stack, u8 flag) {
    if (!stack) {
        return;
    }

    stack->flags |= flag;
}

#endif
