// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_UTIL_H
#define DATADOG_AGENT_RTLOADER_UTIL_H

/*! \file util.h
    \brief RtLoader util builtin header file.

    The prototypes here defined provide functions to initialize the python util
    builtin module, and set its relevant callbacks for the rtloader caller.
*/

#include <Python.h>
#include <rtloader_types.h>

#define UTIL_MODULE_NAME "util"

#ifdef __cplusplus
extern "C" {
#endif

/*! \fn void PyInit_util()
    \brief Initializes the util builtin python module.

    The 'util' python builtin is created with the methods from the PyMethodDef
    array in 'util.c' and registered into python. This function is python3 only.
*/
PyMODINIT_FUNC PyInit_util(void);

#ifdef __cplusplus
}
#endif

#endif
