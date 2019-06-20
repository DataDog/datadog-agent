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
    PyObject *args = NULL;
    PyObject *kwargs = NULL;
    PyObject *load = NULL;
    PyObject *loader = NULL;
    PyObject *yaml = NULL;

    if (!data) {
        goto done;
    }

    char module_name[] = "yaml";
    yaml = PyImport_ImportModule(module_name);
    if (yaml == NULL) {
        goto done;
    }

    char func_name[] = "load";
    load = PyObject_GetAttrString(yaml, func_name);
    if (load == NULL) {
        goto done;
    }

    char c_loader_name[] = "CSafeLoader";
    loader = PyObject_GetAttrString(yaml, c_loader_name);
    if (loader == NULL) {
        PyErr_Clear();
        char loader_name[] = "SafeLoader";
        loader = PyObject_GetAttrString(yaml, loader_name);
        if (loader == NULL) {
            goto done;
        }
    }

    args = PyTuple_New(0);
    if (args == NULL) {
        goto done;
    }
    kwargs = Py_BuildValue("{s:s, s:O}", "stream", data, "Loader", loader);
    if (kwargs == NULL) {
        goto done;
    }
    retval = PyObject_Call(load, args, kwargs);

done:
    Py_XDECREF(kwargs);
    Py_XDECREF(args);
    Py_XDECREF(load);
    Py_XDECREF(yaml);
    return retval;
}

char *as_yaml(PyObject *object) {
    char *retval = NULL;
    PyObject *args = NULL;
    PyObject *kwargs = NULL;
    PyObject *dump = NULL;
    PyObject *dumper = NULL;
    PyObject *dumped = NULL;
    PyObject *yaml = NULL;

    char module_name[] = "yaml";
    yaml = PyImport_ImportModule(module_name);
    if (yaml == NULL) {
        goto done;
    }

    char func_name[] = "dump";
    dump = PyObject_GetAttrString(yaml, func_name);
    if (dump == NULL) {
        goto done;
    }

    char c_dumper_name[] = "CSafeDumper";
    dumper = PyObject_GetAttrString(yaml, c_dumper_name);
    if (dumper == NULL) {
        PyErr_Clear();
        char dumper_name[] = "SafeDumper";
        dumper = PyObject_GetAttrString(yaml, dumper_name);
        if (dumper == NULL) {
            goto done;
        }
    }

    args = PyTuple_New(0);
    kwargs = Py_BuildValue("{s:O, s:O}", "data", object, "Dumper", dumper);
    dumped = PyObject_Call(dump, args, kwargs);
    if (dumped == NULL) {
        goto done;
    }
    retval = as_string(dumped);

done:
    Py_XDECREF(kwargs);
    Py_XDECREF(args);
    Py_XDECREF(dumped);
    Py_XDECREF(dump);
    Py_XDECREF(yaml);
    return retval;
}
