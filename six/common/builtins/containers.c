// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "containers.h"

#include <sixstrings.h>

// these must be set by the Agent
static cb_is_excluded_t cb_is_excluded = NULL;

// forward declarations
static PyObject *is_excluded(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "is_excluded", (PyCFunction)is_excluded, METH_VARARGS, "Returns whether a container is excluded per name and image." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, CONTAINERS_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_containers(void)
{
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_containers()
{
    module = Py_InitModule(CONTAINERS_MODULE_NAME, methods);
}
#endif

void _set_is_excluded_cb(cb_is_excluded_t cb)
{
    cb_is_excluded = cb;
}

PyObject *is_excluded(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_is_excluded == NULL)
        Py_RETURN_NONE;

    char *name;
    char *image;
    if (!PyArg_ParseTuple(args, "ss", &name, &image))
        return NULL;

    int result = cb_is_excluded(name, image);

    if (result > 0)
        Py_RETURN_TRUE;
    Py_RETURN_FALSE;
}
