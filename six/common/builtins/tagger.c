// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#include "cgo_free.h"
#include "stringutils.h"
#include <tagger.h>

// these must be set by the Agent
static cb_tags_t cb_tags = NULL;

int parseArgs(PyObject *args, char **id, int *cardinality)
{
    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "si", id, cardinality)) {
        PyGILState_Release(gstate);
        return 0;
    }
    PyGILState_Release(gstate);
    return 1;
}

PyObject *buildTagsList(char **tags)
{
    if (tags == NULL)
        Py_RETURN_NONE;

    PyObject *res = PyList_New(0);
    int i;
    for (i = 0; tags[i]; i++) {
        PyObject *pyTag = PyStringFromCString(tags[i]);
        cgo_free(tags[i]);
        PyList_Append(res, pyTag);
    }
    cgo_free(tags);
    return res;
}

PyObject *tag(PyObject *self, PyObject *args)
{
    if (cb_tags == NULL)
        Py_RETURN_NONE;

    char *id;
    int cardinality;
    if (!parseArgs(args, &id, &cardinality))
        return NULL;

    if (cardinality != DATADOG_AGENT_SIX_TAGGER_LOW && cardinality != DATADOG_AGENT_SIX_TAGGER_ORCHESTRATOR
        && cardinality != DATADOG_AGENT_SIX_TAGGER_HIGH) {
        PyGILState_STATE gstate = PyGILState_Ensure();
        PyErr_SetString(PyExc_TypeError, "Invalid cardinality");
        PyGILState_Release(gstate);
        return NULL;
    }

    return buildTagsList(cb_tags(id, cardinality));
}

PyObject *get_tags(PyObject *self, PyObject *args)
{
    if (cb_tags == NULL)
        Py_RETURN_NONE;

    char *id;
    int highCard;
    if (!parseArgs(args, &id, &highCard))
        return NULL;

    int cardinality;
    if (highCard > 0)
        cardinality = DATADOG_AGENT_SIX_TAGGER_HIGH;
    else
        cardinality = DATADOG_AGENT_SIX_TAGGER_LOW;

    return buildTagsList(cb_tags(id, cardinality));
}

void _set_tags_cb(cb_tags_t cb)
{
    cb_tags = cb;
}

static PyMethodDef methods[] = {
    { "tag", (PyCFunction)tag, METH_VARARGS, "Get tags for an entity." },
    { "get_tags", (PyCFunction)get_tags, METH_VARARGS, "(Deprecated) Get tags for an entity." },
    { NULL, NULL } // guards
};

static void add_constants(PyObject *m)
{
    PyModule_AddIntConstant(m, "LOW", DATADOG_AGENT_SIX_TAGGER_LOW);
    PyModule_AddIntConstant(m, "ORCHESTRATOR", DATADOG_AGENT_SIX_TAGGER_ORCHESTRATOR);
    PyModule_AddIntConstant(m, "HIGH", DATADOG_AGENT_SIX_TAGGER_HIGH);
}

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, TAGGER_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_tagger(void)
{
    PyObject *module = PyModule_Create(&module_def);
    add_constants(module);
    return module;
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_tagger()
{
    module = Py_InitModule(TAGGER_MODULE_NAME, methods);
    add_constants(module);
}
#endif
