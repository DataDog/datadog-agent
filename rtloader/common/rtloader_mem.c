// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "rtloader_mem.h"

#if defined(__linux__) || defined(_WIN32)
#    include <malloc.h>
#elif defined(__APPLE__) || defined(__FreeBSD__)
#    include <malloc/malloc.h>
#endif
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>

// Get the size of an allocated memory block.
// Note that the size returned here is not less that the requested size (malloc’s argument) but it may be larger.
#if defined(__linux__)
#    define _get_alloc_size(ptr) malloc_usable_size(ptr)
#elif defined(_WIN32)
#    define _get_alloc_size(ptr) _msize(ptr)
#elif defined(__APPLE__) || defined(__FreeBSD__)
#    define _get_alloc_size(ptr) malloc_size(ptr)
#else
#    warning "Metrics `rtloader.allocated_bytes`, `rtloader.freed_bytes` and `rtloader.inuse_bytes`"
#    warning "are available only on Linux, Windows, MacOS and FreeBSD platforms."
#    define _get_alloc_size(ptr) 0
#endif

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
// default memory management functions
rtloader_malloc_t _malloc = malloc;
rtloader_free_t _free = free;
#pragma clang diagnostic pop
#pragma GCC diagnostic pop

static size_t allocations = 0;
static size_t allocated_bytes = 0;
static size_t frees = 0;
static size_t freed_bytes = 0;
static ssize_t inuse_bytes = 0;

void *_tracked_malloc(size_t sz)
{
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
    void *ptr = malloc(sz);
#pragma clang diagnostic pop
#pragma GCC diagnostic pop

    if (ptr != NULL) {
        sz = _get_alloc_size(ptr);
        __atomic_add_fetch(&allocations, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&allocated_bytes, sz, __ATOMIC_RELAXED);
        __atomic_add_fetch(&inuse_bytes, sz, __ATOMIC_RELAXED);
    }

    return ptr;
}

void _tracked_free(void *ptr)
{
    if (ptr != NULL) {
        size_t sz = _get_alloc_size(ptr);
        __atomic_add_fetch(&frees, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&freed_bytes, sz, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&inuse_bytes, sz, __ATOMIC_RELAXED);
    }

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
    free(ptr);
#pragma clang diagnostic pop
#pragma GCC diagnostic pop
}

void _enable_memory_tracker(void)
{
    __atomic_store_n(&_malloc, _tracked_malloc, __ATOMIC_RELAXED);
    __atomic_store_n(&_free, _tracked_free, __ATOMIC_RELAXED);
}

rtloader_malloc_t _get_tracked_malloc(void)
{
    return __atomic_load_n(&_malloc, __ATOMIC_RELAXED);
}

rtloader_free_t _get_tracked_free(void)
{
    return __atomic_load_n(&_free, __ATOMIC_RELAXED);
}

struct memory_stats get_and_reset_memory_stats(void)
{
    return (struct memory_stats){ .allocations = __atomic_exchange_n(&allocations, 0, __ATOMIC_RELAXED),
                                  .allocated_bytes = __atomic_exchange_n(&allocated_bytes, 0, __ATOMIC_RELAXED),
                                  .frees = __atomic_exchange_n(&frees, 0, __ATOMIC_RELAXED),
                                  .freed_bytes = __atomic_exchange_n(&freed_bytes, 0, __ATOMIC_RELAXED),
                                  .inuse_bytes = __atomic_exchange_n(&inuse_bytes, 0, __ATOMIC_RELAXED) };
}

char *strdupe(const char *s1)
{
    char *s2 = NULL;

    if (!(s2 = (char *)_malloc(strlen(s1) + 1))) {
        return NULL;
    }

    return strcpy(s2, s1);
}
