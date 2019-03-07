// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_SIXSTRINGS_H
#define DATADOG_AGENT_SIX_THREE_SIXSTRINGS_H
#include <Python.h>

char *as_string(PyObject *);
PyObject *from_json(const char *);
char *as_json(PyObject *);

#endif
