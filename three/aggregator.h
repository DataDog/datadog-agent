// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_AGGREGATOR_H
#define DATADOG_AGENT_SIX_THREE_AGGREGATOR_H
#include <Python.h>
#include <six_types.h>

PyMODINIT_FUNC PyInit_aggregator(void);

#ifdef __cplusplus
extern "C" {
#endif

void _set_submit_metric_cb(cb_submit_metric_t);

#ifdef __cplusplus
}
#endif

#endif
