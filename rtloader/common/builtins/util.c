// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
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

static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, UTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_util(void)
{
    return PyModule_Create(&module_def);
}

/*! \fn PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs)
    \brief This function provides a standard set of HTTP headers the caller might want to
    use for HTTP requests.
    \param self A PyObject* pointer to the util module.
    \param args A PyObject* pointer to the `agentConfig`, but not expected to be used.
    \param kwargs A PyObject* pointer to a dictonary. If the `http_host` key is present
    it will be added to the headers.
    \return a PyObject * pointer to a python dictionary with the expected headers.

    This function is callable as the `util.headers` python method, the entry point:
    `_public_headers()` is provided in the `datadog_agent` module, the method is duplicated.
*/
PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs)
{
    return _public_headers(self, args, kwargs);
}
