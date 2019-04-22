// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_AGGREGATOR_H
#define DATADOG_AGENT_SIX_THREE_AGGREGATOR_H
#include <Python.h>
#include <six_types.h>

#define AGGREGATOR_MODULE_NAME "aggregator"

#ifdef DATADOG_AGENT_THREE
PyMODINIT_FUNC PyInit_aggregator(void);
#endif

#ifdef __cplusplus
extern "C" {
#endif

#ifdef DATADOG_AGENT_TWO
void Py2_init_aggregator();
#endif

void _set_submit_metric_cb(cb_submit_metric_t);
void _set_submit_service_check_cb(cb_submit_service_check_t);
void _set_submit_event_cb(cb_submit_event_t);

#ifdef __cplusplus
}
#endif

#endif
