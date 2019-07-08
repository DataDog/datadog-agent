// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "_util.h"
#include "cgo_free.h"
#include "memory.h"
#include "stringutils.h"

#include <stdio.h>

// must be set by the caller
static cb_get_subprocess_output_t cb_get_subprocess_output = NULL;

static PyObject *subprocess_output(PyObject *self, PyObject *args);

// Exceptions

/*! \fn void addSubprocessException(PyObject *m)
    \brief Adds a custom SubprocessOutputEmptyError exception to the module passed as parameter.
    \param m A PyObject* pointer to the module we wish to register the exception with.
*/
void addSubprocessException(PyObject *m)
{
    PyObject *SubprocessOutputEmptyError = PyErr_NewException(_SUBPROCESS_OUTPUT_ERROR_NS_NAME, NULL, NULL);
    PyModule_AddObject(m, _SUBPROCESS_OUTPUT_ERROR_NAME, SubprocessOutputEmptyError);
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
#elif defined(DATADOG_AGENT_TWO)
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

/*! \fn void raiseEmptyOutputError()
    \brief sets the SubprocessOutputEmptyError exception as the interpreter error.

    If everything goes well the exception error will be set in the interpreter.
    Otherwise, if the module or the exception class are not found, the relevant
    error will be set in the interpreter instead.
*/
static void raiseEmptyOutputError()
{
    PyObject *utilModule = PyImport_ImportModule(_UTIL_MODULE_NAME);
    if (utilModule == NULL) {
        PyErr_SetString(PyExc_TypeError, "error: no module '" _UTIL_MODULE_NAME "'");
        return;
    }

    PyObject *excClass = PyObject_GetAttrString(utilModule, _SUBPROCESS_OUTPUT_ERROR_NAME);
    if (excClass == NULL) {
        Py_DecRef(utilModule);
        PyErr_SetString(PyExc_TypeError, "no attribute '" _SUBPROCESS_OUTPUT_ERROR_NS_NAME "' found");
        return;
    }

    PyErr_SetString(excClass, "get_subprocess_output expected output but had none.");
    Py_DecRef(excClass);
    Py_DecRef(utilModule);
}

/*! \fn PyObject *subprocess_output(PyObject *self, PyObject *args)
    \brief This function implements the `_util.subprocess_output` _and_ `_util.get_subprocess_output`
    python method, allowing to execute a subprocess and collect its output.
    \param self A PyObject* pointer to the _util module.
    \param args A PyObject* pointer to the args tuple with the desired subprocess commands, and
    optionally a boolean raise_on_empty flag.
    \return a PyObject * pointer to a python tuple with the stdout, stderr output and the
    command exit code.

    This function is callable as the `_util.subprocess_output` or `_util.get_subprocess_output`
    python methods. The command arguments list is fed to the CGO callback, where the command is
    executed in go-land. The stdout, stderr and exit codes for the command are returned by the
    callback; these are then converted into python strings and integer respectively and returned
    in a tuple. If the optional `raise_on_empty` boolean flag is set, and the command output is
    empty an exception will be raised: the error will be set in the interpreter and NULL will be
    returned.
*/
PyObject *subprocess_output(PyObject *self, PyObject *args)
{
    int i;
    int raise = 0;
    int ret_code = 0;
    int subprocess_args_sz;
    char **subprocess_args = NULL;
    char *c_stdout = NULL;
    char *c_stderr = NULL;
    char *exception = NULL;
    PyObject *cmd_args = NULL;
    PyObject *cmd_raise_on_empty = NULL;
    PyObject *pyResult = NULL;

    if (!cb_get_subprocess_output) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    // `cmd_args` is mandatory and should be a list, `cmd_raise_on_empty` is an optional
    // boolean. The string after the ':' is used as the function name in error messages.
    if (!PyArg_ParseTuple(args, "O|O:get_subprocess_output", &cmd_args, &cmd_raise_on_empty)) {
        goto cleanup;
    }

    if (!PyList_Check(cmd_args)) {
        PyErr_SetString(PyExc_TypeError, "command args not a list");
        goto cleanup;
    }

    // We already PyList_Check cmd_args, so PyList_Size won't fail and return -1
    subprocess_args_sz = PyList_Size(cmd_args);
    if (subprocess_args_sz == 0) {
        PyErr_SetString(PyExc_TypeError, "invalid command: empty list");
        goto cleanup;
    }

    if (!(subprocess_args = (char **)_malloc(sizeof(*subprocess_args) * (subprocess_args_sz + 1)))) {
        PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
        goto cleanup;
    }

    // init to NULL for safety - could use memset, but this is safer.
    for (i = 0; i <= subprocess_args_sz; i++) {
        subprocess_args[i] = NULL;
    }

    for (i = 0; i < subprocess_args_sz; i++) {
        char *subprocess_arg = as_string(PyList_GetItem(cmd_args, i));

        if (subprocess_arg == NULL) {
            PyErr_SetString(PyExc_TypeError, "command argument must be valid strings");
            goto cleanup;
        }

        subprocess_args[i] = subprocess_arg;
    }

    if (cmd_raise_on_empty != NULL && !PyBool_Check(cmd_raise_on_empty)) {
        PyErr_SetString(PyExc_TypeError, "bad raise_on_empty argument: should be bool");
        goto cleanup;
    }

    if (cmd_raise_on_empty == Py_True) {
        raise = 1;
    }

    // Release the GIL so Python can execute other checks while Go runs the subprocess
    PyGILState_Release(gstate);
    PyThreadState *Tstate = PyEval_SaveThread();

    cb_get_subprocess_output(subprocess_args, &c_stdout, &c_stderr, &ret_code, &exception);

    // Acquire the GIL now that Go is done
    PyEval_RestoreThread(Tstate);
    gstate = PyGILState_Ensure();

    if (raise && strlen(c_stdout) == 0) {
        raiseEmptyOutputError();
        goto cleanup;
    }

    if (exception) {
        PyErr_SetString(PyExc_Exception, exception);
        goto cleanup;
    }

    PyObject *pyStdout = NULL;
    if (c_stdout) {
        pyStdout = PyStringFromCString(c_stdout);
    } else {
        Py_INCREF(Py_None);
        pyStdout = Py_None;
    }

    PyObject *pyStderr = NULL;
    if (c_stderr) {
        pyStderr = PyStringFromCString(c_stderr);
    } else {
        Py_INCREF(Py_None);
        pyStderr = Py_None;
    }

    pyResult = PyTuple_New(3);
    PyTuple_SetItem(pyResult, 0, pyStdout);
    PyTuple_SetItem(pyResult, 1, pyStderr);
#ifdef DATADOG_AGENT_THREE
    PyTuple_SetItem(pyResult, 2, PyLong_FromLong(ret_code));
#else
    PyTuple_SetItem(pyResult, 2, PyInt_FromLong(ret_code));
#endif

cleanup:
    if (c_stdout) {
        cgo_free(c_stdout);
    }
    if (c_stderr) {
        cgo_free(c_stderr);
    }
    if (exception) {
        cgo_free(exception);
    }

    if (subprocess_args) {
        for (i = 0; i <= subprocess_args_sz && subprocess_args[i]; i++) {
            _free(subprocess_args[i]);
        }
        _free(subprocess_args);
    }

    // Please note that if we get here we have a matching PyGILState_Ensure above, so we're safe.
    PyGILState_Release(gstate);

    // pyResult will be NULL in the face of error to raise the exception set by PyErr_SetString
    return pyResult;
}
