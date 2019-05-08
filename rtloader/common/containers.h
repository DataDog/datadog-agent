// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_CONTAINERS_H
#define DATADOG_AGENT_SIX_CONTAINERS_H
#include <Python.h>
#include <six_types.h>

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_containers(void);
#endif

#define CONTAINERS_MODULE_NAME "containers"

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_containers();
#endif

void _set_is_excluded_cb(cb_is_excluded_t);

#ifdef __cplusplus
}
#endif

#endif
