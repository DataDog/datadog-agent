// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_AGGREGATOR_H
#define DATADOG_AGENT_SIX_THREE_AGGREGATOR_H
#include <Python.h>
#include <six_types.h>

PyMODINIT_FUNC PyInit_aggregator(void);

static PyObject *submit_metric(PyObject *self, PyObject *args);
static PyObject *submit_service_check(PyObject *self, PyObject *args);
static PyObject *submit_event(PyObject *self, PyObject *args);

#endif
