// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_UTIL_H
#define DATADOG_AGENT_SIX_UTIL_H
#include <Python.h>
#include <six_types.h>

#define UTIL_MODULE_NAME "util"

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_util(void);
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_util();
#endif

#ifdef __cplusplus
}
#endif

#endif
