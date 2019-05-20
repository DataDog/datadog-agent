// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_KUBEUTIL_H
#define DATADOG_AGENT_SIX_KUBEUTIL_H
#include <Python.h>
#include <six_types.h>

#define KUBEUTIL_MODULE_NAME "kubeutil"

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_kubeutil(void);
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_kubeutil();
#endif

void _set_get_connection_info_cb(cb_get_connection_info_t);

#ifdef __cplusplus
}
#endif

#endif
