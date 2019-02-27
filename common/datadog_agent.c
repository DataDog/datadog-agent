// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "datadog_agent.h"

#include <assert.h>
#include <sixstrings.h>

#define MODULE_NAME "datadog_agent"

// these must be set by the Agent
static cb_get_version_t cb_get_version = NULL;
static cb_get_config_t cb_get_config = NULL;

// forward declarations
static PyObject *get_version(PyObject *self, PyObject *args);
static PyObject *get_config(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "get_version", (PyCFunction)get_version, METH_NOARGS, "Get Agent version." },
    { "get_config", (PyCFunction)get_config, METH_VARARGS, "Get an Agent config item." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_datadog_agent(void) {
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_datadog_agent() {
    module = Py_InitModule(MODULE_NAME, methods);
}
#endif

void _set_get_version_cb(cb_get_version_t cb) {
    cb_get_version = cb;
}

void _set_get_config_cb(cb_get_config_t cb) {
    cb_get_config = cb;
}

PyObject *get_version(PyObject *self, PyObject *args) {
    // callback must be set
    assert(cb_get_version != NULL);

    char *v;
    cb_get_version(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/**
 * Before Six the Agent used reflection to inspect the contents of a configuration
 * value and the CPython API to perform conversion to a Python equivalent. Such
 * a conversion wouldn't be possible in a Python-agnostic way so we use JSON to
 * pass the data from Go to Python. The configuration value is loaded in the Agent,
 * marshalled into JSON and passed as a `char*` to Six, where the string is
 * decoded back to Python and passed to the caller. JSON usage is transparent to
 * the caller, who would receive a Python object as returned from `json.loads`.
 */
PyObject *get_config(PyObject *self, PyObject *args) {
    // callback must be set
    assert(cb_get_config != NULL);

    char *key;
    if (!PyArg_ParseTuple(args, "s", &key)) {
        Py_RETURN_NONE;
    }

    char *data;
    cb_get_config(key, &data);

    // new ref
    PyObject *value = from_json(data);
    if (value == NULL) {
        Py_RETURN_NONE;
    }
    return value;
}
