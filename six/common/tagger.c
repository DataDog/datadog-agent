// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#include "sixstrings.h"
#include <tagger.h>

// these must be set by the Agent
static cb_get_tags_t cb_get_tags = NULL;

PyObject *get_tags(PyObject *self, PyObject *args)
{
    if (cb_get_tags == NULL)
        Py_RETURN_NONE;

    char *id;
    int highCard;
    if (!PyArg_ParseTuple(args, "si", &id, &highCard)) {
        Py_RETURN_NONE;
    }

    char *data = NULL;
    cb_get_tags(id, highCard, &data);

    // new ref
    PyObject *value = from_json(data);
    // TODO: free data
    if (value == NULL) {
        Py_RETURN_NONE;
    }
    return value;
}

void _set_get_tags_cb(cb_get_tags_t cb)
{
    cb_get_tags = cb;
}

static PyMethodDef methods[] = {
    { "get_tags", (PyCFunction)get_tags, METH_VARARGS, "Get tags for an entity." }, { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, TAGGER_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_tagger(void)
{
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_tagger()
{
    module = Py_InitModule(TAGGER_MODULE_NAME, methods);
}
#endif
