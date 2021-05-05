// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

#ifndef DATADOG_AGENT_RTLOADER_THREE_TOPOLOGY_H
#define DATADOG_AGENT_RTLOADER_THREE_TOPOLOGY_H

/*! \file topology.h
    \brief RtLoader topology builtin header file.

    The prototypes here defined provide functions to initialize the python topology
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \def AGGREGATOR_MODULE_NAME
    \brief Topology module name definition.
*/
/*! \fn PyMODINIT_FUNC PyInit_topology(void)
    \brief Initializes the topology builtin python module.

    The python python builtin is created and registered here as per the module_def
    PyMethodDef definition in `topology.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python3
    only.
*/
/*! \fn void Py2_init_topology()
    \brief Initializes the topology builtin python module.

    The topology python builtin is created and registered here as per the module_def
    PyMethodDef definition in `topology.c` with the corresponding C-implemented python
    methods. A fresh reference to the module is created here. This function is python2
    only.
*/
/*! \fn void _set_submit_component_cb(cb_submit_component_t)
    \brief Sets the submit component callback to be used by rtloader for topology component submission.
    \param object A function pointer with cb_submit_component_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_relation_cb(cb_submit_relation_t)
    \brief Sets the submit relation callback to be used by rtloader for topology relation submission.
    \param object A function pointer with cb_submit_relation_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_start_snapshot_cb(cb_submit_start_snapshot_t)
    \brief Sets the submit start snapshot callback to be used by rtloader to signal the topology snapshot start.
    \param object A function pointer with cb_submit_start_snapshot_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_stop_snapshot_cb(cb_submit_stop_snapshot_t)
    \brief Sets the submit stop snapshot callback to be used by rtloader to signal the topology snapshot stop.
    \param object A function pointer with cb_submit_stop_snapshot_t function prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#include <Python.h>
#include <rtloader_types.h>

#define TOPOLOGY_MODULE_NAME "topology"

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_topology(void);
#elif defined(DATADOG_AGENT_TWO)
void Py2_init_topology();
#endif

void _set_submit_component_cb(cb_submit_component_t);
void _set_submit_relation_cb(cb_submit_relation_t);
void _set_submit_start_snapshot_cb(cb_submit_start_snapshot_t);
void _set_submit_stop_snapshot_cb(cb_submit_stop_snapshot_t);

#ifdef __cplusplus
}
#endif

#endif
