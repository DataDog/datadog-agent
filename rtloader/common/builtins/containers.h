// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_CONTAINERS_H
#define DATADOG_AGENT_RTLOADER_CONTAINERS_H

/*! \file containers.h
    \brief RtLoader containers builtin header file.

    The prototypes here defined provide functions to initialize the python containers
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \fn PyMODINIT_FUNC PyInit_containers(void)
    \brief Initializes the containers builtin python module.

    The containers python builtin is created and registered here as per the module_def
    PyMethodDef definition in `containers.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python3
    only.
*/
/*! \fn void _set_is_excluded_cb(cb_is_excluded_t)
    \brief Sets a callback to be used by rtloader to determine if a container is excluded
    from metric collection.
    \param object A function pointer with cb_is_excluded_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#include <Python.h>
#include <rtloader_types.h>

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_containers(void);
#endif

#define CONTAINERS_MODULE_NAME "containers"

#ifdef __cplusplus
extern "C" {
#endif

void _set_is_excluded_cb(cb_is_excluded_t);

#ifdef __cplusplus
}
#endif

#endif
