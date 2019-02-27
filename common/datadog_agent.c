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

// forward declarations
static PyObject *get_version(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "get_version", (PyCFunction)get_version, METH_VARARGS, "Get Agent version." }, { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_datadog_agent(void) {
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// module object storage
static PyObject *module;

void Py2_init_datadog_agent() {
    module = Py_InitModule(MODULE_NAME, methods);
}
#endif

void _set_get_version_cb(cb_get_version_t cb) {
    cb_get_version = cb;
}

PyObject *get_version(PyObject *self, PyObject *args) {
    Py_RETURN_NONE;
}
