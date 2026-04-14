// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "containers.h"

#include <stringutils.h>

// these must be set by the Agent
static cb_is_excluded_t cb_is_excluded = NULL;

// forward declarations
static PyObject *is_excluded(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "is_excluded", (PyCFunction)is_excluded, METH_VARARGS,
      "Returns whether a container is excluded per name, image and namespace." },
    { NULL, NULL } // guards
};

/*
 * Sub-interpreter support (Python 3.14+): Multi-phase module initialization
 * =========================================================================
 *
 * Converts the containers module from single-phase to multi-phase init to
 * allow importing in Python sub-interpreters with per-interpreter GIL.
 *
 * This module has one callback pointer (cb_is_excluded) and no constants
 * or setup work, so no Py_mod_exec slot is needed.
 *
 * m_size = 0: The single callback pointer is a process-global static with
 * set-once-read-many semantics, safe for concurrent access.
 *
 * See aggregator.c for a detailed explanation of multi-phase init, m_size,
 * and Py_MOD_PER_INTERPRETER_GIL_SUPPORTED rationale.
 */
#if PY_VERSION_HEX >= 0x030E0000

static PyModuleDef_Slot containers_slots[] = {
    {Py_mod_multiple_interpreters, Py_MOD_PER_INTERPRETER_GIL_SUPPORTED},
    {0, NULL}  /* sentinel */
};

static struct PyModuleDef module_def = {
    PyModuleDef_HEAD_INIT,
    CONTAINERS_MODULE_NAME,  /* m_name: "containers" */
    NULL,                    /* m_doc */
    0,                       /* m_size: no per-interpreter C state */
    methods,                 /* m_methods */
    containers_slots,        /* m_slots */
    NULL,                    /* m_traverse */
    NULL,                    /* m_clear */
    NULL                     /* m_free */
};

PyMODINIT_FUNC PyInit_containers(void)
{
    return PyModuleDef_Init(&module_def);
}

#else /* Python < 3.14: original single-phase initialization */

static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, CONTAINERS_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_containers(void)
{
    return PyModule_Create(&module_def);
}

#endif /* PY_VERSION_HEX >= 0x030E0000 */

void _set_is_excluded_cb(cb_is_excluded_t cb)
{
    cb_is_excluded = cb;
}

/*! \fn PyObject *is_excluded(PyObject *self, PyObject *args)
    \brief Method to determine whether a container is excluded from metric
    collection or not.
    \param self A PyObject* pointer to the containers module.
    \param args A PyObject* pointer to the python args, typically expected to
    contain the container name, the image name and an optional namespace as strings.
    \return a PyObject * pointer, typically a boolean reflecting if the container
    should be excluded and None, if the callback has not been defined.

    This method will let us know if a container is excluded and calls the cgo-bound
    cb_is_excluded callback. The cgo callback is not expected to have any memory side
    effects and so no additional cleanup is necessary after invoking it.
*/
PyObject *is_excluded(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_is_excluded == NULL) {
        Py_RETURN_NONE;
    }

    char *name;
    char *image;
    char *namespace = NULL;
    if (!PyArg_ParseTuple(args, "ss|s", &name, &image, &namespace)) {
        return NULL;
    }

    int result = cb_is_excluded(name, image, namespace);

    if (result > 0) {
        Py_RETURN_TRUE;
    }
    Py_RETURN_FALSE;
}
