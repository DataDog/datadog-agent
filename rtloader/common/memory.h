// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_MEMORY_H
#define DATADOG_AGENT_RTLOADER_MEMORY_H

/*! \file memory.h
    \brief RtLoader memory wrapper header file.

    The prototypes here defined provide functions to allocate and free memory.
    The goal is to allow us to track allocations if desired.
*/

#include "rtloader_types.h"

#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

/*! \fn void _set_memory_tracker_cb(cb_memory_tracker_t cb)
    \brief Sets a callback to be used by rtloader to add memory tracking stats.
    \param object A function pointer to the callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
void _set_memory_tracker_cb(cb_memory_tracker_t);

/*! \fn void *_malloc(size_t sz)
    \brief Basic malloc wrapper that will also keep memory stats if enabled.
    \param sz the number of bytes to allocate.
*/
void *_malloc(size_t sz);

/*! \fn void _free(void *ptr)
    \brief Basic free wrapper that will also keep memory stats if enabled.
    \param ptr the pointer to the heap region you wish to free.
*/
void _free(void *ptr);

#undef strdup
#ifdef __cplusplus
char *strdup(const char *s1) __THROW;
#else
char *strdup(const char *s1);
#endif

#ifdef __cplusplus
}
#endif

#endif
