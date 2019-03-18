// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_UTIL__H
#define DATADOG_AGENT_SIX_UTIL__H
#include <Python.h>
#include <six_types.h>

#define _UTIL_MODULE_NAME "_util"

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit__util(void);
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init__util();
#endif

void _set_get_subprocess_output_cb(cb_get_subprocess_output_t);
#ifdef __cplusplus
}
#endif

#endif
