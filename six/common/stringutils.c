// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "stringutils.h"

#include <six_types.h>

char *as_string(PyObject *object)
{
    if (object == NULL) {
        return NULL;
    }

    char *retval = NULL;

#ifdef DATADOG_AGENT_THREE
    if (!PyUnicode_Check(object)) {
        return NULL;
    }

    PyObject *temp_bytes = PyUnicode_AsEncodedString(object, "UTF-8", "strict");
    if (temp_bytes == NULL) {
        return NULL;
    }

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

PyObject *from_json(const char *data)
{
    PyObject *retval = NULL;
    PyObject *json = NULL;
    PyObject *loads = NULL;

    if (!data) {
        goto done;
    }

    char module_name[] = "json";
    json = PyImport_ImportModule(module_name);
    if (json == NULL) {
        goto done;
    }

    char func_name[] = "loads";
    loads = PyObject_GetAttrString(json, func_name);
    if (loads == NULL) {
        goto done;
    }

    retval = PyObject_CallFunction(loads, "s", data);

done:
    Py_XDECREF(json);
    Py_XDECREF(loads);
    return retval;
}

char *as_json(PyObject *object)
{
    char *retval = NULL;
    PyObject *json = NULL;
    PyObject *dumps = NULL;
    PyObject *dumped = NULL;

    char module_name[] = "json";
    json = PyImport_ImportModule(module_name);
    if (json == NULL) {
        goto done;
    }

    char func_name[] = "dumps";
    dumps = PyObject_GetAttrString(json, func_name);
    if (dumps == NULL) {
        goto done;
    }

    dumped = PyObject_CallFunctionObjArgs(dumps, object, NULL);
    retval = as_string(dumped);

done:
    Py_XDECREF(json);
    Py_XDECREF(dumps);
    Py_XDECREF(dumped);
    return retval;
}
