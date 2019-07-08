// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "memory.h"

// these must be set by the Agent
static cb_cgo_free_t cb_cgo_free = NULL;

void _set_cgo_free_cb(cb_cgo_free_t cb) {
    cb_cgo_free = cb;
}

// On windows we cannot free memory block from another DLL. Agent's Callbacks
// will return memory block to free, this is why we need a pointer to a CGO
// free method to release memory allocated in the agent once we're done with
// them.
void cgo_free(void *ptr) {
    // Technically this is not thread-safe as `cb_cgo_free` assignment
    // is not atomic. Since the setter is called very early on and is
    // a one-time operation we can live with it. Should that change
    // we'd need to set a memory barrier here, and in `_set_cgo_free_cb()`
    if (cb_cgo_free == NULL || ptr == NULL) {
        return;
    }
    cb_cgo_free(ptr);
}
