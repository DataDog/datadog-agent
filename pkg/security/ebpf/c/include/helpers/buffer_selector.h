#ifndef _HELPERS_BUFFER_SELECTOR_H
#define _HELPERS_BUFFER_SELECTOR_H

#include "maps.h"

static __attribute__((always_inline)) struct bpf_map_def *select_buffer(void *front_buffer,
    void *back_buffer,
    u32 selector_key) {
    u32 *buffer_id = bpf_map_lookup_elem(&buffer_selector, &selector_key);
    if (buffer_id == NULL) {
        return NULL;
    }

    return *buffer_id ? back_buffer : front_buffer;
}

#endif
