// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_UTIL_H
#define DATADOG_AGENT_SIX_UTIL_H

/*! \file util.h
    \brief Six util builtin header file.

    The prototypes here defined provide functions to initialize the python util
    builtin module, and set its relevant callbacks for the six caller.
*/

#include <Python.h>
#include <six_types.h>

#define UTIL_MODULE_NAME "util"

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_THREE

/*! \fn void PyInit_util()
    \brief Initializes the util builtin python module.

    The 'util' python builtin is created with the methods from the PyMethodDef
    array in 'util.c' and registered into python. This function is python3 only.
*/
PyMODINIT_FUNC PyInit_util(void);
#elif defined(DATADOG_AGENT_TWO)

/*! \fn void Py2_init_util()
    \brief Initializes the util builtin python module.

    The 'util' python builtin is created with the methods from the PyMethodDef
    array in 'util.c' and registered into python. This function is python2 only.
*/
void Py2_init_util();
#endif

#ifdef __cplusplus
}
#endif

#endif
