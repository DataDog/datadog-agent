// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_DATADOG_AGENT_H
#define DATADOG_AGENT_SIX_DATADOG_AGENT_H
#include <Python.h>
#include <six_types.h>

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_datadog_agent(void);
#endif

#define DATADOG_AGENT_MODULE_NAME "datadog_agent"

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_datadog_agent();
#endif

void _set_get_version_cb(cb_get_version_t);
void _set_get_config_cb(cb_get_config_t);
void _set_headers_cb(cb_headers_t);
void _set_get_hostname_cb(cb_get_hostname_t);
void _set_get_clustername_cb(cb_get_clustername_t);
void _set_log_cb(cb_log_t);
void _set_set_external_tags_cb(cb_set_external_tags_t);

// provide a non-static entry point for the `headers` method; headers is duplicated
// in the `util` module; allow it to be called directly
PyObject *_public_headers(PyObject *self, PyObject *args, PyObject *kwargs);

#ifdef __cplusplus
}
#endif

#endif
