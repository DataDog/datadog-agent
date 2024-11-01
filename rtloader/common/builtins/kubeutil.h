// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_KUBEUTIL_H
#define DATADOG_AGENT_RTLOADER_KUBEUTIL_H

/*! \file kubeutil.h
    \brief RtLoader Kubeutil builtin header file.

    The prototypes here defined provide functions to initialize the python kubeutil
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \fn PyMODINIT_FUNC PyInit_kubeutil(void)
    \brief Initializes the kubeutil builtin python module.

    The kubeutil python builtin is created and registered here as per the module_def
    PyMethodDef definition. The get_connection_info method is registered with the
    module. A fresh reference to the module is created here. This function is python3
    only.
*/
/*! \fn void _set_get_connection_info_cb(cb_get_connection_info_t)
    \brief Sets a callback to be used by rtloader for kubernetes connection information
    retrieval.
    \param object A function pointer with cb_get_connection_info_t prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#include <Python.h>
#include <rtloader_types.h>

#define KUBEUTIL_MODULE_NAME "kubeutil"

#ifdef __cplusplus
extern "C" {
#endif

// PyMODINIT_FUNC macro already specifies extern "C", nesting these is legal
PyMODINIT_FUNC PyInit_kubeutil(void);

void _set_get_connection_info_cb(cb_get_connection_info_t);

#ifdef __cplusplus
}
#endif

#endif // DATADOG_AGENT_RTLOADER_KUBEUTIL_H
