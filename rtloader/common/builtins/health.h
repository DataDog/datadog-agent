// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

#ifndef DATADOG_AGENT_RTLOADER_THREE_HEALTH_H
#define DATADOG_AGENT_RTLOADER_THREE_HEALTH_H

/*! \file health.h
    \brief RtLoader health builtin header file.

    The prototypes here defined provide functions to initialize the python health
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \def HEALTH_MODULE_NAME
    \brief Health module name definition.
*/
/*! \fn PyMODINIT_FUNC PyInit_health(void)
    \brief Initializes the health builtin python module.

    The python python builtin is created and registered here as per the module_def
    PyMethodDef definition in `health.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python3
    only.
*/
/*! \fn void Py2_init_health()
    \brief Initializes the health builtin python module.

    The health python builtin is created and registered here as per the module_def
    PyMethodDef definition in `health.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python2
    only.
*/
/*! \fn void _set_submit_health_check_data_t(cb_submit_health_check_data_t)
    \brief Sets the submit health check data callback to be used by rtloader for check data submission.
    \param object A function pointer with cb_submit_health_check_data_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_health_start_snapshot_t(cb_submit_health_start_snapshot_t)
    \brief Sets the submit start health snapshot callback to be used by rtloader to signal the health snapshot start.
    \param object A function pointer with cb_submit_health_start_snapshot_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_health_stop_snapshot_t(cb_submit_health_stop_snapshot_t)
    \brief Sets the submit health stop snapshot callback to be used by rtloader to signal the health snapshot stop.
    \param object A function pointer with cb_submit_health_stop_snapshot_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#include <Python.h>
#include <rtloader_types.h>

#define HEALTH_MODULE_NAME "health"

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_health(void);
#elif defined(DATADOG_AGENT_TWO)
void Py2_init_health();
#endif

void _set_submit_health_check_data_cb(cb_submit_health_check_data_t);
void _set_submit_health_start_snapshot_cb(cb_submit_health_start_snapshot_t);
void _set_submit_health_stop_snapshot_cb(cb_submit_health_stop_snapshot_t);


#ifdef __cplusplus
}
#endif

#endif
