// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "kubeutil.h"

#include "cgo_free.h"
#include "stringutils.h"


// these must be set by the Agent
static cb_get_connection_info_t cb_get_connection_info = NULL;

// forward declarations
static PyObject *get_connection_info(PyObject *, PyObject *);

static PyMethodDef methods[] = {
    { "get_connection_info", (PyCFunction)get_connection_info, METH_NOARGS, "Get kubelet connection information." },
    { NULL, NULL } // guards
};

/*
 * Sub-interpreter support (Python 3.13+): Multi-phase module initialization
 * =========================================================================
 *
 * Converts the kubeutil module from single-phase to multi-phase init to allow
 * importing in Python sub-interpreters with per-interpreter GIL.
 *
 * This module has one callback pointer (cb_get_connection_info) and no
 * constants or setup work, so no Py_mod_exec slot is needed.
 *
 * m_size = 0: The single callback pointer is a process-global static with
 * set-once-read-many semantics, safe for concurrent access.
 *
 * See aggregator.c for a detailed explanation of multi-phase init, m_size,
 * and Py_MOD_PER_INTERPRETER_GIL_SUPPORTED rationale.
 */
#if PY_VERSION_HEX >= 0x030D0000

static PyModuleDef_Slot kubeutil_slots[] = {
    {Py_mod_multiple_interpreters, Py_MOD_PER_INTERPRETER_GIL_SUPPORTED},
    {0, NULL}  /* sentinel */
};

static struct PyModuleDef module_def = {
    PyModuleDef_HEAD_INIT,
    KUBEUTIL_MODULE_NAME,  /* m_name: "kubeutil" */
    NULL,                  /* m_doc */
    0,                     /* m_size: no per-interpreter C state */
    methods,               /* m_methods */
    kubeutil_slots,        /* m_slots */
    NULL,                  /* m_traverse */
    NULL,                  /* m_clear */
    NULL                   /* m_free */
};

PyMODINIT_FUNC PyInit_kubeutil(void)
{
    return PyModuleDef_Init(&module_def);
}

#else /* Python < 3.13: original single-phase initialization */

static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, KUBEUTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_kubeutil(void)
{
    return PyModule_Create(&module_def);
}

#endif /* PY_VERSION_HEX >= 0x030D0000 */

void _set_get_connection_info_cb(cb_get_connection_info_t cb)
{
    cb_get_connection_info = cb;
}

/*! \fn void get_connection_info(PyObject *self, PyObject *args)
    \brief Implements the python method to collect the kubernetes connection information
    by calling the corresponding callback.
    \param self A PyObject* pointer to the kubeutil module.
    \param args A PyObject* pointer to an empty tuple as this method has no input args.
    \return a PyObject * pointer to a python dictionary containing the K8s connection info.

    This function is callable as the `kubeutil.get_connection_info` python method, the
    callback is expected to have been set previously, if not `None` will be returned.
*/
PyObject *get_connection_info(PyObject *self, PyObject *args)
{
    char *data = NULL;

    // callback must be set
    if (cb_get_connection_info == NULL) {
        Py_RETURN_NONE;
    }

    cb_get_connection_info(&data);

    // create a new ref
    PyObject *conn_info_dict = from_json(data);

    // free the memory allocated by the Agent
    cgo_free(data);

    if (conn_info_dict == NULL || !PyDict_Check(conn_info_dict)) {
        // clear error set by `from_json` (if any)
        PyErr_Clear();
        // create a new ref and drop the other one
        Py_XDECREF(conn_info_dict);
        return PyDict_New();
    }

    return conn_info_dict;
}
