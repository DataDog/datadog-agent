// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "memory.h"

#include <stdlib.h>

// default memory management functions
static rtloader_malloc_t rt_malloc = malloc;
static rtloader_free_t rt_free = free;

// these must be set by the Agent
static cb_memory_tracker_t cb_memory_tracker = NULL;

void _set_memory_tracker_cb(cb_memory_tracker_t cb) {
    cb_memory_tracker = cb;
}

void *_malloc(size_t sz) {
    void *ptr = NULL;
    ptr = rt_malloc(sz);

    if (ptr && cb_memory_tracker) {
        cb_memory_tracker(ptr, sz, DATADOG_AGENT_RTLOADER_ALLOCATION);
    }

    return ptr;
}

void _free(void *ptr) {
    rt_free(ptr);

    if (ptr && cb_memory_tracker) {
        cb_memory_tracker(ptr, 0, DATADOG_AGENT_RTLOADER_FREE);
    }
}
