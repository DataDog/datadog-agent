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

// DATADOG_AGENT_THREE implementation is the default
#ifdef DATADOG_AGENT_TWO
    if (!PyString_Check(object) && !PyUnicode_Check(object)) {
        return NULL;
    }

    char *tmp = PyString_AsString(object);
    if (tmp == NULL) {
        // PyString_AsString might raise an error when python can't encode a
        // unicode string to byte
        PyErr_Clear();
        return  NULL;
    }
    retval = _strdup(tmp);
#else
    if (!PyUnicode_Check(object)) {
        return NULL;
    }

    PyObject *temp_bytes = PyUnicode_AsEncodedString(object, "UTF-8", "strict");
    if (temp_bytes == NULL) {
        // PyUnicode_AsEncodedString might raise an error if the codec raised an
        // exception
        PyErr_Clear();
        return NULL;
    }

    retval = _strdup(PyBytes_AS_STRING(temp_bytes));
    Py_XDECREF(temp_bytes);
#endif

    return retval;
}

PyObject *from_yaml(const char *data) {
    PyObject *retval = NULL;
    PyObject *yaml = NULL;
    PyObject *safe_load = NULL;

    if (!data) {
        goto done;
    }

    char module_name[] = "yaml";
    yaml = PyImport_ImportModule(module_name);
    if (yaml == NULL) {
        goto done;
    }

    char func_name[] = "safe_load";
    safe_load = PyObject_GetAttrString(yaml, func_name);
    if (safe_load == NULL) {
        goto done;
    }

    retval = PyObject_CallFunction(safe_load, "s", data);

done:
    // shouldn't make much of a difference, but let's DECREF in reverse order
    Py_XDECREF(safe_load);
    Py_XDECREF(yaml);
    return retval;
}

char *as_yaml(PyObject *object) {
    char *retval = NULL;
    PyObject *yaml = NULL;
    PyObject *safe_dump = NULL;
    PyObject *dumped = NULL;

    char module_name[] = "yaml";
    yaml = PyImport_ImportModule(module_name);
    if (yaml == NULL) {
        goto done;
    }

    char func_name[] = "safe_dump";
    safe_dump = PyObject_GetAttrString(yaml, func_name);
    if (safe_dump == NULL) {
        goto done;
    }

    dumped = PyObject_CallFunctionObjArgs(safe_dump, object, NULL);
    if (dumped == NULL) {
        goto done;
    }
    retval = as_string(dumped);

done:
    // shouldn't make much of a difference, but let's DECREF in reverse order
    Py_XDECREF(dumped);
    Py_XDECREF(safe_dump);
    Py_XDECREF(yaml);
    return retval;
}
