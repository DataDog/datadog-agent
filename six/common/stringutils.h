// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_STRINGUTILS_H
#define DATADOG_AGENT_SIX_STRINGUTILS_H

#ifdef __cplusplus
extern "C" {
#endif

#include <Python.h>

int init_stringutils(void);
char *as_string(PyObject *);
PyObject *from_yaml(const char *);
char *as_yaml(PyObject *);

#ifdef DATADOG_AGENT_THREE
#    define PyStringFromCString(x) PyUnicode_FromString(x)
#elif defined(DATADOG_AGENT_TWO)
#    define PyStringFromCString(x) PyString_FromString(x)
#endif

#ifdef __cplusplus
}
#endif

#endif
