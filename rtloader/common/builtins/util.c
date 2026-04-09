// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "util.h"
#include "datadog_agent.h"

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

/*
 * Sub-interpreter support (Python 3.13+): Multi-phase module initialization
 * =========================================================================
 *
 * Converts the util module from single-phase to multi-phase init to allow
 * importing in Python sub-interpreters with per-interpreter GIL.
 *
 * This module provides a legacy compatibility wrapper around the datadog_agent
 * module's headers() function. It has no callback pointers of its own (it
 * delegates to datadog_agent's callbacks) and no constants or setup work,
 * so no Py_mod_exec slot is needed.
 *
 * m_size = 0: No module-level C state. The underlying callbacks live in
 * datadog_agent.c as process-global statics with set-once-read-many semantics.
 *
 * See aggregator.c for a detailed explanation of multi-phase init, m_size,
 * and Py_MOD_PER_INTERPRETER_GIL_SUPPORTED rationale.
 */
#if PY_VERSION_HEX >= 0x030D0000

static PyModuleDef_Slot util_slots[] = {
    {Py_mod_multiple_interpreters, Py_MOD_PER_INTERPRETER_GIL_SUPPORTED},
    {0, NULL}  /* sentinel */
};

static struct PyModuleDef module_def = {
    PyModuleDef_HEAD_INIT,
    UTIL_MODULE_NAME,  /* m_name: "util" */
    NULL,              /* m_doc */
    0,                 /* m_size: no per-interpreter C state */
    methods,           /* m_methods */
    util_slots,        /* m_slots */
    NULL,              /* m_traverse */
    NULL,              /* m_clear */
    NULL               /* m_free */
};

PyMODINIT_FUNC PyInit_util(void)
{
    return PyModuleDef_Init(&module_def);
}

#else /* Python < 3.13: original single-phase initialization */

static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, UTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_util(void)
{
    return PyModule_Create(&module_def);
}

#endif /* PY_VERSION_HEX >= 0x030D0000 */

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
