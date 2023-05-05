#ifndef __PROTOCOL_ROUTING_HELPERS_H
#define __PROTOCOL_ROUTING_HELPERS_H

#include "ktypes.h"
#include "protocols/classification/defs.h"

static __always_inline bool is_last_program_from_layer(classification_prog_t current_program) {
    classification_prog_t next_program = current_program+1;
    if (next_program == __PROG_APPLICATION ||
        next_program == __PROG_API ||
        next_program == __PROG_ENCRYPTION) {
        return true;
    }

    return false;
}

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wtautological-overlap-compare"
static __always_inline u16 get_current_program_layer(classification_prog_t current_program) {
    if (current_program > __PROG_APPLICATION && current_program < __PROG_API) {
        return LAYER_APPLICATION_BIT;
    }

    if (current_program > __PROG_API && current_program < __PROG_ENCRYPTION) {
        return LAYER_API_BIT;
    }

    if (current_program > __PROG_ENCRYPTION && current_program < CLASSIFICATION_PROG_MAX) {
        return LAYER_ENCRYPTION_BIT;
    }

    return 0;
}
#pragma clang diagnostic pop

static __always_inline classification_prog_t next_layer_entrypoint(usm_context_t *usm_ctx) {
    u16 to_skip = usm_ctx->routing_skip_layers;

    if (!(to_skip&LAYER_APPLICATION_BIT)) {
        return __PROG_APPLICATION+1;
    }
    if (!(to_skip&LAYER_API_BIT)) {
        return __PROG_API+1;
    }
    if (!(to_skip&LAYER_ENCRYPTION_BIT)) {
        return __PROG_ENCRYPTION+1;
    }

    return CLASSIFICATION_PROG_UNKNOWN;
}

// empty_program_layer returns true in no programs are definied in the
// classification_prog_t enum for a particular layer
// The `layer_delimiter` argument to this function should be one of the following 3:
// 1) __PROG_APPLICATION
// 2) __PROG_API
// 3) __PROG_ENCRYPTION
static __always_inline bool is_empty_program_layer(classification_prog_t layer_delimiter) {
    return is_last_program_from_layer(layer_delimiter) || (layer_delimiter+1) == CLASSIFICATION_PROG_MAX;
}

#endif
