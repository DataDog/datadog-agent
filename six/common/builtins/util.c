// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "util.h"
#include "datadog_agent.h"
#include "util.h"

#include <stringutils.h>

static PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *get_hostname(PyObject *self, PyObject *args);
static PyObject *get_clustername(PyObject *self, PyObject *args);
static PyObject *log_message(PyObject *self, PyObject *args);
static PyObject *set_external_tags(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "headers", (PyCFunction)headers, METH_VARARGS | METH_KEYWORDS, "Get standard set of HTTP headers." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, UTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_util(void)
{
    return PyModule_Create(&module_def);
}
#elif defined(DATADOG_AGENT_TWO)
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_util()
{
    module = Py_InitModule(UTIL_MODULE_NAME, methods);
}
#endif

// headers entry point is provided in the `datadog_agent module.

PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs)
{
    return _public_headers(self, args, kwargs);
}
