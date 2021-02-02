// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "rtloader_mem.h"

#include <stdlib.h>
#include <string.h>

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
// default memory management functions
static rtloader_malloc_t rt_malloc = malloc;
static rtloader_free_t rt_free = free;
#pragma clang diagnostic pop
#pragma GCC diagnostic pop

// these must be set by the Agent
static cb_memory_tracker_t cb_memory_tracker = NULL;

void _set_memory_tracker_cb(cb_memory_tracker_t cb) {

    // Memory barrier for a little bit of safety on sets
    __sync_synchronize();
    cb_memory_tracker = cb;
}

cb_memory_tracker_t _get_memory_tracker_cb(void) {

    // Memory barrier for a little bit of safety on gets
    __sync_synchronize();
    return cb_memory_tracker;
}

void *_malloc(size_t sz) {
    void *ptr = NULL;
    ptr = rt_malloc(sz);

    // This is currently thread-unsafe, so be sure to set the callback before
    // running this code.
    if (ptr && cb_memory_tracker) {
        cb_memory_tracker(ptr, sz, DATADOG_AGENT_RTLOADER_ALLOCATION);
    }

    return ptr;
}

void _free(void *ptr) {
    rt_free(ptr);

    // This is currently thread-unsafe, so be sure to set the callback before
    // running this code.
    if (ptr && cb_memory_tracker) {
        cb_memory_tracker(ptr, 0, DATADOG_AGENT_RTLOADER_FREE);
    }
}

char *strdupe(const char *s1) {
    char * s2 = NULL;

    if (!(s2 = (char *)_malloc(strlen(s1)+1))) {
        return NULL;
    }

    return strcpy(s2, s1);
}
