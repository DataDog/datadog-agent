// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "_util.h"

#include <cgo_free.h>
#include <stdio.h>
#include <stringutils.h>

// must be set by the caller
static cb_get_subprocess_output_t cb_get_subprocess_output = NULL;

static PyObject *subprocess_output(PyObject *self, PyObject *args);

// Exceptions
PyObject *SubprocessOutputEmptyError;
void addSubprocessException(PyObject *m)
{
    SubprocessOutputEmptyError = PyErr_NewException("_util.SubprocessOutputEmptyError", NULL, NULL);
    Py_INCREF(SubprocessOutputEmptyError);
    PyModule_AddObject(m, "SubprocessOutputEmptyError", SubprocessOutputEmptyError);
}

static PyMethodDef methods[] = {
    { "subprocess_output", (PyCFunction)subprocess_output, METH_VARARGS | METH_KEYWORDS,
      "Exec a process and return the output." },
    { "get_subprocess_output", (PyCFunction)subprocess_output, METH_VARARGS | METH_KEYWORDS,
      "Exec a process and return the output." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, _UTIL_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit__util(void)
{
    PyObject *m = PyModule_Create(&module_def);
    addSubprocessException(m);
    return m;
}
#endif

#ifdef DATADOG_AGENT_TWO
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init__util()
{
    module = Py_InitModule(_UTIL_MODULE_NAME, methods);
    addSubprocessException(module);
}
#endif

void _set_get_subprocess_output_cb(cb_get_subprocess_output_t cb)
{
    cb_get_subprocess_output = cb;
}

static void raiseEmptyOutputError()
{
    PyObject *utilModule = PyImport_ImportModule("_util");
    if (utilModule == NULL) {
        PyErr_SetString(PyExc_TypeError, "error: no module '_util'");
        return;
    }

    PyObject *excClass = PyObject_GetAttrString(utilModule, "SubprocessOutputEmptyError");
    if (excClass == NULL) {
        Py_DecRef(utilModule);
        PyErr_SetString(PyExc_TypeError, "no attribute '_util.SubprocessOutputEmptyError' found");
        return;
    }

    PyErr_SetString(excClass, "get_subprocess_output expected output but had none.");
    Py_DecRef(excClass);
    Py_DecRef(utilModule);
}

PyObject *subprocess_output(PyObject *self, PyObject *args)
{
    if (!cb_get_subprocess_output)
        Py_RETURN_NONE;

    int raise = 0;
    int ret_code = 0;
    int subprocess_args_sz;
    char **subprocess_args = NULL;
    char *c_stdout = NULL;
    char *c_stderr = NULL;
    char *exception = NULL;
    PyObject *cmd_args = NULL;
    PyObject *cmd_raise_on_empty = NULL;
    PyObject *py_result = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "O|O:get_subprocess_output", &cmd_args, &cmd_raise_on_empty)) {
        goto error;
    }

    if (!PyList_Check(cmd_args)) {
        PyErr_SetString(PyExc_TypeError, "command args not a list");
        goto error;
    }

    subprocess_args_sz = PyList_Size(cmd_args);
    if (subprocess_args_sz == 0) {
        PyErr_SetString(PyExc_TypeError, "invalid command: empty list");
        goto error;
    }

    if (!(subprocess_args = (char **)malloc(sizeof(*subprocess_args) * (subprocess_args_sz + 1)))) {
        PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
        goto error;
    }

    subprocess_args[subprocess_args_sz] = NULL;
    int i;
    for (i = 0; i < subprocess_args_sz; i++) {
        char *subprocess_arg = as_string(PyList_GetItem(cmd_args, i));

        if (subprocess_arg == NULL) {
            // cleanup
            int j;
            for (j = 0; j < i; j++)
                free(subprocess_args[j]);
            free(subprocess_args);

            PyErr_SetString(PyExc_TypeError, "command argument must be valid strings");
            goto error;
        }

        subprocess_args[i] = subprocess_arg;
    }

    if (cmd_raise_on_empty != NULL && !PyBool_Check(cmd_raise_on_empty)) {
        PyErr_SetString(PyExc_TypeError, "bad raise_on_empty argument: should be bool");
        goto error;
    }

    if (cmd_raise_on_empty == Py_True)
        raise = 1;

    PyGILState_Release(gstate);
    PyThreadState *Tstate = PyEval_SaveThread();

    cb_get_subprocess_output(subprocess_args, &c_stdout, &c_stderr, &ret_code, &exception);

    PyEval_RestoreThread(Tstate);
    gstate = PyGILState_Ensure();

    if (raise && strlen(c_stdout) == 0) {
        raiseEmptyOutputError();
        goto error;
    }

    for (i = 0; subprocess_args[i]; i++)
        free(subprocess_args[i]);
    free(subprocess_args);

    if (exception) {
        PyErr_SetString(PyExc_Exception, exception);
        cgo_free(exception);
        goto error;
    }

    PyObject *pyStdout = NULL;
    if (c_stdout) {
        pyStdout = PyStringFromCString(c_stdout);
        cgo_free(c_stdout);
    } else {
        Py_INCREF(Py_None);
        pyStdout = Py_None;
    }

    PyObject *pyStderr = NULL;
    if (c_stderr) {
        pyStderr = PyStringFromCString(c_stderr);
        cgo_free(c_stderr);
    } else {
        Py_INCREF(Py_None);
        pyStderr = Py_None;
    }

    PyObject *pyResult = PyTuple_New(3);
    PyTuple_SetItem(pyResult, 0, pyStdout);
    PyTuple_SetItem(pyResult, 1, pyStderr);
#ifdef DATADOG_AGENT_THREE
    PyTuple_SetItem(pyResult, 2, PyLong_FromLong(ret_code));
#else
    PyTuple_SetItem(pyResult, 2, PyInt_FromLong(ret_code));
#endif

    PyGILState_Release(gstate);
    return pyResult;

error:
    if (c_stdout)
        cgo_free(c_stdout);
    if (c_stderr)
        cgo_free(c_stderr);
    if (exception)
        cgo_free(exception);
    PyGILState_Release(gstate);
    // we need to return NULL to raise the exception set by PyErr_SetString
    return NULL;
}
