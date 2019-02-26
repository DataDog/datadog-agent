// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "sixstrings.h"

#include <six_types.h>

char *as_string(PyObject *object) {
    if (object == NULL) {
        return NULL;
    }

    char *retval = NULL;

#ifdef DATADOG_AGENT_THREE
    if (!PyUnicode_Check(object)) {
        return NULL;
    }

    PyObject *temp_bytes = PyUnicode_AsEncodedString(object, "UTF-8", "strict");
    retval = _strdup(PyBytes_AS_STRING(temp_bytes));
    Py_XDECREF(temp_bytes);
#else
    if (!PyString_Check(object)) {
        return NULL;
    }

    retval = _strdup(PyString_AS_STRING(object));
#endif
    return retval;
}
