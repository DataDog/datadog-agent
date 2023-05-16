#ifndef __PROTOCOL_ROUTING_HELPERS_H
#define __PROTOCOL_ROUTING_HELPERS_H

#include "ktypes.h"
#include "protocols/classification/defs.h"

// has_available_program returns true when there is another program from within
// the same protocol layer or false otherwise
static __always_inline bool has_available_program(classification_prog_t current_program) {
    classification_prog_t next_program = current_program+1;
    if (next_program == __PROG_APPLICATION ||
        next_program == __PROG_API ||
        next_program == __PROG_ENCRYPTION ||
        next_program == CLASSIFICATION_PROG_MAX) {
        return false;
    }

    return true;
}

#pragma clang diagnostic push
// The following check is ignored because *currently* there are no API or
// Encryption classification programs registerd.
// Therefore the enum containing all BPF programs looks like the following:
//
// typedef enum {
//     CLASSIFICATION_PROG_UNKNOWN = 0,
//     __PROG_APPLICATION,
//     APPLICATION_PROG_A
//     APPLICATION_PROG_B
//     APPLICATION_PROG_C
//     ...
//     __PROG_API,
//     // No programs here
//     __PROG_ENCRYPTION,
//     // No programs here
//     CLASSIFICATION_PROG_MAX,
// } classification_prog_t;
//
// Which means that the following conditionals will always evaluate to false:
// a) current_program > __PROG_API && current_program < __PROG_ENCRYPTION
// b) current_program > __PROG_ENCRYPTION && current_program < CLASSIFICATION_PROG_MAX
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

#endif
