// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_DATADOG_AGENT_H
#define DATADOG_AGENT_RTLOADER_DATADOG_AGENT_H

/*! \file datadog_agent.h
    \brief RtLoader datadog_agent builtin header file.

    The prototypes here defined provide functions to initialize the python datadog_agent
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \fn PyMODINIT_FUNC PyInit_datadog_agent(void)
    \brief Initializes the datadog_agent builtin python module.

    The datadog_agent python builtin is created and registered here as per the module_def
    PyMethodDef definition in `datadog_agent.c` with the corresponding C-implemented python
    methods . A fresh reference to the module is created here. This function is
    python3 only.
*/
/*! \fn void _set_get_version_cb(cb_get_version_t)
    \brief Sets a callback to be used by rtloader to collect the agent version.
    \param object A function pointer with cb_get_version_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_get_config_cb(cb_get_config_t)
    \brief Sets a callback to be used by rtloader to collect the agent configuration.
    \param object A function pointer with cb_get_config_t prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_headers_cb(cb_headers_t)
    \brief Sets a callback to be used by rtloader to collect the typical HTTP headers for
    agent requests.
    \param object A function pointer with cb_headers_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_get_hostname_cb(cb_get_hostname_t)
    \brief Sets a callback to be used by rtloader to collect the canonical hostname from the
    agent.
    \param object A function pointer with cb_get_hostname_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_get_host_tags_cb(cb_get_host_tags_t)
    \brief Sets a callback to be used by rtloader to collect the canonical hostname from the
    agent.
    \param object A function pointer with cb_get_host_tags_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_get_clustername_cb(cb_get_clustername_t)
    \brief Sets a callback to be used by rtloader to collect the K8s clustername from the
    agent.
    \param object A function pointer with cb_get_clustername_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_log_cb(cb_log_t)
    \brief Sets a callback to be used by rtloader to allow using the agent's go-native
    logging facilities to log messages.
    \param object A function pointer with cb_log_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_send_log_cb(cb_send_log_t)
    \brief Sets a callback to be used by rtloader to allow for submitting a log for a given
    check instance.
    \param object A function pointer with cb_send_log_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_set_check_metadata_cb(cb_set_check_metadata_t)
    \brief Sets a callback to be used by rtloader to allow setting metadata for a given
    check instance.
    \param object A function pointer with cb_set_check_metadata_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_set_external_tags_cb(cb_set_external_tags_t)
    \brief Sets a callback to be used by rtloader to allow setting external tags for a given
    hostname.
    \param object A function pointer with cb_set_external_tags_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn PyObject *_public_headers(PyObject *self, PyObject *args, PyObject *kwargs);
    \brief Non-static entrypoint to the headers function; providing HTTP headers for agent
    requests.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to the `agentConfig`, but not expected to be used.
    \param kwargs A PyObject* pointer to a dictonary. If the `http_host` key is present
    it will be added to the headers.
    \return a PyObject * pointer to a python dictionary with the expected headers.

    The headers python method is duplicated and may be called from the `util` _and_
    `datadog_agent` modules. The goal of this wrapper is simply to avoid duplicate code,
    allowing us to call the headers function directly.
*/
/*! \fn void _set_write_persistent_cache_cb(cb_write_persistent_cache_t)
    \brief Sets a callback to be used by rtloader to allow storing data for a given
    check instance.
    \param object A function pointer with cb_write_persistent_cache_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_read_persistent_cache_cb(cb_read_persistent_cache_t)
    \brief Sets a callback to be used by rtloader to allow retrieving data for a given
    check instance.
    \param object A function pointer with cb_read_persistent_cache_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#include <Python.h>
#include <rtloader_types.h>

#define DATADOG_AGENT_MODULE_NAME "datadog_agent"

#ifdef __cplusplus
extern "C" {
#endif

PyMODINIT_FUNC PyInit_datadog_agent(void);

void _set_get_clustername_cb(cb_get_clustername_t);
void _set_get_config_cb(cb_get_config_t);
void _set_get_hostname_cb(cb_get_hostname_t);
void _set_get_host_tags_cb(cb_get_host_tags_t);
void _set_tracemalloc_enabled_cb(cb_tracemalloc_enabled_t);
void _set_get_version_cb(cb_get_version_t);
void _set_headers_cb(cb_headers_t);
void _set_log_cb(cb_log_t);
void _set_send_log_cb(cb_send_log_t);
void _set_set_check_metadata_cb(cb_set_check_metadata_t);
void _set_set_external_tags_cb(cb_set_external_tags_t);
void _set_write_persistent_cache_cb(cb_write_persistent_cache_t);
void _set_read_persistent_cache_cb(cb_read_persistent_cache_t);
void _set_obfuscate_sql_cb(cb_obfuscate_sql_t);
void _set_obfuscate_sql_exec_plan_cb(cb_obfuscate_sql_exec_plan_t);
void _set_get_process_start_time_cb(cb_get_process_start_time_t);
void _set_obfuscate_mongodb_string_cb(cb_obfuscate_mongodb_string_t);
void _set_emit_agent_telemetry_cb(cb_emit_agent_telemetry_t);

PyObject *_public_headers(PyObject *self, PyObject *args, PyObject *kwargs);

#ifdef __cplusplus
}
#endif

#endif
