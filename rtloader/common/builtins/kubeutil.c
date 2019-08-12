// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "kubeutil.h"

#include "cgo_free.h"
#include "stringutils.h"


// these must be set by the Agent
static cb_get_connection_info_t cb_get_connection_info = NULL;

// forward declarations
static PyObject *get_connection_info();

static PyMethodDef methods[] = {
    { "get_connection_info", (PyCFunction)get_connection_info, METH_NOARGS, "Get kubelet connection information." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, KUBEUTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_kubeutil(void)
{
    return PyModule_Create(&module_def);
}
#elif defined(DATADOG_AGENT_TWO)
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_kubeutil()
{
    module = Py_InitModule(KUBEUTIL_MODULE_NAME, methods);
}
#endif

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
    PyObject *conn_info_dict = from_yaml(data);

    // free the memory allocated by the Agent
    cgo_free(data);

    if (conn_info_dict == NULL || !PyDict_Check(conn_info_dict)) {
        // clear error set by `from_yaml` (if any)
        PyErr_Clear();
        // create a new ref and drop the other one
        Py_XDECREF(conn_info_dict);
        return PyDict_New();
    }

    return conn_info_dict;
}
