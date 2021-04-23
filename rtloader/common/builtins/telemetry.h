// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

#ifndef DATADOG_AGENT_RTLOADER_THREE_TELEMETRY_H
#define DATADOG_AGENT_RTLOADER_THREE_TELEMETRY_H

/*! \file telemetry.h
    \brief RtLoader telemetry builtin header file.
    The prototypes here defined provide functions to initialize the python telemetry
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \def AGGREGATOR_MODULE_NAME
    \brief Telemetry module name definition.
*/
/*! \fn PyMODINIT_FUNC PyInit_telemetry(void)
    \brief Initializes the telemtry builtin python module.
    The python python builtin is created and registered here as per the module_def
    PyMethodDef definition in `telemetry.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python3
    only.
*/
/*! \fn void Py2_init_telemetry()
    \brief Initializes the telemetry builtin python module.
    The telemetry python builtin is created and registered here as per the module_def
    PyMethodDef definition in `telemetry.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python2
    only.
*/
/*! \fn void _set_submit_topology_event(cb_submit_topology_event_t)
    \brief Sets the submit topology event callback to be used by rtloader for topology event submission.
    \param object A function pointer with cb_submit_topology_event_t function prototype to the
    callback function.
    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#include <Python.h>
#include <rtloader_types.h>

#define TELEMETRY_MODULE_NAME "telemetry"

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_telemetry(void);
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_telemetry();
#endif

void _set_submit_topology_event_cb(cb_submit_topology_event_t);

#ifdef __cplusplus
}
#endif

#endif