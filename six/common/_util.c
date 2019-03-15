// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "_util.h"

#include <sixstrings.h>
#include <stdio.h>

// must be set by the caller
static cb_get_subprocess_output_t cb_get_subprocess_output = NULL;

static PyObject *subprocess_output(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "subprocess_output", (PyCFunction)subprocess_output, METH_VARARGS | METH_KEYWORDS,
      "Exec a process and return the output." },
    { "get_subprocess_output", (PyCFunction)subprocess_output, METH_VARARGS | METH_KEYWORDS,
      "Exec a process and return the output." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, _UTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit__util(void) {
    return PyModule_Create(&module_def);
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init__util() {
    module = Py_InitModule(_UTIL_MODULE_NAME, methods);
}
#endif

void _set_get_subprocess_output_cb(cb_get_subprocess_output_t cb) {
    cb_get_subprocess_output = cb;
}
PyObject *subprocess_output(PyObject *self, PyObject *args) {
    if (!cb_get_subprocess_output) {
        Py_RETURN_NONE;
    }

    PyObject *cmd_args, *cmd_raise_on_empty;
    int raise = 1, i = 0;
    int subprocess_args_sz;
    char **subprocess_args, *subprocess_arg;
    PyObject *py_result;

    PyGILState_STATE gstate = PyGILState_Ensure();

    cmd_raise_on_empty = NULL;
    if (!PyArg_ParseTuple(args, "O|O:get_subprocess_output", &cmd_args, &cmd_raise_on_empty)) {
        PyGILState_Release(gstate);
        Py_RETURN_NONE;
    }

    if (!PyList_Check(cmd_args)) {
        PyErr_SetString(PyExc_TypeError, "command args not a list");
        PyGILState_Release(gstate);
        Py_RETURN_NONE;
    }

    if (cmd_raise_on_empty != NULL && !PyBool_Check(cmd_raise_on_empty)) {
        PyErr_SetString(PyExc_TypeError, "bad raise_on_empty_argument - should be bool");
        PyGILState_Release(gstate);
        Py_RETURN_NONE;
    }

    if (cmd_raise_on_empty != NULL) {
        raise = (int)(cmd_raise_on_empty == Py_True);
    }

    subprocess_args_sz = PyList_Size(cmd_args);
    if (!(subprocess_args = (char **)malloc(sizeof(char *) * subprocess_args_sz))) {
        PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
        PyGILState_Release(gstate);
        Py_RETURN_NONE;
    }

    for (i = 0; i < subprocess_args_sz; i++) {
        subprocess_arg = as_string(PyList_GetItem(cmd_args, i));
        if (subprocess_arg == NULL) {
            PyErr_SetString(PyExc_Exception, "unable to parse arguments to cgo/go-land");
            free(subprocess_args);
            PyGILState_Release(gstate);
            Py_RETURN_NONE;
        }
        subprocess_args[i] = subprocess_arg;
    }

    PyGILState_Release(gstate);
    char *output = NULL;
    PyObject *pyOutput = NULL;
    cb_get_subprocess_output(subprocess_args, subprocess_args_sz, raise, &output);
    if (subprocess_args) {
        for (i = 0; i < subprocess_args_sz; i++) {
            free(subprocess_args[i]);
        }
    }
    free(subprocess_args);
    if (output) {
        pyOutput = PyStringFromCString(output);
    }
    return pyOutput;
}