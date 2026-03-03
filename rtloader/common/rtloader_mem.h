// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_MEM_H
#define DATADOG_AGENT_RTLOADER_MEM_H

/*! \file rtloader_mem.h
    \brief RtLoader memory wrapper header file.

    The prototypes here defined provide functions to allocate and free memory.
    The goal is to allow us to track allocations if desired.
*/

#include "rtloader_types.h"

#include <stdlib.h>
#include <sys/types.h>

#define MEM_DEPRECATION_MSG                                                                                            \
    "raw primitives should not be used in the context"                                                                 \
    "of the rtloader"

#ifdef __cplusplus
extern "C" {
#endif

extern void *malloc(size_t size) __attribute__((deprecated(MEM_DEPRECATION_MSG)));
extern void free(void *ptr) __attribute__((deprecated(MEM_DEPRECATION_MSG)));

/*! \fn void _enable_memory_tracker(void)
    \brief Enables memory tracking stats for the rtloader.
*/
void _enable_memory_tracker(void);

/*! \fn rtloader_malloc_t _get_tracked_malloc(void)
    \brief Gets the current tracked malloc function.
*/
rtloader_malloc_t _get_tracked_malloc(void);

/*! \fn rtloader_free_t _get_tracked_free(void)
    \brief Gets the current tracked free function.
*/
rtloader_free_t _get_tracked_free(void);

/*! \fn void *_malloc(size_t sz)
    \brief Basic malloc wrapper that will also keep memory stats if enabled.
    \param sz the number of bytes to allocate.

    This function is thread unsafe in its access to the memory tracker. Only, use this
    logic once the memory tracker has be set (or tracking remains disabled).
*/
extern void *(*_malloc)(size_t sz);

/*! \fn void _free(void *ptr)
    \brief Basic free wrapper that will also keep memory stats if enabled.
    \param ptr the pointer to the heap region you wish to free.

    This function is thread unsafe in its access to the memory tracker. Only, use this
    logic once the memory tracker has be set (or tracking remains disabled).
*/
extern void (*_free)(void *ptr);

struct memory_stats {
    size_t allocations;
    size_t allocated_bytes;
    size_t frees;
    size_t freed_bytes;
    ssize_t inuse_bytes;
};

struct memory_stats get_and_reset_memory_stats(void);

#ifdef __cplusplus
#    ifndef __GLIBC__
#        define __THROW
#    endif

char *strdupe(const char *s1) __THROW;
#else
char *strdupe(const char *s1);
#endif

#ifdef __cplusplus
}
#endif

#endif
