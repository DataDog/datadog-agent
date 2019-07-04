// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_CGO_FREE_H
#define DATADOG_AGENT_RTLOADER_CGO_FREE_H

/*! \file cgo_free.h
    \brief RtLoader cgo_free builtin header file.

    The prototypes here defined provide functions to free up memory
    allocated from cgo. This is required by windows for memory protection.
*/

#include <rtloader_types.h>

#ifdef __cplusplus
extern "C" {
#endif

/*! \fn void _set_cgo_free_cb(cb_cgo_free_t cb)
    \brief Sets a callback to be used by rtloader to free memory allocated by the
    rtloader's caller and passed into rtloader.
    \param object A function pointer to the callback function.

    On Windows we cannot free a memory block from another DLL. This is why we
    need to call back to the allocating DLL if it wishes to release allocated memory.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
void _set_cgo_free_cb(cb_cgo_free_t);

/*! \fn void cgo_free(void *ptr)
    \brief Frees memory that was originally allocated by the rtloader's caller.
    \param object A pointer to the memory block to free.

    On Windows we cannot free a memory block from another DLL. This is why we
    need to call an external free method to release memory allocated externally.
*/
void DATADOG_AGENT_RTLOADER_API cgo_free(void *ptr);

#ifdef __cplusplus
}
#endif

#endif
